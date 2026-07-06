// Package engine 是输入法核心引擎
package engine

import (
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zai/goime/internal/clipboard"
	"github.com/zai/goime/internal/dict"
	"github.com/zai/goime/internal/fuzzy"
	"github.com/zai/goime/internal/pinyin"
	"github.com/zai/goime/internal/segmenter"
	"github.com/zai/goime/internal/userdict"
)

// Candidate 候选词
type Candidate struct {
        Word   string  // 候选词
        Pinyin string  // 对应拼音
        Score  float64 // 综合得分
        Source string  // 来源："dict" | "user" | "fuzzy" | "typo"
}

// Engine 输入法引擎
type Engine struct {
        dict       *dict.Dict
        fuzzy      *fuzzy.Engine
        segmenter  *segmenter.Segmenter

        mu            sync.RWMutex
        userFreq      map[string]float64 // 内存缓存（带时间衰减后的值）
        userLastUsed  map[string]time.Time // 最近使用时间（用于时间衰减）
        userStore     *userdict.Store    // 持久化存储（可选）
        commitHistory []string           // 最近提交的词（用于上下文联想）
        contextPairs  map[string]float64 // 上下文共现频次: key = "prevWord|candidateWord"

        // 排序权重
        wPinyinMatch float64
        wFreq        float64
        wUserFreq    float64
        wContext     float64
        wFuzzy       float64
        wTypo        float64

        // 时间衰减参数
        decayHalfLife time.Duration // 半衰期（默认 24 小时）

        // 上下文联想参数
        maxContextHistory int // 上下文历史最大长度

        // N-gram 自动剪枝参数
        maxContextPairs   int           // contextPairs 最大条目数
        pruneThreshold    float64       // 低于此频次的条目被剪枝
        lastPruneTime     time.Time     // 上次剪枝时间
        pruneInterval     time.Duration // 剪枝间隔

        // 剪切板历史
        clipboardHistory *clipboard.History
}

// Config 引擎配置
type Config struct {
        WPinyinMatch float64
        WFreq        float64
        WUserFreq    float64
        WContext     float64
        WFuzzy       float64
        WTypo        float64
}

// DefaultConfig 默认配置
func DefaultConfig() Config {
        return Config{
                WPinyinMatch: 100.0,
                WFreq:        1.0,
                WUserFreq:    50.0,
                WContext:     30.0,
                WFuzzy:       0.7,
                WTypo:        0.5,
        }
}

// New 创建引擎
func New(d *dict.Dict, cfg Config) *Engine {
        // 尝试加载 2-gram 模型（失败则降级到纯词频）
        var seg *segmenter.Segmenter
        if bm, err := segmenter.LoadBigramModel(); err == nil {
                seg, _ = segmenter.NewWithBigram(d, bm)
        } else {
                seg = segmenter.New(d)
        }
        return &Engine{
                dict:              d,
                fuzzy:             fuzzy.New(),
                segmenter:         seg,
                userFreq:          make(map[string]float64),
                userLastUsed:      make(map[string]time.Time),
                contextPairs:      make(map[string]float64),
                wPinyinMatch:      cfg.WPinyinMatch,
                wFreq:             cfg.WFreq,
                wUserFreq:         cfg.WUserFreq,
                wContext:          cfg.WContext,
                wFuzzy:            cfg.WFuzzy,
                wTypo:             cfg.WTypo,
                decayHalfLife:     24 * time.Hour,
                maxContextHistory: 50,
                maxContextPairs:   10000,
                pruneThreshold:    1.0,
                pruneInterval:     1 * time.Hour,
                clipboardHistory:  clipboard.New(50), // 保存最近 50 条
        }
}

// NewDefault 用默认配置创建
func NewDefault(d *dict.Dict) *Engine {
        return New(d, DefaultConfig())
}

// NewWithUserStore 创建带持久化用户词典的引擎
// path 为空时使用默认路径 ~/.samime/userdict
// 加载用户频次和上下文共现到内存
func NewWithUserStore(d *dict.Dict, path string) (*Engine, error) {
        store, err := userdict.New(path)
        if err != nil {
                return nil, err
        }
        e := NewDefault(d)
        e.userStore = store
        // 加载已有用户频次到内存
        e.userFreq = store.All()
        // 加载已有上下文共现到内存
        e.contextPairs = store.AllContext()
        return e, nil
}

// Search 输入拼音串，返回候选列表
// 流程：
//  1. 切分音节
//  2. 精确匹配
//  3. 模糊音展开匹配
//  4. 拼写错误容错匹配
//  5. 综合排序
func (e *Engine) Search(input string) []Candidate {
        input = strings.ToLower(strings.TrimSpace(input))
        if len(input) == 0 {
                return nil
        }
        start := time.Now()
        defer func() {
                _ = start
        }()

        // 切分音节
        syls := pinyin.Segment(input)
        if len(syls) == 0 {
                // 切分失败：尝试特殊匹配模式
                candMap := make(map[string]*Candidate)

                // 模式 1: 单声母联想（输入 "n"/"w"/"z" 等纯声母时）
                if pinyin.IsInitial(input) {
                        e.singleInitialMatch(input, candMap)
                        // 续接联想（搜狗核心特性）：即使切分失败，也尝试续接预测
                        // 例：上次提交"今天天"，输入"q" → 联想"气怎么样"
                        if len(e.commitHistory) > 0 {
                                // 构造伪音节用于续接匹配
                                pseudoSyls := []pinyin.Syllable{{Initial: input, Final: "", Raw: input}}
                                e.continuationMatch(input, pseudoSyls, candMap)
                                // 单字母全句补全（搜狗核心特性）：基于上下文 N-gram 预测整句
                                // 例：上次提交"今天天"，输入"w" → "我要去吃饭"等完整句预测
                                // 例：上次提交"我"，输入"y" → "要去博物馆"
                                e.singleCharSentencePredict(input, candMap)
                        }
                        result := e.sortCandidates(candMap)
                        if len(result) > 50 {
                                result = result[:50]
                        }
                        return result
                }

                // 模式 2: 首字母缩写（输入 "nh"/"zg" 等多声母组合时）
                // 检查每个字符是否都是合法声母
                if len(input) >= 2 && len(input) <= 6 && isAllInitials(input) {
                        // 构造伪音节
                        pseudoSyls := make([]pinyin.Syllable, len(input))
                        for i, c := range input {
                                pseudoSyls[i] = pinyin.Syllable{Initial: string(c), Final: "", Raw: string(c)}
                        }
                        e.acronymMatch(pseudoSyls, candMap)
                        result := e.sortCandidates(candMap)
                        if len(result) > 50 {
                                result = result[:50]
                        }
                        return result
                }

                return nil
        }

        // 收集候选，用 map 去重
        candMap := make(map[string]*Candidate)

        // 1. 精确匹配
        e.exactMatch(syls, candMap)

        // 1.5 整句切分匹配（多音节时启用）
        if len(syls) >= 2 {
                e.segmentMatch(syls, candMap)
        }

        // 1.6 首字母缩写联想（多音节短输入）
        if len(syls) >= 2 && len(input) <= 6 {
                e.acronymMatch(syls, candMap)
        }

        // 1.7 简拼/全拼混合匹配（搜狗核心特性）
        // 输入 "nhao" → "你好"（n 简拼 + hao 全拼）
        // 输入 "shfa" → "书法"（sh 简拼 + fa 全拼）
        // 输入 "zhg"  → "中国"（zh 简拼 + g 简拼，acronymMatch 已覆盖，这里处理混合）
        // 当切分中既有单字符声母（简拼）又有完整音节（全拼）时触发
        if len(syls) >= 2 && hasMixedInitialAndFull(syls) {
                e.mixedMatch(syls, candMap)
        }

        // 2. 模糊音匹配
        e.fuzzyMatch(syls, candMap)

        // 3. 拼写错误容错
        e.typoMatch(input, syls, candMap)

        // 3.5 声母遗漏容错（仅对单韵母输入）
        if len(syls) == 1 && pinyin.IsFinal(syls[0].Raw) {
                e.missingInitialMatch(syls[0].Raw, candMap)
        }

        // 4. 前缀匹配（输入过程中）
        if len(candMap) < 10 {
                e.prefixMatch(input, candMap)
        }

        // 4.6 混合音节前缀匹配：输入 "henh"（hen + h）时，
        // 用完整音节串 + 尾部声母做前缀查找，能匹配 "henhao" → "很好"
        // 这样用户输入到 henh 就能看到"很好"候选，不必输完整 henhao
        if hasTrailingInitial(syls) {
                e.mixedInitialMatch(syls, candMap)
        }

        // 4.7 整句容错匹配：对输入做 DP 切分，尝试把输入拆成多个词组组合。
        // 能处理：
        //   - 漏字：nizanal -> 你在哪里（ni zai na li）
        //   - 混合缩写：wzaiszdn -> 我在深圳等你（w zai s z d n）
        // 触发条件放宽：长句（>=5 音节）总是触发（搜狗长句核心能力），
        // 短句候选不足 15 时触发（之前门槛 <5 太高，长句常被短路）
        if len(syls) >= 5 || len(candMap) < 15 {
                e.sentenceMatch(input, candMap)
        }

        // 4.5 单声母联想：输入 "n" 等单声母时返回高频字
        if len(syls) == 1 && pinyin.IsInitial(syls[0].Raw) && len(candMap) < 5 {
                e.singleInitialMatch(syls[0].Raw, candMap)
        }

        // 4.55 长距容错匹配（搜狗核心特性）：处理长距离漏字/错字
        // 例：woyaochfan（漏 i）→ 我要吃饭；wyacf（首字母缩写）→ 我要吃饭
        // 触发条件：长输入（>=3 音节）+ 候选不足 20 时
        if len(syls) >= 3 && len(candMap) < 20 {
                e.longDistanceMatch(input, syls, candMap)
        }

        // 4.8 续接联想（搜狗核心特性）：基于上一次提交的末字 + bigram 预测
        // 例：上次提交"今天天"，本次输入"q" → 联想"气怎么样"（天→气 bigram 续接）
        // 触发条件：有提交历史 + 输入较短（<=4 音节，避免长句干扰）
        if len(e.commitHistory) > 0 && len(syls) <= 4 {
                e.continuationMatch(input, syls, candMap)
        }

        // 5. 排序
        result := e.sortCandidates(candMap)

        // 限制返回数量
        if len(result) > 50 {
                result = result[:50]
        }
        return result
}

// exactMatch 精确匹配
func (e *Engine) exactMatch(syls []pinyin.Syllable, out map[string]*Candidate) {
        joined := pinyin.Join(syls)
        entries := e.dict.Lookup(joined)
        for _, ent := range entries {
                key := ent.Word + "|" + ent.Pinyin
                if _, ok := out[key]; !ok {
                        out[key] = &Candidate{
                                Word:   ent.Word,
                                Pinyin: ent.Pinyin,
                                Score:  e.wPinyinMatch + ent.Freq*e.wFreq,
                                Source: "dict",
                        }
                }
        }

        // 子序列匹配仅在整体 lookup 失败时启用，避免与整词竞争
        // 展开 head×tail 的 Top-K 组合（之前只取每段第 1 个，覆盖面窄）
        if len(syls) >= 2 && len(entries) == 0 {
                const topK = 3 // 每段取 Top 3
                for i := 1; i < len(syls); i++ {
                        headSyls := syls[:i]
                        tailSyls := syls[i:]
                        headJoined := pinyin.Join(headSyls)
                        tailJoined := pinyin.Join(tailSyls)
                        headEntries := e.dict.Lookup(headJoined)
                        tailEntries := e.dict.Lookup(tailJoined)
                        if len(headEntries) == 0 || len(tailEntries) == 0 {
                                continue
                        }
                        // 限制每段取 Top-K，避免组合爆炸（最多 topK*topK=9 组合/切分点）
                        hLimit := topK
                        if hLimit > len(headEntries) {
                                hLimit = len(headEntries)
                        }
                        tLimit := topK
                        if tLimit > len(tailEntries) {
                                tLimit = len(tailEntries)
                        }
                        for hi := 0; hi < hLimit; hi++ {
                                for ti := 0; ti < tLimit; ti++ {
                                        combined := headEntries[hi].Word + tailEntries[ti].Word
                                        combinedPy := headJoined + tailJoined
                                        key := combined + "|" + combinedPy
                                        if _, ok := out[key]; !ok {
                                                // 子序列组合分数低于整词，避免压过整词
                                                // 排名越靠后分越低（hi+ti 越大分越低）
                                                rankPenalty := 1.0 - float64(hi+ti)*0.1
                                                if rankPenalty < 0.5 {
                                                        rankPenalty = 0.5
                                                }
                                                score := (e.wPinyinMatch*0.3 +
                                                        (headEntries[hi].Freq+tailEntries[ti].Freq)*e.wFreq*0.1) * rankPenalty
                                                out[key] = &Candidate{
                                                        Word:   combined,
                                                        Pinyin: combinedPy,
                                                        Score:  score,
                                                        Source: "dict",
                                                }
                                        }
                                }
                        }
                }
        }
}

// segmentMatch 整句切分匹配
// 用动态规划找出最优词组合，作为候选
// segmentMatch 整句切分匹配（多候选版，借鉴搜狗 N-best 切分）
// 用 DP 找最优切分，再对每段取 Top-K 候选做笛卡尔积组合，
// 用 bigram 评分选 Top-N 整句候选（而非只输出单一最优切分）。
//
// 之前只产生 1 个候选，且 bigram 因字节索引 bug 失效；
// 现在修复 bigram 后，多候选竞争能让长句首选准确率显著提升。
func (e *Engine) segmentMatch(syls []pinyin.Syllable, out map[string]*Candidate) {
	joined := pinyin.Join(syls)
	// 已经被 exactMatch 命中的整词，跳过
	if entries := e.dict.Lookup(joined); len(entries) > 0 {
		return
	}
	// 切分并获取每段 Top-K 候选（SegmentAndCombine 已实现但之前未接入）
	const topK = 3 // 每段取 Top 3 候选词
	const maxCandidates = 10 // 最多输出 10 个整句候选
	segments := e.segmenter.SegmentAndCombine(joined, topK)
	if len(segments) <= 1 {
		return // 切分失败或单字
	}

	// 检查所有段是否都有候选
	for _, seg := range segments {
		if len(seg) == 0 {
			return // 有段切不出来，放弃
		}
	}

	// 笛卡尔积组合 + Beam Search 剪枝
	// beam: 当前保留的候选组合列表，每个元素是 (wordSeq, pySeq, bigramScore)
	type beamItem struct {
		words []string
		pys   []string
		score float64 // 累计 bigram log prob
	}
	const beamWidth = 8 // Beam 宽度，控制组合爆炸

	beam := []beamItem{{words: nil, pys: nil, score: 0}}
	for _, seg := range segments {
		var newBeam []beamItem
		for _, bi := range beam {
			for _, ent := range seg {
				newWords := append(append([]string(nil), bi.words...), ent.Word)
				newPys := append(append([]string(nil), bi.pys...), ent.Pinyin)
				// 增量计算 bigram：新词与前一词的连接分
				var incScore float64
				if len(bi.words) == 0 {
					incScore = e.bigramLogProb([]string{ent.Word})
				} else {
					// 前词末字 + 新词首字 + 新词内部
					prev := bi.words[len(bi.words)-1]
					incScore = e.bigramConnLogProb(prev, ent.Word)
				}
				newBeam = append(newBeam, beamItem{
					words: newWords,
					pys:   newPys,
					score: bi.score + incScore,
				})
			}
		}
		// Beam 剪枝：按 score 降序保留 Top beamWidth
		if len(newBeam) > beamWidth {
			sort.Slice(newBeam, func(i, j int) bool {
				return newBeam[i].score > newBeam[j].score
			})
			newBeam = newBeam[:beamWidth]
		}
		beam = newBeam
	}

	// 按 bigram 分数排序输出 Top-N
	sort.Slice(beam, func(i, j int) bool {
		return beam[i].score > beam[j].score
	})
	count := 0
	for _, bi := range beam {
		if count >= maxCandidates {
			break
		}
		if len(bi.words) == 0 {
			continue
		}
		word := strings.Join(bi.words, "")
		py := strings.Join(bi.pys, "")
		key := word + "|" + py
		if _, ok := out[key]; ok {
			continue
		}
		// 评分：切分质量 + bigram 分数转换
		// bigram score 是负数（log prob），越接近 0 越好
		// 转换为正向加分：qualityBonus = (baseBM - score) * 2，分数越高 bonus 越小
		baseScore := e.wPinyinMatch * 0.7
		// bigram 越好（score 越接近 0），bonus 越高
		// score 典型范围 -30 ~ -5，映射到 bonus 0 ~ 30
		bonus := 0.0
		if bi.score > -30 {
			bonus = (30 + bi.score) * 1.0 // score=-5→25, score=-20→10, score=-30→0
			if bonus < 0 {
				bonus = 0
			}
		}
		out[key] = &Candidate{
			Word:   word,
			Pinyin: py,
			Score:  baseScore + bonus,
			Source: "segment",
		}
		count++
	}
}

// bigramLogProb 单词的 bigram 自身分数（词内字符对 + 句首）
func (e *Engine) bigramLogProb(words []string) float64 {
	if !e.segmenter.HasBigram() {
		return -10 // 无 bigram 时给固定负分
	}
	return e.segmenter.BigramSentenceLogProb(words)
}

// bigramConnLogProb 两个词连接处的 bigram 分数
// 计算前词末字 + 新词首字 + 新词内部字符对
func (e *Engine) bigramConnLogProb(prev, cur string) float64 {
	if !e.segmenter.HasBigram() || prev == "" || cur == "" {
		return -10
	}
	// 用 segmenter 的接口计算 [prev, cur] 的 bigram 分
	// 减去 prev 单独的分数，得到连接处的增量
	scoreBoth := e.segmenter.BigramSentenceLogProb([]string{prev, cur})
	scorePrev := e.segmenter.BigramSentenceLogProb([]string{prev})
	return scoreBoth - scorePrev
}

// isAllInitials 检查字符串每个字符是否都是合法声母（单字符）
func isAllInitials(s string) bool {
        if len(s) == 0 {
                return false
        }
        for _, c := range s {
                if !pinyin.IsInitial(string(c)) {
                        return false
                }
        }
        return true
}

// acronymMatch 首字母缩写联想
// 输入 "nh" → 联想 "你好"（n+h 首字母缩写）
// 输入 "zg" → 联想 "中国"
// 输入 "kk" → 联想 "看看","可靠","开口" 等
// 优化：直接使用 dict 预计算的 acronymIndex（O(1)），
// 避免遍历所有以首声母开头的拼音并逐个 Segment（之前 130 万词条下 ~200ms，现在 <1ms）
func (e *Engine) acronymMatch(syls []pinyin.Syllable, out map[string]*Candidate) {
	// 只对每个音节都是单字母（即纯声母）的情况生效
	for _, s := range syls {
		if len(s.Raw) != 1 || !pinyin.IsInitial(s.Raw) {
			return // 不是首字母缩写模式
		}
	}

	// 拼接声母缩写
	acronym := ""
	for _, s := range syls {
		acronym += s.Raw
	}

	// 用预计算索引直接查
	entries := e.dict.LookupByAcronym(acronym)
	count := 0
	for _, ent := range entries {
		if count >= 10 {
			break
		}
		key := ent.Word + "|" + ent.Pinyin
		if _, exists := out[key]; !exists {
			score := e.wPinyinMatch*0.4 + ent.Freq*e.wFreq*0.3
			out[key] = &Candidate{
				Word:   ent.Word,
				Pinyin: ent.Pinyin,
				Score:  score,
				Source: "acronym",
			}
			count++
		}
	}
}

// hasMixedInitialAndFull 检测切分中是否既有单字符声母（简拼）又有完整音节（全拼）
// 用于触发简拼/全拼混合匹配（搜狗核心特性）
// 例：[n, hao] → true（n 简拼 + hao 全拼）
//     [ni, hao] → false（全是全拼）
//     [n, h] → false（全是简拼，由 acronymMatch 处理）
// 注意：zh/ch/sh 作为单字符声母处理时，Raw 长度是 2，需特殊判断
func hasMixedInitialAndFull(syls []pinyin.Syllable) bool {
	hasShort := false // 有简拼（单字符声母）
	hasFull := false  // 有全拼（带韵母的完整音节）
	for _, s := range syls {
		if s.Final == "" && s.Initial != "" {
			// 纯声母音节（简拼）：Raw 是单字符声母，或 zh/ch/sh
			if len(s.Raw) <= 2 && pinyin.IsInitial(s.Raw) {
				hasShort = true
			}
		} else if s.Initial != "" && s.Final != "" {
			// 完整音节（全拼）
			hasFull = true
		}
	}
	return hasShort && hasFull
}

// mixedMatch 简拼/全拼混合匹配（搜狗核心特性）
// 处理输入中简拼和全拼任意混合的情况：
//   - "nhao"  → "你好"（n 简拼 + hao 全拼）
//   - "shfa"  → "书法"（sh 简拼 + fa 全拼）
//   - "nih"   → "你好"（ni 全拼 + h 简拼）
//   - "nhaoa" → "你好啊"（n 简拼 + hao 全拼 + a 全拼）
//
// 算法：把简拼音节展开为可能的完整音节前缀，用 trie 前缀查找匹配的词条，
// 然后按"简拼音节数"分级评分（简拼越少分越高，符合搜狗的匹配度排序）。
func (e *Engine) mixedMatch(syls []pinyin.Syllable, out map[string]*Candidate) {
	// 统计简拼和全拼音节
	shortCount := 0 // 简拼音节数
	for _, s := range syls {
		if s.Final == "" && s.Initial != "" && len(s.Raw) <= 2 && pinyin.IsInitial(s.Raw) {
			shortCount++
		}
	}
	if shortCount == 0 {
		return // 无简拼，不触发
	}

	// 构造查询模式：简拼音节用声母做前缀，全拼音节用完整拼音
	// 用 trie 前缀查找所有匹配的拼音串，再 lookup 对应词条
	// 例：[n, hao] → 前缀 "n" + 精确 "hao" → 查找所有 "n*hao" 形式的拼音
	//
	// 实现策略：递归展开简拼音节为所有可能的声母开头拼音前缀，
	// 与全拼音节拼接，做 trie 前缀查找。为控制组合爆炸，限制总候选数。

	const maxCandidates = 30
	// 递归收集所有可能的拼音组合
	// 用 DFS 生成拼音组合并查找
	var dfs func(idx int, curPy string, shortUsed int)
	count := 0
	dfs = func(idx int, curPy string, shortUsed int) {
		if count >= maxCandidates {
			return
		}
		if idx == len(syls) {
			// 完整组合，做精确 lookup
			entries := e.dict.Lookup(curPy)
			for i, ent := range entries {
				if i >= 3 {
					break
				}
				if count >= maxCandidates {
					return
				}
				// 词长必须等于音节数
				if len([]rune(ent.Word)) != len(syls) {
					continue
				}
				key := ent.Word + "|" + ent.Pinyin
				if _, exists := out[key]; !exists {
					// 分级评分：简拼越少分越高
					// 全拼匹配度最高(1.0)，每多一个简拼降 0.15
					matchRatio := 1.0 - float64(shortUsed)*0.15
					if matchRatio < 0.4 {
						matchRatio = 0.4
					}
					score := e.wPinyinMatch*0.6*matchRatio + ent.Freq*e.wFreq*0.4
					out[key] = &Candidate{
						Word:   ent.Word,
						Pinyin: ent.Pinyin,
						Score:  score,
						Source: "mixed",
					}
					count++
				}
			}
			return
		}

		s := syls[idx]
		if s.Final == "" && s.Initial != "" && len(s.Raw) <= 2 && pinyin.IsInitial(s.Raw) {
			// 简拼音节：展开为所有以该声母开头的完整音节
			// 用 LookupByInitial 获取该声母的高频单字（已按词频降序），
			// 取其拼音作为展开候选，保证高频音节（如 n→ni, na, neng）优先
			initialEntries := e.dict.LookupByInitial(s.Raw)
			// 限制每个简拼展开的候选数，避免组合爆炸
			expanded := 0
			seenPy := make(map[string]bool)
			for _, ent := range initialEntries {
				if expanded >= 12 {
					break
				}
				// 单字拼音即为展开后的完整音节
				fullPy := ent.Pinyin
				if seenPy[fullPy] {
					continue
				}
				seenPy[fullPy] = true
				dfs(idx+1, curPy+fullPy, shortUsed+1)
				expanded++
			}
		} else {
			// 全拼音节：直接拼接
			dfs(idx+1, curPy+s.Raw, shortUsed)
		}
	}

	dfs(0, "", 0)
}

// missingInitialMatch 声母遗漏容错
// 用户输入纯韵母 "ao" 时，联想 "hao"(好)、"nao"(闹)、"gao"(高) 等
func (e *Engine) missingInitialMatch(final string, out map[string]*Candidate) {
        // 枚举所有声母 + 该韵母的组合
        allInitials := []string{"b", "p", "m", "f", "d", "t", "n", "l",
                "g", "k", "h", "j", "q", "x", "r", "z", "c", "s",
                "zh", "ch", "sh", "y", "w"}
        for _, ini := range allInitials {
                combined := ini + final
                // 必须是合法音节
                if !pinyin.IsValidSyllable(combined) {
                        continue
                }
                entries := e.dict.Lookup(combined)
                for i, ent := range entries {
                        if i >= 2 {
                                break
                        }
                        key := ent.Word + "|" + ent.Pinyin
                        if _, exists := out[key]; !exists {
                                // 声母遗漏容错分数较低
                                score := (e.wPinyinMatch + ent.Freq*e.wFreq) * 0.4 * e.wTypo
                                out[key] = &Candidate{
                                        Word:   ent.Word,
                                        Pinyin: ent.Pinyin,
                                        Score:  score,
                                        Source: "missing-initial",
                                }
                        }
                }
        }
}

// singleInitialMatch 单声母联想
// 输入 "n" 等单声母时，返回以该声母开头的高频字
// 优化：直接使用 dict 预计算的 LookupByInitial 缓存（O(1)），
// 避免遍历所有以该声母开头的拼音（之前 130 万词条下 ~200ms，现在 <1ms）
func (e *Engine) singleInitialMatch(initial string, out map[string]*Candidate) {
	entries := e.dict.LookupByInitial(initial)
	count := 0
	for _, ent := range entries {
		if count >= 10 {
			break
		}
		key := ent.Word + "|" + ent.Pinyin
		if _, exists := out[key]; !exists {
			// 单声母联想分数较低
			score := e.wPinyinMatch*0.2 + ent.Freq*e.wFreq*0.3
			out[key] = &Candidate{
				Word:   ent.Word,
				Pinyin: ent.Pinyin,
				Score:  score,
				Source: "initial",
			}
			count++
		}
	}
}

// fuzzyMatch 模糊音匹配
func (e *Engine) fuzzyMatch(syls []pinyin.Syllable, out map[string]*Candidate) {
        rawSyls := make([]string, len(syls))
        for i, s := range syls {
                rawSyls[i] = s.Raw
        }
        // 用带模糊音节数的展开，实现分级评分
        // 模糊音节越少分越高（搜狗的模糊距离分级）
        combos := e.fuzzy.ExpandAllWithCount(rawSyls)
        origJoined := pinyin.Join(syls)
        totalSyls := len(syls)
        for _, combo := range combos {
                joined := strings.Join(combo.Syls, "")
                // 跳过和原始相同的（已由 exactMatch 处理）
                if joined == origJoined {
                        continue
                }
                entries := e.dict.Lookup(joined)
                // 分级评分：模糊音节数越少，折扣越少
                // 0 个模糊=1.0（不应出现，已跳过），1 个模糊=0.9，2 个=0.8，依此类推
                // 相比旧版统一 0.7 折扣，模糊少的候选现在能排更高
                fuzzyRatio := 1.0 - float64(combo.FuzzyCount)*0.1
                if fuzzyRatio < 0.4 {
                        fuzzyRatio = 0.4
                }
                // 归一化到音节数：模糊 1 个音节在 2 音节词里影响 50%，在 5 音节词里只影响 20%
                if totalSyls > 0 {
                        fuzzyRatio = 1.0 - float64(combo.FuzzyCount)/float64(totalSyls)*0.5
                }
                for _, ent := range entries {
                        key := ent.Word + "|" + ent.Pinyin
                        if existing, ok := out[key]; ok {
                                // 取较高分
                                if existing.Source == "dict" {
                                        continue
                                }
                        }
                        if _, ok := out[key]; !ok {
                                out[key] = &Candidate{
                                        Word:   ent.Word,
                                        Pinyin: ent.Pinyin,
                                        Score:  (e.wPinyinMatch + ent.Freq*e.wFreq) * e.wFuzzy * fuzzyRatio,
                                        Source: "fuzzy",
                                }
                        }
                }
        }
}

// typoMatch 拼写错误容错（增强版，覆盖搜狗级联纠错的第一层）
// 支持 4 种错误类型，每种最多一处操作：
//   - 替换（replace）：邻键误触，如 nihap→nihao
//   - 删除（delete）：多打字，如 nihaoo→nihao
//   - 插入（insert）：漏打字，如 niha→nihao（受邻键限制）
//   - 转置（transpose）：相邻颠倒，如 nihoa→nihao
//
// 评分分级：替换>转置>删除>插入（基于常见误打统计），
// 每种都按 wTypo 折扣，但替换折扣最少（最可能是真实意图）。
func (e *Engine) typoMatch(originalInput string, syls []pinyin.Syllable, out map[string]*Candidate) {
        origSylCount := len(syls)
        variants := e.fuzzy.TypoVariantsDetailed(originalInput)

        // 每种错误类型的评分系数（越小分越低）
        // 替换最常见且保留长度，置信度最高；插入/删除改变长度，置信度较低
        kindWeight := map[string]float64{
                "replace":   1.0,
                "transpose": 0.9,
                "delete":    0.8,
                "insert":    0.7,
        }

        for _, v := range variants {
                if v.Text == originalInput || v.Cost == 0 {
                        continue
                }
                varSyls := pinyin.Segment(v.Text)
                if len(varSyls) == 0 {
                        continue
                }
                // 音节数约束：
                // - replace 不改变长度，音节数应一致
                // - transpose/insert/delete 可能改变切分，允许音节数差 1
                //   例：nihoa(3音节) 转置→ nihao(2音节)
                //       nihaoo(3音节) 删除→ nihao(2音节)
                sylDelta := len(varSyls) - origSylCount
                switch v.Kind {
                case "replace":
                        if sylDelta != 0 {
                                continue
                        }
                default: // transpose, delete, insert
                        if sylDelta < -1 || sylDelta > 1 {
                                continue
                        }
                }
                joined := pinyin.Join(varSyls)
                entries := e.dict.Lookup(joined)
                weight := kindWeight[v.Kind]
                if weight == 0 {
                        weight = 0.7
                }
                // 词长必须等于变体音节数（删除/插入后音节数可能变）
                expectedLen := len(varSyls)
                for _, ent := range entries {
                        if len([]rune(ent.Word)) != expectedLen {
                                continue
                        }
                        key := ent.Word + "|" + ent.Pinyin
                        if _, ok := out[key]; !ok {
                                score := (e.wPinyinMatch + ent.Freq*e.wFreq) * e.wTypo * weight
                                out[key] = &Candidate{
                                        Word:   ent.Word,
                                        Pinyin: ent.Pinyin,
                                        Score:  score,
                                        Source: "typo",
                                }
                        }
                }
        }
}

// prefixMatch 前缀匹配（用于输入过程中）
// 优化：用 LookupPrefixEntries 一次性获取前缀下所有词条（按词频降序），
// 避免逐个 Lookup（之前 130 万词条下输入 "n" 要 ~200ms）
func (e *Engine) prefixMatch(input string, out map[string]*Candidate) {
	entries := e.dict.LookupPrefixEntries(input, 50)
	for _, ent := range entries {
		if ent.Pinyin == input {
			continue // 已被 exactMatch 处理
		}
		key := ent.Word + "|" + ent.Pinyin
		if _, ok := out[key]; !ok {
			out[key] = &Candidate{
				Word:   ent.Word,
				Pinyin: ent.Pinyin,
				Score:  e.wPinyinMatch*0.5 + ent.Freq*e.wFreq*0.3,
				Source: "dict",
			}
		}
	}
}

// sentenceMatch 整句容错匹配
//
// 对输入串做 DP 切分，每个位置尝试：
//  1. 完整音节（1-6 字符，IsValidSyllable）
//  2. 单字符声母（b/p/m/f/...，作为词首字母缩写）
//
// 切分后，对每个音节序列做"贪心最长词组匹配"：
// 从左到右，尝试匹配词典中最长的词组（1-4 音节），
// 把匹配到的词组拼接成整句候选。
//
// 这能处理：
//   - 漏字：nizanal -> ni+zai+na+li -> 你在哪里（zanal 容错切分为 zai+na+li）
//   - 混合缩写：wzaiszdn -> w+zai+s+z+d+n -> 我+在+深+圳+等+你
//
// 为了控制复杂度，最多尝试前 N 种切分，且总候选数上限 30。
// 之前上限 20 切分/10 候选对长句不够，放宽到 50 切分/20 候选。
func (e *Engine) sentenceMatch(input string, out map[string]*Candidate) {
	if len(input) < 3 {
		return
	}
	// 收集所有可能的切分（限制数量避免爆炸）
	// 长句放宽切分上限，让更多切分路径参与竞争
	splitLimit := 50
	if len(input) > 15 {
		splitLimit = 80 // 长句允许更多切分
	}
	var allSplits [][]string
	collectSplits(input, 0, nil, &allSplits, splitLimit)

        type result struct {
                word    string
                pinyin  string
                segs    int // 匹配的音节数
                covered int // 覆盖的输入字符数
        }
        var results []result

        for _, split := range allSplits {
                // 贪心最长匹配：从左到右匹配词组
                word, py, segs, covered := e.greedyMatchSentence(split)
                if word != "" && segs >= 2 {
                        results = append(results, result{word, py, segs, covered})
                }
        }

        // 按覆盖字符数降序、音节数降序排序
        sort.Slice(results, func(i, j int) bool {
                if results[i].covered != results[j].covered {
                        return results[i].covered > results[j].covered
                }
                return results[i].segs > results[j].segs
        })

        added := 0
        for _, r := range results {
                if added >= 20 {
                        break
                }
                key := r.word + "|" + r.pinyin
                if _, ok := out[key]; ok {
                        continue
                }
                // 分数根据覆盖率：覆盖率越高分越高
                coverage := float64(r.covered) / float64(len(input))
                score := e.wPinyinMatch * 0.6 * coverage
                out[key] = &Candidate{
                        Word:   r.word,
                        Pinyin: r.pinyin,
                        Score:  score,
                        Source: "sentence",
                }
                added++
        }
}

// collectSplits 递归收集输入串的所有可能切分
// 每个切分元素要么是完整音节，要么是单字符声母
func collectSplits(input string, pos int, cur []string, out *[][]string, limit int) {
        if len(*out) >= limit {
                return
        }
        if pos == len(input) {
                cp := make([]string, len(cur))
                copy(cp, cur)
                *out = append(*out, cp)
                return
        }
        // 尝试音节长度 1-6
        for l := 1; l <= 6 && pos+l <= len(input); l++ {
                syl := input[pos : pos+l]
                if pinyin.IsValidSyllable(syl) {
                        cur = append(cur, syl)
                        collectSplits(input, pos+l, cur, out, limit)
                        cur = cur[:len(cur)-1]
                } else if l == 1 && pinyin.IsInitial(syl) {
                        // 单字符声母作为缩写音节
                        cur = append(cur, syl)
                        collectSplits(input, pos+l, cur, out, limit)
                        cur = cur[:len(cur)-1]
                }
        }
}

// greedyMatchSentence 贪心最长词组匹配
// 对切分后的音节序列，从左到右尝试匹配 1-6 音节的词组
// 返回拼接的整句、拼音、匹配音节数、覆盖字符数
//
// 匹配策略（按优先级）：
//  1. 精确匹配：joined 完整音节串在词典中
//  2. 前缀扩展匹配：如果当前 span 末尾是单字符声母（缩写），
//     把它和前面的音节合并做前缀查找。例如 [hen][gao][x] 中
//     匹配到 [gao][x] 时，用 "gaox" 前缀查到 "gaoxing"(高兴)
//
// 这样 hengaox -> hen(很) + gao+x(高兴) = 很高兴
//
// span 上限 6（之前 4）：支持更长词组，如"woyaoqu"(我要去)、
// "bowuguan"(博物馆)等 3-6 音节词组，提升长句首选准确率
func (e *Engine) greedyMatchSentence(syls []string) (string, string, int, int) {
        var word, py strings.Builder
        segs := 0
        covered := 0
        i := 0
        for i < len(syls) {
                matched := false
                // 尝试 6-1 音节的词组（最长优先）
                maxSpan := 6
                if len(syls)-i < maxSpan {
                        maxSpan = len(syls) - i
                }
                for span := maxSpan; span >= 1; span-- {
                        joined := strings.Join(syls[i:i+span], "")
                        // 策略 1: 精确匹配
                        entries := e.dict.Lookup(joined)
                        if len(entries) > 0 {
                                ent := entries[0]
                                word.WriteString(ent.Word)
                                py.WriteString(ent.Pinyin)
                                segs += span
                                for j := i; j < i+span; j++ {
                                        covered += len(syls[j])
                                }
                                i += span
                                matched = true
                                break
                        }
                        // 策略 2: 前缀扩展匹配
                        // 仅当 span >= 2 且最后一个音节是单字符声母时
                        if span >= 2 && len(syls[i+span-1]) == 1 && pinyin.IsInitial(syls[i+span-1]) {
                                prefixes := e.dict.LookupPrefix(joined)
                                if len(prefixes) > 0 {
                                        // 取第一个匹配的前缀对应的词
                                        for _, pf := range prefixes {
                                                pfEntries := e.dict.Lookup(pf)
                                                if len(pfEntries) > 0 {
                                                        ent := pfEntries[0]
                                                        word.WriteString(ent.Word)
                                                        py.WriteString(ent.Pinyin)
                                                        segs += span
                                                        for j := i; j < i+span; j++ {
                                                                covered += len(syls[j])
                                                        }
                                                        i += span
                                                        matched = true
                                                        break
                                                }
                                        }
                                        if matched {
                                                break
                                        }
                                }
                        }
                }
                if !matched {
                        // 无法匹配，跳过当前音节
                        i++
                }
        }
        return word.String(), py.String(), segs, covered
}


// continuationMatch 续接联想（搜狗核心特性）
// 基于上一次提交的末字 + bigram 模型预测下一个可能的字/词，
// 再结合当前输入拼音做前缀查找，生成"前词+续接词"的整句候选。
//
// 例：上次提交"今天天"，本次输入"q"
//   1. 取末字"天"作为上下文锚点
//   2. bigram.TopNext("天") 返回 [气, 上, 下, 里, ...]（按概率降序）
//   3. 对每个续接字，查其拼音是否以 "q" 开头（气→qi，匹配！）
//   4. 用"气"做前缀查找词典，得到"气怎么样""气候""气氛"等
//   5. 生成候选"今天天气怎么样"等，加分基于 bigram 概率
//
// 同时查 contextPairs（用户私有共现）做个性化续接。
func (e *Engine) continuationMatch(input string, syls []pinyin.Syllable, out map[string]*Candidate) {
	if len(e.commitHistory) == 0 {
		return
	}
	lastCommit := e.commitHistory[len(e.commitHistory)-1]
	if lastCommit == "" {
		return
	}
	// 取末字（rune 索引）
	lastChars := []rune(lastCommit)
	if len(lastChars) == 0 {
		return
	}
	lastChar := string(lastChars[len(lastChars)-1])

	// 策略 1：bigram 续接预测
	// 取末字的 Top-K 续接字，对每个续接字检查拼音是否匹配当前输入
	if e.segmenter.HasBigram() {
		const topK = 15
		nexts := e.segmenter.BigramTopNext(lastChar, topK)
		for _, nx := range nexts {
			nxPy := e.lookupCharPinyin(nx.Word)
			if nxPy == "" {
				continue
			}
			// 检查续接字拼音是否以当前输入开头（前缀匹配）
			if !pyMatchesInput(nxPy, input) {
				continue
			}
			// 续接字匹配！现在用续接字做前缀查找，扩展为多字词
			e.expandContinuation(lastCommit, nx, input, out)
		}
	}

	// 策略 2：用户私有 contextPairs 续接
	// 查 contextPairs 中 prev=lastCommit 或 prev=lastChar 的续接词
	e.userContextContinuation(lastCommit, lastChar, input, out)
}

// lookupCharPinyin 查询单字的拼音（从词典反查）
// 返回第一个匹配的拼音（多音字取最常用）
// 用 dict 的 charToPinyin 缓存，O(1) 查询
func (e *Engine) lookupCharPinyin(ch string) string {
	if ch == "" {
		return ""
	}
	return e.dict.LookupCharPinyin(ch)
}

// pyMatchesInput 检查拼音 py 是否匹配用户输入 input
// 匹配规则：
//   - input 是 py 的前缀（如 input="q", py="qi" → 匹配）
//   - input 是 py 的声母缩写（如 input="qx", py="qixy" → 不匹配，需完整音节）
//   - input == py（精确匹配）
func pyMatchesInput(py, input string) bool {
	if input == "" || py == "" {
		return false
	}
	// 前缀匹配
	if strings.HasPrefix(py, input) {
		return true
	}
	return false
}

// expandContinuation 用续接字做前缀查找，生成"前词+续接词"的整句候选
// lastCommit: 上次提交的词（如"今天天"）
// nextEntry: bigram 预测的续接字（如"气"）
// input: 当前输入（如"q"）
//
// 三种扩展策略（按分数从高到低排序）：
//  1. 词典前缀查找：从词典找出以续接字拼音开头的多字词（如"气息""气体"）
//  2. 多级 bigram 链：用 bigram 模型递归续接，生成"气怎么样"这样的连续词组
//     （词典可能没有"气怎么样"，但 bigram("气","怎")("怎","么")("么","样") 概率很高）
//  3. 单字续接兜底：只有续接字本身（如"今天天气"）
func (e *Engine) expandContinuation(lastCommit string, nx segmenter.NextEntry, input string, out map[string]*Candidate) {
	// 用续接字的拼音做前缀查找，获取多字词候选
	nxPy := e.lookupCharPinyin(nx.Word)
	if nxPy == "" {
		return
	}

	bonus := 0.0
	if nx.LogProb > -10 {
		bonus = (10 + nx.LogProb) * 5 // -2→40, -5→25, -10→0
		if bonus < 0 {
			bonus = 0
		}
	}

	// 策略 1：词典前缀查找（如"气息""气体"）
	entries := e.dict.LookupPrefixEntries(nxPy, 20)
	dictCount := 0
	for _, ent := range entries {
		if dictCount >= 5 {
			break
		}
		// 只取以续接字开头的词
		if !strings.HasPrefix(ent.Word, nx.Word) {
			continue
		}
		combined := lastCommit + ent.Word
		key := combined + "|"
		if _, exists := out[key]; exists {
			continue
		}
		// 评分：与 dict 同级基础分 + bigram bonus + 词频
		score := e.wPinyinMatch + bonus + ent.Freq*e.wFreq*0.5
		out[key] = &Candidate{
			Word:   combined,
			Pinyin: "",
			Score:  score,
			Source: "continuation",
		}
		dictCount++
	}

	// 策略 2：多级 bigram 链扩展（如"气"→"气怎"→"气怎么"→"气怎么样"）
	// 用 Beam Search，每级保留 Top-K，最多扩展到 4 字
	e.bigramChainExpand(lastCommit, nx, out)

	// 策略 3：高频问句后缀扩展
	// 当续接字是单字（如"气"）时，附加常见问句后缀词（如"怎么样""怎么办"）
	// 生成"气怎么样""气怎么办"等候选。这是 char-level bigram 的补充：
	// bigram("气","怎") 通常 OOV，但"怎么样"是高频词，作为整体附加更合理。
	e.questionSuffixExpand(lastCommit, nx, out)

	// 策略 4：单字续接兜底
	if dictCount == 0 {
		combined := lastCommit + nx.Word
		key := combined + "|"
		if _, exists := out[key]; !exists {
			score := e.wPinyinMatch + bonus
			out[key] = &Candidate{
				Word:   combined,
				Pinyin: "",
				Score:  score,
				Source: "continuation",
			}
		}
	}
}

// questionSuffixExpand 高频问句后缀扩展
// 对续接字 nx.Word（如"气"），附加常见问句后缀词（如"怎么样"），生成"气怎么样"等候选。
//
// 这是 char-level bigram 的补充：bigram("气","怎") 通常 OOV，但"怎么样"作为
// 整体高频词附加更合理（搜狗输入法也是靠 word-level 模型实现这种续接）。
//
// 后缀词列表为常见问句词，评分基于：
//   - 后缀词词频（高词频加分）
//   - bigram 链整体评分（nx.Word + 后缀词内部 bigram）
func (e *Engine) questionSuffixExpand(lastCommit string, nx segmenter.NextEntry, out map[string]*Candidate) {
	// 高频问句后缀词（按词频排序，常见问句词）
	suffixes := []string{
		"怎么样", "怎么办", "是什么", "为什么", "多少",
		"吗", "呢", "吧", "好吗", "行吗", "了", "的",
	}

	nxBonus := 0.0
	if nx.LogProb > -10 {
		nxBonus = (10 + nx.LogProb) * 5
		if nxBonus < 0 {
			nxBonus = 0
		}
	}

	for _, suf := range suffixes {
		combined := lastCommit + nx.Word + suf
		key := combined + "|"
		if _, exists := out[key]; exists {
			continue
		}
		// 查后缀词词频（如果在词典中）
		sufFreq := 0.0
		if entries := e.dict.Lookup(e.wordPinyin(suf)); len(entries) > 0 {
			for _, ent := range entries {
				if ent.Word == suf && ent.Freq > sufFreq {
					sufFreq = ent.Freq
				}
			}
		}
		// 计算后缀词内部的 bigram 评分（如"怎么样"的 怎→么→样）
		// 这反映后缀词本身的连贯性
		sufBigramScore := e.bigramLogProb([]string{suf})
		// 评分：基础分 + nx 的 bigram bonus + 后缀词频加成 + 后缀 bigram 加成
		// sufBigramScore 范围约 -10 ~ -3，转换为正向 bonus
		sufBonus := 0.0
		if sufBigramScore > -15 {
			sufBonus = (15 + sufBigramScore) * 2
			if sufBonus < 0 {
				sufBonus = 0
			}
		}
		// 按后缀长度分级词频权重：
		//   1字（吗/呢/吧/了/的）权重低（太通用，词频高但语义弱）
		//   2字（好吗/行吗）权重中
		//   3字（怎么样/怎么办/是什么/为什么/多少）权重高（问句核心词）
		// 这样"怎么样"能超过"了"排在前面
		sufLen := len([]rune(suf))
		freqWeight := 0.1
		switch sufLen {
		case 2:
			freqWeight = 0.3
		case 3:
			freqWeight = 0.6
		}
		score := e.wPinyinMatch + nxBonus + sufFreq*e.wFreq*freqWeight + sufBonus
		// 长后缀平方级加分（让3字后缀显著占优）
		score += float64(sufLen*sufLen) * 10.0
		out[key] = &Candidate{
			Word:   combined,
			Pinyin: "",
			Score:  score,
			Source: "continuation",
		}
	}
}

// wordPinyin 计算词的拼音（拼接每个字的拼音）
func (e *Engine) wordPinyin(word string) string {
	chars := []rune(word)
	py := ""
	for _, c := range chars {
		py += e.lookupCharPinyin(string(c))
	}
	return py
}

// bigramChainExpand 多级 bigram 续接链扩展
// 从续接字 nx.Word 出发，用 bigram 模型递归预测下一个字，
// 生成"气怎么样"这样的连续词组（词典可能没有，但 bigram 概率连续高）。
//
// 算法：Beam Search
//   - beam: 当前保留的候选链列表，每个元素是 (chain字串, 累计 log prob)
//   - 每一级：对 beam 中每个链，取末字的 Top-K 续接字，扩展出新链
//   - 保留 Top beamWidth 个链进入下一级
//   - 最多扩展到 maxDepth 字（不含起始字）
//
// 例：nx.Word="气"
//   第1级：bigram("气") → [怎, 体, 候, 氛, ...]，生成"气怎""气体""气候"...
//   第2级：bigram("怎") → [么]，生成"气怎么"
//   第3级：bigram("么") → [样], 生成"气怎么样"
//   保留分数最高的几个，作为续接候选
func (e *Engine) bigramChainExpand(lastCommit string, nx segmenter.NextEntry, out map[string]*Candidate) {
	if !e.segmenter.HasBigram() {
		return
	}
	const (
		maxDepth    = 3  // 在起始字之后再扩展 3 个字（总长 4 字）
		beamWidth   = 5  // Beam 宽度
		topKPerStep = 4  // 每步取 Top-4 续接字
		minLogProb  = -8.0 // 低于此 log prob 的续接字跳过（避免低质量扩展）
	)

	type chainItem struct {
		chars   []string // 已生成的字（含起始字）
		logProb float64  // 累计 log prob
	}

	beam := []chainItem{{chars: []string{nx.Word}, logProb: nx.LogProb}}

	for depth := 0; depth < maxDepth; depth++ {
		var newBeam []chainItem
		for _, cur := range beam {
			last := cur.chars[len(cur.chars)-1]
			nexts := e.segmenter.BigramTopNext(last, topKPerStep)
			for _, nxt := range nexts {
				if nxt.LogProb < minLogProb {
					continue
				}
				// 跳过重复字（如"气气"）
				if nxt.Word == last {
					continue
				}
				newChars := make([]string, len(cur.chars)+1)
				copy(newChars, cur.chars)
				newChars[len(cur.chars)] = nxt.Word
				newBeam = append(newBeam, chainItem{
					chars:   newChars,
					logProb: cur.logProb + nxt.LogProb,
				})
			}
		}
		if len(newBeam) == 0 {
			break
		}
		// 按 log prob 降序保留 Top-K
		sort.Slice(newBeam, func(i, j int) bool {
			return newBeam[i].logProb > newBeam[j].logProb
		})
		if len(newBeam) > beamWidth {
			newBeam = newBeam[:beamWidth]
		}
		beam = newBeam

		// 把当前 beam 中的链生成为候选词
		// 仅保留长度 >= depth+2 的链（即至少扩展了 1 字）
		for _, ci := range beam {
			if len(ci.chars) < 2 {
				continue
			}
			chainWord := strings.Join(ci.chars, "")
			// 跳过词典已有的词（避免与策略 1 重复）
			if e.dict.HasWord(chainWord) {
				continue
			}
			combined := lastCommit + chainWord
			key := combined + "|"
			if _, exists := out[key]; exists {
				continue
			}
			// 评分：基础分 + 累计 bigram bonus
			// ci.logProb 范围约 -6 ~ -2，转换为正向 bonus
			chainBonus := 0.0
			if ci.logProb > -15 {
				chainBonus = (15 + ci.logProb) * 4 // -2→52, -6→36, -15→0
				if chainBonus < 0 {
					chainBonus = 0
				}
			}
			// 长链加分（更长更可能是完整词组）
			lengthBonus := float64(len(ci.chars)) * 5.0
			score := e.wPinyinMatch + chainBonus + lengthBonus
			out[key] = &Candidate{
				Word:   combined,
				Pinyin: "",
				Score:  score,
				Source: "continuation",
			}
		}
	}
}

// singleCharSentencePredict 单字母全句补全（搜狗核心特性）
//
// 用户输入单个声母（如"w"）时，结合上下文 N-gram 预测完整句子。
// 与 continuationMatch 的区别：
//   - continuationMatch 只预测"前词+1个续接词"（如"今天天气怎么样"）
//   - singleCharSentencePredict 预测"前词+多词整句"（如"我要去吃饭"）
//
// 算法：从续接字出发，用 bigram 链扩展到 5-8 字的完整句，
// 每一级用首字母匹配用户输入的字母（首字母缩写式整句预测）。
//
// 例：commitHistory=["我"]，input="y"
//   1. bigram("我") → [要, 也, 是, 在, 们, ...]
//   2. 对每个续接字，检查首字母是否匹配 "y"（要→yao→y，匹配！）
//   3. 从"要"出发，bigram 链扩展 4-6 字：
//      要→去→博→物→馆 → 我要去博物馆
//      要→吃→饭 → 我要吃饭
//      要→去→印→度 → 我要去印度
//   4. 用 bigram 累计分数排序
//
// 注意：这是基于上下文的预测，不需要用户输入完整拼音。
// 用户只输入一个字母，就能预测整句，类似搜狗的"超长句预测"。
func (e *Engine) singleCharSentencePredict(input string, out map[string]*Candidate) {
	if !e.segmenter.HasBigram() || len(e.commitHistory) == 0 {
		return
	}
	lastCommit := e.commitHistory[len(e.commitHistory)-1]
	if lastCommit == "" {
		return
	}
	lastChars := []rune(lastCommit)
	if len(lastChars) == 0 {
		return
	}
	lastChar := string(lastChars[len(lastChars)-1])

	// 取 lastChar 的 Top-K 续接字
	// topK=30：扩大候选面，让"要"（"我"的续接字，排名较低）也能进入预测
	const topK = 30
	nexts := e.segmenter.BigramTopNext(lastChar, topK)

	for _, nx := range nexts {
		// 检查续接字拼音是否以 input 开头
		nxPy := e.lookupCharPinyin(nx.Word)
		if nxPy == "" || !pyMatchesInput(nxPy, input) {
			continue
		}
		// 从这个续接字出发，扩展 3-5 级 bigram 链生成完整句
		e.sentenceChainExpand(lastCommit, nx, out)
	}
}

// sentenceChainExpand 整句链扩展
// 从续接字 nx 出发，用 bigram 模型递归扩展 3-5 级，生成完整句候选。
// 与 bigramChainExpand 的区别：
//   - bigramChainExpand 只扩展续接字本身（如"气怎么样"）
//   - sentenceChainExpand 扩展整个句子（如"我要去吃饭"），覆盖更长距离
//
// 算法：Beam Search，每级保留 Top-K，最终保留 Top-N 整句候选
// 评分基于累计 bigram 概率 + 整句长度加成（更长更优先）
func (e *Engine) sentenceChainExpand(lastCommit string, nx segmenter.NextEntry, out map[string]*Candidate) {
	const (
		minDepth    = 2              // 最少扩展 2 级（总长 3 字）
		maxDepth    = 5              // 最多扩展 5 级（总长 6 字）
		beamWidth   = 6              // Beam 宽度（扩大，让更多路径竞争）
		topKPerStep = 5              // 每步取 Top-5 续接字（扩大，覆盖更多低频但合理的续接）
		minLogProb  = -9.0           // 放宽阈值（之前 -7 太严，过滤掉"要→去"等合理但低频续接）
		maxCandidates = 8            // 最多输出 8 个整句候选
	)

	type chainItem struct {
		chars   []string
		logProb float64
	}

	beam := []chainItem{{chars: []string{nx.Word}, logProb: nx.LogProb}}
	bestChains := []chainItem{}

	for depth := 0; depth < maxDepth; depth++ {
		var newBeam []chainItem
		for _, cur := range beam {
			last := cur.chars[len(cur.chars)-1]
			nexts := e.segmenter.BigramTopNext(last, topKPerStep)
			for _, nxt := range nexts {
				if nxt.LogProb < minLogProb {
					continue
				}
				if nxt.Word == last {
					continue
				}
				newChars := make([]string, len(cur.chars)+1)
				copy(newChars, cur.chars)
				newChars[len(cur.chars)] = nxt.Word
				newBeam = append(newBeam, chainItem{
					chars:   newChars,
					logProb: cur.logProb + nxt.LogProb,
				})
			}
		}
		if len(newBeam) == 0 {
			break
		}
		sort.Slice(newBeam, func(i, j int) bool {
			return newBeam[i].logProb > newBeam[j].logProb
		})
		if len(newBeam) > beamWidth {
			newBeam = newBeam[:beamWidth]
		}
		beam = newBeam

		// 达到最短长度后，开始收集候选
		if depth+1 >= minDepth {
			bestChains = append(bestChains, beam...)
		}
	}

	if len(bestChains) == 0 {
		return
	}

	// 按累计 log prob 排序，取 Top-N
	sort.Slice(bestChains, func(i, j int) bool {
		return bestChains[i].logProb > bestChains[j].logProb
	})
	if len(bestChains) > maxCandidates*2 {
		bestChains = bestChains[:maxCandidates*2]
	}

	added := 0
	for _, ci := range bestChains {
		if added >= maxCandidates {
			break
		}
		if len(ci.chars) < minDepth+1 {
			continue
		}
		chainWord := strings.Join(ci.chars, "")
		// 跳过词典已有的词（避免与 expandContinuation 策略1重复）
		if e.dict.HasWord(chainWord) {
			continue
		}
		combined := lastCommit + chainWord
		key := combined + "|"
		if _, exists := out[key]; exists {
			continue
		}
		// 评分：基础分 + 累计 bigram bonus + 长度平方级加分
		// ci.logProb 范围约 -10 ~ -3，转换为正向 bonus
		chainBonus := 0.0
		if ci.logProb > -18 {
			chainBonus = (18 + ci.logProb) * 5
			if chainBonus < 0 {
				chainBonus = 0
			}
		}
		// 长句平方级加分（让 5-6 字的整句显著占优）
		chainLen := len(ci.chars)
		lengthBonus := float64(chainLen*chainLen) * 6.0
		score := e.wPinyinMatch + chainBonus + lengthBonus
		out[key] = &Candidate{
			Word:   combined,
			Pinyin: "",
			Score:  score,
			Source: "continuation",
		}
		added++
	}
}

// longDistanceMatch 长距容错匹配（搜狗核心特性）
//
// 处理长距离的漏字、错字、隔字匹配，弥补 typoMatch 只处理相邻错误的不足。
// 典型场景：
//   - 漏字：输入 "woyaochfan"（漏了"i"）→ 应能匹配 "我要吃饭"
//   - 错字：输入 "woyaochifbn"（"a"打成"b"）→ 应能匹配 "我要吃饭"
//   - 隔字：输入 "wyacf"（每字首字母缩写）→ 应能匹配 "我要吃饭"
//
// 算法：滑动窗口 + 容错匹配
//   1. 对输入串做多种切分尝试（已有 sentenceMatch 处理）
//   2. 对每个切分，允许每段有 1 个字母的偏差（漏字/错字）
//   3. 用 Levenshtein 距离衡量匹配度，距离越近分越高
//
// 这里实现的是"段级容错"：把输入切分为多段，每段允许 1 个字母偏差，
// 用前缀查找扩展候选，再用 bigram 评分选最优整句。
func (e *Engine) longDistanceMatch(input string, syls []pinyin.Syllable, out map[string]*Candidate) {
	if len(syls) < 3 {
		return // 短输入不需要长距容错
	}

	// 策略 1：漏字容错
	// 对输入的每个位置，尝试插入 1 个字母，看是否能匹配到更好的整句
	// 例：woyaochfan（漏 i）→ 插入 i → woyaochifan → 我要吃饭
	e.fuzzyDeletionMatch(input, out)

	// 策略 2：隔字匹配（首字母缩写式整句）
	// 如果输入是 4-10 字母串，用首字母缩写匹配整句
	// 不要求 isAllInitials（允许韵母开头字母如'a'代表'ai'），因为搜狗缩写支持混合
	if len(input) >= 4 && len(input) <= 10 {
		e.longAcronymMatch(input, out)
	}
}

// fuzzyDeletionMatch 漏字容错：尝试在输入每个位置插入 1 个字母
// 例：woyaochfan → 尝试在每个位置插入 a-z → 找到 woyaochifan → 我要吃饭
//
// 实现说明：插入字母后，直接调用 segmentMatch 做 DP 切分 + bigram 评分，
// 因为很多整句（如"我要吃饭"）不在词典整词索引里，需要切分匹配。
func (e *Engine) fuzzyDeletionMatch(input string, out map[string]*Candidate) {
	if len(input) < 5 || len(input) > 20 {
		return
	}

	// 收集所有容错后的候选词及其最高分
	type cand struct {
		word  string
		score float64
	}
	candMap := make(map[string]float64)

	letters := "abcdefghijklmnopqrstuvwxyz"
	for i := 1; i < len(input); i++ {
		for _, c := range letters {
			newInput := input[:i] + string(c) + input[i:]
			newSyls := pinyin.Segment(newInput)
			if len(newSyls) < 2 {
				continue
			}
			// 用 segmentMatch 做 DP 切分 + bigram 评分
			// 把结果临时收集到 tempMap
			tempMap := make(map[string]*Candidate)
			e.segmentMatch(newSyls, tempMap)
			// 合并到 candMap，记录最高分
			for _, cand := range tempMap {
				if existing, ok := candMap[cand.Word]; !ok || cand.Score > existing {
					candMap[cand.Word] = cand.Score
				}
			}
		}
	}

	// 按分数排序，取 Top-N 加入输出
	added := 0
	type kv struct {
		word  string
		score float64
	}
	var sorted []kv
	for w, s := range candMap {
		sorted = append(sorted, kv{w, s})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].score > sorted[j].score
	})
	for _, p := range sorted {
		if added >= 8 {
			break
		}
		key := p.word + "|"
		if _, exists := out[key]; exists {
			continue
		}
		// 容错匹配分数：与正常 sentence 匹配持平（不打折，让正确容错候选能竞争）
		// 之前 *0.7 导致"我要吃饭"(52.8)被"我要吃反"(60)压过
		// 这里用 *0.95，略低于正常匹配但能竞争
		score := p.score * 0.95
		out[key] = &Candidate{
			Word:   p.word,
			Pinyin: "",
			Score:  score,
			Source: "fuzzy_long",
		}
		added++
	}
}

// longAcronymMatch 长首字母缩写整句匹配
// 输入 "wycf"（每字首字母）→ 匹配 "我要吃饭"
//
// 算法：用 bigram 链 + 首字母过滤，从第一个字母开始扩展整个输入串。
// 每一级匹配 1 个首字母，用 Beam Search 保留 Top-K 路径。
// 不依赖词典 acronymIndex（很多长句不在整词索引里），纯靠 bigram 预测。
func (e *Engine) longAcronymMatch(input string, out map[string]*Candidate) {
	if !e.segmenter.HasBigram() || len(input) < 3 || len(input) > 8 {
		return
	}

	// Beam Search：每级匹配 1 个首字母
	type chainItem struct {
		chars   []string // 已生成的字
		logProb float64  // 累计 bigram log prob
	}

	// 初始化：第一级用首字母索引获取候选字
	firstLetter := string(input[0])
	firstEntries := e.dict.LookupByInitial(firstLetter)
	var beam []chainItem
	seedLimit := 15
	if len(firstEntries) > seedLimit {
		firstEntries = firstEntries[:seedLimit]
	}
	// 常见句首字加分表（这些字常作为句子开头，应该优先）
	sentenceStartBonus := map[string]float64{
		"我": 2.0, "你": 1.5, "他": 1.0, "她": 1.0,
		"今": 1.5, "明": 1.0, "天": 1.0,
		"想": 1.0, "要": 1.0, "去": 1.0, "吃": 1.0,
		"这": 1.0, "那": 1.0, "什": 1.0, "怎": 1.0,
		"在": 1.0, "有": 1.0, "是": 1.0, "不": 1.0,
	}
	for _, ent := range firstEntries {
		// 只取单字作为种子
		if len([]rune(ent.Word)) != 1 {
			continue
		}
		// 用 unigram 概率作为初始分（freq 越高分越高）
		initLogProb := -3.0
		if ent.Freq > 1000 {
			initLogProb = -2.0
		} else if ent.Freq > 100 {
			initLogProb = -3.0
		} else {
			initLogProb = -4.0
		}
		// 常见句首字加分
		if bonus, ok := sentenceStartBonus[ent.Word]; ok {
			initLogProb += bonus
		}
		beam = append(beam, chainItem{
			chars:   []string{ent.Word},
			logProb: initLogProb,
		})
	}
	if len(beam) == 0 {
		return
	}

	// 逐级扩展剩余字母
	const (
		beamWidth   = 12
		topKPerStep = 12
		minLogProb  = -12.0
	)

	for i := 1; i < len(input); i++ {
		targetLetter := string(input[i])
		var newBeam []chainItem
		// 对每个 beam 项，先用 bigram 扩展，再用 LookupByInitial 补充
		// 补充覆盖 bigram 没学到但常见的续接（如"我"→"要"→"吃"→"饭"）
		for _, cur := range beam {
			last := cur.chars[len(cur.chars)-1]
			nexts := e.segmenter.BigramTopNext(last, topKPerStep)
			bigramHitCount := 0
			for _, nx := range nexts {
				if nx.LogProb < minLogProb {
					continue
				}
				nxPy := e.lookupCharPinyin(nx.Word)
				if nxPy == "" || !strings.HasPrefix(nxPy, targetLetter) {
					continue
				}
				newChars := make([]string, len(cur.chars)+1)
				copy(newChars, cur.chars)
				newChars[len(cur.chars)] = nx.Word
				newBeam = append(newBeam, chainItem{
					chars:   newChars,
					logProb: cur.logProb + nx.LogProb,
				})
				bigramHitCount++
			}
			// 总是补充 LookupByInitial 候选（覆盖 bigram 没学到的续接）
			// 限制补充数量，避免爆炸
			letterEntries := e.dict.LookupByInitial(targetLetter)
			supplementLimit := 15
			if len(letterEntries) > supplementLimit {
				letterEntries = letterEntries[:supplementLimit]
			}
			for _, ent := range letterEntries {
				if len([]rune(ent.Word)) != 1 {
					continue
				}
				// 避免与 bigram 命中的重复
				dup := false
				for _, nx := range nexts {
					if nx.Word == ent.Word {
						dup = true
						break
					}
				}
				if dup {
					continue
				}
				newChars := make([]string, len(cur.chars)+1)
				copy(newChars, cur.chars)
				newChars[len(cur.chars)] = ent.Word
				// 评分：检查"前字+该字"是否构成词典词组
				// 词组命中给高分（-1.0，接近 bigram 高分），非词组给低分（-6.0）
				supplementLogProb := -6.0
				bigramWord := last + ent.Word
				if e.dict.HasWord(bigramWord) {
					supplementLogProb = -1.0 // 是词组，给高分
				}
				newBeam = append(newBeam, chainItem{
					chars:   newChars,
					logProb: cur.logProb + supplementLogProb,
				})
			}
		}
		if len(newBeam) == 0 {
			break // 这一字母匹配失败
		}
		sort.Slice(newBeam, func(i, j int) bool {
			return newBeam[i].logProb > newBeam[j].logProb
		})
		if len(newBeam) > beamWidth {
			newBeam = newBeam[:beamWidth]
		}
		beam = newBeam
	}

	// 生成候选
	added := 0
	for _, ci := range beam {
		if added >= 6 {
			break
		}
		if len(ci.chars) < len(input) {
			continue // 没匹配完整输入，跳过
		}
		chainWord := strings.Join(ci.chars, "")
		key := chainWord + "|"
		if _, exists := out[key]; exists {
			continue
		}
		// 评分：基础分 + bigram bonus + 完整匹配加成 + 词组覆盖率加成
		bonus := 0.0
		if ci.logProb > -20 {
			bonus = (20 + ci.logProb) * 4
			if bonus < 0 {
				bonus = 0
			}
		}
		// 完整匹配所有字母的加成
		completeBonus := float64(len(input)) * 15.0

		// 词组覆盖率加成：检查链能分解为多少个 2 字词组
		// 如"我要吃饭"可分解为"我要"+"吃饭"，覆盖率 100%，加分高
		// 如"我一吃饭"只能分解为"吃饭"，覆盖率 50%，加分低
		wordGroupBonus := 0.0
		for j := 0; j+1 < len(ci.chars); j++ {
			twoCharWord := ci.chars[j] + ci.chars[j+1]
			if e.dict.HasWord(twoCharWord) {
				wordGroupBonus += 20.0 // 每个 2 字词组加 20 分
			}
		}

		score := e.wPinyinMatch*0.7 + bonus + completeBonus + wordGroupBonus
		out[key] = &Candidate{
			Word:   chainWord,
			Pinyin: "",
			Score:  score,
			Source: "acronym_long",
		}
		added++
	}
}


// userContextContinuation 用户私有上下文续接
// 从 contextPairs 中查找 prev=lastCommit 或 prev=lastChar 的续接词
func (e *Engine) userContextContinuation(lastCommit, lastChar, input string, out map[string]*Candidate) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	// 查 prev=lastCommit 的续接词
	for key, freq := range e.contextPairs {
		// key 格式: "prev|cur" 或 "prev1\tprev2|cur"
		// 只处理 2-gram（无 tab）
		if strings.Contains(key, "\t") {
			continue
		}
		parts := strings.SplitN(key, "|", 2)
		if len(parts) != 2 {
			continue
		}
		prev, cur := parts[0], parts[1]
		// 检查 prev 是否匹配上次提交（完整词或末字）
		if prev != lastCommit && prev != lastChar {
			continue
		}
		// 检查 cur 的拼音是否匹配输入
		curPy := e.lookupCharPinyinFirst(cur)
		if curPy == "" || !pyMatchesInput(curPy, input) {
			continue
		}
		// 生成续接候选
		combined := lastCommit + cur
		ckey := combined + "|"
		if _, exists := out[ckey]; exists {
			continue
		}
		// 用户频次越高分越高
		bonus := freq * 5
		if bonus > 50 {
			bonus = 50
		}
		score := e.wPinyinMatch*0.6 + bonus
		out[ckey] = &Candidate{
			Word:   combined,
			Pinyin: "",
			Score:  score,
			Source: "continuation",
		}
	}
}

// lookupCharPinyinFirst 取词首字的拼音
func (e *Engine) lookupCharPinyinFirst(word string) string {
	if word == "" {
		return ""
	}
	chars := []rune(word)
	if len(chars) == 0 {
		return ""
	}
	firstChar := string(chars[0])
	return e.lookupCharPinyin(firstChar)
}

// 例如 [hen, h] 返回 true（h 是纯声母，Final 为空）
func hasTrailingInitial(syls []pinyin.Syllable) bool {
        if len(syls) < 2 {
                return false
        }
        last := syls[len(syls)-1]
        // 纯声母音节：Initial 非空，Final 为空，Raw 是单字符声母
        return last.Final == "" && len(last.Raw) == 1 && pinyin.IsInitial(last.Raw)
}

// mixedInitialMatch 混合音节前缀匹配
// 输入 "henh"（切分为 [hen, h]）时，用 "henh" 作为前缀查找词典，
// 能匹配到 "henhao"（很好）、"henhei"（很黑）等，让用户输入到 henh 就出候选。
// 分数介于精确匹配和前缀匹配之间。
func (e *Engine) mixedInitialMatch(syls []pinyin.Syllable, out map[string]*Candidate) {
        // 拼接完整输入串作为前缀
        prefix := pinyin.Join(syls)
        if len(prefix) < 2 {
                return
        }
        prefixes := e.dict.LookupPrefix(prefix)
        for _, py := range prefixes {
                if py == prefix {
                        continue // 已被 exactMatch 处理
                }
                entries := e.dict.Lookup(py)
                for i, ent := range entries {
                        if i >= 3 {
                                break
                        }
                        key := ent.Word + "|" + ent.Pinyin
                        if _, ok := out[key]; !ok {
                                out[key] = &Candidate{
                                        Word:   ent.Word,
                                        Pinyin: ent.Pinyin,
                                        // 分数略高于普通 prefixMatch，因为是更精确的混合匹配
                                        Score:  e.wPinyinMatch*0.7 + ent.Freq*e.wFreq*0.4,
                                        Source: "dict",
                                }
                        }
                }
        }
}

// sortCandidates 排序候选
// 综合考虑：
//   - 拼音匹配度 + 词频（已在前面计算）
//   - 用户频次（带时间衰减）
//   - 2-gram 上下文联想（前一个提交词与候选词的共现频次）
//   - 3-gram 上下文联想（前两个提交词与候选词的共现频次，权重更高）
func (e *Engine) sortCandidates(in map[string]*Candidate) []Candidate {
        e.mu.RLock()
        defer e.mu.RUnlock()

        // 获取上下文历史
        var lastWord string
        var prevWord string
        histLen := len(e.commitHistory)
        if histLen >= 1 {
                lastWord = e.commitHistory[histLen-1]
        }
        if histLen >= 2 {
                prevWord = e.commitHistory[histLen-2]
        }

        result := make([]Candidate, 0, len(in))
        for _, c := range in {
                // 1. 用户频次加权（已在 Commit 中做时间衰减）
                userKey := c.Word + "|" + c.Pinyin
                if uf, ok := e.userFreq[userKey]; ok {
                        c.Score += uf * e.wUserFreq
                }

                // 2. 2-gram 上下文联想加权
                if lastWord != "" && lastWord != c.Word {
                        ctxKey := lastWord + "|" + c.Word
                        if coFreq, ok := e.contextPairs[ctxKey]; ok && coFreq > 0 {
                                c.Score += coFreq * e.wContext
                        }
                }

                // 3. 3-gram 上下文联想加权（权重更高，因为更具体）
                if prevWord != "" && lastWord != "" && lastWord != c.Word {
                        ctxKey := prevWord + "\t" + lastWord + "|" + c.Word
                        if coFreq, ok := e.contextPairs[ctxKey]; ok && coFreq > 0 {
                                // 3-gram 权重是 2-gram 的 1.5 倍（更精确的上下文）
                                c.Score += coFreq * e.wContext * 1.5
                        }
                }

                // 4. 重复提交加权：如果候选词与上一次提交相同
                if lastWord == c.Word {
                        c.Score += e.wContext * 0.5
                }

                result = append(result, *c)
        }
        sort.Slice(result, func(i, j int) bool {
                // 主排序：分数降序
                if result[i].Score != result[j].Score {
                        return result[i].Score > result[j].Score
                }
                // 次排序：来源优先级（dict > segment > acronym > fuzzy > typo > initial > missing-initial）
                return sourcePriority(result[i].Source) > sourcePriority(result[j].Source)
        })

        // 去重：相同汉字只保留最高分（避免同字不同拼音占多个位置）
        seen := make(map[string]bool)
        deduped := result[:0]
        for _, c := range result {
                if seen[c.Word] {
                        continue
                }
                seen[c.Word] = true
                deduped = append(deduped, c)
        }
        return deduped
}

// sourcePriority 来源优先级（数字越大越优先）
func sourcePriority(s string) int {
        switch s {
        case "dict":
                return 100
        case "segment":
                return 90
        case "mixed": // 简拼/全拼混合匹配（搜狗核心特性）
                return 80
        case "continuation": // 续接联想（搜狗核心特性），基于上下文预测
                return 75
        case "acronym":
                return 70
        case "sentence": // 整句容错匹配，之前缺失导致同分时被排末尾
                return 65
        case "fuzzy":
                return 60
        case "typo":
                return 50
        case "missing-initial":
                return 40
        case "initial":
                return 30
        default:
                return 0
        }
}

// Commit 用户选定了某个候选词，更新用户频次
// - 时间衰减：旧频次按半衰期衰减后再 +1
// - 2-gram 上下文：记录 (上一个词, 当前词) 共现
// - 3-gram 上下文：记录 (前两个词, 当前词) 共现
// - 持久化：用户频次和上下文都写入 BadgerDB
func (e *Engine) Commit(word, py string) {
        e.mu.Lock()
        key := word + "|" + py
        now := time.Now()

        // 时间衰减：如果之前有频次，按距离上次使用的时间衰减
        if oldFreq, ok := e.userFreq[key]; ok {
                if lastUsed, lutOk := e.userLastUsed[key]; lutOk {
                        elapsed := now.Sub(lastUsed)
                        decay := math.Pow(0.5, float64(elapsed)/float64(e.decayHalfLife))
                        e.userFreq[key] = oldFreq*decay + 1.0
                } else {
                        e.userFreq[key] = oldFreq + 1.0
                }
        } else {
                e.userFreq[key] = 1.0
        }
        e.userLastUsed[key] = now

        // 收集上下文更新（在锁外持久化）
        var ctxUpdates []struct{ prev, cur string }
        histLen := len(e.commitHistory)

        // 2-gram: 上一个词 → 当前词
        if histLen >= 1 {
                prev := e.commitHistory[histLen-1]
                if prev != word {
                        ctxKey := prev + "|" + word
                        e.contextPairs[ctxKey]++
                        ctxUpdates = append(ctxUpdates, struct{ prev, cur string }{prev, word})
                }
        }

        // 3-gram: 前两个词 → 当前词（用 "prev1\tprev2|cur" 作为键）
        if histLen >= 2 {
                prev1 := e.commitHistory[histLen-2]
                prev2 := e.commitHistory[histLen-1]
                if prev2 != word { // 不记录自连接
                        ctxKey := prev1 + "\t" + prev2 + "|" + word
                        e.contextPairs[ctxKey]++
                        // 3-gram 也持久化（用 tab 分隔的 key）
                        ctxUpdates = append(ctxUpdates, struct{ prev, cur string }{prev1 + "\t" + prev2, word})
                }
        }

        e.commitHistory = append(e.commitHistory, word)
        if len(e.commitHistory) > e.maxContextHistory {
                e.commitHistory = e.commitHistory[len(e.commitHistory)-e.maxContextHistory:]
        }

        // === N-gram 自动剪枝 ===
        // 超过最大条目数或距离上次剪枝超过间隔时触发
        pruneNow := time.Now()
        if len(e.contextPairs) > e.maxContextPairs ||
                (pruneNow.Sub(e.lastPruneTime) > e.pruneInterval && len(e.contextPairs) > 100) {
                e.pruneContextPairs(pruneNow)
        }

        store := e.userStore
        clipHist := e.clipboardHistory
        e.mu.Unlock()

        // 持久化（在锁外，避免阻塞）
        if store != nil {
                if err := store.Incr(word, py); err != nil {
                        log.Printf("[engine] userdict.Incr failed: %v", err)
                }
                // 持久化上下文
                for _, cu := range ctxUpdates {
                        if err := store.IncrContext(cu.prev, cu.cur); err != nil {
                                log.Printf("[engine] userdict.IncrContext failed: %v", err)
                                break
                        }
                }
        }

        // 记录到剪切板历史
        if clipHist != nil {
                clipHist.Add(word, py, "user")
        }
}

// pruneContextPairs 剪枝 contextPairs
// 策略：
//   1. 删除频次 < pruneThreshold 的条目
//   2. 如果仍超过 maxContextPairs，按频次排序保留 Top-N
// 必须在持有 e.mu 锁时调用
func (e *Engine) pruneContextPairs(_ time.Time) {
        // 阶段 1：删除低频条目
        for k, v := range e.contextPairs {
                if v < e.pruneThreshold {
                        delete(e.contextPairs, k)
                }
        }

        // 阶段 2：如果仍超过上限，按频次保留 Top-N
        if len(e.contextPairs) > e.maxContextPairs {
                type kv struct {
                        key string
                        val float64
                }
                var entries []kv
                for k, v := range e.contextPairs {
                        entries = append(entries, kv{k, v})
                }
                // 排序（降序）
                sort.Slice(entries, func(i, j int) bool {
                        return entries[i].val > entries[j].val
                })
                // 保留 Top-N
                keep := entries[:e.maxContextPairs]
                e.contextPairs = make(map[string]float64, len(keep))
                for _, kv := range keep {
                        e.contextPairs[kv.key] = kv.val
                }
        }

        e.lastPruneTime = time.Now()
}

// Close 关闭引擎，释放资源
func (e *Engine) Close() error {
        e.mu.Lock()
        defer e.mu.Unlock()
        if e.userStore != nil {
                return e.userStore.Close()
        }
        return nil
}

// ResetContext 重置上下文（用户按了标点或换行）
func (e *Engine) ResetContext() {
        e.mu.Lock()
        defer e.mu.Unlock()
        e.commitHistory = nil
}

// UserFreq 获取用户频次（用于持久化）
func (e *Engine) UserFreq() map[string]float64 {
        e.mu.RLock()
        defer e.mu.RUnlock()
        out := make(map[string]float64, len(e.userFreq))
        for k, v := range e.userFreq {
                out[k] = v
        }
        return out
}

// LoadUserFreq 加载用户频次
func (e *Engine) LoadUserFreq(m map[string]float64) {
        e.mu.Lock()
        defer e.mu.Unlock()
        for k, v := range m {
                e.userFreq[k] = v
        }
}

// Dict 返回词典引用
func (e *Engine) Dict() *dict.Dict {
        return e.dict
}

// Fuzzy 返回模糊音引擎
func (e *Engine) Fuzzy() *fuzzy.Engine {
        return e.fuzzy
}

// Clipboard 返回剪切板历史
func (e *Engine) Clipboard() *clipboard.History {
        return e.clipboardHistory
}

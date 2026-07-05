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

        // 4.7 整句容错匹配：当候选数不足时，对输入做 DP 切分，
        // 尝试把输入拆成多个词组组合。能处理：
        //   - 漏字：nizanal -> 你在哪里（ni zai na li）
        //   - 混合缩写：wzaiszdn -> 我在深圳等你（w zai s z d n）
        if len(candMap) < 5 {
                e.sentenceMatch(input, candMap)
        }

        // 4.5 单声母联想：输入 "n" 等单声母时返回高频字
        if len(syls) == 1 && pinyin.IsInitial(syls[0].Raw) && len(candMap) < 5 {
                e.singleInitialMatch(syls[0].Raw, candMap)
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
func (e *Engine) segmentMatch(syls []pinyin.Syllable, out map[string]*Candidate) {
        joined := pinyin.Join(syls)
        // 已经被 exactMatch 命中的整词，跳过
        if entries := e.dict.Lookup(joined); len(entries) > 0 {
                return
        }
        words, pinyins, score := e.segmenter.Segment(joined)
        if len(words) <= 1 {
                return // 切分失败或单字
        }
        // 组合所有非空词
        var combined strings.Builder
        var combinedPy strings.Builder
        allFound := true
        for i, w := range words {
                if w == "" {
                        allFound = false
                        break
                }
                combined.WriteString(w)
                combinedPy.WriteString(pinyins[i])
        }
        if !allFound || combined.Len() == 0 {
                return
        }
        word := combined.String()
        py := combinedPy.String()
        key := word + "|" + py
        if _, ok := out[key]; !ok {
                // 切分组合分数：基于切分的对数概率转换
                // score 是负数（log prob），越接近 0 越好
                // 转换为正向得分：用 max(0, 100 + score * 5) 作为基础
                baseScore := e.wPinyinMatch * 0.7 // 切分组合略低于整词
                if score < -20 {
                        baseScore *= 0.5 // 切分质量差时降权
                }
                out[key] = &Candidate{
                        Word:   word,
                        Pinyin: py,
                        Score:  baseScore,
                        Source: "segment",
                }
        }
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
// 为了控制复杂度，最多尝试前 N 种切分，且总候选数上限 20。
func (e *Engine) sentenceMatch(input string, out map[string]*Candidate) {
        if len(input) < 3 {
                return
        }
        // 收集所有可能的切分（限制数量避免爆炸）
        var allSplits [][]string
        collectSplits(input, 0, nil, &allSplits, 20)

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
                if added >= 10 {
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
// 对切分后的音节序列，从左到右尝试匹配 1-4 音节的词组
// 返回拼接的整句、拼音、匹配音节数、覆盖字符数
//
// 匹配策略（按优先级）：
//  1. 精确匹配：joined 完整音节串在词典中
//  2. 前缀扩展匹配：如果当前 span 末尾是单字符声母（缩写），
//     把它和前面的音节合并做前缀查找。例如 [hen][gao][x] 中
//     匹配到 [gao][x] 时，用 "gaox" 前缀查到 "gaoxing"(高兴)
//
// 这样 hengaox -> hen(很) + gao+x(高兴) = 很高兴
func (e *Engine) greedyMatchSentence(syls []string) (string, string, int, int) {
        var word, py strings.Builder
        segs := 0
        covered := 0
        i := 0
        for i < len(syls) {
                matched := false
                // 尝试 4-1 音节的词组（最长优先）
                for span := 4; span >= 1 && i+span <= len(syls); span-- {
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

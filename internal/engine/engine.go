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
        if len(syls) >= 2 && len(entries) == 0 {
                for i := 1; i < len(syls); i++ {
                        headSyls := syls[:i]
                        tailSyls := syls[i:]
                        headJoined := pinyin.Join(headSyls)
                        tailJoined := pinyin.Join(tailSyls)
                        headEntries := e.dict.Lookup(headJoined)
                        tailEntries := e.dict.Lookup(tailJoined)
                        if len(headEntries) > 0 && len(tailEntries) > 0 {
                                combined := headEntries[0].Word + tailEntries[0].Word
                                combinedPy := headJoined + tailJoined
                                key := combined + "|" + combinedPy
                                if _, ok := out[key]; !ok {
                                        // 子序列组合分数低于整词，避免压过整词
                                        score := e.wPinyinMatch*0.3 +
                                                (headEntries[0].Freq+tailEntries[0].Freq)*e.wFreq*0.1
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
// 策略：把每个音节的声母作为缩写，匹配词典中以这些声母开头的多字词
func (e *Engine) acronymMatch(syls []pinyin.Syllable, out map[string]*Candidate) {
        // 只对每个音节都是单字母（即纯声母）的情况生效
        for _, s := range syls {
                if len(s.Raw) != 1 || !pinyin.IsInitial(s.Raw) {
                        return // 不是首字母缩写模式
                }
        }

        // 收集所有声母
        initials := make([]string, len(syls))
        for i, s := range syls {
                initials[i] = s.Raw
        }

        // 在词典中查找以这些声母开头的多字词
        firstInitial := initials[0]
        candidates := e.dict.LookupPrefix(firstInitial)

        // 收集所有匹配的候选（不提前截断，最后按词频排序取 Top N）
        type cand struct {
                Word string
                Pinyin string
                Freq float64
        }
        var matched []cand

        for _, py := range candidates {
                // 检查这个拼音的首字母序列是否匹配
                pySyls := pinyin.Segment(py)
                if len(pySyls) != len(initials) {
                        continue
                }
                ok := true
                for i, s := range pySyls {
                        if s.Initial != initials[i] {
                                ok = false
                                break
                        }
                }
                if !ok {
                        continue
                }
                // 匹配，取这个词的所有候选
                entries := e.dict.Lookup(py)
                for i, ent := range entries {
                        if i >= 3 {
                                break // 每个拼音只取前 3 个
                        }
                        // 词的汉字数应该等于声母数
                        if len([]rune(ent.Word)) != len(initials) {
                                continue
                        }
                        matched = append(matched, cand{ent.Word, ent.Pinyin, ent.Freq})
                }
        }

        // 按词频降序排序
        sort.Slice(matched, func(i, j int) bool {
                return matched[i].Freq > matched[j].Freq
        })

        // 取前 10 个加入候选
        count := 0
        for _, c := range matched {
                if count >= 10 {
                        break
                }
                key := c.Word + "|" + c.Pinyin
                if _, exists := out[key]; !exists {
                        score := e.wPinyinMatch*0.4 + c.Freq*e.wFreq*0.3
                        out[key] = &Candidate{
                                Word:   c.Word,
                                Pinyin: c.Pinyin,
                                Score:  score,
                                Source: "acronym",
                        }
                        count++
                }
        }
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
func (e *Engine) singleInitialMatch(initial string, out map[string]*Candidate) {
        // 找所有以该声母开头的拼音
        candidates := e.dict.LookupPrefix(initial)

        type entry struct {
                Word   string
                Pinyin string
                Freq   float64
        }
        var allEntries []entry

        for _, py := range candidates {
                // 只取拼音正好是 声母+韵母 的（即完整音节）
                if !pinyin.IsValidSyllable(py) {
                        continue
                }
                entries := e.dict.Lookup(py)
                for i, ent := range entries {
                        if i >= 1 {
                                break // 每个拼音只取最高频
                        }
                        // 只取单字（汉字数 == 1）
                        if len([]rune(ent.Word)) != 1 {
                                continue
                        }
                        allEntries = append(allEntries, entry{ent.Word, ent.Pinyin, ent.Freq})
                }
        }

        // 按词频排序
        sort.Slice(allEntries, func(i, j int) bool {
                return allEntries[i].Freq > allEntries[j].Freq
        })

        // 取前 10 个
        count := 0
        for _, ent := range allEntries {
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
        combinations := e.fuzzy.ExpandAll(rawSyls)
        for _, combo := range combinations {
                joined := strings.Join(combo, "")
                // 跳过和原始相同的（已由 exactMatch 处理）
                if joined == pinyin.Join(syls) {
                        continue
                }
                entries := e.dict.Lookup(joined)
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
                                        Score:  (e.wPinyinMatch + ent.Freq*e.wFreq) * e.wFuzzy,
                                        Source: "fuzzy",
                                }
                        }
                }
        }
}

// typoMatch 拼写错误容错
// 只在变体长度等于原始输入长度，且 segment 后音节数相同时生效
func (e *Engine) typoMatch(originalInput string, syls []pinyin.Syllable, out map[string]*Candidate) {
        origLen := len(originalInput)
        origSylCount := len(syls)
        variants := e.fuzzy.TypoVariants(originalInput)
        for _, v := range variants {
                if v == originalInput {
                        continue
                }
                // 长度必须一致
                if len(v) != origLen {
                        continue
                }
                varSyls := pinyin.Segment(v)
                if len(varSyls) == 0 {
                        continue
                }
                // 音节数必须一致
                if len(varSyls) != origSylCount {
                        continue
                }
                joined := pinyin.Join(varSyls)
                entries := e.dict.Lookup(joined)
                for _, ent := range entries {
                        // 必须是多音节词（避免单字 typo）
                        if len(ent.Word) < origSylCount {
                                continue
                        }
                        key := ent.Word + "|" + ent.Pinyin
                        if _, ok := out[key]; !ok {
                                out[key] = &Candidate{
                                        Word:   ent.Word,
                                        Pinyin: ent.Pinyin,
                                        Score:  (e.wPinyinMatch + ent.Freq*e.wFreq) * e.wTypo,
                                        Source: "typo",
                                }
                        }
                }
        }
}

// prefixMatch 前缀匹配（用于输入过程中）
func (e *Engine) prefixMatch(input string, out map[string]*Candidate) {
        // 找所有以 input 开头的拼音
        prefixes := e.dict.LookupPrefix(input)
        for _, py := range prefixes {
                if py == input {
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
                                        Score:  e.wPinyinMatch*0.5 + ent.Freq*e.wFreq*0.3,
                                        Source: "dict",
                                }
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
        case "acronym":
                return 70
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

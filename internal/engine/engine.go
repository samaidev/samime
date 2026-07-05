// Package engine 是输入法核心引擎
package engine

import (
        "log"
        "sort"
        "strings"
        "sync"
        "time"

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
        userFreq      map[string]float64 // 内存缓存
        userStore     *userdict.Store    // 持久化存储（可选）
        commitHistory []string

        // 排序权重
        wPinyinMatch float64
        wFreq        float64
        wUserFreq    float64
        wContext     float64
        wFuzzy       float64
        wTypo        float64
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
                dict:         d,
                fuzzy:        fuzzy.New(),
                segmenter:    seg,
                userFreq:     make(map[string]float64),
                wPinyinMatch: cfg.WPinyinMatch,
                wFreq:        cfg.WFreq,
                wUserFreq:    cfg.WUserFreq,
                wContext:     cfg.WContext,
                wFuzzy:       cfg.WFuzzy,
                wTypo:        cfg.WTypo,
        }
}

// NewDefault 用默认配置创建
func NewDefault(d *dict.Dict) *Engine {
        return New(d, DefaultConfig())
}

// NewWithUserStore 创建带持久化用户词典的引擎
// path 为空时使用默认路径 ~/.samime/userdict
func NewWithUserStore(d *dict.Dict, path string) (*Engine, error) {
        store, err := userdict.New(path)
        if err != nil {
                return nil, err
        }
        e := NewDefault(d)
        e.userStore = store
        // 加载已有用户频次到内存
        e.userFreq = store.All()
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

// sortCandidates 排序候选
func (e *Engine) sortCandidates(in map[string]*Candidate) []Candidate {
        e.mu.RLock()
        defer e.mu.RUnlock()

        result := make([]Candidate, 0, len(in))
        for _, c := range in {
                // 加上用户频次
                userKey := c.Word + "|" + c.Pinyin
                if uf, ok := e.userFreq[userKey]; ok {
                        c.Score += uf * e.wUserFreq
                }
                // 加上上下文权重
                if len(e.commitHistory) > 0 {
                        last := e.commitHistory[len(e.commitHistory)-1]
                        if last == c.Word {
                                c.Score += e.wContext
                        }
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
// 如果启用了持久化，会异步写入 BadgerDB
func (e *Engine) Commit(word, py string) {
        e.mu.Lock()
        key := word + "|" + py
        e.userFreq[key]++
        e.commitHistory = append(e.commitHistory, word)
        if len(e.commitHistory) > 100 {
                e.commitHistory = e.commitHistory[len(e.commitHistory)-100:]
        }
        store := e.userStore
        e.mu.Unlock()

        // 持久化（在锁外，避免阻塞）
        if store != nil {
                if err := store.Incr(word, py); err != nil {
                        log.Printf("[engine] userdict.Incr failed: %v", err)
                }
        }
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

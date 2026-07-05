// Package engine 是输入法核心引擎
package engine

import (
        "sort"
        "strings"
        "sync"
        "time"

        "github.com/zai/goime/internal/dict"
        "github.com/zai/goime/internal/fuzzy"
        "github.com/zai/goime/internal/pinyin"
        "github.com/zai/goime/internal/segmenter"
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
        userFreq      map[string]float64 // key: word|pinyin, val: 频次
        commitHistory []string           // 最近提交的词（用于上下文）

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
        return &Engine{
                dict:         d,
                fuzzy:        fuzzy.New(),
                segmenter:    segmenter.New(d),
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

        // 2. 模糊音匹配
        e.fuzzyMatch(syls, candMap)

        // 3. 拼写错误容错
        e.typoMatch(input, syls, candMap)

        // 4. 前缀匹配（输入过程中）
        if len(candMap) < 10 {
                e.prefixMatch(input, candMap)
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
                // 加上上下文权重（前一个提交的词是否与当前候选常共现）
                // 简化处理：如果候选词与上一次提交相同，加权
                if len(e.commitHistory) > 0 {
                        last := e.commitHistory[len(e.commitHistory)-1]
                        if last == c.Word {
                                c.Score += e.wContext
                        }
                }
                result = append(result, *c)
        }
        sort.Slice(result, func(i, j int) bool {
                return result[i].Score > result[j].Score
        })
        return result
}

// Commit 用户选定了某个候选词，更新用户频次
func (e *Engine) Commit(word, py string) {
        e.mu.Lock()
        defer e.mu.Unlock()
        key := word + "|" + py
        e.userFreq[key]++
        e.commitHistory = append(e.commitHistory, word)
        if len(e.commitHistory) > 100 {
                e.commitHistory = e.commitHistory[len(e.commitHistory)-100:]
        }
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

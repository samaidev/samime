// Package fuzzy 实现模糊音与拼写容错
package fuzzy

import (
        "strings"
)

// FuzzyPair 模糊音对（双向）
type FuzzyPair struct {
        A string
        B string
}

// DefaultPairs 默认模糊音配置
// 兼容 Rime 的常见模糊音方案 + 常见方言模糊
var DefaultPairs = []FuzzyPair{
        // 标准模糊音（Rime 默认）
        {"z", "zh"},
        {"c", "ch"},
        {"s", "sh"},
        {"n", "l"},
        {"an", "ang"},
        {"en", "eng"},
        {"in", "ing"},
        {"ian", "iang"},
        {"uan", "uang"},
        {"ou", "uo"},

        // 方言模糊音
        {"f", "h"},   // 福建口音：f/h 混淆（如 发/花）
        {"l", "r"},   // 湖南/四川口音：l/r 混淆（如 来/然）
        {"v", "u"},   // 部分输入法：v/u 互换
        {"v", "w"},   // v/w 混淆
        {"h", "k"},   // 客家口音：h/k 部分混淆

        // 韵母细化
        {"ong", "eng"},  // ong/eng 混淆（如 中/正）
        {"un", "uen"},   // un/uen 等价
        {"ue", "ve"},    // ue/ve 等价（如 雪/学）
        {"ie", "iai"},   // 部分方言
}

// QWERTY 键盘邻键图（用于拼写错误容错）
var qwertyNeighbors = map[byte]string{
        'q': "wa", 'w': "qase", 'e': "wsdr", 'r': "edft", 't': "rfgy",
        'y': "tghu", 'u': "yhji", 'i': "ujko", 'o': "iklp", 'p': "ol",
        'a': "qwsz", 's': "awdezx", 'd': "serfcx", 'f': "drtgvc", 'g': "ftyhbv",
        'h': "gyujnb", 'j': "huikmn", 'k': "jiolm", 'l': "kop",
        'z': "asx", 'x': "zsdc", 'c': "xdfv", 'v': "cfgb", 'b': "vghn",
        'm': "njk", 'n': "bjkm",
}

// Engine 模糊音引擎
type Engine struct {
        pairs    []FuzzyPair
        enabled  bool
        neighbor bool // 是否启用邻键容错
}

// New 创建模糊音引擎
func New() *Engine {
        return &Engine{
                pairs:    DefaultPairs,
                enabled:  true,
                neighbor: true,
        }
}

// SetEnabled 开关模糊音
func (e *Engine) SetEnabled(b bool) { e.enabled = b }

// SetNeighbor 开关邻键容错
func (e *Engine) SetNeighbor(b bool) { e.neighbor = b }

// SetPairs 自定义模糊音对
func (e *Engine) SetPairs(p []FuzzyPair) { e.pairs = p }

// Expand 对单个音节展开模糊音变体
// 例：输入 "ni" -> ["ni", "li"] (n/l 模糊)
// 例：输入 "zhong" -> ["zhong", "zong"] (zh/z 模糊)
func (e *Engine) Expand(syllable string) []string {
        if !e.enabled {
                return []string{syllable}
        }
        result := map[string]bool{syllable: true}
        for _, p := range e.pairs {
                if strings.HasPrefix(syllable, p.A) {
                        rest := syllable[len(p.A):]
                        result[p.B+rest] = true
                }
                if strings.HasPrefix(syllable, p.B) {
                        rest := syllable[len(p.B):]
                        result[p.A+rest] = true
                }
        }
        out := make([]string, 0, len(result))
        for k := range result {
                out = append(out, k)
        }
        return out
}

// ExpandAll 对多个音节展开，返回所有组合
// 例：["ni", "hao"] -> ["nihao", "lihao", "nihao", "lihao"]
// 去重后返回
func (e *Engine) ExpandAll(syllables []string) [][]string {
        if len(syllables) == 0 {
                return nil
        }
        if !e.enabled {
                return [][]string{syllables}
        }
        // 每个音节展开
        expanded := make([][]string, len(syllables))
        for i, s := range syllables {
                expanded[i] = e.Expand(s)
        }
        // 笛卡尔积
        result := [][]string{{}}
        for _, ex := range expanded {
                var newResult [][]string
                for _, r := range result {
                        for _, v := range ex {
                                rr := make([]string, len(r)+1)
                                copy(rr, r)
                                rr[len(r)] = v
                                newResult = append(newResult, rr)
                        }
                }
                result = newResult
        }
        // 去重
        seen := make(map[string]bool)
        unique := make([][]string, 0, len(result))
        for _, r := range result {
                key := strings.Join(r, "|")
                if !seen[key] {
                        seen[key] = true
                        unique = append(unique, r)
                }
        }
        return unique
}

// ExpandAllWithCount 对多个音节展开，返回所有组合及模糊音节数
// fuzzyCount 表示该组合中有几个音节被模糊（用于分级评分）
// 模糊音节越少，匹配度越高（搜狗的模糊距离分级）
func (e *Engine) ExpandAllWithCount(syllables []string) []FuzzyCombo {
        if len(syllables) == 0 {
                return nil
        }
        if !e.enabled {
                orig := make([]string, len(syllables))
                copy(orig, syllables)
                return []FuzzyCombo{{Syls: orig, FuzzyCount: 0}}
        }
        // 每个音节展开，并标记是否为模糊变体
        type variant struct {
                s     string
                fuzzy bool
        }
        expanded := make([][]variant, len(syllables))
        for i, s := range syllables {
                variants := []variant{{s: s, fuzzy: false}}
                seen := map[string]bool{s: true}
                for _, p := range e.pairs {
                        if strings.HasPrefix(s, p.A) {
                                rest := s[len(p.A):]
                                v := p.B + rest
                                if !seen[v] {
                                        seen[v] = true
                                        variants = append(variants, variant{s: v, fuzzy: true})
                                }
                        }
                        if strings.HasPrefix(s, p.B) {
                                rest := s[len(p.B):]
                                v := p.A + rest
                                if !seen[v] {
                                        seen[v] = true
                                        variants = append(variants, variant{s: v, fuzzy: true})
                                }
                        }
                }
                expanded[i] = variants
        }
        // 笛卡尔积
        result := []FuzzyCombo{{Syls: nil, FuzzyCount: 0}}
        for _, ex := range expanded {
                var newResult []FuzzyCombo
                for _, r := range result {
                        for _, v := range ex {
                                rr := FuzzyCombo{
                                        Syls:       append(append([]string(nil), r.Syls...), v.s),
                                        FuzzyCount: r.FuzzyCount,
                                }
                                if v.fuzzy {
                                        rr.FuzzyCount++
                                }
                                newResult = append(newResult, rr)
                        }
                }
                result = newResult
        }
        // 去重
        seen := make(map[string]bool)
        unique := make([]FuzzyCombo, 0, len(result))
        for _, r := range result {
                key := strings.Join(r.Syls, "|")
                if !seen[key] {
                        seen[key] = true
                        unique = append(unique, r)
                }
        }
        return unique
}

// FuzzyCombo 模糊音组合（带模糊音节数）
type FuzzyCombo struct {
        Syls       []string
        FuzzyCount int // 被模糊的音节数
}

// NeighborKeys 返回某键的邻键（含自身）
func (e *Engine) NeighborKeys(c byte) []string {
        if !e.neighbor {
                return []string{string(c)}
        }
        result := []string{string(c)}
        if n, ok := qwertyNeighbors[c]; ok {
                for i := 0; i < len(n); i++ {
                        result = append(result, string(n[i]))
                }
        }
        return result
}

// TypoVariant 拼写错误变体（带类型和代价）
type TypoVariant struct {
	Text  string
	Cost  int    // 编辑代价（1=单次操作，2=两次操作）
	Kind  string // "replace" | "delete" | "insert" | "transpose"
}

// TypoVariants 生成拼写错误容错变体
// 策略：每个字符替换为邻键，最多替换一处
// 例：输入 "nihap" -> ["nihap", "nihao", "nihap" -> ...]
// 保留旧接口（无代价信息），返回所有变体字符串
func (e *Engine) TypoVariants(input string) []string {
        vs := e.TypoVariantsDetailed(input)
        out := make([]string, 0, len(vs))
        seen := make(map[string]bool, len(vs))
        for _, v := range vs {
                if !seen[v.Text] {
                        seen[v.Text] = true
                        out = append(out, v.Text)
                }
        }
        return out
}

// TypoVariantsDetailed 生成带类型和代价的拼写错误变体
// 覆盖 4 种常见错误（搜狗级联纠错的第一层）：
//  1. 替换（replace）：邻键误触，如 nihap→nihao（p→o）
//  2. 删除（delete）：多打字，如 nihaoo→nihao（删除末尾 o）
//  3. 插入（insert）：漏打字，如 nihao→niha（在 h 后插入 o）
//  4. 转置（transpose）：相邻颠倒，如 nihoa→nihao（o/a 互换）
//
// 每种最多一处操作（代价=1），避免组合爆炸。
// 邻键替换用 QWERTY 邻接矩阵；插入/删除对所有字母生效；
// 转置仅对相邻不同字符生效。
func (e *Engine) TypoVariantsDetailed(input string) []TypoVariant {
        if len(input) == 0 {
                return []TypoVariant{{Text: input, Cost: 0, Kind: "original"}}
        }
        seen := make(map[string]bool)
        var out []TypoVariant
        add := func(text, kind string, cost int) {
                if text == "" || seen[text] {
                        return
                }
                seen[text] = true
                out = append(out, TypoVariant{Text: text, Cost: cost, Kind: kind})
        }

        // 原始
        add(input, "original", 0)

        if !e.neighbor {
                return out
        }

        n := len(input)
        // 1. 邻键替换（最多一处）
        for i := 0; i < n; i++ {
                c := input[i]
                neighbors := e.NeighborKeys(c)
                for _, nb := range neighbors {
                        if nb == string(c) {
                                continue
                        }
                        add(input[:i]+nb+input[i+1:], "replace", 1)
                }
        }

        // 2. 删除（多打字，最多删一处）
        // 例：nihaoo→nihao, nihaoa→nihao
        for i := 0; i < n; i++ {
                add(input[:i]+input[i+1:], "delete", 1)
        }

        // 3. 插入（漏打字，最多插一处）
        // 例：niha→nihao（在 ha 后插入 o）
        // 限制：只插入相邻字符的邻键，避免 26*n 组合爆炸
        for i := 0; i <= n; i++ {
                // 取前后字符的邻键作为候选插入字符
                candidates := make(map[byte]bool)
                if i > 0 {
                        if nbs, ok := qwertyNeighbors[input[i-1]]; ok {
                                for j := 0; j < len(nbs); j++ {
                                        candidates[nbs[j]] = true
                                }
                        }
                }
                if i < n {
                        if nbs, ok := qwertyNeighbors[input[i]]; ok {
                                for j := 0; j < len(nbs); j++ {
                                        candidates[nbs[j]] = true
                                }
                        }
                }
                for c := range candidates {
                        add(input[:i]+string(c)+input[i:], "insert", 1)
                }
        }

        // 4. 转置（相邻字符颠倒，最多一处）
        // 例：nihoa→nihao（o/a 互换）
        for i := 0; i < n-1; i++ {
                if input[i] == input[i+1] {
                        continue // 相同字符转置无意义
                }
                add(input[:i]+string(input[i+1])+string(input[i])+input[i+2:], "transpose", 1)
        }

        return out
}

// EditDistance 编辑距离（Levenshtein）
func EditDistance(a, b string) int {
        la, lb := len(a), len(b)
        if la == 0 {
                return lb
        }
        if lb == 0 {
                return la
        }
        dp := make([]int, lb+1)
        for j := 0; j <= lb; j++ {
                dp[j] = j
        }
        for i := 1; i <= la; i++ {
                prev := dp[0]
                dp[0] = i
                for j := 1; j <= lb; j++ {
                        temp := dp[j]
                        cost := 1
                        if a[i-1] == b[j-1] {
                                cost = 0
                        }
                        dp[j] = min3(dp[j]+1, dp[j-1]+1, prev+cost)
                        prev = temp
                }
        }
        return dp[lb]
}

func min3(a, b, c int) int {
        m := a
        if b < m {
                m = b
        }
        if c < m {
                m = c
        }
        return m
}

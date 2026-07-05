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

// TypoVariants 生成拼写错误容错变体
// 策略：每个字符替换为邻键，最多替换一处
// 例：输入 "nihap" -> ["nihap", "nihao", "nihap" -> ...]
func (e *Engine) TypoVariants(input string) []string {
        if !e.neighbor || len(input) == 0 {
                return []string{input}
        }
        result := map[string]bool{input: true}
        for i := 0; i < len(input); i++ {
                c := input[i]
                neighbors := e.NeighborKeys(c)
                for _, n := range neighbors {
                        if n == string(c) {
                                continue
                        }
                        variant := input[:i] + n + input[i+1:]
                        result[variant] = true
                }
        }
        out := make([]string, 0, len(result))
        for k := range result {
                out = append(out, k)
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

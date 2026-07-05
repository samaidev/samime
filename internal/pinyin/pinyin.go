// Package pinyin 实现拼音切分与声韵母识别
package pinyin

import (
        "strings"
)

// 声母表（按长度降序排列，便于最长匹配）
var initials = []string{
        "zh", "ch", "sh",
        "b", "p", "m", "f",
        "d", "t", "n", "l",
        "g", "k", "h",
        "j", "q", "x",
        "r", "z", "c", "s",
        "y", "w",
}

// 韵母表
var finals = map[string]bool{
        "a": true, "o": true, "e": true, "i": true, "u": true, "v": true,
        "ai": true, "ei": true, "ui": true, "ao": true, "ou": true, "iu": true,
        "ie": true, "ve": true, "er": true,
        "an": true, "en": true, "in": true, "un": true, "vn": true,
        "ang": true, "eng": true, "ing": true, "ong": true,
        "ia": true, "iao": true, "ian": true, "iang": true, "iong": true,
        "ua": true, "uai": true, "uan": true, "uang": true,
        "uo": true, "ue": true,
}

// initialSet 用于快速判断
var initialSet map[string]bool

// syllableSet 预计算的所有合法音节集合，用于 O(1) 查找
// 避免 IsValidSyllable 每次都遍历 initials（23次字符串比较）
var syllableSet map[string]bool

func init() {
        initialSet = make(map[string]bool, len(initials))
        for _, s := range initials {
                initialSet[s] = true
        }
        // 预计算所有合法音节：声母+韵母 和 独立韵母
        syllableSet = make(map[string]bool, 500)
        for f := range finals {
                syllableSet[f] = true // 独立韵母音节
        }
        for _, ini := range initials {
                for f := range finals {
                        syllableSet[ini+f] = true
                }
        }
}

// IsInitial 是否为声母
func IsInitial(s string) bool {
        return initialSet[s]
}

// IsFinal 是否为韵母
func IsFinal(s string) bool {
        return finals[s]
}

// IsValidSyllable 是否为合法拼音音节
func IsValidSyllable(s string) bool {
        s = strings.ToLower(s)
        if len(s) == 0 {
                return false
        }
        // a, o, e 等可独立成音节
        if finals[s] {
                return true
        }
        // 尝试拆分声母 + 韵母
        for _, ini := range initials {
                if strings.HasPrefix(s, ini) {
                        rest := s[len(ini):]
                        if len(rest) == 0 {
                                // 单声母不能成音节（除特殊）
                                return false
                        }
                        if finals[rest] {
                                return true
                        }
                }
        }
        return false
}

// IsValidSyllableFast 快速判断是否为合法拼音音节（O(1) map 查找）
// 输入必须已是小写。用预计算的 syllableSet 代替遍历 initials。
// 用于高频调用场景（如 acronymIndex 构建）。
func IsValidSyllableFast(s string) bool {
        return syllableSet[s]
}

// Syllable 单个拼音音节
type Syllable struct {
        Initial string // 声母
        Final   string // 韵母
        Raw     string // 原始输入
}

// Segment 将连续拼音串切分为音节序列
// 采用动态规划 + 词典最长匹配
// 输入："nihao" -> [{n,i}, {h,ao}]
func Segment(input string) []Syllable {
        input = strings.ToLower(strings.TrimSpace(input))
        if len(input) == 0 {
                return nil
        }
        n := len(input)
        // dp[i] = 从位置 i 到末尾的最优切分（音节数最少）
        // 用 dp[i] = -1 表示不可达
        dp := make([]int, n+1)
        prev := make([]int, n+1) // 记录前驱位置
        length := make([]int, n+1)
        for i := 0; i <= n; i++ {
                dp[i] = -1
        }
        dp[n] = 0

        for i := n - 1; i >= 0; i-- {
                // 尝试所有可能的音节长度 1-6
                for l := 1; l <= 6 && i+l <= n; l++ {
                        syl := input[i : i+l]
                        if IsValidSyllable(syl) {
                                if dp[i+l] != -1 {
                                        cand := dp[i+l] + 1
                                        if dp[i] == -1 || cand < dp[i] {
                                                dp[i] = cand
                                                prev[i] = i + l
                                                length[i] = l
                                        }
                                }
                        }
                }
        }

        if dp[0] == -1 {
                // 无法完整切分，退化为单字切分
                return fallbackSegment(input)
        }

        // 回溯
        var result []Syllable
        for i := 0; i < n; {
                j := prev[i]
                raw := input[i:j]
                result = append(result, makeSyllable(raw))
                i = j
        }
        return result
}

// makeSyllable 拆分声母韵母
func makeSyllable(raw string) Syllable {
        for _, ini := range initials {
                if strings.HasPrefix(raw, ini) && len(raw) > len(ini) {
                        rest := raw[len(ini):]
                        if finals[rest] {
                                return Syllable{Initial: ini, Final: rest, Raw: raw}
                        }
                }
        }
        // 纯韵母
        return Syllable{Initial: "", Final: raw, Raw: raw}
}

// fallbackSegment 当 DP 失败时的兜底：贪心最长匹配
func fallbackSegment(input string) []Syllable {
        var result []Syllable
        i := 0
        for i < len(input) {
                matched := false
                // 优先长音节
                for l := 6; l >= 1; l-- {
                        if i+l > len(input) {
                                continue
                        }
                        syl := input[i : i+l]
                        if IsValidSyllable(syl) {
                                result = append(result, makeSyllable(syl))
                                i += l
                                matched = true
                                break
                        }
                }
                if !matched {
                        // 当前位置单字符如果是合法声母，保留为声母音节
                        // 这样 "kk" 会切分成 [k, k]，让 engine 的 acronymMatch
                        // 能匹配"看看""可靠""开口"等双声母缩写词；
                        // "henh" 仍切分成 [hen, h] 匹配"很好"
                        ch := input[i : i+1]
                        if IsInitial(ch) {
                                result = append(result, Syllable{
                                        Initial: ch,
                                        Final:   "",
                                        Raw:     ch,
                                })
                                i++
                                matched = true
                        }
                        if !matched {
                                // 跳过非法字符
                                i++
                        }
                }
        }
        return result
}

// Join 拼接音节原始串
func Join(syls []Syllable) string {
        parts := make([]string, len(syls))
        for i, s := range syls {
                parts[i] = s.Raw
        }
        return strings.Join(parts, "")
}

// JoinWithSep 用分隔符拼接
func JoinWithSep(syls []Syllable, sep string) string {
        parts := make([]string, len(syls))
        for i, s := range syls {
                parts[i] = s.Raw
        }
        return strings.Join(parts, sep)
}

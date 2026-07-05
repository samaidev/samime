package fuzzy

import (
        "strings"
        "testing"
)

// === 模糊音边缘测试 ===

func TestExpandEmpty(t *testing.T) {
        e := New()
        if v := e.Expand(""); len(v) != 1 || v[0] != "" {
                t.Errorf("Expand('') = %v, want ['']", v)
        }
}

func TestExpandSingleInvalid(t *testing.T) {
        e := New()
        // 'b' 不是任何模糊音对的成员
        v := e.Expand("b")
        if len(v) != 1 || v[0] != "b" {
                t.Errorf("Expand('b') = %v, want ['b']", v)
        }
}

func TestExpandVeryLong(t *testing.T) {
        e := New()
        // Expand 是对单音节展开，长字符串被当作一个音节
        // 'n' 开头 → 触发 n/l 模糊 → 2 个变体
        long := strings.Repeat("ni", 50) // 100 字符
        v := e.Expand(long)
        if len(v) != 2 {
                t.Errorf("Expand(long 'ni' repeat) = %d variants, want 2 (n/l fuzzy)", len(v))
        }
}

func TestExpandAllEmpty(t *testing.T) {
        e := New()
        if v := e.ExpandAll(nil); len(v) != 0 {
                t.Errorf("ExpandAll(nil) = %v, want nil", v)
        }
        if v := e.ExpandAll([]string{}); len(v) != 0 {
                t.Errorf("ExpandAll([]) = %v, want nil", v)
        }
        if v := e.ExpandAll([]string{""}); len(v) != 1 {
                t.Errorf("ExpandAll(['']) = %v, want 1", v)
        }
}

func TestExpandAllCombinatorialExplosion(t *testing.T) {
        e := New()
        // 多个可模糊音节会笛卡尔积爆炸
        // n/l 模糊：每个 ni 都变成 [ni, li]
        syls := []string{"ni", "ni", "ni", "ni", "ni", "ni", "ni", "ni"} // 8 个
        v := e.ExpandAll(syls)
        // 每个 ni -> [ni, li] = 2^8 = 256
        if len(v) != 256 {
                t.Errorf("ExpandAll(8 x ni) = %d, want 256 (2^8)", len(v))
        }
}

func TestExpandAllCombinatorialLimit(t *testing.T) {
        e := New()
        // 15 个 ni -> 2^15 = 32768
        syls := make([]string, 15)
        for i := range syls {
                syls[i] = "ni"
        }
        v := e.ExpandAll(syls)
        if len(v) != 32768 {
                t.Errorf("ExpandAll(15 x ni) = %d, want 32768 (2^15)", len(v))
        }
}

func TestDisabledFuzzy(t *testing.T) {
        e := New()
        e.SetEnabled(false)
        v := e.Expand("ni")
        if len(v) != 1 || v[0] != "ni" {
                t.Errorf("disabled Expand('ni') = %v, want ['ni']", v)
        }
}

func TestCustomPairs(t *testing.T) {
        e := New()
        e.SetPairs([]FuzzyPair{{"f", "h"}}) // 福建口音 f/h
        v := e.Expand("fen")
        hasFen, hasHen := false, false
        for _, s := range v {
                if s == "fen" {
                        hasFen = true
                }
                if s == "hen" {
                        hasHen = true
                }
        }
        if !hasFen {
                t.Errorf("missing fen")
        }
        if !hasHen {
                t.Errorf("missing hen (f/h fuzzy)")
        }
        // 默认的 n/l 不应该再生效
        v = e.Expand("ni")
        hasLi := false
        for _, s := range v {
                if s == "li" {
                        hasLi = true
                }
        }
        if hasLi {
                t.Errorf("default n/l should not apply after SetPairs")
        }
}

// === 邻键容错测试 ===

func TestNeighborKeysEdgeCases(t *testing.T) {
        e := New()
        // 数字键（没有邻键定义）
        v := e.NeighborKeys('1')
        if len(v) != 1 || v[0] != "1" {
                t.Errorf("NeighborKeys('1') = %v, want ['1']", v)
        }
        // 符号
        v = e.NeighborKeys('!')
        if len(v) != 1 || v[0] != "!" {
                t.Errorf("NeighborKeys('!') = %v, want ['!']", v)
        }
        // 空字节
        v = e.NeighborKeys(0)
        if len(v) != 1 {
                t.Errorf("NeighborKeys(0) = %v, want ['\\0']", v)
        }
}

func TestNeighborKeysDisabled(t *testing.T) {
        e := New()
        e.SetNeighbor(false)
        v := e.NeighborKeys('a')
        if len(v) != 1 || v[0] != "a" {
                t.Errorf("disabled NeighborKeys('a') = %v, want ['a']", v)
        }
}

func TestTypoVariantsEmpty(t *testing.T) {
        e := New()
        v := e.TypoVariants("")
        if len(v) != 1 || v[0] != "" {
                t.Errorf("TypoVariants('') = %v, want ['']", v)
        }
}

func TestTypoVariantsSingleChar(t *testing.T) {
        e := New()
        v := e.TypoVariants("a")
        // 'a' 的邻键是 qwsz
        expected := map[string]bool{"a": true, "q": true, "w": true, "s": true, "z": true}
        got := make(map[string]bool)
        for _, s := range v {
                got[s] = true
        }
        for k := range expected {
                if !got[k] {
                        t.Errorf("TypoVariants('a') missing %q, got %v", k, v)
                }
        }
}

func TestTypoVariantsVeryLong(t *testing.T) {
        e := New()
        // 1000 字符的输入，每个位置生成邻键变体（共 ~4000+ 个变体）
        long := strings.Repeat("a", 1000)
        v := e.TypoVariants(long)
        // 不去重，所以变体数 = 1 + 1000 * 4 = 4001
        if len(v) < 1000 {
                t.Errorf("TypoVariants(long) = %d, want at least 1000", len(v))
        }
        t.Logf("TypoVariants(1000 chars) -> %d variants", len(v))
}

// === 编辑距离测试 ===

func TestEditDistanceEdgeCases(t *testing.T) {
        cases := []struct {
                a, b string
                want int
        }{
                {"", "", 0},
                {"a", "", 1},
                {"", "a", 1},
                {"abc", "abc", 0},
                {"abc", "abcd", 1},      // 插入
                {"abcd", "abc", 1},      // 删除
                {"abc", "abd", 1},       // 替换
                {"abc", "xyz", 3},       // 全替换
                {"a", "a", 0},
                {"a", "b", 1},
                {"ab", "ba", 2},         // 两个替换（不是转置）
                {"kitten", "sitting", 3}, // 经典案例
                {"Sunday", "Saturday", 3},
                {"", "verylongstring", 14}, // 一边为空
        }
        for _, c := range cases {
                got := EditDistance(c.a, c.b)
                if got != c.want {
                        t.Errorf("EditDistance(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
                }
        }
}

func TestEditDistanceVeryLong(t *testing.T) {
        // 1000 字符的相同字符串
        long := strings.Repeat("a", 1000)
        if d := EditDistance(long, long); d != 0 {
                t.Errorf("EditDistance(same long) = %d, want 0", d)
        }
        // 完全不同
        a := strings.Repeat("a", 1000)
        b := strings.Repeat("b", 1000)
        if d := EditDistance(a, b); d != 1000 {
                t.Errorf("EditDistance(different long) = %d, want 1000", d)
        }
}

func TestEditDistanceChinese(t *testing.T) {
        // 中文字符在 Go 中是 UTF-8 多字节（每个汉字 3 字节）
        // EditDistance 按 byte 计算
        // 你好 = E4 BD A0 E5 A5 BD
        // 你坏 = E4 BD A0 E5 9D 8F  (后 2 字节不同：A5→9D, BD→8F)
        // 所以 "你好"→"你坏" 距离 = 2（不是 3，因为首字节 E5 相同）
        cases := []struct {
                a, b string
                want int
        }{
                {"你好", "你好", 0},
                {"你好", "你坏", 2},        // 好→坏，UTF-8 后 2 字节不同
                {"你好", "你好啊", 3},      // 插入 1 个汉字 = 3 字节
                {"你好啊", "你好", 3},      // 删除
                {"中国人", "中国", 3},      // 删除 1 汉字 = 3 字节
                {"中国", "美国", 3},        // 中→美，UTF-8 三字节都不同
        }
        for _, c := range cases {
                got := EditDistance(c.a, c.b)
                if got != c.want {
                        t.Errorf("EditDistance(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
                }
        }
}

// === 综合压力测试 ===

func TestNoPanicOnExtremeInputs(t *testing.T) {
        e := New()
        // 不调用 EditDistance 的超长字符串（O(n²) 会卡死）
        cases := []string{
                "",
                "\x00",
                "\xff",
                strings.Repeat("a", 1000),     // 减小到 1000
                strings.Repeat("ni", 500),
                "\n\r\t",
                "!@#$%^&*()",
                "你好世界",
        }
        for _, c := range cases {
                _ = e.Expand(c)
                _ = e.TypoVariants(c)
                // EditDistance 用较短的输入
                _ = EditDistance(c, c[:min(len(c), 100)])
        }
}

func min(a, b int) int {
        if a < b {
                return a
        }
        return b
}

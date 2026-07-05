package fuzzy

import (
        "testing"
)

func TestExpandNihao(t *testing.T) {
        e := New()
        // "ni" 在默认配置下应该展开为 "ni" 和 "li"（n/l 模糊）
        vars := e.Expand("ni")
        if len(vars) < 2 {
                t.Errorf("Expand(ni) got %v, want at least 2", vars)
        }
        hasNi, hasLi := false, false
        for _, v := range vars {
                if v == "ni" {
                        hasNi = true
                }
                if v == "li" {
                        hasLi = true
                }
        }
        if !hasNi || !hasLi {
                t.Errorf("Expand(ni) = %v, want both ni and li", vars)
        }
}

func TestExpandZhong(t *testing.T) {
        e := New()
        vars := e.Expand("zhong")
        // zh/z 模糊 + ong/eng（不，应该是 ong/ang？不，默认只有 an/ang，没有 ong/什么）
        // zh -> z, 所以 "zhong" -> "zong"
        hasZhong, hasZong := false, false
        for _, v := range vars {
                if v == "zhong" {
                        hasZhong = true
                }
                if v == "zong" {
                        hasZong = true
                }
        }
        if !hasZhong {
                t.Errorf("missing zhong in %v", vars)
        }
        if !hasZong {
                t.Errorf("missing zong (zh/z fuzzy) in %v", vars)
        }
}

func TestExpandAll(t *testing.T) {
        e := New()
        results := e.ExpandAll([]string{"ni", "hao"})
        // ni -> [ni, li] (n/l 模糊), hao -> [hao]
        // 笛卡尔积 = 2 * 1 = 2
        if len(results) != 2 {
                t.Errorf("ExpandAll([ni hao]) got %d results, want 2: %v", len(results), results)
        }
        // 应包含 nihao 和 lihao
        hasNihao, hasLihao := false, false
        for _, r := range results {
                joined := r[0] + r[1]
                if joined == "nihao" {
                        hasNihao = true
                }
                if joined == "lihao" {
                        hasLihao = true
                }
        }
        if !hasNihao || !hasLihao {
                t.Errorf("missing nihao or lihao: %v", results)
        }
}

func TestNeighborKeys(t *testing.T) {
        e := New()
        n := e.NeighborKeys('a')
        // a 的邻键至少应有 q, w, s, z
        for _, want := range []string{"a", "q", "w", "s", "z"} {
                found := false
                for _, got := range n {
                        if got == want {
                                found = true
                                break
                        }
                }
                if !found {
                        t.Errorf("NeighborKeys(a) = %v, missing %s", n, want)
                }
        }
}

func TestTypoVariants(t *testing.T) {
        e := New()
        vars := e.TypoVariants("nihao")
        if len(vars) < 2 {
                t.Errorf("TypoVariants(nihao) got %d, want >= 2", len(vars))
        }
        // 应包含原值
        found := false
        for _, v := range vars {
                if v == "nihao" {
                        found = true
                        break
                }
        }
        if !found {
                t.Errorf("TypoVariants missing original: %v", vars)
        }
}

func TestEditDistance(t *testing.T) {
        cases := []struct {
                a, b string
                want int
        }{
                {"", "", 0},
                {"a", "", 1},
                {"", "a", 1},
                {"a", "a", 0},
                {"ab", "a", 1},
                {"abc", "abd", 1},
                {"kitten", "sitting", 3},
                {"nihao", "nihao", 0},
                {"nihao", "nihal", 1},
        }
        for _, c := range cases {
                got := EditDistance(c.a, c.b)
                if got != c.want {
                        t.Errorf("EditDistance(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
                }
        }
}

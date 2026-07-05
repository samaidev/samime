package pinyin

import (
        "testing"
)

func TestIsValidSyllable(t *testing.T) {
        cases := []struct {
                in   string
                want bool
        }{
                {"ni", true},
                {"hao", true},
                {"zhong", true},
                {"x", false},
                {"abc", false},
                {"a", true},
                {"o", true},
                {"e", true},
                {"ang", true},
                {"qvx", false},
        }
        for _, c := range cases {
                got := IsValidSyllable(c.in)
                if got != c.want {
                        t.Errorf("IsValidSyllable(%q) = %v, want %v", c.in, got, c.want)
                }
        }
}

func TestSegment(t *testing.T) {
        cases := []struct {
                in   string
                want int // 期望切分后的音节数
        }{
                {"nihao", 2},
                {"zhongguo", 2}, // zhong + guo
                {"a", 1},
                {"wo", 1},
                {"xian", 1},     // xian 可以是单音节
                {"shurufa", 3},
                {"pinyin", 2},
                {"nihaozhongguo", 4}, // ni+hao+zhong+guo
        }
        for _, c := range cases {
                got := Segment(c.in)
                if len(got) != c.want {
                        t.Errorf("Segment(%q) got %d syllables (%v), want %d", c.in, len(got), got, c.want)
                }
        }
}

func TestSegmentNihao(t *testing.T) {
        syls := Segment("nihao")
        if len(syls) != 2 {
                t.Fatalf("expected 2 syllables, got %d", len(syls))
        }
        if syls[0].Initial != "n" || syls[0].Final != "i" {
                t.Errorf("syllable 0 = %+v, want n+i", syls[0])
        }
        if syls[1].Initial != "h" || syls[1].Final != "ao" {
                t.Errorf("syllable 1 = %+v, want h+ao", syls[1])
        }
}

func TestSegmentZhongguo(t *testing.T) {
        syls := Segment("zhongguo")
        if len(syls) != 2 {
                t.Fatalf("expected 2 syllables, got %d: %+v", len(syls), syls)
        }
        if syls[0].Initial != "zh" || syls[0].Final != "ong" {
                t.Errorf("syllable 0 = %+v, want zh+ong", syls[0])
        }
        if syls[1].Initial != "g" || syls[1].Final != "uo" {
                t.Errorf("syllable 1 = %+v, want g+uo", syls[1])
        }
}

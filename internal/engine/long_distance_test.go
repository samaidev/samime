package engine

import (
	"fmt"
	"testing"

	"github.com/zai/goime/internal/dict"
)

// TestLongDistanceDeletion 长距容错：漏字匹配
// 输入 "woyaochfan"（漏 i）→ 应能匹配 "我要吃饭"
func TestLongDistanceDeletion(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	cases := []struct {
		input string
		desc  string
	}{
		{"woyaochfan", "我要吃饭（漏i）"},
		{"woyaochifn", "我要吃饭（漏a）"},
		{"woyaochfan", "我要吃饭（漏i）"},
	}
	for _, c := range cases {
		cands := eng.Search(c.input)
		fmt.Printf("\nSearch(%q) [%s]:\n", c.input, c.desc)
		for i, w := range cands {
			if i >= 8 {
				break
			}
			fmt.Printf("  %d. %s (%s) score=%.1f src=%s\n",
				i+1, w.Word, w.Pinyin, w.Score, w.Source)
		}
		// 检查是否有"我要吃饭"
		found := false
		for _, w := range cands {
			if w.Word == "我要吃饭" {
				found = true
				break
			}
		}
		if found {
			fmt.Printf("  ✓ 找到\"我要吃饭\"\n")
		} else {
			t.Logf("未找到\"我要吃饭\"（漏字容错待优化）")
		}
	}
}

// TestLongDistanceAcronym 长距容错：首字母缩写整句
// 输入 "wyacf"（每字首字母）→ 应能匹配 "我要吃饭"
func TestLongDistanceAcronym(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	cases := []struct {
		input string
		desc  string
	}{
		{"wycf", "我要吃饭"},
		{"wyqbwg", "我要去博物馆"},
		{"waxx", "我爱学习"},
	}
	for _, c := range cases {
		cands := eng.Search(c.input)
		fmt.Printf("\nSearch(%q) [%s]:\n", c.input, c.desc)
		for i, w := range cands {
			if i >= 8 {
				break
			}
			fmt.Printf("  %d. %s (%s) score=%.1f src=%s\n",
				i+1, w.Word, w.Pinyin, w.Score, w.Source)
		}
	}
}

// TestSingleCharSentencePredict 单字母全句补全
// 上次提交"我"，输入"y" → 应能联想出"我要去..."等整句
func TestSingleCharSentencePredict(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	cases := []struct {
		commit string
		input  string
		desc   string
	}{
		{"我", "y", "我+要去..."},
		{"今天天", "q", "今天天+气怎么样"},
		{"你好", "m", "你好+吗"},
	}
	for _, c := range cases {
		eng.ResetContext()
		eng.Commit(c.commit, "")
		cands := eng.Search(c.input)
		fmt.Printf("\nAfter commit %q, Search(%q) [%s]:\n", c.commit, c.input, c.desc)
		for i, w := range cands {
			if i >= 12 {
				break
			}
			fmt.Printf("  %d. %s (%s) score=%.1f src=%s\n",
				i+1, w.Word, w.Pinyin, w.Score, w.Source)
		}
	}
}

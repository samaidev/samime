package engine

import (
	"fmt"
	"testing"

	"github.com/zai/goime/internal/dict"
	"github.com/zai/goime/internal/pinyin"
)

// TestKK_DoubleInitial 验证输入 "kk" 能返回多字词（看看、可靠、开口等）
// 之前 fallbackSegment 把 "kk" 切成单个 [k]，导致 acronymMatch 不触发。
// 修复后 "kk" 切成 [k, k]，acronymMatch 能匹配双声母缩写词。
func TestKK_DoubleInitial(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	// 验证 Segment("kk") 返回 2 个音节
	syls := pinyin.Segment("kk")
	if len(syls) != 2 {
		t.Fatalf("Segment(\"kk\") = %d syls, want 2", len(syls))
	}
	for i, s := range syls {
		if s.Initial != "k" || s.Raw != "k" {
			t.Errorf("syls[%d] = %+v, want {Initial:k Raw:k}", i, s)
		}
	}

	cands := eng.Search("kk")
	if len(cands) == 0 {
		t.Fatal("Search(\"kk\") returned 0 candidates")
	}

	// 检查是否有多字词（至少 1 个 2 字以上候选）
	hasMultiChar := false
	var words []string
	for _, c := range cands {
		words = append(words, c.Word)
		if len([]rune(c.Word)) >= 2 {
			hasMultiChar = true
		}
	}
	fmt.Printf("Search(\"kk\") = %d candidates: %v\n", len(cands), words)

	if !hasMultiChar {
		t.Errorf("Search(\"kk\") has no multi-char words, all single: %v", words)
	}

	// 期望包含"看看"
	foundKankan := false
	for _, c := range cands {
		if c.Word == "看看" {
			foundKankan = true
		}
	}
	if !foundKankan {
		t.Errorf("Search(\"kk\") does not contain 看看, got: %v", words)
	}
}

// TestRegressions 验证修复 fallbackSegment 后单字母联想没退化
func TestRegressions(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	// 单字母联想应仍返回高频单字
	cands := eng.Search("k")
	if len(cands) == 0 {
		t.Fatal("Search(\"k\") returned 0 candidates")
	}
	fmt.Printf("Search(\"k\") = %d candidates (first 5): ", len(cands))
	for i, c := range cands {
		if i >= 5 { break }
		fmt.Printf("%s ", c.Word)
	}
	fmt.Println()

	// nh 缩写应仍返回多字词
	cands2 := eng.Search("nh")
	var words []string
	for _, c := range cands2 {
		words = append(words, c.Word)
	}
	fmt.Printf("Search(\"nh\") = %v\n", words)
	hasMulti := false
	for _, c := range cands2 {
		if len([]rune(c.Word)) >= 2 {
			hasMulti = true
		}
	}
	if !hasMulti {
		t.Errorf("Search(\"nh\") has no multi-char words: %v", words)
	}
}



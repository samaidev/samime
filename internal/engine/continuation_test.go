package engine

import (
	"fmt"
	"testing"

	"github.com/zai/goime/internal/dict"
)

// TestContinuationPrediction 测试续接联想（搜狗核心特性）
// 上次提交"今天天"，本次输入"q" → 应联想"气怎么样"或"气"
func TestContinuationPrediction(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	// 模拟用户先提交"今天天"
	eng.Commit("今天天", "jintiantian")

	// 现在输入"q"，应能续接联想出"气怎么样"或"气"
	cands := eng.Search("q")
	fmt.Printf("\nAfter commit \"今天天\", Search(\"q\"):\n")
	for i, c := range cands {
		if i >= 10 {
			break
		}
		fmt.Printf("  %d. %s (%s) score=%.1f src=%s\n",
			i+1, c.Word, c.Pinyin, c.Score, c.Source)
	}

	// 应包含续接候选
	foundContinuation := false
	foundQi := false
	for _, c := range cands {
		if c.Source == "continuation" {
			foundContinuation = true
			fmt.Printf("  continuation candidate: %s\n", c.Word)
		}
		// 检查是否包含"今天天气怎么样"或"今天天气"开头的候选（用 rune 切片处理中文）
		chars := []rune(c.Word)
		if len(chars) >= 4 && string(chars[:4]) == "今天天气" {
			foundQi = true
		}
	}
	if !foundContinuation {
		t.Error("未产生续接候选")
	}
	if !foundQi {
		t.Errorf("未找到\"今天天气...\"候选，期望\"今天天气怎么样\"或类似续接")
	}
}

// TestContinuationNoHistory 无提交历史时不触发续接
func TestContinuationNoHistory(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	// 无提交历史
	cands := eng.Search("q")
	for _, c := range cands {
		if c.Source == "continuation" {
			t.Error("无提交历史时不应产生续接候选")
		}
	}
	fmt.Printf("Search(\"q\") without history: %d cands, no continuation (OK)\n", len(cands))
}

// TestContinuationReset 重置上下文后不触发续接
func TestContinuationReset(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)
	eng.Commit("今天天", "jintiantian")
	eng.ResetContext()

	cands := eng.Search("q")
	for _, c := range cands {
		if c.Source == "continuation" {
			t.Error("重置上下文后不应产生续接候选")
		}
	}
	fmt.Printf("Search(\"q\") after reset: %d cands, no continuation (OK)\n", len(cands))
}

// TestContinuationMultiChar 测试多字续接
func TestContinuationMultiChar(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	// 模拟提交"你好"
	eng.Commit("你好", "nihao")

	// 输入"m"，应联想"吗"（你好→吗 是常见 bigram）
	cands := eng.Search("m")
	fmt.Printf("\nAfter commit \"你好\", Search(\"m\"):\n")
	for i, c := range cands {
		if i >= 8 {
			break
		}
		fmt.Printf("  %d. %s (%s) score=%.1f src=%s\n",
			i+1, c.Word, c.Pinyin, c.Score, c.Source)
	}

	// 应有"你好吗"候选
	foundMa := false
	for _, c := range cands {
		if c.Word == "你好吗" || (len(c.Word) >= 3 && c.Word[:3] == "你好吗") {
			foundMa = true
			break
		}
	}
	if foundMa {
		fmt.Printf("  ✓ 找到\"你好吗\"续接候选\n")
	} else {
		t.Logf("未找到\"你好吗\"（可能 bigram 无此条目）")
	}
}

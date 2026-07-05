//go:build integration

// 端到端集成测试：模拟完整输入流程
package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zai/goime/internal/dict"
	"github.com/zai/goime/internal/engine"
	"github.com/zai/goime/internal/ibus"
)

func TestE2EBasicInput(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Fatalf("load dict: %v", err)
	}
	eng := engine.NewDefault(d)
	ibusEng := ibus.New(eng)

	var committed []string
	ibusEng.OnCommitted = func(text string) {
		committed = append(committed, text)
	}

	// 输入 "nihao" + 回车
	for _, c := range "nihao" {
		ibusEng.ProcessKey(string(c))
	}
	ibusEng.ProcessKey("Return")

	if len(committed) != 1 {
		t.Fatalf("expected 1 commit, got %d: %v", len(committed), committed)
	}
	if committed[0] != "你好" {
		t.Errorf("expected '你好', got %q", committed[0])
	}
	t.Logf("E2E basic: nihao -> %s", committed[0])
}

func TestE2ESelectionByNumber(t *testing.T) {
	d, _ := dict.LoadEmbedded()
	eng := engine.NewDefault(d)
	ibusEng := ibus.New(eng)

	var committed []string
	ibusEng.OnCommitted = func(text string) {
		committed = append(committed, text)
	}

	// 输入 "nihao" 然后选第 2 个候选
	for _, c := range "nihao" {
		ibusEng.ProcessKey(string(c))
	}
	cands := ibusEng.Candidates()
	if len(cands) < 2 {
		t.Fatalf("need at least 2 candidates, got %d", len(cands))
	}
	expected := cands[1].Word
	ibusEng.ProcessKey("2")

	if len(committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(committed))
	}
	if committed[0] != expected {
		t.Errorf("expected %q, got %q", expected, committed[0])
	}
	t.Logf("E2E selection: nihao + 2 -> %s", committed[0])
}

func TestE2EBackspace(t *testing.T) {
	d, _ := dict.LoadEmbedded()
	eng := engine.NewDefault(d)
	ibusEng := ibus.New(eng)

	// 输入 "niha" 后退格一次，再输入 "o"
	for _, c := range "niha" {
		ibusEng.ProcessKey(string(c))
	}
	ibusEng.ProcessKey("BackSpace")
	if ibusEng.Preedit() != "nih" {
		t.Errorf("after backspace, preedit = %q, want 'nih'", ibusEng.Preedit())
	}
	ibusEng.ProcessKey("o")
	if ibusEng.Preedit() != "niho" {
		t.Errorf("after retype, preedit = %q, want 'niho'", ibusEng.Preedit())
	}
}

func TestE2EEscape(t *testing.T) {
	d, _ := dict.LoadEmbedded()
	eng := engine.NewDefault(d)
	ibusEng := ibus.New(eng)

	for _, c := range "nihao" {
		ibusEng.ProcessKey(string(c))
	}
	ibusEng.ProcessKey("Escape")
	if ibusEng.Preedit() != "" {
		t.Errorf("after ESC, preedit = %q, want empty", ibusEng.Preedit())
	}
	if len(ibusEng.Candidates()) != 0 {
		t.Errorf("after ESC, candidates = %v, want empty", ibusEng.Candidates())
	}
}

func TestE2EMultipleSentences(t *testing.T) {
	d, _ := dict.LoadEmbedded()
	eng := engine.NewDefault(d)
	ibusEng := ibus.New(eng)

	var allCommitted []string
	ibusEng.OnCommitted = func(text string) {
		allCommitted = append(allCommitted, text)
	}

	sentences := []struct {
		input string
	}{
		{"nihao"},
		{"zhongguo"},
		{"shurufa"},
		{"xuesheng"},
	}
	for _, s := range sentences {
		for _, c := range s.input {
			ibusEng.ProcessKey(string(c))
		}
		ibusEng.ProcessKey("Return")
		ibusEng.ProcessKey("Escape") // 重置上下文
	}

	if len(allCommitted) != len(sentences) {
		t.Fatalf("expected %d commits, got %d: %v", len(sentences), len(allCommitted), allCommitted)
	}
	for i, s := range sentences {
		t.Logf("  %s -> %s", s.input, allCommitted[i])
	}
}

func TestE2EFuzzyInput(t *testing.T) {
	d, _ := dict.LoadEmbedded()
	eng := engine.NewDefault(d)

	// 模糊音测试：n/l 混淆
	cases := []struct {
		name string
		py   string
	}{
		{"lihao (n/l)", "lihao"},
		{"zongguo (zh/z)", "zongguo"},
	}
	for _, c := range cases {
		cands := eng.Search(c.py)
		t.Logf("  fuzzy: %s -> %s", c.name, topWords(cands, 3))
	}
}

func TestE2ETypoInput(t *testing.T) {
	d, _ := dict.LoadEmbedded()
	eng := engine.NewDefault(d)

	cases := []struct {
		name string
		py   string
	}{
		{"nigao (h->g)", "nigao"},
		{"nohao (i->o)", "nohao"},
	}
	for _, c := range cases {
		cands := eng.Search(c.py)
		t.Logf("  typo: %s -> %s", c.name, topWords(cands, 3))
	}
}

func TestE2ELatencyUnder16ms(t *testing.T) {
	d, _ := dict.LoadEmbedded()
	eng := engine.NewDefault(d)

	queries := []string{
		"nihao", "zhongguo", "shurufa", "pinyin", "xuesheng",
		"laoshi", "diannao", "shouji", "pengyou", "gongzuo",
	}

	// 预热
	for _, q := range queries {
		_ = eng.Search(q)
	}

	// 测量
	const maxLatency = 16 * time.Millisecond
	var slowCount int
	for iter := 0; iter < 100; iter++ {
		for _, q := range queries {
			t0 := time.Now()
			_ = eng.Search(q)
			dur := time.Since(t0)
			if dur > maxLatency {
				slowCount++
				t.Logf("  slow: %s took %v", q, dur)
			}
		}
	}
	totalQueries := 100 * len(queries)
	if slowCount > totalQueries/10 { // 允许 10% 超时
		t.Errorf("too many slow queries: %d/%d (>16ms)", slowCount, totalQueries)
	}
	t.Logf("latency check: %d/%d queries > 16ms (allowed 10%%)", slowCount, totalQueries)
}

func TestE2EBenchmarkFull(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}
	d, _ := dict.LoadEmbedded()
	eng := engine.NewDefault(d)

	queries := []string{
		"nihao", "zhongguo", "shurufa", "pinyin", "xuesheng",
		"laoshi", "diannao", "shouji", "pengyou", "gongzuo",
		"nihaozhongguo", "wome", "shijie", "jiandan", "yixia",
		"rengongzhineng", "jiqixuexi", "shenduxuexi", "yunjisuan", "qukuailian",
	}
	const N = 2000
	t0 := time.Now()
	for i := 0; i < N; i++ {
		q := queries[i%len(queries)]
		_ = eng.Search(q)
	}
	dur := time.Since(t0)
	avg := dur / time.Duration(N)
	t.Logf("benchmark: %d queries | total %v | avg %v/q | qps=%.0f",
		N, dur, avg, float64(N)/dur.Seconds())

	// 输出报告到 stderr
	fmt.Fprintf(os.Stderr, "\n[Benchmark] %d queries, avg %v/q, qps=%.0f\n", N, avg, float64(N)/dur.Seconds())
}

func topWords(cands []engine.Candidate, n int) string {
	if len(cands) == 0 {
		return "-"
	}
	parts := make([]string, 0, n)
	for i, c := range cands {
		if i >= n {
			break
		}
		parts = append(parts, c.Word)
	}
	return strings.Join(parts, " ")
}

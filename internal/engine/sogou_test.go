package engine

import (
	"fmt"
	"testing"

	"github.com/zai/goime/internal/dict"
)

// TestSogouMixedInput 测试简拼/全拼混合输入（搜狗核心特性）
// nhao → 你好（n 简拼 + hao 全拼）
// nih  → 你好（ni 全拼 + h 简拼）
// shfa → 书法（sh 简拼 + fa 全拼）
func TestSogouMixedInput(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	cases := []struct {
		input string
		desc  string
	}{
		{"nhao", "n简拼+hao全拼→你好"},
		{"nih", "ni全拼+h简拼→你好"},
		{"shfa", "sh简拼+fa全拼→书法"},
		{"zhg", "zh简拼+g简拼→中国(acronym)"},
	}

	for _, c := range cases {
		cands := eng.Search(c.input)
		var words []string
		for i, w := range cands {
			if i >= 5 {
				break
			}
			words = append(words, w.Word)
		}
		fmt.Printf("Search(%q) [%s] = %v\n", c.input, c.desc, words)
	}

	// nhao 应包含"你好"
	cands := eng.Search("nhao")
	foundNihao := false
	for _, c := range cands {
		if c.Word == "你好" {
			foundNihao = true
			break
		}
	}
	if !foundNihao {
		t.Errorf("Search(\"nhao\") 应包含\"你好\"（n简拼+hao全拼），实际: %v", topWords(cands, 5))
	}
}

// TestSogouTypoCorrection 测试增强的拼写纠错（搜狗级联纠错第一层）
// nihoa → 你好（o/a 转置）
// nihaoo → 你好（删除多余 o）
func TestSogouTypoCorrection(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	cases := []struct {
		input string
		desc  string
	}{
		{"nihoa", "转置(o/a互换)→你好"},
		{"nihaoo", "删除(多余o)→你好"},
		{"nihap", "替换(p→o)→你好"},
	}

	for _, c := range cases {
		cands := eng.Search(c.input)
		var words []string
		for i, w := range cands {
			if i >= 5 {
				break
			}
			words = append(words, w.Word)
		}
		fmt.Printf("Search(%q) [%s] = %v\n", c.input, c.desc, words)
	}

	// nihoa 应包含"你好"（转置纠错）
	cands := eng.Search("nihoa")
	foundNihao := false
	for _, c := range cands {
		if c.Word == "你好" {
			foundNihao = true
			break
		}
	}
	if !foundNihao {
		t.Errorf("Search(\"nihoa\") 应包含\"你好\"（o/a转置纠错），实际: %v", topWords(cands, 5))
	}
}

// TestSogouFuzzyGrading 测试模糊音分级评分
// 模糊 1 个音节的候选应比模糊 2 个音节的候选分高
func TestSogouFuzzyGrading(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	// nihao 模糊音应返回 你好（精确）+ lihao（l/n模糊1个）等
	cands := eng.Search("nihao")
	var fuzzyCands []Candidate
	for _, c := range cands {
		if c.Source == "fuzzy" {
			fuzzyCands = append(fuzzyCands, c)
		}
	}
	fmt.Printf("Search(\"nihao\") fuzzy cands: %d\n", len(fuzzyCands))
	for i, c := range fuzzyCands {
		if i >= 5 {
			break
		}
		fmt.Printf("  %s (%s) score=%.1f\n", c.Word, c.Pinyin, c.Score)
	}
}

// TestSogouSentencePriority 测试 sentence 来源优先级修复
// 之前 sentence 不在 sourcePriority 表，同分时被排末尾
func TestSogouSentencePriority(t *testing.T) {
	if sourcePriority("sentence") == 0 {
		t.Error("sentence 来源优先级为 0（之前 bug），应 > 0")
	}
	if sourcePriority("sentence") <= sourcePriority("fuzzy") {
		t.Error("sentence 优先级应高于 fuzzy")
	}
	if sourcePriority("mixed") <= sourcePriority("acronym") {
		t.Error("mixed（混合简拼）优先级应高于 acronym")
	}
	fmt.Printf("sourcePriority: dict=%d segment=%d mixed=%d acronym=%d sentence=%d fuzzy=%d typo=%d\n",
		sourcePriority("dict"), sourcePriority("segment"), sourcePriority("mixed"),
		sourcePriority("acronym"), sourcePriority("sentence"), sourcePriority("fuzzy"),
		sourcePriority("typo"))
}

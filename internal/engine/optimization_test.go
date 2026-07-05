package engine

import (
	"testing"
)

// === 单字母联想测试 ===

func TestSearchSingleLetterN(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	cands := e.Search("n")
	if len(cands) == 0 {
		t.Fatal("Search('n') should return candidates")
	}
	t.Logf("n -> top 5:")
	for i, c := range cands {
		if i >= 5 {
			break
		}
		t.Logf("  %d. %s (%s) src=%s", i+1, c.Word, c.Pinyin, c.Source)
	}
	// 应包含 "你" 或 "年" 等高频字
	hasHighFreq := false
	for _, c := range cands {
		if c.Word == "你" || c.Word == "年" || c.Word == "那" || c.Word == "能" {
			hasHighFreq = true
			break
		}
	}
	if !hasHighFreq {
		t.Errorf("expected high-freq char like 你/年/那/能, got %v", topWords(cands, 5))
	}
}

func TestSearchSingleLetterAllInitials(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	initials := []string{"b", "p", "m", "f", "d", "t", "n", "l",
		"g", "k", "h", "j", "q", "x", "r", "z", "c", "s",
		"y", "w"}
	for _, ini := range initials {
		cands := e.Search(ini)
		if len(cands) == 0 {
			t.Errorf("Search(%q) returned 0 candidates", ini)
			continue
		}
		t.Logf("%s -> %s", ini, topWords(cands, 3))
	}
}

// === 首字母缩写联想测试 ===

func TestSearchAcronymNH(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	cands := e.Search("nh")
	if len(cands) == 0 {
		t.Fatal("Search('nh') should return candidates")
	}
	t.Logf("nh -> top 5:")
	for i, c := range cands {
		if i >= 5 {
			break
		}
		t.Logf("  %d. %s (%s) src=%s", i+1, c.Word, c.Pinyin, c.Source)
	}
	// 应包含 "你好"
	hasNihao := false
	for _, c := range cands {
		if c.Word == "你好" {
			hasNihao = true
			break
		}
	}
	if !hasNihao {
		t.Errorf("'你好' should be in nh acronym candidates: %v", topWords(cands, 10))
	}
}

func TestSearchAcronymZG(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	cands := e.Search("zg")
	if len(cands) == 0 {
		t.Fatal("Search('zg') should return candidates")
	}
	t.Logf("zg -> top 5: %s", topWords(cands, 5))
	// 应包含 "中国" 或 "祖国" 或 "最高" 等
	hasExpected := false
	for _, c := range cands {
		if c.Word == "中国" || c.Word == "祖国" || c.Word == "最高" || c.Word == "资格" {
			hasExpected = true
			break
		}
	}
	if !hasExpected {
		t.Errorf("expected 中国/祖国/最高/资格, got %v", topWords(cands, 10))
	}
}

func TestSearchAcronymBJ(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	cands := e.Search("bj")
	if len(cands) == 0 {
		t.Fatal("Search('bj') should return candidates")
	}
	t.Logf("bj -> top 5: %s", topWords(cands, 5))
	hasBeijing := false
	for _, c := range cands {
		if c.Word == "北京" {
			hasBeijing = true
			break
		}
	}
	if !hasBeijing {
		t.Errorf("'北京' should be in bj acronym candidates: %v", topWords(cands, 10))
	}
}

// === 声母遗漏容错测试 ===

func TestSearchMissingInitialAo(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	cands := e.Search("ao")
	if len(cands) == 0 {
		t.Fatal("Search('ao') should return candidates")
	}
	t.Logf("ao -> top 5: %s", topWords(cands, 5))
	// 应包含 "好" (hao)、"高" (gao)、"到" (dao) 等
	hasExpected := false
	for _, c := range cands {
		if c.Word == "好" || c.Word == "高" || c.Word == "到" || c.Word == "老" {
			hasExpected = true
			break
		}
	}
	if !hasExpected {
		t.Errorf("expected 好/高/到/老, got %v", topWords(cands, 10))
	}
}

func TestSearchMissingInitialAllFinals(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	finals := []string{"a", "o", "e", "ai", "ei", "ao", "ou", "an", "en", "ang", "eng", "ong"}
	for _, f := range finals {
		cands := e.Search(f)
		if len(cands) == 0 {
			t.Errorf("Search(%q) returned 0 candidates", f)
			continue
		}
		t.Logf("%s -> %s", f, topWords(cands, 3))
	}
}

// === 候选词去重测试 ===

func TestSearchNoDuplicateWord(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	cands := e.Search("nihao")
	seen := make(map[string]bool)
	for _, c := range cands {
		if seen[c.Word] {
			t.Errorf("duplicate word found: %s", c.Word)
		}
		seen[c.Word] = true
	}
}

// === 模糊音增强测试 ===

func TestSearchFuzzyFH(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	// f/h 模糊：fahao 应该能联想到 hahao 或 fafao
	cands := e.Search("fahao")
	t.Logf("fahao (f/h fuzzy) -> top 5: %s", topWords(cands, 5))
}

func TestSearchFuzzyLR(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	// l/r 模糊
	cands := e.Search("ran")
	t.Logf("ran (l/r fuzzy) -> top 5: %s", topWords(cands, 5))
}

// === 综合场景测试 ===

func TestSearchTypicalUsageScenarios(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)

	scenarios := []struct {
		name string
		py   string
		desc string
	}{
		{"基础-你好", "nihao", "整词"},
		{"整句-woaixuexi", "woaixuexi", "整句切分"},
		{"单字母-n", "n", "单声母联想"},
		{"单字母-w", "w", "单声母联想"},
		{"缩写-nh", "nh", "首字母缩写"},
		{"缩写-zg", "zg", "首字母缩写"},
		{"缩写-bj", "bj", "首字母缩写"},
		{"声母遗漏-ao", "ao", "纯韵母"},
		{"声母遗漏-ang", "ang", "纯韵母"},
		{"模糊音-lihao", "lihao", "n/l 模糊"},
		{"拼写错误-nigao", "nigao", "邻键容错"},
	}

	for _, s := range scenarios {
		cands := e.Search(s.py)
		top := "-"
		if len(cands) > 0 {
			top = cands[0].Word
		}
		t.Logf("%-30s (%s) -> %s", s.name, s.desc, top)
	}
}

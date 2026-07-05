package dict

import (
	"strings"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	d, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded failed: %v", err)
	}
	if d.Size() == 0 {
		t.Fatal("empty dict")
	}
	t.Logf("loaded %d entries", d.Size())
}

func TestLookupNihao(t *testing.T) {
	d, err := LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded: %v", err)
	}
	entries := d.Lookup("nihao")
	if len(entries) == 0 {
		t.Fatal("no entries for 'nihao'")
	}
	// 应该能找到 "你好"
	found := false
	for _, e := range entries {
		if e.Word == "你好" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("'你好' not found in nihao results: %v", entries[:min(5, len(entries))])
	}
}

func TestLookupZhongguo(t *testing.T) {
	d, _ := LoadEmbedded()
	entries := d.Lookup("zhongguo")
	if len(entries) == 0 {
		t.Fatal("no entries for 'zhongguo'")
	}
	t.Logf("zhongguo -> %v", entries[:min(5, len(entries))])
}

func TestPrefixMatch(t *testing.T) {
	d, _ := LoadEmbedded()
	matches := d.LookupPrefix("ni")
	if len(matches) == 0 {
		t.Fatal("no prefix matches for 'ni'")
	}
	hasNihao := false
	for _, m := range matches {
		if m == "nihao" {
			hasNihao = true
		}
	}
	if !hasNihao {
		t.Errorf("'nihao' not in prefix matches: %v", matches[:min(10, len(matches))])
	}
}

func TestStats(t *testing.T) {
	d, _ := LoadEmbedded()
	s := d.Stats()
	if s.TotalEntries < 1000 {
		t.Errorf("too few entries: %d", s.TotalEntries)
	}
	t.Logf("stats: %+v", s)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 防止 strings 未使用警告
var _ = strings.ToLower

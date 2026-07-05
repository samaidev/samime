package dict

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookupEmpty(t *testing.T) {
	d, _ := LoadEmbedded()
	if entries := d.Lookup(""); len(entries) != 0 {
		t.Errorf("Lookup('') should return empty, got %d", len(entries))
	}
}

func TestLookupNonExistent(t *testing.T) {
	d, _ := LoadEmbedded()
	// 完全不存在的拼音
	if entries := d.Lookup("zzzzzzzzz"); len(entries) != 0 {
		t.Errorf("Lookup('zzzzzzzzz') should return empty, got %d", len(entries))
	}
	if entries := d.Lookup("qqqqqq"); len(entries) != 0 {
		t.Errorf("Lookup('qqqqqq') should return empty, got %d", len(entries))
	}
}

func TestLookupVeryLongPinyin(t *testing.T) {
	d, _ := LoadEmbedded()
	long := strings.Repeat("a", 100)
	if entries := d.Lookup(long); len(entries) != 0 {
		t.Errorf("Lookup(very long) should return empty, got %d", len(entries))
	}
}

func TestLookupCaseInsensitive(t *testing.T) {
	d, _ := LoadEmbedded()
	// 拼音存储是小写，但查询也要小写
	cands1 := d.Lookup("nihao")
	cands2 := d.Lookup("NIHAO")
	if len(cands1) > 0 && len(cands2) == 0 {
		// Lookup 不应该做大小写转换（避免性能损失）
		// 这是预期行为，调用方负责小写化
		t.Logf("Lookup is case-sensitive (expected): nihao=%d, NIHAO=%d",
			len(cands1), len(cands2))
	}
}

func TestPrefixMatchEmpty(t *testing.T) {
	d, _ := LoadEmbedded()
	if matches := d.LookupPrefix(""); len(matches) != 0 {
		t.Errorf("LookupPrefix('') should return empty, got %d", len(matches))
	}
}

func TestPrefixMatchNonExistent(t *testing.T) {
	d, _ := LoadEmbedded()
	if matches := d.LookupPrefix("zzzzz"); len(matches) != 0 {
		t.Errorf("LookupPrefix('zzzzz') should return empty, got %d", len(matches))
	}
}

func TestPrefixMatchSingleChar(t *testing.T) {
	d, _ := LoadEmbedded()
	matches := d.LookupPrefix("a")
	if len(matches) == 0 {
		t.Error("LookupPrefix('a') should return matches")
	}
	// 'a' 应该匹配 'a', 'ai', 'ao', 'an', 'ang' 等
	expectedPinyins := []string{"a", "ai", "ao", "an", "ang"}
	for _, exp := range expectedPinyins {
		found := false
		for _, m := range matches {
			if m == exp {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q in LookupPrefix('a') results", exp)
		}
	}
}

func TestPrefixMatchVeryShort(t *testing.T) {
	d, _ := LoadEmbedded()
	// 单字符前缀应该返回大量结果
	matches := d.LookupPrefix("n")
	t.Logf("LookupPrefix('n') -> %d matches", len(matches))
	if len(matches) < 10 {
		t.Errorf("expected many matches for 'n', got %d", len(matches))
	}
}

func TestPrefixMatchVeryLong(t *testing.T) {
	d, _ := LoadEmbedded()
	// 超长前缀
	long := strings.Repeat("a", 50)
	matches := d.LookupPrefix(long)
	if len(matches) != 0 {
		t.Errorf("LookupPrefix(very long) should return empty, got %d", len(matches))
	}
}

func TestLoadFromReaderMalformed(t *testing.T) {
	// 各种畸形输入
	cases := []string{
		"",
		"# only comment",
		"word",                    // 只有 word，没有 pinyin
		"word pinyin",             // 缺频次（应该用默认 1）
		"word pinyin not_a_number", // 频次不是数字
		"word pinyin -1",          // 负频次
		"word pinyin 0",           // 零频次
		"word pinyin 1e10",        // 科学计数法
		"word pinyin 1.5e-3",
		"   word   pinyin   100   ", // 多余空格
		"\tword\tpinyin\t100\t",     // Tab 分隔
		"\n\n\nword pinyin 100\n\n", // 多空行
	}
	for _, c := range cases {
		d := New()
		err := d.LoadFromReader(strings.NewReader(c), "test")
		if err != nil {
			t.Errorf("LoadFromReader(%q) returned error: %v", c, err)
		}
		// 不应 panic
	}
}

func TestLoadFromReaderSpecialChars(t *testing.T) {
	// 特殊字符的词条
	input := `# test
你好 nihao 100
， ， 1     # 标点
空格 kongge 1
 emoji 1
日本人 ribenren 50
EnglishWord english 1   # 英文混合
`
	d := New()
	if err := d.LoadFromReader(strings.NewReader(input), "test"); err != nil {
		t.Fatal(err)
	}
	if d.Size() == 0 {
		t.Error("expected non-empty dict")
	}
}

func TestLoadFromReaderLargeFile(t *testing.T) {
	// 10 万行（模拟大词库）
	var sb strings.Builder
	sb.WriteString("# large test\n")
	for i := 0; i < 100000; i++ {
		sb.WriteString("词")
		sb.WriteString(" ci")
		sb.WriteString(" 1\n")
	}
	d := New()
	if err := d.LoadFromReader(strings.NewReader(sb.String()), "large"); err != nil {
		t.Fatal(err)
	}
	if d.Size() != 100000 {
		t.Errorf("expected 100000 entries, got %d", d.Size())
	}
}

func TestLoadFromReaderDuplicateEntries(t *testing.T) {
	// 重复词条
	input := `你好 nihao 100
你好 nihao 200
你好 nihao 50
世界 shijie 80
`
	d := New()
	d.LoadFromReader(strings.NewReader(input), "dup")
	// 所有重复都应该被保留（不去重）
	entries := d.Lookup("nihao")
	if len(entries) != 3 {
		t.Errorf("expected 3 duplicate entries, got %d", len(entries))
	}
}

func TestLoadFromFileNonExistent(t *testing.T) {
	_, err := LoadFromFile("/nonexistent/path/file.txt")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoadFromFileNoPermission(t *testing.T) {
	// 无权限文件（root 才能读）
	tmpFile := filepath.Join(os.TempDir(), "noperm.txt")
	os.WriteFile(tmpFile, []byte("word pinyin 1"), 0000)
	defer os.Remove(tmpFile)
	defer os.Chmod(tmpFile, 0644)

	if _, err := LoadFromFile(tmpFile); err == nil {
		// 在 root 用户下可能不报错
		t.Logf("LoadFromFile on no-perm file did not fail (running as root?)")
	}
}

func TestStatsConsistency(t *testing.T) {
	d, _ := LoadEmbedded()
	s1 := d.Stats()
	s2 := d.Stats()
	if s1 != s2 {
		t.Errorf("Stats() not consistent: %+v vs %+v", s1, s2)
	}
	if s1.TotalEntries <= 0 {
		t.Errorf("TotalEntries should be positive, got %d", s1.TotalEntries)
	}
	if s1.UniquePinyin > s1.TotalEntries {
		t.Errorf("UniquePinyin (%d) > TotalEntries (%d)",
			s1.UniquePinyin, s1.TotalEntries)
	}
}

func TestLookupMultiEdgeCases(t *testing.T) {
	d, _ := LoadEmbedded()
	// nil 输入
	if entries := d.LookupMulti(nil); len(entries) != 0 {
		t.Errorf("LookupMulti(nil) should return empty, got %d", len(entries))
	}
	// 空切片
	if entries := d.LookupMulti([]string{}); len(entries) != 0 {
		t.Errorf("LookupMulti([]) should return empty, got %d", len(entries))
	}
	// 单元素不存在
	if entries := d.LookupMulti([]string{"zzzzz"}); len(entries) != 0 {
		t.Errorf("LookupMulti([zzzzz]) should return empty, got %d", len(entries))
	}
	// 多元素，第一个存在
	entries := d.LookupMulti([]string{"ni", "hao"})
	if len(entries) == 0 {
		t.Error("LookupMulti([ni hao]) should return entries")
	}
}

package pinyin

import (
	"strings"
	"testing"
)

// === 边缘情况测试 ===

func TestSegmentEmpty(t *testing.T) {
	if s := Segment(""); len(s) != 0 {
		t.Errorf("empty input should return empty, got %v", s)
	}
	if s := Segment("   "); len(s) != 0 {
		t.Errorf("whitespace input should return empty, got %v", s)
	}
}

func TestSegmentSingleChar(t *testing.T) {
	cases := []string{"a", "e", "o", "i", "u"}
	for _, c := range cases {
		s := Segment(c)
		if len(s) != 1 {
			t.Errorf("Segment(%q) got %d syllables, want 1: %v", c, len(s), s)
		}
	}
}

func TestSegmentSingleInvalidChar(t *testing.T) {
	// 单个非拼音字母
	cases := []string{"b", "d", "g", "x", "z"}
	for _, c := range cases {
		s := Segment(c)
		// 单声母不能成音节，应该走 fallback 返回 1 个空音节
		if len(s) > 1 {
			t.Errorf("Segment(%q) got %d, want <= 1", c, len(s))
		}
	}
}

func TestSegmentVeryLong(t *testing.T) {
	// 1000 个音节的超长输入
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("a")
	}
	input := sb.String()
	s := Segment(input)
	// "a" 是合法音节，应该被切成 1000 个
	if len(s) != 1000 {
		t.Errorf("long input: got %d syllables, want 1000", len(s))
	}
}

func TestSegmentMixedValidInvalid(t *testing.T) {
	// 有效 + 无效字符混合
	cases := []struct {
		in   string
		want int // 预期音节数（不严格）
	}{
		{"niha0", 2},      // 0 是数字，会触发 fallback
		{"ni ha", 2},      // 空格分隔（无分隔符处理）
		{"nihao!", 2},     // 标点
		{"a1b2c3", 0},     // 全是字母+数字混合
		{"abc", 0},        // 非拼音
	}
	for _, c := range cases {
		s := Segment(c.in)
		t.Logf("Segment(%q) -> %d syllables: %v", c.in, len(s), s)
		// 不严格断言，仅记录行为
	}
}

func TestSegmentAmbiguous(t *testing.T) {
	// 歧义切分：xian 可以是 xi'an 或 xian
	cases := []struct {
		in   string
		desc string
	}{
		{"xian", "可以是 xi+an 或 xian"},
		{"xianin", "xi+an+in / xian+in / xi+anin 等"},
		{"fangan", "fang+an / fan+gan"},
		{"nanbu", "nan+bu / nan+bu"},
		{"tiananmen", "tian+an+men"},
	}
	for _, c := range cases {
		s := Segment(c.in)
		t.Logf("%-12q (%s) -> %d 音节: %v", c.in, c.desc, len(s), s)
	}
}

func TestSegmentCaseInsensitive(t *testing.T) {
	// 大小写混合
	cases := []string{"NIHAO", "NiHao", "nIHao"}
	for _, c := range cases {
		s := Segment(c)
		if len(s) != 2 {
			t.Errorf("Segment(%q) got %d, want 2 (ni+hao)", c, len(s))
		}
		if len(s) >= 2 {
			if s[0].Raw != "ni" || s[1].Raw != "hao" {
				t.Errorf("Segment(%q) = %v, want [ni hao]", c, s)
			}
		}
	}
}

func TestSegmentNumbersAndSymbols(t *testing.T) {
	// 数字、符号
	cases := []string{
		"123",
		"!@#$%",
		"ni1hao2",
		"你好", // 中文字符
		"\t\n\r",
	}
	for _, c := range cases {
		s := Segment(c)
		t.Logf("Segment(%q) -> %d 音节: %v", c, len(s), s)
		// 不应 panic
	}
}

func TestSegmentToneNumbers(t *testing.T) {
	// 用户可能输入声调数字（实际不应该，但容错）
	cases := []string{
		"ni3hao3",  // 带声调
		"ma1ma1",   // mā mā
		"zhong1guo2",
	}
	for _, c := range cases {
		s := Segment(c)
		t.Logf("Segment(%q) -> %v", c, s)
	}
}

func TestSegmentNoPanicOnExtremeInput(t *testing.T) {
	// 确保各种极端输入不 panic
	cases := []string{
		"",
		" ",
		"\x00",
		"\xff\xff\xff",
		strings.Repeat("a", 100000),
		strings.Repeat("x", 1000), // 全无效
		"                ",
	}
	for _, c := range cases {
		_ = Segment(c)
	}
}

func TestSyllableStructure(t *testing.T) {
	// 验证音节结构正确
	cases := []struct {
		in         string
		wantInit   string
		wantFinal  string
	}{
		{"ni", "n", "i"},
		{"hao", "h", "ao"},
		{"zhong", "zh", "ong"},
		{"a", "", "a"},
		{"er", "", "er"},
	}
	for _, c := range cases {
		syls := Segment(c.in)
		if len(syls) != 1 {
			t.Errorf("Segment(%q) got %d, want 1", c.in, len(syls))
			continue
		}
		s := syls[0]
		if s.Initial != c.wantInit {
			t.Errorf("Segment(%q).Initial = %q, want %q", c.in, s.Initial, c.wantInit)
		}
		if s.Final != c.wantFinal {
			t.Errorf("Segment(%q).Final = %q, want %q", c.in, s.Final, c.wantFinal)
		}
	}
}

func TestIsValidSyllableEdgeCases(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"a", true},
		{"e", true},
		{"o", true},
		{"i", true},
		{"u", true},
		{"v", true},
		{"ng", false},      // ng 不是合法音节
		{"z", false},       // 单声母不行
		{"zh", false},
		{"x", false},
		{"xx", false},
		{"aaa", false},
		{"ang", true},
		{"zhid", false},    // zh + id (id 不是韵母)
	}
	for _, c := range cases {
		got := IsValidSyllable(c.in)
		if got != c.want {
			t.Errorf("IsValidSyllable(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

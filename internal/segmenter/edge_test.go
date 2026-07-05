package segmenter

import (
        "strings"
        "testing"
)

func TestSegmentEmpty(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        words, _, score := s.Segment("")
        if len(words) != 0 {
                t.Errorf("Segment('') got %d words, want 0", len(words))
        }
        if score != 0 {
                t.Errorf("Segment('') score = %v, want 0", score)
        }
}

func TestSegmentWhitespace(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        words, _, _ := s.Segment("   ")
        if len(words) != 0 {
                t.Errorf("Segment('   ') got %d words, want 0", len(words))
        }
}

func TestSegmentSingleSyllable(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        cases := []string{"a", "e", "o", "ni", "hao"}
        for _, c := range cases {
                words, _, _ := s.Segment(c)
                t.Logf("Segment(%q) -> %v", c, words)
        }
}

func TestSegmentNonAlpha(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        cases := []string{
                "123",
                "!@#$",
                "你好",
                "\x00\x01",
                "abc123def",
        }
        for _, c := range cases {
                words, _, _ := s.Segment(c)
                t.Logf("Segment(%q) -> %v", c, words)
                // 不应 panic
        }
}

func TestSegmentVeryLong(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        // 1000 个音节
        input := strings.Repeat("a", 1000)
        words, pinyins, score := s.Segment(input)
        t.Logf("Segment(1000 'a') -> %d words, score=%v", len(words), score)
        // 不应 panic，不应超时
        _ = pinyins
}

func TestSegmentNoValidPinyin(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        // 全是非拼音字母
        cases := []string{
                "xxxxxx",
                "vvvvvv",
                "qqqqqq",
        }
        for _, c := range cases {
                words, _, _ := s.Segment(c)
                t.Logf("Segment(%q) -> %d words: %v", c, len(words), words)
        }
}

func TestSegmentMixedValidInvalid(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        cases := []string{
                "niXXXhao",       // 中间有无效
                "123nihao",
                "nihao123",
                "n i h a o",      // 空格分隔
        }
        for _, c := range cases {
                words, _, _ := s.Segment(c)
                t.Logf("Segment(%q) -> %v", c, words)
        }
}

func TestSegmentAmbiguity(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        // 经典歧义切分
        cases := []struct {
                in   string
                desc string
        }{
                {"xian", "xi'an vs xian"},
                {"fangan", "fang'an vs fan'gan"},
                {"nanbu", "nan'bu vs nanbu"},
                {"zhongguo", "整词 vs zhong+guo"},
                {"tiananmen", "tian'an'men"},
                {"woaixuexi", "wo'ai'xuexi"},
                {"zhongguorenmin", "zhongguo'renmin vs zhong'guo'ren'min"},
                {"beijingshigongjiaojika", "长串"},
        }
        for _, c := range cases {
                words, pinyins, score := s.Segment(c.in)
                t.Logf("%-25q (%s) -> %v (pinyins=%v, score=%.3f)",
                        c.in, c.desc, words, pinyins, score)
        }
}

func TestSegmentWithBigramAmbiguity(t *testing.T) {
        d, _ := loadTestDict()
        bm, err := LoadBigramModel()
        if err != nil {
                t.Skipf("bigram model not loaded: %v", err)
        }
        s, _ := NewWithBigram(d, bm)

        cases := []string{
                "xian",
                "fangan",
                "woaixuexi",
                "zhongguorenmin",
                "tiananmen",
        }
        for _, c := range cases {
                words, _, score := s.Segment(c)
                t.Logf("[bigram] Segment(%q) -> %v (score=%.3f)", c, words, score)
        }
}

func TestSegmentNoPanic(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        // 注意：超长输入 O(n²) 切分可能慢，限制长度
        cases := []string{
                "",
                " ",
                "\x00",
                "\xff",
                strings.Repeat("a", 1000),
                strings.Repeat("x", 500),
                "\n\r\t",
                "!@#$%^&*()_+",
                "你好世界",
        }
        for _, c := range cases {
                _, _, _ = s.Segment(c)
        }
}

func TestSegmentAndCombineEdgeCases(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        // 空
        if r := s.SegmentAndCombine("", 5); len(r) != 0 {
                t.Errorf("SegmentAndCombine('') = %v, want empty", r)
        }
        // 不存在：返回非空切片但每个元素是 nil（切分了但没候选）
        r := s.SegmentAndCombine("zzzzzzzzz", 5)
        t.Logf("SegmentAndCombine(zzzzz) -> %d segments (each may be nil)", len(r))
        // topK = 0：候选数 0 但返回切片
        r = s.SegmentAndCombine("nihao", 0)
        t.Logf("SegmentAndCombine(nihao, 0) -> %d segments", len(r))

        // 正常情况
        r = s.SegmentAndCombine("nihao", 3)
        if len(r) == 0 {
                t.Error("SegmentAndCombine(nihao, 3) should return segments")
        }
}

func TestBigramModelLoadTwice(t *testing.T) {
        // 多次加载不应有问题
        bm1, err1 := LoadBigramModel()
        bm2, err2 := LoadBigramModel()
        if err1 != nil || err2 != nil {
                t.Fatalf("load errors: %v, %v", err1, err2)
        }
        if bm1.Stats() != bm2.Stats() {
                t.Errorf("models differ: %+v vs %+v", bm1.Stats(), bm2.Stats())
        }
}

func TestBigramModelLogProbEdgeCases(t *testing.T) {
        bm, err := LoadBigramModel()
        if err != nil {
                t.Skipf("model not loaded: %v", err)
        }
        // 空
        if lp := bm.LogProb("", ""); lp == 0 {
                t.Error("LogProb('','') should not be 0")
        }
        // OOV
        lp1 := bm.LogProb("zzz", "yyy")  // 都是 OOV
        if lp1 != bm.oovLogProb {
                t.Errorf("OOV LogProb = %v, want %v", lp1, bm.oovLogProb)
        }
        // 已知上下文 + OOV 词
        lp2 := bm.LogProb("<s>", "zzzz") // <s> 已知，zzzz OOV
        if lp2 != bm.oovLogProb {
                t.Errorf("LogProb(<s>, OOV) = %v, want %v", lp2, bm.oovLogProb)
        }
}

func TestBigramSentenceLogProbEmpty(t *testing.T) {
        bm, _ := LoadBigramModel()
        if lp := bm.SentenceLogProb(nil); lp != 0 {
                t.Errorf("SentenceLogProb(nil) = %v, want 0", lp)
        }
        if lp := bm.SentenceLogProb([]string{}); lp != 0 {
                t.Errorf("SentenceLogProb([]) = %v, want 0", lp)
        }
}

func TestBigramSentenceLogProbSingleWord(t *testing.T) {
        bm, _ := LoadBigramModel()
        lp := bm.SentenceLogProb([]string{"你好"})
        if lp == 0 {
                t.Error("SentenceLogProb([你好]) should not be 0")
        }
        t.Logf("SentenceLogProb([你好]) = %.3f", lp)
}

func TestSetBigramWeights(t *testing.T) {
        d, _ := loadTestDict()
        bm, _ := LoadBigramModel()
        s, _ := NewWithBigram(d, bm)

        s.SetBigramWeights(0.5, 0.5)
        // 不应 panic
        s.Segment("nihao")

        s.SetBigramWeights(0.0, 1.0) // 纯 bigram
        s.Segment("nihao")

        s.SetBigramWeights(1.0, 0.0) // 纯词频
        s.Segment("nihao")
}

func TestHasBigram(t *testing.T) {
        d, _ := loadTestDict()
        s1 := New(d)
        if s1.HasBigram() {
                t.Error("New() should not have bigram")
        }
        bm, _ := LoadBigramModel()
        s2, _ := NewWithBigram(d, bm)
        if !s2.HasBigram() {
                t.Error("NewWithBigram() should have bigram")
        }
}

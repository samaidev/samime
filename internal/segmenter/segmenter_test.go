package segmenter

import (
        "strings"
        "testing"

        "github.com/zai/goime/internal/dict"
)

func loadTestDict() (*dict.Dict, error) {
        return dict.LoadEmbedded()
}

func TestSegmentNihao(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        words, pinyins, score := s.Segment("nihao")
        if len(words) == 0 {
                t.Fatal("no segments")
        }
        t.Logf("nihao -> words=%v pinyins=%v score=%.4f", words, pinyins, score)
        // 应该切成一个词 "你好"
        if len(words) != 1 || words[0] != "你好" {
                t.Errorf("expected ['你好'], got %v", words)
        }
}

func TestSegmentWoAiXueXi(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        words, pinyins, score := s.Segment("woaixuexi")
        t.Logf("woaixuexi -> words=%v pinyins=%v score=%.4f", words, pinyins, score)
        combined := strings.Join(words, "")
        if combined == "" {
                t.Errorf("combined word is empty: %v", words)
        }
        // 至少应该切出 2-4 个词
        if len(words) < 2 {
                t.Errorf("expected at least 2 segments, got %d: %v", len(words), words)
        }
}

func TestSegmentZhongGuoRen(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        words, pinyins, score := s.Segment("zhongguoren")
        t.Logf("zhongguoren -> words=%v pinyins=%v score=%.4f", words, pinyins, score)
        // 应该切成 "中国" + "人"
        if len(words) != 2 {
                t.Errorf("expected 2 segments, got %d: %v", len(words), words)
        }
        if words[0] != "中国" {
                t.Errorf("expected '中国', got %q", words[0])
        }
}

func TestSegmentPinyin(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        words, _, _ := s.Segment("pinyin")
        t.Logf("pinyin -> %v", words)
        if len(words) != 1 || words[0] != "拼音" {
                t.Errorf("expected ['拼音'], got %v", words)
        }
}

func TestSegmentLonger(t *testing.T) {
        d, _ := loadTestDict()
        s := New(d)
        cases := []string{
                "shurufa",     // 输入法
                "jiqixuexi",   // 机器学习
                "rengongzhineng", // 人工智能
                "jiandan",     // 简单
                "yixia",       // 一下
                "wome",        // 我么
                "shijie",      // 世界
        }
        for _, c := range cases {
                words, _, score := s.Segment(c)
                combined := strings.Join(words, "")
                t.Logf("%-20s -> %v (combined=%q) score=%.4f", c, words, combined, score)
        }
}

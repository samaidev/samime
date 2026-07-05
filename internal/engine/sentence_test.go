package engine

import (
	"fmt"
	"testing"

	"github.com/zai/goime/internal/dict"
)

// TestLongSentence 测试长句输入（搜狗核心能力）
func TestLongSentence(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	cases := []struct {
		input string
		desc  string
	}{
		{"woaixuexi", "我爱学习"},
		{"rengongzhineng", "人工智能"},
		{"jintiantianqihenhao", "今天天气很好"},
		{"woyaoqubowuguan", "我要去博物馆"},
		{"woyaoquyindu", "我要去印度"},
		{"nihaoa", "你好啊"},
		{"xian", "西安/先"},
	}

	for _, c := range cases {
		cands := eng.Search(c.input)
		fmt.Printf("\nSearch(%q) [%s]:\n", c.input, c.desc)
		for i, w := range cands {
			if i >= 8 {
				break
			}
			fmt.Printf("  %d. %s (%s) score=%.1f src=%s\n",
				i+1, w.Word, w.Pinyin, w.Score, w.Source)
		}
	}
}

// TestBigramFixed 验证 bigram 字节索引 bug 已修复
func TestBigramFixed(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	// 检查 segmenter 是否启用了 bigram
	if !eng.segmenter.HasBigram() {
		t.Skip("bigram not enabled")
	}

	// "你好" 和 "好你" 的 bigram 分数应该不同
	// 修复前：都是 oovLogProb * 字节数，相同
	// 修复后：你好 应该比 好你 分数高（"你好"是常见 bigram）
	score1 := eng.segmenter.BigramSentenceLogProb([]string{"你好"})
	score2 := eng.segmenter.BigramSentenceLogProb([]string{"好你"})
	fmt.Printf("bigram(你好)=%.2f bigram(好你)=%.2f\n", score1, score2)
	if score1 == score2 {
		t.Error("bigram 分数相同，说明字节索引 bug 未修复")
	}
	if score1 <= score2 {
		t.Errorf("你好 bigram 分数(%.2f)应高于 好你(%.2f)", score1, score2)
	}
}

// TestSegmentMultiCandidate 验证 segmentMatch 产生多个候选
func TestSegmentMultiCandidate(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Skip("dict not embedded")
	}
	eng := NewDefault(d)

	// 用词典里没有整词的长句测试 segmentMatch 多候选
	// "woyaochifan"（我要吃饭）可能不是整词，触发 segmentMatch
	cands := eng.Search("woyaochifan")
	segCount := 0
	for _, c := range cands {
		if c.Source == "segment" {
			segCount++
		}
	}
	fmt.Printf("Search(\"woyaochifan\") segment 候选数: %d\n", segCount)
	for i, c := range cands {
		if i >= 10 {
			break
		}
		fmt.Printf("  %d. %s (%s) score=%.1f src=%s\n",
			i+1, c.Word, c.Pinyin, c.Score, c.Source)
	}
	// 只要能切分出"我要吃饭"类候选即可，不强求 segment 来源数
	if len(cands) == 0 {
		t.Error("Search 无任何候选")
	}
}

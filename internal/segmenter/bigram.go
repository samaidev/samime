// Package segmenter 整句切分器
package segmenter

import (
	"bufio"
	"embed"
	"fmt"
	"math"
	"strconv"
	"strings"
)

//go:embed data/bigram.txt
var bigramFS embed.FS

// BigramModel 2-gram 语言模型
type BigramModel struct {
	// P(w2|w1) -> log prob
	bigram map[string]map[string]float64
	// 字符单字频次（用于 OOV 平滑）
	unigram map[string]float64
	// 词表大小
	vocabSize int
	// 平滑因子
	alpha float64
	// OOV 的 log prob
	oovLogProb float64
}

// LoadBigramModel 从内嵌文件加载
func LoadBigramModel() (*BigramModel, error) {
	f, err := bigramFS.Open("data/bigram.txt")
	if err != nil {
		return nil, fmt.Errorf("open bigram.txt: %w", err)
	}
	defer f.Close()

	m := &BigramModel{
		bigram:    make(map[string]map[string]float64),
		unigram:   make(map[string]float64),
		alpha:     0.1,
		oovLogProb: -20,
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			// 解析 header 中的参数
			if strings.Contains(line, "vocab_size=") {
				parts := strings.Fields(line)
				for _, p := range parts {
					if strings.HasPrefix(p, "vocab_size=") {
						m.vocabSize, _ = strconv.Atoi(strings.TrimPrefix(p, "vocab_size="))
					} else if strings.HasPrefix(p, "alpha=") {
						m.alpha, _ = strconv.ParseFloat(strings.TrimPrefix(p, "alpha="), 64)
					} else if strings.HasPrefix(p, "oov_logprob=") {
						m.oovLogProb, _ = strconv.ParseFloat(strings.TrimPrefix(p, "oov_logprob="), 64)
					}
				}
			}
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 3 {
			continue
		}
		w1, w2 := parts[0], parts[1]
		lp, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			continue
		}
		if m.bigram[w1] == nil {
			m.bigram[w1] = make(map[string]float64)
		}
		m.bigram[w1][w2] = lp
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return m, nil
}

// LogProb 计算 P(w2 | w1) 的对数概率
// 如果 (w1, w2) 在模型中，直接返回
// 否则返回 OOV log prob（带平滑）
func (m *BigramModel) LogProb(w1, w2 string) float64 {
	if inner, ok := m.bigram[w1]; ok {
		if lp, ok := inner[w2]; ok {
			return lp
		}
	}
	return m.oovLogProb
}

// SentenceLogProb 计算整句的对数概率（用于排序）
// 句首用 <s>，句尾用 </s>
//
// 注意：必须用 rune 索引而非 byte 索引，因为中文 UTF-8 每字 3 字节，
// 之前用 string(w[0]) 取首字节导致永远命中 OOV，bigram 完全失效。
func (m *BigramModel) SentenceLogProb(words []string) float64 {
	if len(words) == 0 {
		return 0
	}
	var total float64
	prev := "<s>"
	for _, w := range words {
		// 转为 rune 切片以正确处理中文
		chars := []rune(w)
		if len(chars) == 0 {
			continue
		}
		// 第一个字符与前驱
		total += m.LogProb(prev, string(chars[0]))
		// 词内字符对
		for i := 0; i < len(chars)-1; i++ {
			total += m.LogProb(string(chars[i]), string(chars[i+1]))
		}
		prev = string(chars[len(chars)-1])
	}
	total += m.LogProb(prev, "</s>")
	return total
}

// VocabSize 词表大小
func (m *BigramModel) VocabSize() int { return m.vocabSize }

// Stats 统计信息
type BigramStats struct {
	Contexts    int
	Bigrams     int
	VocabSize   int
	OOVLogProb  float64
}

func (m *BigramModel) Stats() BigramStats {
	var total int
	for _, inner := range m.bigram {
		total += len(inner)
	}
	return BigramStats{
		Contexts:   len(m.bigram),
		Bigrams:    total,
		VocabSize:  m.vocabSize,
		OOVLogProb: m.oovLogProb,
	}
}

// 防止 math 未使用
var _ = math.Pi

// Package segmenter 整句切分器
// 给定一串连续拼音，找出最优的"词组合"
// 例如 "woaixuexi" -> ["我","爱","学习"] 或 ["我爱","学习"]
//
// 算法：动态规划
// dp[i] = 从位置 i 到末尾的最优切分（最大对数概率）
// 对每个 i，枚举所有可能的词长 (1..maxWordLen)，
//   dp[i] = max(dp[i+l] + log(P(word|pinyin[i:i+l])))
// 其中 P 由词典词频决定，平滑处理 OOV
package segmenter

import (
	"math"
	"strings"

	"github.com/zai/goime/internal/dict"
	"github.com/zai/goime/internal/pinyin"
)

// Segmenter 整句切分器
type Segmenter struct {
	dict       *dict.Dict
	maxWordLen int // 最大词长（音节数，默认 8）

	unknownPenalty float64 // OOV 词的惩罚（log-prob）
	logTotal       float64 // log(总词频) 用于归一化
}

// New 创建切分器
func New(d *dict.Dict) *Segmenter {
	stats := d.Stats()
	logTotal := math.Log(stats.TotalFreq + 1)
	return &Segmenter{
		dict:           d,
		maxWordLen:     8,
		unknownPenalty: -20.0,
		logTotal:       logTotal,
	}
}

// Segment 切分整句
// 返回：词序列 + 对应拼音序列 + 总分
func (s *Segmenter) Segment(input string) ([]string, []string, float64) {
	input = strings.ToLower(strings.TrimSpace(input))
	if len(input) == 0 {
		return nil, nil, 0
	}

	n := len(input)
	dp := make([]float64, n+1)
	backtrack := make([]int, n+1)
	backtrackPy := make([]string, n+1)
	backtrackWord := make([]string, n+1)

	dp[n] = 0
	for i := n - 1; i >= 0; i-- {
		dp[i] = math.Inf(-1)
		maxLen := s.maxWordLen * 6
		if i+maxLen > n {
			maxLen = n - i
		}
		for l := 1; l <= maxLen; l++ {
			j := i + l
			if dp[j] == math.Inf(-1) {
				continue
			}
			substr := input[i:j]
			syls := pinyin.Segment(substr)
			if len(syls) == 0 {
				continue
			}
			joinedPy := pinyin.Join(syls)
			entries := s.dict.Lookup(joinedPy)
			var logProb float64
			var word string
			if len(entries) > 0 {
				best := entries[0]
				logProb = math.Log(best.Freq+1) - s.logTotal
				word = best.Word
			} else {
				if len(syls) == 1 {
					logProb = s.unknownPenalty
					word = ""
				} else {
					continue
				}
			}
			score := logProb + dp[j]
			if score > dp[i] {
				dp[i] = score
				backtrack[i] = j
				backtrackPy[i] = joinedPy
				backtrackWord[i] = word
			}
		}
		if dp[i] == math.Inf(-1) {
			dp[i] = s.unknownPenalty + dp[i+1]
			backtrack[i] = i + 1
			backtrackPy[i] = string(input[i])
			backtrackWord[i] = ""
		}
	}

	var words, pinyins []string
	for i := 0; i < n; {
		j := backtrack[i]
		words = append(words, backtrackWord[i])
		pinyins = append(pinyins, backtrackPy[i])
		i = j
	}
	return words, pinyins, dp[0]
}

// SegmentAndCombine 切分后组合候选
func (s *Segmenter) SegmentAndCombine(input string, topK int) [][]dict.Entry {
	words, pinyins, _ := s.Segment(input)
	if len(words) == 0 {
		return nil
	}
	result := make([][]dict.Entry, len(pinyins))
	for i, py := range pinyins {
		entries := s.dict.Lookup(py)
		if len(entries) == 0 {
			result[i] = nil
			continue
		}
		if len(entries) > topK {
			entries = entries[:topK]
		}
		cp := make([]dict.Entry, len(entries))
		for k, e := range entries {
			cp[k] = *e
		}
		result[i] = cp
	}
	return result
}

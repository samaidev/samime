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
        "sort"
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

        // 2-gram 语言模型（可选，启用后切分质量更高）
        bigram       *BigramModel
        useBigram    bool
        // 词频与 2-gram 的混合权重
        wFreq   float64 // 词频权重
        wBigram float64 // 2-gram 权重
}

// New 创建切分器（仅基于词频）
func New(d *dict.Dict) *Segmenter {
        stats := d.Stats()
        logTotal := math.Log(stats.TotalFreq + 1)
        return &Segmenter{
                dict:           d,
                maxWordLen:     8,
                unknownPenalty: -20.0,
                logTotal:       logTotal,
                wFreq:          1.0,
                wBigram:        0.0,
        }
}

// NewWithBigram 创建带 2-gram 的切分器
func NewWithBigram(d *dict.Dict, bm *BigramModel) (*Segmenter, error) {
        s := New(d)
        s.bigram = bm
        s.useBigram = bm != nil
        // 默认权重：词频 0.4, 2-gram 0.6（2-gram 更能解决歧义）
        s.wFreq = 0.4
        s.wBigram = 0.6
        return s, nil
}

// SetBigramWeights 调整权重
func (s *Segmenter) SetBigramWeights(wFreq, wBigram float64) {
        s.wFreq = wFreq
        s.wBigram = wBigram
}

// HasBigram 是否启用了 2-gram
func (s *Segmenter) HasBigram() bool { return s.useBigram }

// BigramSentenceLogProb 计算词序列的 bigram 对数概率（导出版，供 engine 调用）
func (s *Segmenter) BigramSentenceLogProb(words []string) float64 {
	if !s.useBigram || s.bigram == nil {
		return -10
	}
	return s.bigram.SentenceLogProb(words)
}

// BigramTopNext 返回前驱词的 Top-K 续接候选（导出版，供 engine 调用）
// 用于续接联想：基于上一次提交的末字预测下一个可能的字
func (s *Segmenter) BigramTopNext(prev string, topK int) []NextEntry {
	if !s.useBigram || s.bigram == nil {
		return nil
	}
	return s.bigram.TopNext(prev, topK)
}

// Segment 切分整句
// 返回：词序列 + 对应拼音序列 + 总分
//
// 算法：动态规划
//   dp[i] = max over j>i of (word_score(i,j) + dp[j])
//
// word_score 由两部分组成（如果启用了 bigram）：
//   1. 词频对数概率: log(freq) - log(total)
//   2. 2-gram 上下文概率: 句首 <s> + 词内 + 词间
//
// 如果未启用 bigram，仅用词频
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
                        score := logProb*s.wFreq + dp[j]
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

        // 回溯
        var words, pinyins []string
        for i := 0; i < n; {
                j := backtrack[i]
                words = append(words, backtrackWord[i])
                pinyins = append(pinyins, backtrackPy[i])
                i = j
        }

        // 如果启用了 2-gram，对结果做一次重排：枚举切分位置的几种变体，用 2-gram 选最优
        if s.useBigram && s.bigram != nil {
                words, pinyins, dp[0] = s.rerankWithBigram(input, words, pinyins)
        }

        return words, pinyins, dp[0]
}

// rerankWithBigram 用 2-gram 模型对当前切分结果重排
// 简化策略：对每个切分边界，尝试合并相邻两个词或拆分一个词，看 2-gram 是否更优
func (s *Segmenter) rerankWithBigram(input string, words, pinyins []string) ([]string, []string, float64) {
        if len(words) <= 1 {
                // 单词，计算 2-gram 分数
                score := s.bigram.SentenceLogProb(words)
                return words, pinyins, score
        }

        // 当前方案的 2-gram 分数
        bestWords := words
        bestPinyins := pinyins
        bestScore := s.bigram.SentenceLogProb(words)

        // 尝试合并相邻两个词（如果合并后仍在词典中）
        for i := 0; i < len(bestWords)-1; i++ {
                if bestWords[i] == "" || bestWords[i+1] == "" {
                        continue
                }
                combinedPy := bestPinyins[i] + bestPinyins[i+1]
                entries := s.dict.Lookup(combinedPy)
                if len(entries) == 0 {
                        continue
                }
                // 试合并
                newWords := make([]string, 0, len(bestWords)-1)
                newPinyins := make([]string, 0, len(bestWords)-1)
                newWords = append(newWords, bestWords[:i]...)
                newPinyins = append(newPinyins, bestPinyins[:i]...)
                newWords = append(newWords, entries[0].Word)
                newPinyins = append(newPinyins, combinedPy)
                newWords = append(newWords, bestWords[i+2:]...)
                newPinyins = append(newPinyins, bestPinyins[i+2:]...)
                score := s.bigram.SentenceLogProb(newWords) * s.wBigram
                // 加上词频得分（合并后通常是更长的词，词频得分也加进来）
                freqScore := (math.Log(entries[0].Freq+1) - s.logTotal) * s.wFreq
                totalScore := score + freqScore
                if totalScore > bestScore {
                        bestWords = newWords
                        bestPinyins = newPinyins
                        bestScore = totalScore
                }
        }

        return bestWords, bestPinyins, bestScore
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


// SplitPath 切分路径（用于 N-best）
type SplitPath struct {
        Words   []string
        Pinyins []string
        Score   float64
}

// SegmentNBest 返回 Top-N 切分路径
// 算法：DP 表每位置存 Top-K 候选回溯点，回溯时枚举路径组合
// 用于整句重排：多种切分路径参与竞争，bigram 选最优
func (s *Segmenter) SegmentNBest(input string, n int) []SplitPath {
        input = strings.ToLower(strings.TrimSpace(input))
        if len(input) == 0 || n <= 0 {
                return nil
        }

        type cand struct {
                endPos int
                py     string
                word   string
                score  float64
        }
        // dp[i] = 从位置 i 出发的 Top-K 候选列表
        const k = 5 // 每位置保留 Top-5 候选（v4: 从 3 提到 5，枚举更多切分）
        dp := make([][]cand, len(input)+1)
        dp[len(input)] = []cand{{endPos: len(input), py: "", word: "", score: 0}}

        for i := len(input) - 1; i >= 0; i-- {
                var cands []cand
                maxLen := s.maxWordLen * 6
                if i+maxLen > len(input) {
                        maxLen = len(input) - i
                }
                for l := 1; l <= maxLen; l++ {
                        j := i + l
                        if len(dp[j]) == 0 {
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
                        // 对每个 j 的 Top-K 后继，生成本位置的候选
                        for _, next := range dp[j] {
                                score := logProb*s.wFreq + next.score
                                cands = append(cands, cand{endPos: j, py: joinedPy, word: word, score: score})
                        }
                }
                if len(cands) == 0 {
                        // OOV 单字 fallback
                        if i+1 <= len(input) && len(dp[i+1]) > 0 {
                                next := dp[i+1][0]
                                dp[i] = []cand{{endPos: i + 1, py: string(input[i]), word: "", score: s.unknownPenalty + next.score}}
                        }
                        continue
                }
                // 按 score 降序保留 Top-K
                sort.Slice(cands, func(a, b int) bool { return cands[a].score > cands[b].score })
                if len(cands) > k {
                        cands = cands[:k]
                }
                dp[i] = cands
        }

        // 回溯：从 dp[0] 出发，DFS 枚举路径，收集 Top-N
        var paths []SplitPath
        var backtrack func(pos int, words, pinyins []string, score float64)
        backtrack = func(pos int, words, pinyins []string, score float64) {
                if pos >= len(input) {
                        // 到达末尾，记录路径（words/pinyins 已是正序，无需反转）
                        cpWords := make([]string, len(words))
                        cpPys := make([]string, len(pinyins))
                        copy(cpWords, words)
                        copy(cpPys, pinyins)
                        paths = append(paths, SplitPath{Words: cpWords, Pinyins: cpPys, Score: score})
                        return
                }
                for _, c := range dp[pos] {
                        // copy 避免共享底层数组
                        newWords := make([]string, len(words)+1)
                        copy(newWords, words)
                        newWords[len(words)] = c.word
                        newPys := make([]string, len(pinyins)+1)
                        copy(newPys, pinyins)
                        newPys[len(pinyins)] = c.py
                        backtrack(c.endPos, newWords, newPys, score+c.score)
                        if len(paths) >= n*4 {
                                return // 收集足够多就停止
                        }
                }
        }
        backtrack(0, nil, nil, 0)

        // 按 score 降序排序
        sort.Slice(paths, func(a, b int) bool { return paths[a].Score > paths[b].Score })
        if len(paths) > n {
                paths = paths[:n]
        }
        return paths
}

// SegmentAndCombineNBest 基于 N-best 切分的候选组合
// 对每条切分路径，每段取 Top-K 候选词，用 bigram 全句评分选 Top-N
func (s *Segmenter) SegmentAndCombineNBest(input string, topK, nSplits, nResults int) []SplitPath {
        paths := s.SegmentNBest(input, nSplits)
        if len(paths) == 0 {
                return nil
        }

        type result struct {
                words []string
                pys   []string
                score float64
        }
        var allResults []result

        for _, path := range paths {
                // 对这条切分路径，每段取 Top-K 候选，beam search 组合
                beam := []struct {
                        words []string
                        pys   []string
                        score float64
                }{{words: nil, pys: nil, score: 0}}

                for segIdx, py := range path.Pinyins {
                        entries := s.dict.Lookup(py)
                        if len(entries) == 0 {
                                // OOV，用空词
                                for bi := range beam {
                                        beam[bi].words = append(beam[bi].words, "")
                                        beam[bi].pys = append(beam[bi].pys, py)
                                }
                                continue
                        }
                        if len(entries) > topK {
                                entries = entries[:topK]
                        }
                        var newBeam []struct {
                                words []string
                                pys   []string
                                score float64
                        }
                        for _, bi := range beam {
                                for _, ent := range entries {
                                        newWords := append(append([]string(nil), bi.words...), ent.Word)
                                        newPys := append(append([]string(nil), bi.pys...), ent.Pinyin)
                                        // bigram 连接分
                                        var incScore float64
                                        if len(bi.words) == 0 {
                                                incScore = s.bigram.SentenceLogProb([]string{ent.Word})
                                        } else {
                                                prev := bi.words[len(bi.words)-1]
                                                // 前词末字 + 新词首字 + 新词内部
                                                scoreBoth := s.bigram.SentenceLogProb([]string{prev, ent.Word})
                                                scorePrev := s.bigram.SentenceLogProb([]string{prev})
                                                incScore = scoreBoth - scorePrev
                                        }
                                        // 加词频
                                        freqScore := (math.Log(ent.Freq+1) - s.logTotal) * s.wFreq
                                        newBeam = append(newBeam, struct {
                                                words []string
                                                pys   []string
                                                score float64
                                        }{newWords, newPys, bi.score + incScore*s.wBigram + freqScore})
                                }
                        }
                        // Beam 剪枝
                        if len(newBeam) > 12 {
                                sort.Slice(newBeam, func(a, b int) bool { return newBeam[a].score > newBeam[b].score })
                                newBeam = newBeam[:12]
                        }
                        beam = newBeam
                        _ = segIdx
                }
                for _, bi := range beam {
                        if len(bi.words) == 0 {
                                continue
                        }
                        allResults = append(allResults, result{bi.words, bi.pys, bi.score})
                }
        }

        // 全局排序取 Top-N
        sort.Slice(allResults, func(a, b int) bool { return allResults[a].score > allResults[b].score })
        if len(allResults) > nResults {
                allResults = allResults[:nResults]
        }

        out := make([]SplitPath, len(allResults))
        for i, r := range allResults {
                out[i] = SplitPath{
                        Words:   r.words,
                        Pinyins: r.pys,
                        Score:   r.score,
                }
        }
        return out
}


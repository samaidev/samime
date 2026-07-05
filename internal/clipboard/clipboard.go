// Package clipboard 剪切板历史记录
// 保存用户最近提交的 50 条文本，提供查询/恢复 API
package clipboard

import (
	"sync"
	"time"
)

// HistoryEntry 历史条目
type HistoryEntry struct {
	Word   string    // 提交的文本
	Pinyin string    // 对应拼音
	Time   time.Time // 提交时间
	Source string    // 来源（engine.Commit 的候选 source）
}

// History 剪切板历史
type History struct {
	mu      sync.RWMutex
	entries []HistoryEntry
	maxSize int
}

// New 创建历史记录器
func New(maxSize int) *History {
	if maxSize <= 0 {
		maxSize = 50
	}
	return &History{
		entries: make([]HistoryEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// Add 添加一条记录
func (h *History) Add(word, pinyin, source string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.entries = append(h.entries, HistoryEntry{
		Word:   word,
		Pinyin: pinyin,
		Time:   time.Now(),
		Source: source,
	})

	// 超出上限则移除最旧的
	if len(h.entries) > h.maxSize {
		h.entries = h.entries[len(h.entries)-h.maxSize:]
	}
}

// All 返回所有记录（最新的在前）
func (h *History) All() []HistoryEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]HistoryEntry, len(h.entries))
	copy(out, h.entries)
	// 反转：最新的在前
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Recent 返回最近 N 条
func (h *History) Recent(n int) []HistoryEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if n <= 0 || n > len(h.entries) {
		n = len(h.entries)
	}
	// 最新 N 条，按时间倒序
	start := len(h.entries) - n
	out := make([]HistoryEntry, n)
	copy(out, h.entries[start:])
	// 反转
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// Get 获取第 idx 条（0 = 最新）
func (h *History) Get(idx int) (HistoryEntry, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if idx < 0 || idx >= len(h.entries) {
		return HistoryEntry{}, false
	}
	// 转换为正序索引
	realIdx := len(h.entries) - 1 - idx
	if realIdx < 0 {
		return HistoryEntry{}, false
	}
	return h.entries[realIdx], true
}

// Search 搜索历史（按词或拼音包含子串）
func (h *History) Search(query string) []HistoryEntry {
	h.mu.RLock()
	defer h.mu.RUnlock()
	var out []HistoryEntry
	for i := len(h.entries) - 1; i >= 0; i-- {
		e := h.entries[i]
		if contains(e.Word, query) || contains(e.Pinyin, query) {
			out = append(out, e)
		}
	}
	return out
}

// Clear 清空历史
func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries = h.entries[:0]
}

// Size 当前记录数
func (h *History) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.entries)
}

// Contains 是否包含某词
func (h *History) Contains(word string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, e := range h.entries {
		if e.Word == word {
			return true
		}
	}
	return false
}

// contains 字符串包含
func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

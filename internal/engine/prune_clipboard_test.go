package engine

import (
	"testing"
	"time"
)

// === N-gram 自动剪枝测试 ===

func TestPruneContextPairsLowFreq(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.pruneThreshold = 2.0 // 频次 < 2 的剪枝
	e.mu.Unlock()

	// 建立 3 条上下文，频次不同
	e.mu.Lock()
	e.contextPairs["a|b"] = 1  // 低频，应被剪枝
	e.contextPairs["c|d"] = 5  // 高频，保留
	e.contextPairs["e|f"] = 10 // 高频，保留
	e.mu.Unlock()

	e.mu.Lock()
	e.pruneContextPairs(time.Now())
	freqAB := e.contextPairs["a|b"]
	freqCD := e.contextPairs["c|d"]
	freqEF := e.contextPairs["e|f"]
	e.mu.Unlock()

	if freqAB != 0 {
		t.Errorf("low-freq 'a|b' should be pruned, got %v", freqAB)
	}
	if freqCD == 0 {
		t.Error("high-freq 'c|d' should be kept")
	}
	if freqEF == 0 {
		t.Error("high-freq 'e|f' should be kept")
	}
}

func TestPruneContextPairsMaxSize(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.maxContextPairs = 5
	e.pruneThreshold = 0 // 不按频次剪枝，只按数量
	e.mu.Unlock()

	// 建立 10 条上下文
	e.mu.Lock()
	for i := 0; i < 10; i++ {
		// 频次递增，让 Top-5 是后 5 条
		e.contextPairs["w"+string(rune('a'+i))+"|x"] = float64(i + 1)
	}
	e.mu.Unlock()

	e.mu.Lock()
	e.pruneContextPairs(time.Now())
	size := len(e.contextPairs)
	e.mu.Unlock()

	if size != 5 {
		t.Errorf("after prune, size = %d, want 5", size)
	}

	// 验证保留的是频次最高的 5 条
	e.mu.RLock()
	for i := 0; i < 5; i++ {
		key := "w" + string(rune('a'+i)) + "|x"
		if _, ok := e.contextPairs[key]; ok {
			t.Errorf("low-freq %q should be pruned", key)
		}
	}
	for i := 5; i < 10; i++ {
		key := "w" + string(rune('a'+i)) + "|x"
		if _, ok := e.contextPairs[key]; !ok {
			t.Errorf("high-freq %q should be kept", key)
		}
	}
	e.mu.RUnlock()
}

func TestPruneTriggerOnExceedMax(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.maxContextPairs = 3
	e.pruneThreshold = 0
	e.mu.Unlock()

	// 通过 Commit 触发剪枝
	for i := 0; i < 5; i++ {
		e.Commit("词"+string(rune('a'+i)), "ci"+string(rune('a'+i)))
	}

	e.mu.RLock()
	size := len(e.contextPairs)
	e.mu.RUnlock()
	if size > 3 {
		t.Errorf("contextPairs should be pruned to <= 3, got %d", size)
	}
}

// === 剪切板历史测试 ===

func TestClipboardHistoryOnCommit(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)

	// Commit 后应该记录到剪切板
	e.Commit("你好", "nihao")
	e.Commit("世界", "shijie")

	clip := e.Clipboard()
	if clip == nil {
		t.Fatal("Clipboard() should not be nil")
	}
	if clip.Size() != 2 {
		t.Errorf("clipboard size = %d, want 2", clip.Size())
	}

	// 最新的应该是 "世界"
	all := clip.All()
	if all[0].Word != "世界" {
		t.Errorf("newest = %s, want 世界", all[0].Word)
	}
}

func TestClipboardHistoryMax50(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)

	// Commit 60 次
	for i := 0; i < 60; i++ {
		e.Commit("词"+string(rune('a'+i%26))+string(rune('0'+i/26)), "ci")
	}

	clip := e.Clipboard()
	if clip.Size() != 50 {
		t.Errorf("clipboard size = %d, want 50 (max)", clip.Size())
	}
}

func TestClipboardHistoryClear(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.Commit("你好", "nihao")
	e.Commit("世界", "shijie")

	clip := e.Clipboard()
	clip.Clear()

	if clip.Size() != 0 {
		t.Errorf("after Clear, size = %d, want 0", clip.Size())
	}
}

func TestClipboardHistorySearch(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.Commit("你好", "nihao")
	e.Commit("世界", "shijie")
	e.Commit("你好啊", "nihaoa")

	clip := e.Clipboard()
	results := clip.Search("你好")
	if len(results) != 2 {
		t.Errorf("Search('你好') = %d, want 2", len(results))
	}
}

// === 综合：剪枝 + 剪切板 ===

func TestPruneDoesNotAffectClipboard(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.maxContextPairs = 3
	e.pruneThreshold = 0
	e.mu.Unlock()

	// 提交 10 次，剪切板应该有 10 条
	for i := 0; i < 10; i++ {
		e.Commit("词"+string(rune('a'+i)), "ci")
	}

	// contextPairs 应该被剪枝
	e.mu.RLock()
	ctxSize := len(e.contextPairs)
	e.mu.RUnlock()
	if ctxSize > 3 {
		t.Errorf("contextPairs should be pruned, got %d", ctxSize)
	}

	// 剪切板不应该被剪枝（独立的）
	clip := e.Clipboard()
	if clip.Size() != 10 {
		t.Errorf("clipboard should have 10 entries, got %d", clip.Size())
	}
}

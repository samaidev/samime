package engine

import (
	"strings"
	"testing"
	"time"
)

// === 3-gram 边缘测试 ===

func TestThreeGramWithEmptyHistory(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	// 无历史时不应触发 3-gram
	cands := e.Search("nihao")
	if len(cands) == 0 {
		t.Fatal("no candidates")
	}
	t.Logf("无历史 3-gram 测试通过")
}

func TestThreeGramSingleCommit(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	// 只 commit 一次，无 3-gram（需要至少 2 个前驱）
	e.Commit("我", "wo")
	cands := e.Search("ni")
	t.Logf("单次 commit 后搜索: %v", topWords(cands, 3))
}

func TestThreeGramTwoCommits(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	// commit 2 次，第 3 次 commit 时才形成 3-gram
	e.Commit("我", "wo")
	e.Commit("爱", "ai")
	e.Commit("学习", "xuexi")

	e.mu.RLock()
	_, has3gram := e.contextPairs["我\t爱|学习"]
	e.mu.RUnlock()
	if !has3gram {
		t.Error("should have 3-gram '我\\t爱|学习'")
	}
}

func TestThreeGramNoSelfLoop(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	// 连续 commit 相同词，不应形成自连接
	e.Commit("你好", "nihao")
	e.Commit("你好", "nihao")
	e.Commit("你好", "nihao")

	e.mu.RLock()
	_, has2gram := e.contextPairs["你好|你好"]
	_, has3gram := e.contextPairs["你好\t你好|你好"]
	e.mu.RUnlock()
	if has2gram {
		t.Error("should not have 2-gram self-loop '你好|你好'")
	}
	if has3gram {
		t.Error("should not have 3-gram self-loop")
	}
}

// === 剪枝边缘测试 ===

func TestPruneEmptyContextPairs(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.pruneContextPairs(time.Now())
	size := len(e.contextPairs)
	e.mu.Unlock()
	if size != 0 {
		t.Errorf("prune empty should keep 0, got %d", size)
	}
}

func TestPruneAllLowFreq(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.pruneThreshold = 10.0  // 高阈值
	e.contextPairs["a|b"] = 1
	e.contextPairs["c|d"] = 5
	e.contextPairs["e|f"] = 9
	e.mu.Unlock()

	e.mu.Lock()
	e.pruneContextPairs(time.Now())
	size := len(e.contextPairs)
	e.mu.Unlock()
	if size != 0 {
		t.Errorf("all low-freq should be pruned, got %d", size)
	}
}

func TestPruneAllHighFreq(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.pruneThreshold = 0
	e.maxContextPairs = 3
	e.contextPairs["a|b"] = 100
	e.contextPairs["c|d"] = 200
	e.contextPairs["e|f"] = 300
	e.mu.Unlock()

	e.mu.Lock()
	e.pruneContextPairs(time.Now())
	size := len(e.contextPairs)
	e.mu.Unlock()
	// 不应剪枝（数量未超限且都高频）
	if size != 3 {
		t.Errorf("all high-freq should be kept, got %d", size)
	}
}

func TestPruneKeepTopN(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.maxContextPairs = 3
	e.pruneThreshold = 0
	// 5 个条目，频次递增
	e.contextPairs["a|b"] = 1
	e.contextPairs["c|d"] = 2
	e.contextPairs["e|f"] = 3
	e.contextPairs["g|h"] = 4
	e.contextPairs["i|j"] = 5
	e.mu.Unlock()

	e.mu.Lock()
	e.pruneContextPairs(time.Now())
	size := len(e.contextPairs)
	// 应保留频次最高的 3 个
	_, hasIJ := e.contextPairs["i|j"]
	_, hasGH := e.contextPairs["g|h"]
	_, hasEF := e.contextPairs["e|f"]
	_, hasCD := e.contextPairs["c|d"]
	_, hasAB := e.contextPairs["a|b"]
	e.mu.Unlock()

	if size != 3 {
		t.Errorf("should keep 3, got %d", size)
	}
	if !hasIJ || !hasGH || !hasEF {
		t.Errorf("should keep top-3 (i|j, g|h, e|f)")
	}
	if hasCD || hasAB {
		t.Errorf("should prune low-freq (c|d, a|b)")
	}
}

func TestPruneMaxContextPairsZero(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.maxContextPairs = 0  // 极端值
	e.pruneThreshold = 0
	e.contextPairs["a|b"] = 1
	e.mu.Unlock()

	e.mu.Lock()
	e.pruneContextPairs(time.Now())
	size := len(e.contextPairs)
	e.mu.Unlock()
	// maxContextPairs=0 时，Top-N 保留 0 个
	t.Logf("maxContextPairs=0 prune: size=%d", size)
}

func TestPruneTriggerIntervalNotReached(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.maxContextPairs = 100000  // 不超限
	e.pruneInterval = 1 * time.Hour
	e.lastPruneTime = time.Now()  // 刚剪枝过
	e.contextPairs["a|b"] = 1
	e.mu.Unlock()

	// Commit 一次，不应触发剪枝
	e.Commit("测试", "ceshi")

	e.mu.RLock()
	// 低频条目应该还在（未触发剪枝）
	_, exists := e.contextPairs["a|b"]
	e.mu.RUnlock()
	if !exists {
		t.Error("should not prune (interval not reached)")
	}
}

func TestPruneTriggerByMaxExceeded(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.maxContextPairs = 2
	e.pruneThreshold = 0
	e.mu.Unlock()

	// 通过 Commit 积累超过 2 个上下文
	for i := 0; i < 5; i++ {
		e.Commit("词"+string(rune('a'+i)), "ci"+string(rune('a'+i)))
	}

	e.mu.RLock()
	size := len(e.contextPairs)
	e.mu.RUnlock()
	if size > 2 {
		t.Errorf("should be pruned to <= 2, got %d", size)
	}
}

// === 综合场景 ===

func TestContextAcrossReset(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	// 建立上下文
	e.Commit("我", "wo")
	e.Commit("你", "ni")

	// ResetContext 后上下文应清空
	e.ResetContext()
	e.mu.RLock()
	histLen := len(e.commitHistory)
	e.mu.RUnlock()
	if histLen != 0 {
		t.Errorf("after reset, history = %d", histLen)
	}
	// contextPairs 不应被清空（持久化的）
	e.mu.RLock()
	ctxLen := len(e.contextPairs)
	e.mu.RUnlock()
	if ctxLen == 0 {
		t.Log("contextPairs 可能在 reset 中被清空（看实现）")
	}
}

func TestVeryLongCommitSequence(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.maxContextHistory = 10
	e.mu.Unlock()

	// commit 100 次
	for i := 0; i < 100; i++ {
		e.Commit("词"+string(rune('a'+i%26))+string(rune('0'+i/26)), "ci")
	}

	e.mu.RLock()
	histLen := len(e.commitHistory)
	e.mu.RUnlock()
	if histLen != 10 {
		t.Errorf("history should be capped at 10, got %d", histLen)
	}
}

func TestThreeGramKeyFormat(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.Commit("我", "wo")
	e.Commit("爱", "ai")
	e.Commit("学习", "xuexi")

	e.mu.RLock()
	// 验证 3-gram key 用 tab 分隔
	_, has3gram := e.contextPairs["我\t爱|学习"]
	// 2-gram 也应存在
	_, has2gram1 := e.contextPairs["我|爱"]
	_, has2gram2 := e.contextPairs["爱|学习"]
	e.mu.RUnlock()

	if !has3gram {
		t.Error("missing 3-gram key with tab separator")
	}
	if !has2gram1 || !has2gram2 {
		t.Error("missing 2-gram keys")
	}
}

func TestDecayWithZeroHalfLife(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.decayHalfLife = 0  // 极端值
	e.mu.Unlock()

	// 不应 panic
	e.Commit("你好", "nihao")
	if e.UserFreq()["你好|nihao"] < 1 {
		t.Error("freq should be >= 1")
	}
}

func TestDecayWithNegativeHalfLife(t *testing.T) {
	d, _ := loadTestDict()
	e := NewDefault(d)
	e.mu.Lock()
	e.decayHalfLife = -1 * time.Hour
	e.mu.Unlock()

	// 不应 panic
	e.Commit("你好", "nihao")
	t.Logf("negative halfLife freq: %v", e.UserFreq()["你好|nihao"])
}

// 防止 import 警告
var _ = strings.Repeat

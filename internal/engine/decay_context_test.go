package engine

import (
        "testing"
        "time"
)

// === 时间衰减测试 ===

func TestTimeDecayBasic(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        // 提交 "你好" 5 次
        for i := 0; i < 5; i++ {
                e.Commit("你好", "nihao")
        }
        uf := e.UserFreq()
        if uf["你好|nihao"] < 1 {
                t.Errorf("after 5 commits, freq should be >= 1, got %v", uf["你好|nihao"])
        }
        t.Logf("5 次 commit 后频次: %v", uf["你好|nihao"])
}

func TestTimeDecayAfterWait(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        // 缩短半衰期方便测试
        e.mu.Lock()
        e.decayHalfLife = 100 * time.Millisecond
        e.mu.Unlock()

        // 提交 10 次
        for i := 0; i < 10; i++ {
                e.Commit("你好", "nihao")
        }
        freq1 := e.UserFreq()["你好|nihao"]
        t.Logf("10 次 commit 后频次: %v", freq1)

        // 等待 500ms（5 个半衰期）
        time.Sleep(500 * time.Millisecond)

        // 再提交一次，频次应该比之前小很多（衰减后 +1）
        e.Commit("你好", "nihao")
        freq2 := e.UserFreq()["你好|nihao"]
        t.Logf("衰减 500ms 后再 commit 1 次频次: %v", freq2)

        // 衰减后 +1 应该远小于 freq1
        if freq2 >= freq1 {
                t.Errorf("after decay, freq should be much smaller: got %v, before %v", freq2, freq1)
        }
}

func TestTimeDecayRecentHigher(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        e.mu.Lock()
        e.decayHalfLife = 50 * time.Millisecond
        e.mu.Unlock()

        // 词 A：很久前提交 10 次（最后一次后开始衰减）
        for i := 0; i < 10; i++ {
                e.Commit("你好", "nihao")
        }
        // 不等待，记录 A 的当前频次
        freqA_immediate := e.UserFreq()["你好|nihao"]

        // 等待 4 个半衰期，A 的频次应该衰减
        time.Sleep(200 * time.Millisecond)
        freqA_after := e.UserFreq()["你好|nihao"]
        t.Logf("A 立即: %v, A 衰减后(未再提交): %v", freqA_immediate, freqA_after)

        // 由于 userFreq 存的是 commit 时的计算结果，不会自动衰减
        // 验证：再次 commit 时会触发衰减
        e.Commit("你好", "nihao")
        freqA_recommit := e.UserFreq()["你好|nihao"]
        t.Logf("A 衰减后再 commit 1 次: %v", freqA_recommit)

        // 验证衰减生效：recommit 后的频次应该远小于 immediate
        if freqA_recommit >= freqA_immediate {
                t.Errorf("after decay + 1 commit, freq should be smaller: immediate=%v, recommit=%v",
                        freqA_immediate, freqA_recommit)
        }

        // 词 B：刚刚提交 3 次（无衰减）
        for i := 0; i < 3; i++ {
                e.Commit("世界", "shijie")
        }
        freqB := e.UserFreq()["世界|shijie"]
        t.Logf("B (新, 3次无衰减): %v", freqB)

        // B（新鲜 3 次）应该比 A（衰减后 +1）频次高
        if freqB < freqA_recommit {
                t.Errorf("recent B should have higher freq than decayed A: B=%v, A_recommit=%v",
                        freqB, freqA_recommit)
        }
}

// === 上下文联想测试 ===

func TestContextAssociationBasic(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        // 模拟用户多次输入 "我你" 的组合
        for i := 0; i < 5; i++ {
                e.Commit("我", "wo")
                e.Commit("你", "ni")
        }

        // 现在提交 "我"，然后搜索 "n"，"你" 应该排第一或前几
        e.Commit("我", "wo")
        cands := e.Search("n")
        t.Logf("提交'我'后搜索'n' -> top 5:")
        for i, c := range cands {
                if i >= 5 {
                        break
                }
                t.Logf("  %d. %s (score=%.2f, src=%s)", i+1, c.Word, c.Score, c.Source)
        }

        // 验证 "你" 在前 3
        if len(cands) == 0 {
                t.Fatal("no candidates")
        }
        inTop3 := false
        for i, c := range cands {
                if i >= 3 {
                        break
                }
                if c.Word == "你" {
                        inTop3 = true
                        break
                }
        }
        if !inTop3 {
                t.Errorf("'你' should be in top 3 after committing '我', got: %v",
                        topWords(cands, 5))
        }
}

func TestContextAssociationNotTriggered(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        // 没有任何上下文历史，搜索 "n"
        cands := e.Search("n")
        if len(cands) == 0 {
                t.Fatal("no candidates")
        }
        t.Logf("无上下文搜索'n' -> top 3: %v", topWords(cands, 3))
}

func TestContextAssociationWithReset(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        // 建立上下文
        for i := 0; i < 5; i++ {
                e.Commit("我", "wo")
                e.Commit("你", "ni")
        }

        // 重置上下文
        e.ResetContext()

        // 搜索 "n"，不应该有上下文加成
        cands := e.Search("n")
        if len(cands) == 0 {
                t.Fatal("no candidates")
        }
        t.Logf("ResetContext 后搜索'n' -> top 3: %v", topWords(cands, 3))
}

func TestContextAssociationMultiplePairs(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        // 建立多种上下文组合
        // "我你" 5 次，"我他" 1 次
        for i := 0; i < 5; i++ {
                e.Commit("我", "wo")
                e.Commit("你", "ni")
        }
        e.Commit("我", "wo")
        e.Commit("他", "ta")

        // 提交 "我"，然后搜索 "n" 和 "t"
        e.Commit("我", "wo")

        candsN := e.Search("n")
        candsT := e.Search("t")
        t.Logf("提交'我'后:")
        t.Logf("  搜索'n' -> top 3: %v", topWords(candsN, 3))
        t.Logf("  搜索't' -> top 3: %v", topWords(candsT, 3))

        // "你" 应该比 "他" 排名更靠前（因为共现频次更高）
        rankN, rankT := -1, -1
        for i, c := range candsN {
                if c.Word == "你" {
                        rankN = i
                        break
                }
        }
        for i, c := range candsT {
                if c.Word == "他" {
                        rankT = i
                        break
                }
        }
        t.Logf("  '你' rank=%d, '他' rank=%d", rankN, rankT)
}

// === 综合场景测试 ===

func TestTimeDecayAndContextCombined(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        e.mu.Lock()
        e.decayHalfLife = 100 * time.Millisecond
        e.mu.Unlock()

        // 旧上下文：我你（多次）
        for i := 0; i < 10; i++ {
                e.Commit("我", "wo")
                e.Commit("你", "ni")
        }

        // 等待衰减
        time.Sleep(300 * time.Millisecond)

        // 新上下文：我他（少数）
        for i := 0; i < 2; i++ {
                e.Commit("我", "wo")
                e.Commit("他", "ta")
        }

        // 提交 "我" 后搜索
        e.Commit("我", "wo")
        candsN := e.Search("n")
        candsT := e.Search("t")
        t.Logf("衰减后:")
        t.Logf("  'n' -> top 3: %v", topWords(candsN, 3))
        t.Logf("  't' -> top 3: %v", topWords(candsT, 3))
}

func TestCommitHistoryLength(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        e.mu.Lock()
        e.maxContextHistory = 5
        e.mu.Unlock()

        // 提交 20 个不同的词
        for i := 0; i < 20; i++ {
                e.Commit("词"+string(rune('a'+i)), "ci"+string(rune('a'+i)))
        }
        // 历史应该被限制在 5
        e.mu.RLock()
        histLen := len(e.commitHistory)
        e.mu.RUnlock()
        if histLen != 5 {
                t.Errorf("commit history should be capped at 5, got %d", histLen)
        }
}

func TestContextPairsPersistence(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        // 建立上下文
        for i := 0; i < 3; i++ {
                e.Commit("我", "wo")
                e.Commit("你", "ni")
        }

        // contextPairs 应该有 "我|你" 的记录
        e.mu.RLock()
        freq, ok := e.contextPairs["我|你"]
        e.mu.RUnlock()
        if !ok {
                t.Error("contextPairs should contain '我|你'")
        }
        if freq != 3 {
                t.Errorf("contextPairs['我|你'] = %v, want 3", freq)
        }
}

// 防止 import 未使用
var _ = time.Now

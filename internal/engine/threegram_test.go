package engine

import (
        "os"
        "testing"
        "time"
)

// === 3-gram 上下文测试 ===

func TestThreeGramContextBasic(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        // 模拟用户输入 "我爱学习" 多次（建立 3-gram）
        for i := 0; i < 5; i++ {
                e.Commit("我", "wo")
                e.Commit("爱", "ai")
                e.Commit("学习", "xuexi")
        }

        // 提交 "我" 和 "爱" 后，搜索 "xuexi"（完整拼音）
        // "学习" 通过精确匹配进入候选，3-gram 上下文让它排第一
        e.Commit("我", "wo")
        e.Commit("爱", "ai")
        cands := e.Search("xuexi")
        t.Logf("提交'我爱'后搜索'xuexi' -> top 5:")
        for i, c := range cands {
                if i >= 5 {
                        break
                }
                t.Logf("  %d. %s (score=%.2f, src=%s)", i+1, c.Word, c.Score, c.Source)
        }

        // "学习" 应该是 top 1（3-gram 上下文加权）
        if len(cands) == 0 {
                t.Fatal("no candidates")
        }
        if cands[0].Word != "学习" {
                t.Errorf("'学习' should be top1 after '我爱', got %s", cands[0].Word)
        }
}

func TestThreeGramContextStrongerThanTwoGram(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        // 建立 2-gram: 我→你 (5次)
        for i := 0; i < 5; i++ {
                e.Commit("我", "wo")
                e.Commit("你", "ni")
        }
        // 建立 3-gram: 我→爱→学习 (10次，比 2-gram 更频繁)
        for i := 0; i < 10; i++ {
                e.Commit("我", "wo")
                e.Commit("爱", "ai")
                e.Commit("学习", "xuexi")
        }

        // 提交 "我" 后搜索 "ni"（2-gram: 我→你）
        e.Commit("我", "wo")
        candsN := e.Search("ni")
        t.Logf("提交'我'后搜索'ni' (2-gram 我→你): %v", topWords(candsN, 3))

        // 提交 "我爱" 后搜索 "xuexi"（3-gram: 我+爱→学习）
        e.Commit("爱", "ai")
        candsX := e.Search("xuexi")
        t.Logf("提交'我爱'后搜索'xuexi' (3-gram): %v", topWords(candsX, 3))

        // "学习" 应该是 top 1（3-gram 权重高）
        if len(candsX) == 0 {
                t.Fatal("no candidates for 'xuexi'")
        }
        if candsX[0].Word != "学习" {
                t.Errorf("'学习' should be top1 after '我爱', got %s", candsX[0].Word)
        }
}

// === 上下文持久化测试 ===

func TestContextPersistenceAcrossEngines(t *testing.T) {
        d, _ := loadTestDict()
        tmpDir, _ := os.MkdirTemp("", "samime-ctx-test-*")
        defer os.RemoveAll(tmpDir)

        // 第一个引擎：建立上下文
        e1, err := NewWithUserStore(d, tmpDir+"/userdict")
        if err != nil {
                t.Fatalf("NewWithUserStore 1: %v", err)
        }
        for i := 0; i < 3; i++ {
                e1.Commit("我", "wo")
                e1.Commit("你", "ni")
        }
        // 验证 e1 内存中有上下文
        e1.mu.RLock()
        freq, ok := e1.contextPairs["我|你"]
        e1.mu.RUnlock()
        if !ok || freq != 3 {
                t.Errorf("e1 contextPairs['我|你'] = %v, want 3", freq)
        }
        if err := e1.Close(); err != nil {
                t.Fatalf("e1.Close: %v", err)
        }

        // 第二个引擎：从同一目录加载，应该看到上下文
        e2, err := NewWithUserStore(d, tmpDir+"/userdict")
        if err != nil {
                t.Fatalf("NewWithUserStore 2: %v", err)
        }
        defer e2.Close()

        e2.mu.RLock()
        freq2, ok2 := e2.contextPairs["我|你"]
        e2.mu.RUnlock()
        if !ok2 {
                t.Error("after reopen, contextPairs should contain '我|你'")
        }
        if freq2 != 3 {
                t.Errorf("after reopen, contextPairs['我|你'] = %v, want 3", freq2)
        }
        t.Logf("持久化验证: e1 freq=%v, e2 freq=%v", freq, freq2)
}

func TestThreeGramPersistence(t *testing.T) {
        d, _ := loadTestDict()
        tmpDir, _ := os.MkdirTemp("", "samime-3gram-test-*")
        defer os.RemoveAll(tmpDir)

        // 第一个引擎：建立 3-gram
        e1, err := NewWithUserStore(d, tmpDir+"/userdict")
        if err != nil {
                t.Fatalf("e1: %v", err)
        }
        for i := 0; i < 3; i++ {
                e1.Commit("我", "wo")
                e1.Commit("爱", "ai")
                e1.Commit("学习", "xuexi")
        }
        // 验证 3-gram key 存在
        e1.mu.RLock()
        _, ok := e1.contextPairs["我\t爱|学习"]
        e1.mu.RUnlock()
        if !ok {
                t.Error("e1 should have 3-gram key '我\\t爱|学习'")
        }
        e1.Close()

        // 第二个引擎：加载
        e2, err := NewWithUserStore(d, tmpDir+"/userdict")
        if err != nil {
                t.Fatalf("e2: %v", err)
        }
        defer e2.Close()

        e2.mu.RLock()
        freq, ok := e2.contextPairs["我\t爱|学习"]
        e2.mu.RUnlock()
        if !ok {
                t.Error("after reopen, 3-gram should be persisted")
        }
        t.Logf("3-gram 持久化: '我\\t爱|学习' freq=%v", freq)
}

// === 综合场景测试 ===

func TestContextWithTimeDecay(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        e.mu.Lock()
        e.decayHalfLife = 50 * time.Millisecond
        e.mu.Unlock()

        // 建立上下文
        for i := 0; i < 5; i++ {
                e.Commit("我", "wo")
                e.Commit("你", "ni")
        }

        // 等待一段时间（用户频次衰减，但上下文不衰减）
        time.Sleep(200 * time.Millisecond)

        // 再次提交 "我"，搜索 "n"
        e.Commit("我", "wo")
        cands := e.Search("n")
        t.Logf("衰减后搜索'n' -> top 3: %v", topWords(cands, 3))

        // "你" 仍应该排前面（上下文加权不变）
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
                t.Errorf("'你' should still be in top 3 (context doesn't decay)")
        }
}

func TestContextPairsCountAfterMultipleCommits(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        // 提交序列: A B A C A B
        words := []struct {
                w, p string
        }{
                {"苹果", "pingguo"},
                {"香蕉", "xiangjiao"},
                {"苹果", "pingguo"},
                {"樱桃", "yingtao"},
                {"苹果", "pingguo"},
                {"香蕉", "xiangjiao"},
        }
        for _, w := range words {
                e.Commit(w.w, w.p)
        }

        e.mu.RLock()
        defer e.mu.RUnlock()

        // 应该有这些 2-gram: 苹果→香蕉(2), 香蕉→苹果(1), 苹果→樱桃(1), 樱桃→苹果(1)
        tests := []struct {
                key  string
                want float64
        }{
                {"苹果|香蕉", 2},
                {"香蕉|苹果", 1},
                {"苹果|樱桃", 1},
                {"樱桃|苹果", 1},
        }
        for _, tt := range tests {
                got := e.contextPairs[tt.key]
                if got != tt.want {
                        t.Errorf("contextPairs[%q] = %v, want %v", tt.key, got, tt.want)
                }
        }

        // 3-gram: 苹果+香蕉→苹果(1), 香蕉+苹果→樱桃(1), 苹果+樱桃→苹果(1), 樱桃+苹果→香蕉(1)
        threeGramTests := []struct {
                key  string
                want float64
        }{
                {"苹果\t香蕉|苹果", 1},
                {"香蕉\t苹果|樱桃", 1},
                {"苹果\t樱桃|苹果", 1},
                {"樱桃\t苹果|香蕉", 1},
        }
        for _, tt := range threeGramTests {
                got := e.contextPairs[tt.key]
                if got != tt.want {
                        t.Errorf("3-gram contextPairs[%q] = %v, want %v", tt.key, got, tt.want)
                }
        }
}

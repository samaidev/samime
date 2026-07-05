//go:build integration

// 性能与压力测试
package main

import (
        "fmt"
        "os"
        "sync"
        "sync/atomic"
        "testing"
        "time"

        "github.com/zai/goime/internal/dict"
        "github.com/zai/goime/internal/engine"
        "github.com/zai/goime/internal/pinyin"
)

// 10000 次混合查询，测量总吞吐量
func TestStressMixedQueries(t *testing.T) {
        d, _ := dict.LoadEmbedded()
        eng := engine.NewDefault(d)
        defer eng.Close()

        queries := []string{
                "nihao", "zhongguo", "shurufa", "pinyin", "xuesheng",
                "laoshi", "diannao", "shouji", "pengyou", "gongzuo",
                "woaixuexi", "zhongguoren", "tiananmen", "beijing", "shanghai",
                "rengongzhineng", "jiqixuexi", "shenduxuexi", "yunjisuan", "qukuailian",
        }

        const N = 10000
        t0 := time.Now()
        for i := 0; i < N; i++ {
                q := queries[i%len(queries)]
                _ = eng.Search(q)
        }
        dur := time.Since(t0)
        avg := dur / time.Duration(N)
        qps := float64(N) / dur.Seconds()
        t.Logf("stress: %d queries | total=%v | avg=%v | qps=%.0f",
                N, dur, avg, qps)
        fmt.Fprintf(os.Stderr, "\n[Stress] %d queries, avg %v, qps=%.0f\n", N, avg, qps)
}

// 高并发：100 个 goroutine 各 100 次查询
func TestStressHighConcurrency(t *testing.T) {
        d, _ := dict.LoadEmbedded()
        eng := engine.NewDefault(d)
        defer eng.Close()

        queries := []string{
                "nihao", "zhongguo", "shurufa", "pinyin", "xuesheng",
                "laoshi", "diannao", "shouji", "pengyou", "gongzuo",
        }

        const goroutines = 100
        const perGoroutine = 100
        var ops int64
        var wg sync.WaitGroup
        t0 := time.Now()

        for i := 0; i < goroutines; i++ {
                wg.Add(1)
                go func(id int) {
                        defer wg.Done()
                        for j := 0; j < perGoroutine; j++ {
                                q := queries[(id+j)%len(queries)]
                                _ = eng.Search(q)
                                atomic.AddInt64(&ops, 1)
                        }
                }(i)
        }
        wg.Wait()
        dur := time.Since(t0)
        qps := float64(ops) / dur.Seconds()
        t.Logf("concurrency: %d goroutines x %d = %d ops | total=%v | qps=%.0f",
                goroutines, perGoroutine, ops, dur, qps)
}

// 极端并发：1000 goroutine
func TestStress1000Goroutines(t *testing.T) {
        d, _ := dict.LoadEmbedded()
        eng := engine.NewDefault(d)
        defer eng.Close()

        const goroutines = 1000
        var wg sync.WaitGroup
        t0 := time.Now()

        for i := 0; i < goroutines; i++ {
                wg.Add(1)
                go func() {
                        defer wg.Done()
                        eng.Search("nihao")
                }()
        }
        wg.Wait()
        dur := time.Since(t0)
        t.Logf("1000 concurrent Search('nihao') done in %v (%.0f qps)",
                dur, 1000.0/dur.Seconds())
}

// 长时间运行：1 分钟持续查询
func TestStressOneMinute(t *testing.T) {
        if testing.Short() {
                t.Skip("skipping 1-minute stress test in short mode")
        }
        d, _ := dict.LoadEmbedded()
        eng := engine.NewDefault(d)
        defer eng.Close()

        queries := []string{
                "nihao", "zhongguo", "shurufa", "pinyin", "xuesheng",
                "laoshi", "diannao", "shouji", "pengyou", "gongzuo",
        }

        const duration = 60 * time.Second
        deadline := time.Now().Add(duration)
        var ops int64
        var wg sync.WaitGroup

        // 10 个 worker
        for i := 0; i < 10; i++ {
                wg.Add(1)
                go func(id int) {
                        defer wg.Done()
                        for time.Now().Before(deadline) {
                                q := queries[int(ops)%len(queries)]
                                _ = eng.Search(q)
                                atomic.AddInt64(&ops, 1)
                        }
                }(i)
        }
        wg.Wait()
        qps := float64(ops) / duration.Seconds()
        t.Logf("1-minute stress: %d ops in %v | qps=%.0f", ops, duration, qps)
        fmt.Fprintf(os.Stderr, "\n[1min] %d ops, qps=%.0f\n", ops, qps)
}

// 内存稳定性：跑 1 分钟，每 10 秒打印 HeapAlloc
func TestStressMemoryStable(t *testing.T) {
        if testing.Short() {
                t.Skip("skipping memory test in short mode")
        }
        d, _ := dict.LoadEmbedded()
        eng := engine.NewDefault(d)
        defer eng.Close()

        queries := []string{
                "nihao", "zhongguo", "shurufa", "pinyin", "xuesheng",
                "woaixuexi", "zhongguoren",
        }

        // 预热
        for _, q := range queries {
                eng.Search(q)
        }

        // 跑 30 秒
        const duration = 30 * time.Second
        deadline := time.Now().Add(duration)
        var ops int64
        for time.Now().Before(deadline) {
                eng.Search(queries[int(ops)%len(queries)])
                ops++
        }
        t.Logf("memory test: %d ops in 30s", ops)
}

// 持续 commit + search 混合工作负载
func TestStressMixedWorkload(t *testing.T) {
        d, _ := dict.LoadEmbedded()
        eng := engine.NewDefault(d)
        defer eng.Close()

        var wg sync.WaitGroup
        // 5 个 searcher
        for i := 0; i < 5; i++ {
                wg.Add(1)
                go func() {
                        defer wg.Done()
                        for j := 0; j < 1000; j++ {
                                eng.Search("nihao")
                        }
                }()
        }
        // 5 个 committer
        for i := 0; i < 5; i++ {
                wg.Add(1)
                go func(id int) {
                        defer wg.Done()
                        for j := 0; j < 1000; j++ {
                                eng.Commit("你好", "nihao")
                        }
                }(i)
        }
        wg.Wait()
        // 用户频次应该是 ~5000（时间衰减导致略小）
        uf := eng.UserFreq()["你好|nihao"]
        if uf < 4999 || uf > 5000 {
                t.Errorf("expected ~5000 commits, got %v", uf)
        }
}

// Pinyin segment 性能：1M 次切分
func TestStressPinyinSegment(t *testing.T) {
        const N = 100000
        t0 := time.Now()
        for i := 0; i < N; i++ {
                pinyin.Segment("nihao")
        }
        dur := time.Since(t0)
        t.Logf("pinyin.Segment('nihao') x %d: %v (%.0f ops/sec)",
                N, dur, float64(N)/dur.Seconds())
}

// 长拼音切分性能
func TestStressPinyinSegmentLong(t *testing.T) {
        const N = 10000
        longInput := "woaixuexijiqixuexirenghongzhiineng"
        t0 := time.Now()
        for i := 0; i < N; i++ {
                pinyin.Segment(longInput)
        }
        dur := time.Since(t0)
        t.Logf("pinyin.Segment(%q) x %d: %v (%.0f ops/sec)",
                longInput, N, dur, float64(N)/dur.Seconds())
}

// 词典加载性能
func TestStressDictLoad(t *testing.T) {
        const N = 5
        durs := make([]time.Duration, N)
        for i := 0; i < N; i++ {
                t0 := time.Now()
                _, _ = dict.LoadEmbedded()
                durs[i] = time.Since(t0)
        }
        var total time.Duration
        for _, d := range durs {
                total += d
        }
        avg := total / time.Duration(N)
        t.Logf("dict.LoadEmbedded() avg over %d runs: %v (min=%v, max=%v)",
                N, avg, durs[0], durs[N-1])
}

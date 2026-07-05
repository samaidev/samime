package engine

import (
        "os"
        "strings"
        "sync"
        "testing"

        "github.com/zai/goime/internal/dict"
)

func TestSearchEmpty(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        if c := e.Search(""); len(c) != 0 {
                t.Errorf("Search('') = %v, want empty", c)
        }
        if c := e.Search("   "); len(c) != 0 {
                t.Errorf("Search('   ') = %v, want empty", c)
        }
}

func TestSearchNonAlpha(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        cases := []string{
                "123",
                "!@#$",
                "你好",
                "\x00\x01",
                "\n\r\t",
        }
        for _, c := range cases {
                result := e.Search(c)
                t.Logf("Search(%q) -> %d candidates", c, len(result))
        }
}

func TestSearchVeryLongInput(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        // 100 字符长拼音
        long := strings.Repeat("a", 100)
        c := e.Search(long)
        t.Logf("Search(100 'a') -> %d candidates", len(c))
        // 1000 字符
        long2 := strings.Repeat("a", 1000)
        c2 := e.Search(long2)
        t.Logf("Search(1000 'a') -> %d candidates", len(c2))
}

func TestSearchCaseInsensitive(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        c1 := e.Search("nihao")
        c2 := e.Search("NIHAO")
        c3 := e.Search("NiHao")
        if len(c1) == 0 {
                t.Fatal("Search('nihao') should return candidates")
        }
        if len(c2) == 0 || len(c3) == 0 {
                t.Errorf("case insensitive: nihao=%d, NIHAO=%d, NiHao=%d",
                        len(c1), len(c2), len(c3))
        }
}

func TestCommitEmpty(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        // 空 commit 不应 panic
        e.Commit("", "")
        e.Commit("", "nihao")
        e.Commit("你好", "")
}

func TestCommitVeryLongWord(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        longWord := strings.Repeat("你", 100)
        e.Commit(longWord, "ni")
        if e.UserFreq()[longWord+"|ni"] != 1 {
                t.Error("long word commit failed")
        }
}

func TestCommitManyTimes(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        // 提交 1000 次（时间衰减会让频次略小于 1000，因连续 commit 间隔极短）
        for i := 0; i < 1000; i++ {
                e.Commit("你好", "nihao")
        }
        freq := e.UserFreq()["你好|nihao"]
        // 时间衰减引入浮点误差，但应该接近 1000
        if freq < 999 || freq > 1000 {
                t.Errorf("after 1000 commits, freq = %v (expected ~1000)", freq)
        }
}

func TestCommitManyDifferentWords(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        // 提交 1000 个不同词
        for i := 0; i < 1000; i++ {
                w := "词" + string(rune(i))
                e.Commit(w, "ci"+string(rune('a'+(i%26))))
        }
        if len(e.UserFreq()) < 1000 {
                t.Errorf("expected >= 1000 user freq entries, got %d", len(e.UserFreq()))
        }
}

func TestResetContext(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        e.Commit("你好", "nihao")
        e.ResetContext()
        // 重置后再次搜索，候选应该正常
        c := e.Search("nihao")
        if len(c) == 0 {
                t.Error("Search after ResetContext should still work")
        }
}

func TestLoadUserFreq(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        m := map[string]float64{
                "你好|nihao":   100,
                "世界|shijie": 50,
        }
        e.LoadUserFreq(m)
        if e.UserFreq()["你好|nihao"] != 100 {
                t.Errorf("LoadUserFreq failed")
        }
}

func TestUserFreqIsolation(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        m := e.UserFreq()
        m["hack|hack"] = 9999
        // 修改返回的 map 不应影响引擎内部状态
        if e.UserFreq()["hack|hack"] == 9999 {
                t.Error("UserFreq() should return a copy, not internal map")
        }
}

func TestSearchConcurrent(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        queries := []string{
                "nihao", "zhongguo", "shurufa", "pinyin", "xuesheng",
                "laoshi", "diannao", "shouji", "pengyou", "gongzuo",
        }

        var wg sync.WaitGroup
        for i := 0; i < 10; i++ {
                wg.Add(1)
                go func() {
                        defer wg.Done()
                        for j := 0; j < 100; j++ {
                                q := queries[j%len(queries)]
                                _ = e.Search(q)
                        }
                }()
        }
        wg.Wait()
        // 不应 panic 或 data race
}

func TestCommitConcurrent(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        var wg sync.WaitGroup
        for i := 0; i < 10; i++ {
                wg.Add(1)
                go func(id int) {
                        defer wg.Done()
                        for j := 0; j < 100; j++ {
                                e.Commit("你好", "nihao")
                        }
                }(i)
        }
        wg.Wait()
        // 10 * 100 = 1000 次 commit（时间衰减导致略小于 1000）
        freq := e.UserFreq()["你好|nihao"]
        if freq < 999 || freq > 1000 {
                t.Errorf("concurrent commit: expected ~1000, got %v", freq)
        }
}

func TestSearchAndCommitConcurrent(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)

        var wg sync.WaitGroup
        // 5 个 goroutine 搜索
        for i := 0; i < 5; i++ {
                wg.Add(1)
                go func() {
                        defer wg.Done()
                        for j := 0; j < 100; j++ {
                                _ = e.Search("nihao")
                        }
                }()
        }
        // 5 个 goroutine 提交
        for i := 0; i < 5; i++ {
                wg.Add(1)
                go func() {
                        defer wg.Done()
                        for j := 0; j < 100; j++ {
                                e.Commit("你好", "nihao")
                        }
                }()
        }
        wg.Wait()
}

func TestNewWithUserStore(t *testing.T) {
        d, _ := loadTestDict()
        tmpDir, _ := os.MkdirTemp("", "samime-test-*")
        defer os.RemoveAll(tmpDir)

        // 第一个引擎实例：提交并关闭
        e1, err := NewWithUserStore(d, tmpDir+"/userdict")
        if err != nil {
                t.Fatalf("NewWithUserStore 1: %v", err)
        }
        e1.Commit("你好", "nihao")
        e1.Commit("你好", "nihao")
        if err := e1.Close(); err != nil {
                t.Fatalf("e1.Close: %v", err)
        }

        // 第二个引擎实例：从同一目录加载
        e2, err := NewWithUserStore(d, tmpDir+"/userdict")
        if err != nil {
                t.Fatalf("NewWithUserStore 2: %v", err)
        }
        defer e2.Close()

        if e2.UserFreq()["你好|nihao"] != 2 {
                t.Errorf("persistence: expected 2, got %v",
                        e2.UserFreq()["你好|nihao"])
        }
}

func TestSearchWithSpecialPinyin(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        // 声调数字（容错）
        cases := []string{
                "ni3hao3",     // 带声调
                "NI3HAO3",
                "ni hao",      // 空格分隔
                "ni,hao",      // 标点分隔
        }
        for _, c := range cases {
                result := e.Search(c)
                t.Logf("Search(%q) -> %d candidates, top 3: %v",
                        c, len(result), topWords(result, 3))
        }
}

func TestFuzzyAndTypoCombined(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        // 同时模糊音和拼写错误的极端情况
        cases := []string{
                "ligap",  // l→n (模糊) + g→h (邻键)
                "lihap",  // l→n + a→o 邻键
                "zongkwo",// z→zh + k→g + w→u
        }
        for _, c := range cases {
                result := e.Search(c)
                t.Logf("Search(%q) -> top 3: %v", c, topWords(result, 3))
        }
}

func TestDictAndFuzzyAccessors(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        if e.Dict() == nil {
                t.Error("Dict() should not return nil")
        }
        if e.Fuzzy() == nil {
                t.Error("Fuzzy() should not return nil")
        }
}

func TestSearchAllPinyinSyllables(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        // 测试所有单字拼音（确保引擎不会因任何音节卡死）
        // 这里只测一部分有代表性的
        cases := []string{
                "a", "o", "e", "i", "u", "v",
                "ai", "ei", "ao", "ou", "an", "en", "ang", "eng",
                "ba", "po", "mi", "fu", "de", "te", "ne", "le",
                "zhi", "chi", "shi", "ri", "zi", "ci", "si",
        }
        for _, c := range cases {
                result := e.Search(c)
                if len(result) == 0 {
                        t.Logf("Search(%q) -> 0 candidates (warning)", c)
                }
        }
}

func TestCandidateScore(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        c := e.Search("nihao")
        if len(c) == 0 {
                t.Fatal("no candidates")
        }
        // 验证分数是降序
        for i := 1; i < len(c) && i < 10; i++ {
                if c[i-1].Score < c[i].Score {
                        t.Errorf("candidates not sorted: [%d]=%.2f < [%d]=%.2f",
                                i-1, c[i-1].Score, i, c[i].Score)
                        break
                }
        }
}

// 顶层 _ = dict 用法（避免未使用导入）
var _ = dict.New

func topWords(cands []Candidate, n int) string {
        if len(cands) == 0 {
                return "-"
        }
        var parts []string
        for i, c := range cands {
                if i >= n {
                        break
                }
                parts = append(parts, c.Word)
        }
        return strings.Join(parts, " ")
}

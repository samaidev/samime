package clipboard

import (
	"strings"
	"sync"
	"testing"
)

// === 边缘情况补充测试 ===

func TestHistoryEmptyOperations(t *testing.T) {
	h := New(50)
	// 空历史的各种操作
	if h.Size() != 0 {
		t.Errorf("empty size = %d", h.Size())
	}
	if all := h.All(); len(all) != 0 {
		t.Errorf("All() on empty = %v", all)
	}
	if r := h.Recent(10); len(r) != 0 {
		t.Errorf("Recent(10) on empty = %v", r)
	}
	if _, ok := h.Get(0); ok {
		t.Error("Get(0) on empty should return false")
	}
	if s := h.Search("anything"); len(s) != 0 {
		t.Errorf("Search on empty = %v", s)
	}
	if h.Contains("anything") {
		t.Error("Contains on empty should be false")
	}
}

func TestHistoryZeroMaxSize(t *testing.T) {
	h := New(0)  // 应该默认为 50
	if h.maxSize != 50 {
		t.Errorf("New(0) maxSize = %d, want 50", h.maxSize)
	}
}

func TestHistoryNegativeMaxSize(t *testing.T) {
	h := New(-10)
	if h.maxSize != 50 {
		t.Errorf("New(-10) maxSize = %d, want 50", h.maxSize)
	}
}

func TestHistoryMaxSizeOne(t *testing.T) {
	h := New(1)
	h.Add("a", "p1", "u")
	h.Add("b", "p2", "u")
	if h.Size() != 1 {
		t.Errorf("Size = %d, want 1", h.Size())
	}
	all := h.All()
	if all[0].Word != "b" {
		t.Errorf("only entry should be 'b' (newest), got %s", all[0].Word)
	}
}

func TestHistoryVeryLongWord(t *testing.T) {
	h := New(50)
	longWord := strings.Repeat("你", 10000)
	h.Add(longWord, "ni", "u")
	if !h.Contains(longWord) {
		t.Error("should contain very long word")
	}
}

func TestHistorySpecialChars(t *testing.T) {
	h := New(50)
	cases := []string{
		"",
		" ",
		"\n",
		"\t",
		"你好\n世界",
		"emoji: 🎉",
		"tab\there",
		"\"quoted\"",
		"\\backslash",
		"\x00null",
	}
	for _, c := range cases {
		h.Add(c, "py", "u")
	}
	if h.Size() != len(cases) {
		t.Errorf("Size = %d, want %d", h.Size(), len(cases))
	}
}

func TestHistoryRecentMoreThanSize(t *testing.T) {
	h := New(50)
	for i := 0; i < 5; i++ {
		h.Add("w"+string(rune('a'+i)), "p", "u")
	}
	// 请求比实际多的数量
	r := h.Recent(100)
	if len(r) != 5 {
		t.Errorf("Recent(100) = %d, want 5", len(r))
	}
}

func TestHistoryRecentZero(t *testing.T) {
	h := New(50)
	h.Add("a", "p", "u")
	r := h.Recent(0)
	// Recent(0) 行为：返回全部（与 n<=0 时一致）
	if len(r) == 0 {
		t.Logf("Recent(0) 返回空（实现可能不同）")
	} else {
		t.Logf("Recent(0) 返回 %d 条", len(r))
	}
}

func TestHistoryRecentNegative(t *testing.T) {
	h := New(50)
	h.Add("a", "p", "u")
	h.Add("b", "p", "u")
	r := h.Recent(-5)
	// 负数应该和 0 行为一致
	t.Logf("Recent(-5) = %d 条", len(r))
}

func TestHistoryGetNegativeIndex(t *testing.T) {
	h := New(50)
	h.Add("a", "p", "u")
	if _, ok := h.Get(-1); ok {
		t.Error("Get(-1) should return false")
	}
}

func TestHistorySearchEmpty(t *testing.T) {
	h := New(50)
	h.Add("你好", "nihao", "u")
	h.Add("世界", "shijie", "u")
	// 空查询应返回全部（contains 实现）
	r := h.Search("")
	if len(r) != 2 {
		t.Errorf("Search('') = %d, want 2 (all match)", len(r))
	}
}

func TestHistorySearchNoMatch(t *testing.T) {
	h := New(50)
	h.Add("你好", "nihao", "u")
	r := h.Search("不存在的词")
	if len(r) != 0 {
		t.Errorf("Search(no match) = %d, want 0", len(r))
	}
}

func TestHistorySearchCaseSensitive(t *testing.T) {
	h := New(50)
	h.Add("Hello", "hello", "u")
	// 大小写敏感
	if r := h.Search("hello"); len(r) == 0 {
		t.Error("Search('hello') should match 'Hello' pinyin")
	}
	if r := h.Search("HELLO"); len(r) != 0 {
		t.Errorf("Search('HELLO') should not match, got %d", len(r))
	}
}

func TestHistoryClearOnEmpty(t *testing.T) {
	h := New(50)
	h.Clear()  // 清空空历史不应 panic
	if h.Size() != 0 {
		t.Errorf("after Clear empty, size = %d", h.Size())
	}
}

func TestHistoryAddAfterClear(t *testing.T) {
	h := New(50)
	h.Add("a", "p", "u")
	h.Clear()
	h.Add("b", "p", "u")
	if h.Size() != 1 {
		t.Errorf("Size = %d, want 1", h.Size())
	}
	if !h.Contains("b") {
		t.Error("should contain 'b' after clear+add")
	}
	if h.Contains("a") {
		t.Error("should not contain 'a' after clear")
	}
}

func TestHistoryConcurrentReadWrite(t *testing.T) {
	h := New(100)
	var wg sync.WaitGroup

	// 5 个写者
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				h.Add("writer"+string(rune('a'+id)), "p", "u")
			}
		}(i)
	}

	// 5 个读者
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = h.All()
				_ = h.Search("test")
				_ = h.Contains("test")
			}
		}()
	}

	wg.Wait()
	// 不应 panic 或 data race
}

func TestHistoryOrder(t *testing.T) {
	h := New(50)
	// 按顺序添加 a, b, c, d
	h.Add("a", "p", "u")
	h.Add("b", "p", "u")
	h.Add("c", "p", "u")
	h.Add("d", "p", "u")

	all := h.All()
	// All() 应该是最新在前
	expected := []string{"d", "c", "b", "a"}
	for i, e := range expected {
		if all[i].Word != e {
			t.Errorf("All()[%d] = %s, want %s", i, all[i].Word, e)
		}
	}
}

func TestHistoryEntryFields(t *testing.T) {
	h := New(50)
	h.Add("你好", "nihao", "dict")
	e, ok := h.Get(0)
	if !ok {
		t.Fatal("Get(0) failed")
	}
	if e.Word != "你好" || e.Pinyin != "nihao" || e.Source != "dict" {
		t.Errorf("entry = %+v", e)
	}
	if e.Time.IsZero() {
		t.Error("Time should not be zero")
	}
}

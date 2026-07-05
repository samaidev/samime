package clipboard

import (
	"testing"
)

func TestHistoryAddAndSize(t *testing.T) {
	h := New(50)
	if h.Size() != 0 {
		t.Errorf("initial size = %d, want 0", h.Size())
	}
	h.Add("你好", "nihao", "user")
	h.Add("世界", "shijie", "user")
	if h.Size() != 2 {
		t.Errorf("after 2 adds, size = %d, want 2", h.Size())
	}
}

func TestHistoryMaxSize(t *testing.T) {
	h := New(5)
	for i := 0; i < 10; i++ {
		h.Add("word"+string(rune('a'+i)), "py", "user")
	}
	if h.Size() != 5 {
		t.Errorf("size = %d, want 5 (max)", h.Size())
	}
	// 最旧的 5 条应该被移除
	all := h.All()
	if len(all) != 5 {
		t.Fatalf("All() = %d, want 5", len(all))
	}
	// 最新的应该是 wordj (最后添加的)
	if all[0].Word != "wordj" {
		t.Errorf("newest = %s, want wordj", all[0].Word)
	}
}

func TestHistoryRecent(t *testing.T) {
	h := New(50)
	for i := 0; i < 10; i++ {
		h.Add("w"+string(rune('a'+i)), "p", "u")
	}
	r := h.Recent(3)
	if len(r) != 3 {
		t.Fatalf("Recent(3) = %d, want 3", len(r))
	}
	// 最新的 3 条按倒序：wj, wi, wh
	if r[0].Word != "wj" || r[1].Word != "wi" || r[2].Word != "wh" {
		t.Errorf("Recent(3) = %v %v %v, want wj wi wh", r[0].Word, r[1].Word, r[2].Word)
	}
}

func TestHistoryGet(t *testing.T) {
	h := New(50)
	h.Add("old", "p1", "u")
	h.Add("new", "p2", "u")
	// Get(0) = 最新
	e, ok := h.Get(0)
	if !ok || e.Word != "new" {
		t.Errorf("Get(0) = %v %v, want new true", e.Word, ok)
	}
	// Get(1) = 旧
	e, ok = h.Get(1)
	if !ok || e.Word != "old" {
		t.Errorf("Get(1) = %v %v, want old true", e.Word, ok)
	}
	// 越界
	_, ok = h.Get(5)
	if ok {
		t.Error("Get(5) should return false")
	}
}

func TestHistorySearch(t *testing.T) {
	h := New(50)
	h.Add("你好", "nihao", "u")
	h.Add("世界", "shijie", "u")
	h.Add("你好啊", "nihaoa", "u")

	// 搜索 "你好"
	results := h.Search("你好")
	if len(results) != 2 {
		t.Errorf("Search('你好') = %d, want 2", len(results))
	}
	// 搜索拼音 "nihao"
	results = h.Search("nihao")
	// nihao 和 nihaoa 都包含 nihao
	if len(results) < 2 {
		t.Errorf("Search('nihao') = %d, want >= 2", len(results))
	}
}

func TestHistoryClear(t *testing.T) {
	h := New(50)
	h.Add("a", "p", "u")
	h.Add("b", "p", "u")
	h.Clear()
	if h.Size() != 0 {
		t.Errorf("after Clear, size = %d, want 0", h.Size())
	}
}

func TestHistoryContains(t *testing.T) {
	h := New(50)
	h.Add("你好", "nihao", "u")
	if !h.Contains("你好") {
		t.Error("Contains('你好') should be true")
	}
	if h.Contains("不存在") {
		t.Error("Contains('不存在') should be false")
	}
}

func TestHistoryConcurrency(t *testing.T) {
	h := New(100)
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				h.Add("w", "p", "u")
			}
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	// 10 * 100 = 1000 次添加，但 maxSize=100
	if h.Size() != 100 {
		t.Errorf("after concurrent adds, size = %d, want 100", h.Size())
	}
}

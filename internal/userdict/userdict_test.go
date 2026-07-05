package userdict

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreBasic(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "userdict-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "ud")
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// 初始为空
	if s.Get("你好", "nihao") != 0 {
		t.Error("expected 0 for unknown entry")
	}

	// 增加频次
	if err := s.Incr("你好", "nihao"); err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if err := s.Incr("你好", "nihao"); err != nil {
		t.Fatalf("Incr: %v", err)
	}
	if err := s.Incr("中国", "zhongguo"); err != nil {
		t.Fatalf("Incr: %v", err)
	}

	// 验证
	if got := s.Get("你好", "nihao"); got != 2 {
		t.Errorf("你好|nihao = %v, want 2", got)
	}
	if got := s.Get("中国", "zhongguo"); got != 1 {
		t.Errorf("中国|zhongguo = %v, want 1", got)
	}

	// 验证 All()
	all := s.All()
	if len(all) != 2 {
		t.Errorf("All() = %d entries, want 2", len(all))
	}
	if all["你好|nihao"] != 2 {
		t.Errorf("All()[你好|nihao] = %v, want 2", all["你好|nihao"])
	}
}

func TestStorePersistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "userdict-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	path := filepath.Join(tmpDir, "ud")

	// 第一次打开，写入数据
	s1, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.Incr("你好", "nihao")
	s1.Incr("你好", "nihao")
	s1.Incr("你好", "nihao")
	s1.Incr("世界", "shijie")
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 第二次打开，验证数据还在
	s2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if got := s2.Get("你好", "nihao"); got != 3 {
		t.Errorf("after reopen, 你好|nihao = %v, want 3", got)
	}
	if got := s2.Get("世界", "shijie"); got != 1 {
		t.Errorf("after reopen, 世界|shijie = %v, want 1", got)
	}
}

func TestStoreClear(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "userdict-test-*")
	defer os.RemoveAll(tmpDir)
	s, _ := New(filepath.Join(tmpDir, "ud"))
	defer s.Close()

	s.Incr("a", "a")
	s.Incr("b", "b")
	if err := s.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if got := s.Get("a", "a"); got != 0 {
		t.Errorf("after Clear, a|a = %v, want 0", got)
	}
	if s.Stats().Entries != 0 {
		t.Errorf("after Clear, entries = %d, want 0", s.Stats().Entries)
	}
}

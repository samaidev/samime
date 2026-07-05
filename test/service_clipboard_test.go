//go:build integration

// Service 协议边缘测试（含 clipboard API）
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/zai/goime/internal/dict"
	"github.com/zai/goime/internal/engine"
)

// 复用 service_edge_test.go 的辅助函数
// 这里测试 clipboard 相关的新协议方法

func TestServiceClipboardHistoryEmpty(t *testing.T) {
	addr, cleanup := startTestService(t)
	defer cleanup()

	// 没有任何 commit 时查询 clipboard
	resp, err := sendReq(addr, "clipboard-history", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp["ok"] != true {
		t.Errorf("clipboard-history on empty should return ok=true, got %v", resp)
	}
	t.Logf("空 clipboard-history: %v", resp["error"])
}

func TestServiceClipboardHistoryAfterCommits(t *testing.T) {
	addr, cleanup := startTestService(t)
	defer cleanup()

	// 提交几个词
	for _, w := range []string{"你好", "世界", "中国"} {
		_, err := sendReq(addr, "commit", map[string]string{
			"word":   w,
			"pinyin": "py",
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// 查询 clipboard
	resp, err := sendReq(addr, "clipboard-history", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp["ok"] != true {
		t.Errorf("clipboard-history should return ok=true, got %v", resp)
	}
	// error 字段包含 JSON 数组
	errStr, ok := resp["error"].(string)
	if !ok {
		t.Fatal("error field should be string")
	}
	t.Logf("clipboard 历史: %s", errStr[:min(200, len(errStr))])
}

func TestServiceClipboardClear(t *testing.T) {
	addr, cleanup := startTestService(t)
	defer cleanup()

	// 提交几个词
	sendReq(addr, "commit", map[string]string{"word": "你好", "pinyin": "nihao"})

	// 清空
	resp, err := sendReq(addr, "clipboard-clear", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp["ok"] != true {
		t.Errorf("clipboard-clear should return ok=true, got %v", resp)
	}

	// 再次查询应该是空
	resp, _ = sendReq(addr, "clipboard-history", nil)
	if resp["ok"] != true {
		t.Errorf("clipboard-history after clear should return ok=true")
	}
}

func TestServiceClipboardMax50(t *testing.T) {
	addr, cleanup := startTestService(t)
	defer cleanup()

	// 提交 60 次
	for i := 0; i < 60; i++ {
		sendReq(addr, "commit", map[string]string{
			"word":   "词" + string(rune('a'+i%26)),
			"pinyin": "ci",
		})
	}

	// 查询应最多 50 条
	resp, _ := sendReq(addr, "clipboard-history", nil)
	errStr, _ := resp["error"].(string)
	// 解析 JSON 数组
	var entries []map[string]interface{}
	if err := json.Unmarshal([]byte(errStr), &entries); err != nil {
		t.Fatalf("parse clipboard: %v", err)
	}
	if len(entries) != 50 {
		t.Errorf("clipboard should have 50 entries, got %d", len(entries))
	}
}

func TestServiceSearchWithSpecialPinyin(t *testing.T) {
	addr, cleanup := startTestService(t)
	defer cleanup()

	// 各种特殊拼音
	cases := []string{
		"ni3hao3",   // 声调
		"ni hao",    // 空格
		"NIHAO",     // 大写
		"nihao!",    // 标点
		"",          // 空
		"   ",       // 空白
	}
	for _, c := range cases {
		resp, err := sendReq(addr, "search", map[string]string{"preedit": c})
		if err != nil {
			t.Errorf("search(%q) error: %v", c, err)
			continue
		}
		t.Logf("search(%q) -> ok=%v", c, resp["ok"])
	}
}

func TestServiceCommitWithUnicode(t *testing.T) {
	addr, cleanup := startTestService(t)
	defer cleanup()

	// 各种 Unicode 字符
	cases := []struct {
		word, pinyin string
	}{
		{"你好", "nihao"},
		{"🚀", "emoji"},
		{"日本語", "ribenyu"},
		{"한국어", "hangugeo"},
		{"Ελληνικά", "ellinika"},
		{"Русский", "russkiy"},
		{"مرحبا", "marhaban"},
	}
	for _, c := range cases {
		resp, err := sendReq(addr, "commit", map[string]string{
			"word":   c.word,
			"pinyin": c.pinyin,
		})
		if err != nil {
			t.Errorf("commit(%q) error: %v", c.word, err)
			continue
		}
		if resp["ok"] != true {
			t.Errorf("commit(%q) ok=%v", c.word, resp["ok"])
		}
	}
}

func TestServiceConcurrentClipboard(t *testing.T) {
	addr, cleanup := startTestService(t)
	defer cleanup()

	// 10 个并发客户端同时 commit 和查询 clipboard
	done := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				_, err := sendReq(addr, "commit", map[string]string{
					"word":   "并发词",
					"pinyin": "bingfaci",
				})
				if err != nil {
					done <- err
					return
				}
			}
			// 查询 clipboard
			_, err := sendReq(addr, "clipboard-history", nil)
			done <- err
		}(i)
	}
	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent error: %v", err)
		}
	}
}

func TestServiceLongRunningSession(t *testing.T) {
	addr, cleanup := startTestService(t)
	defer cleanup()

	// 模拟长时间使用：100 次 search + commit + clipboard 查询
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	for i := 0; i < 100; i++ {
		// search
		req := fmt.Sprintf(`{"method":"search","preedit":"nihao"}` + "\n")
		writer.WriteString(req)
		writer.Flush()
		reader.ReadBytes('\n')

		// commit
		req = fmt.Sprintf(`{"method":"commit","word":"你好","pinyin":"nihao"}` + "\n")
		writer.WriteString(req)
		writer.Flush()
		reader.ReadBytes('\n')

		// clipboard
		req = `{"method":"clipboard-history"}` + "\n"
		writer.WriteString(req)
		writer.Flush()
		reader.ReadBytes('\n')
	}
	t.Log("100 轮 search+commit+clipboard 完成")
}

// 防止未使用导入
var _ = strings.Contains
var _ = dict.LoadEmbedded
var _ = engine.NewDefault

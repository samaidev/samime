//go:build integration

// 服务端边缘测试：测试 samime service 的协议鲁棒性
package main

import (
        "bufio"
        "encoding/json"
        "fmt"
        "net"
        "os"
        "strings"
        "testing"
        "time"

        "github.com/zai/goime/internal/dict"
        "github.com/zai/goime/internal/engine"
)

// startTestService 启动一个测试 service（在随机端口）
// 返回 (地址, 清理函数)
func startTestService(t *testing.T) (string, func()) {
        d, err := dict.LoadEmbedded()
        if err != nil {
                t.Fatalf("load dict: %v", err)
        }
        eng, err := engine.NewWithUserStore(d, t.TempDir()+"/userdict")
        if err != nil {
                // 降级到内存模式
                eng = engine.NewDefault(d)
        }

        // 监听随机端口
        l, err := net.Listen("tcp", "127.0.0.1:0")
        if err != nil {
                t.Fatalf("listen: %v", err)
        }
        addr := l.Addr().String()

        go func() {
                for {
                        conn, err := l.Accept()
                        if err != nil {
                                return
                        }
                        go handleTestConn(conn, eng)
                }
        }()

        cleanup := func() {
                l.Close()
                eng.Close()
        }
        return addr, cleanup
}

func handleTestConn(conn net.Conn, eng *engine.Engine) {
        defer conn.Close()
        reader := bufio.NewReader(conn)
        writer := bufio.NewWriter(conn)
        defer writer.Flush()

        for {
                line, err := reader.ReadBytes('\n')
                if len(line) == 0 && err != nil {
                        return
                }
                for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
                        line = line[:len(line)-1]
                }
                if len(line) == 0 {
                        continue
                }

                var req struct {
                        Method  string `json:"method"`
                        Preedit string `json:"preedit,omitempty"`
                        Word    string `json:"word,omitempty"`
                        Pinyin  string `json:"pinyin,omitempty"`
                }
                if err := json.Unmarshal(line, &req); err != nil {
                        writeTestResp(writer, false, "", nil, "invalid json: "+err.Error())
                        continue
                }

                switch req.Method {
                case "ping":
                        writeTestResp(writer, true, "", nil, "")
                case "search":
                        cands := eng.Search(req.Preedit)
                        writeTestResp(writer, true, "", cands, "")
                case "commit":
                        eng.Commit(req.Word, req.Pinyin)
                        writeTestResp(writer, true, req.Word, nil, "")
                case "reset":
                        eng.ResetContext()
                        writeTestResp(writer, true, "", nil, "")
                case "status":
                        s := eng.Dict().Stats()
                        writeTestResp(writer, true, "", nil,
                                fmt.Sprintf("entries=%d pinyins=%d", s.TotalEntries, s.UniquePinyin))
                case "shutdown":
                        writeTestResp(writer, true, "", nil, "")
                        return
                default:
                        writeTestResp(writer, false, "", nil, "unknown method: "+req.Method)
                }
        }
}

func writeTestResp(w *bufio.Writer, ok bool, committed string, cands []engine.Candidate, errMsg string) {
        resp := struct {
                OK         bool                  `json:"ok"`
                Committed  string                `json:"committed,omitempty"`
                Candidates []engine.Candidate    `json:"candidates,omitempty"`
                Error      string                `json:"error,omitempty"`
        }{
                OK: ok, Committed: committed, Candidates: cands, Error: errMsg,
        }
        data, _ := json.Marshal(resp)
        w.Write(data)
        w.WriteByte('\n')
        w.Flush()
}

func sendRaw(addr, line string) (string, error) {
        conn, err := net.Dial("tcp", addr)
        if err != nil {
                return "", err
        }
        defer conn.Close()
        conn.SetDeadline(time.Now().Add(30 * time.Second))  // 长超时，避免 Windows 上慢查询
        if _, err := conn.Write([]byte(line)); err != nil {
                return "", err
        }
        buf := make([]byte, 65536)
        n, err := conn.Read(buf)
        if err != nil {
                return "", err
        }
        return string(buf[:n]), nil
}

func sendReq(addr, method string, extra map[string]string) (map[string]interface{}, error) {
        req := map[string]string{"method": method}
        for k, v := range extra {
                req[k] = v
        }
        data, _ := json.Marshal(req)
        resp, err := sendRaw(addr, string(data)+"\n")
        if err != nil {
                return nil, err
        }
        var m map[string]interface{}
        if err := json.Unmarshal([]byte(strings.TrimSpace(resp)), &m); err != nil {
                return nil, fmt.Errorf("parse %q: %w", resp, err)
        }
        return m, nil
}

// === 边缘测试用例 ===

func TestServiceInvalidJSON(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        cases := []string{
                "not a json\n",
                "{invalid\n",
                "{}\n",                        // 空 JSON 对象
                "[]\n",                        // 数组而非对象
                `{"method":}` + "\n",          // 语法错误
                `{"method":"search"}` + "\n",  // 缺 preedit
                `{"method":"unknown_method"}` + "\n",
        }
        for _, c := range cases {
                resp, err := sendRaw(addr, c)
                if err != nil {
                        t.Errorf("sendRaw(%q) error: %v", c, err)
                        continue
                }
                t.Logf("req=%q -> resp=%q", strings.TrimSpace(c), strings.TrimSpace(resp))
                // 不应 panic，应该返回错误响应
                if !strings.Contains(resp, "ok") {
                        t.Errorf("response should contain 'ok': %q", resp)
                }
        }
}

func TestServiceUnknownMethod(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        resp, err := sendReq(addr, "nonexistent", nil)
        if err != nil {
                t.Fatal(err)
        }
        if resp["ok"] != false {
                t.Errorf("unknown method should return ok=false, got %v", resp)
        }
}

func TestServicePing(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        resp, err := sendReq(addr, "ping", nil)
        if err != nil {
                t.Fatal(err)
        }
        if resp["ok"] != true {
                t.Errorf("ping should return ok=true, got %v", resp)
        }
}

func TestServiceSearchEmpty(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 空 preedit
        resp, err := sendReq(addr, "search", map[string]string{"preedit": ""})
        if err != nil {
                t.Fatal(err)
        }
        t.Logf("search('') -> %v", resp)
}

func TestServiceSearchNonAlpha(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        cases := []string{"123", "!@#", "你好", "\x00\x01"}
        for _, c := range cases {
                resp, err := sendReq(addr, "search", map[string]string{"preedit": c})
                if err != nil {
                        t.Errorf("search(%q) error: %v", c, err)
                        continue
                }
                t.Logf("search(%q) -> ok=%v", c, resp["ok"])
        }
}

func TestServiceSearchVeryLong(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 超长 preedit
        long := strings.Repeat("a", 1000)
        resp, err := sendReq(addr, "search", map[string]string{"preedit": long})
        if err != nil {
                t.Fatal(err)
        }
        t.Logf("search(1000 'a') -> ok=%v", resp["ok"])
}

func TestServiceCommitWithoutWord(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // commit 缺 word 字段：当前实现接受空 word（不报错，但什么都不做）
        resp, err := sendReq(addr, "commit", map[string]string{})
        if err != nil {
                t.Fatal(err)
        }
        // 行为：返回 ok=true（无害操作）
        t.Logf("commit without word -> %v (accepted silently)", resp)
}

func TestServiceCommitVeryLongWord(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        longWord := strings.Repeat("你", 100)
        resp, err := sendReq(addr, "commit", map[string]string{
                "word":   longWord,
                "pinyin": "ni",
        })
        if err != nil {
                t.Fatal(err)
        }
        if resp["ok"] != true {
                t.Errorf("commit long word failed: %v", resp)
        }
}

func TestServiceStatus(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        resp, err := sendReq(addr, "status", nil)
        if err != nil {
                t.Fatal(err)
        }
        if resp["ok"] != true {
                t.Errorf("status failed: %v", resp)
        }
        if errStr, ok := resp["error"].(string); !ok || !strings.Contains(errStr, "entries=") {
                t.Errorf("status should return entries info, got %v", resp["error"])
        }
}

func TestServiceReset(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        resp, err := sendReq(addr, "reset", nil)
        if err != nil {
                t.Fatal(err)
        }
        if resp["ok"] != true {
                t.Errorf("reset failed: %v", resp)
        }
}

func TestServicePersistenceAcrossConnections(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 连接 1：commit
        resp, err := sendReq(addr, "commit", map[string]string{
                "word": "你好", "pinyin": "nihao",
        })
        if err != nil || resp["ok"] != true {
                t.Fatalf("commit: %v / %v", resp, err)
        }

        // 连接 2：search 应该看到 commit 效果
        resp, err = sendReq(addr, "search", map[string]string{"preedit": "nihao"})
        if err != nil {
                t.Fatal(err)
        }
        cands, ok := resp["candidates"].([]interface{})
        if !ok || len(cands) == 0 {
                t.Fatal("no candidates")
        }
        first := cands[0].(map[string]interface{})
        if first["Word"] != "你好" {
                t.Errorf("after commit, top1 should be 你好, got %v", first["Word"])
        }
}

func TestServiceConcurrentClients(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 10 个并发客户端
        done := make(chan error, 10)
        for i := 0; i < 10; i++ {
                go func(id int) {
                        for j := 0; j < 20; j++ {
                                _, err := sendReq(addr, "search", map[string]string{
                                        "preedit": "nihao",
                                })
                                if err != nil {
                                        done <- err
                                        return
                                }
                        }
                        done <- nil
                }(i)
        }
        for i := 0; i < 10; i++ {
                if err := <-done; err != nil {
                        t.Errorf("client error: %v", err)
                }
        }
}

func TestServiceClientDisconnectAbruptly(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 客户端连上后立即断开
        conn, err := net.Dial("tcp", addr)
        if err != nil {
                t.Fatal(err)
        }
        conn.Close()
        // 不应导致服务崩溃
        time.Sleep(100 * time.Millisecond)

        // 验证服务还活着
        resp, err := sendReq(addr, "ping", nil)
        if err != nil || resp["ok"] != true {
                t.Errorf("service should still respond after client disconnect: %v / %v", resp, err)
        }
}

func TestServicePartialJSON(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 发送半个 JSON，再发剩下的
        conn, err := net.Dial("tcp", addr)
        if err != nil {
                t.Fatal(err)
        }
        defer conn.Close()
        conn.SetDeadline(time.Now().Add(5 * time.Second))

        // 发送前半部分
        conn.Write([]byte(`{"method":"pi`))
        time.Sleep(100 * time.Millisecond)
        // 发送后半部分
        conn.Write([]byte(`ng"}` + "\n"))

        buf := make([]byte, 1024)
        n, _ := conn.Read(buf)
        resp := string(buf[:n])
        if !strings.Contains(resp, `"ok":true`) {
                t.Errorf("partial JSON should still work, got %q", resp)
        }
}

func TestServiceManySmallWrites(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        conn, err := net.Dial("tcp", addr)
        if err != nil {
                t.Fatal(err)
        }
        defer conn.Close()
        conn.SetDeadline(time.Now().Add(5 * time.Second))

        // 逐字节发送
        req := `{"method":"ping"}` + "\n"
        for _, c := range req {
                conn.Write([]byte(string(c)))
                time.Sleep(2 * time.Millisecond)
        }

        buf := make([]byte, 1024)
        n, _ := conn.Read(buf)
        resp := string(buf[:n])
        if !strings.Contains(resp, `"ok":true`) {
                t.Errorf("byte-by-byte send failed: %q", resp)
        }
}

func TestServiceLargeResponse(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 查询返回大量候选
        resp, err := sendReq(addr, "search", map[string]string{"preedit": "yi"})
        if err != nil {
                t.Fatal(err)
        }
        cands, _ := resp["candidates"].([]interface{})
        t.Logf("search('yi') -> %d candidates", len(cands))
}

func TestServiceMultipleQueries(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 同一连接发多个请求
        conn, err := net.Dial("tcp", addr)
        if err != nil {
                t.Fatal(err)
        }
        defer conn.Close()
        conn.SetDeadline(time.Now().Add(10 * time.Second))

        queries := []string{
                `{"method":"ping"}`,
                `{"method":"search","preedit":"nihao"}`,
                `{"method":"search","preedit":"zhongguo"}`,
                `{"method":"commit","word":"你好","pinyin":"nihao"}`,
                `{"method":"status"}`,
                `{"method":"reset"}`,
        }
        for _, q := range queries {
                conn.Write([]byte(q + "\n"))
                buf := make([]byte, 65536)
                n, _ := conn.Read(buf)
                t.Logf("req=%s -> resp=%s", q, strings.TrimSpace(string(buf[:n])))
        }
}

func TestServiceNoNewlineAtEnd(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 不带换行的请求
        resp, err := sendRaw(addr, `{"method":"ping"}`)
        if err != nil {
                // 没有 \n 时服务端可能阻塞等待
                t.Logf("no-newline request blocked (expected): %v", err)
                return
        }
        t.Logf("no-newline -> %q", strings.TrimSpace(resp))
}

func TestServiceUTF8InRequest(t *testing.T) {
        addr, cleanup := startTestService(t)
        defer cleanup()

        // 直接用中文 word 提交
        resp, err := sendReq(addr, "commit", map[string]string{
                "word":   "你好世界",
                "pinyin": "nihaoshijie",
        })
        if err != nil {
                t.Fatal(err)
        }
        if resp["ok"] != true {
                t.Errorf("UTF-8 commit failed: %v", resp)
        }
}

// 防止未使用导入
var _ = os.Stderr

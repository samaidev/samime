//go:build !windows && !darwin

// Linux 等非 Windows/macOS 平台：用 TCP 模拟命名管道服务（开发测试用）
package main

import (
        "bufio"
        "encoding/json"
        "fmt"
        "net"
        "os"
        "os/signal"
        "syscall"

        "github.com/zai/goime/internal/engine"
)

const tcpAddr = "127.0.0.1:7788"

func runServicePlatform(eng *engine.Engine) {
        l, err := net.Listen("tcp", tcpAddr)
        if err != nil {
                fmt.Fprintf(os.Stderr, "[service] listen %s failed: %v\n", tcpAddr, err)
                os.Exit(1)
        }
        defer l.Close()
        fmt.Fprintf(os.Stderr, "[service] TCP listening on %s (dev mode)\n", tcpAddr)
        fmt.Fprintf(os.Stderr, "[service] 协议: JSON over line-delimited\n")
        fmt.Fprintf(os.Stderr, "[service] 示例请求: {\"method\":\"search\",\"preedit\":\"nihao\"}\n")

        // 信号处理
        sigCh := make(chan os.Signal, 1)
        signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
        go func() {
                <-sigCh
                fmt.Fprintln(os.Stderr, "[service] shutting down...")
                l.Close()
                os.Exit(0)
        }()

        for {
                conn, err := l.Accept()
                if err != nil {
                        return
                }
                go handleServiceConn(conn, eng)
        }
}

func handleServiceConn(conn net.Conn, eng *engine.Engine) {
        defer conn.Close()
        reader := bufio.NewReader(conn)
        writer := bufio.NewWriter(conn)
        defer writer.Flush()

        for {
                line, err := reader.ReadBytes('\n')
                if len(line) == 0 && err != nil {
                        return
                }
                // 去掉换行
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
                        writeServiceResp(writer, false, "", nil, err.Error())
                        continue
                }

                switch req.Method {
                case "ping":
                        writeServiceResp(writer, true, "", nil, "")
                case "search":
                        cands := eng.Search(req.Preedit)
                        writeServiceResp(writer, true, "", cands, "")
                case "commit":
                        eng.Commit(req.Word, req.Pinyin)
                        writeServiceResp(writer, true, req.Word, nil, "")
                case "reset":
                        eng.ResetContext()
                        writeServiceResp(writer, true, "", nil, "")
                case "status":
                        s := eng.Dict().Stats()
                        writeServiceResp(writer, true, "", nil,
                                fmt.Sprintf("entries=%d pinyins=%d", s.TotalEntries, s.UniquePinyin))
                case "shutdown":
                        writeServiceResp(writer, true, "", nil, "")
                        return
                default:
                        writeServiceResp(writer, false, "", nil, "unknown method: "+req.Method)
                }
        }
}

func writeServiceResp(w *bufio.Writer, ok bool, committed string, cands []engine.Candidate, errMsg string) {
        resp := struct {
                OK          bool                  `json:"ok"`
                Committed   string                `json:"committed,omitempty"`
                Candidates  []engine.Candidate    `json:"candidates,omitempty"`
                Error       string                `json:"error,omitempty"`
        }{
                OK:         ok,
                Committed:  committed,
                Candidates: cands,
                Error:      errMsg,
        }
        data, _ := json.Marshal(resp)
        w.Write(data)
        w.WriteByte('\n')
        w.Flush()
}

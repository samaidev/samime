//go:build !windows && !darwin

// Linux 等非 Windows/macOS 平台
// - 如果检测到 IBUS_ADDRESS 环境变量，启动 IBus engine 模式（被 ibus-daemon 调用时）
// - 否则启动 TCP 服务模式（开发测试用）
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
        "github.com/zai/goime/internal/ibus"
)

const tcpAddr = "127.0.0.1:7788"

func runServicePlatform(eng *engine.Engine) {
        // 检测是否应该启动 IBus engine 模式
        // 1. 检查 --ibus 标志
        // 2. 检查 IBUS_ADDRESS 环境变量
        // 3. 检查 IBus bus 文件是否存在
        ibusAddr := os.Getenv("IBUS_ADDRESS")
        if ibusAddr == "" {
                ibusAddr = ibus.ReadIBusAddressFromBusFile()
        }
        if ibusAddr != "" || hasIBusFlag {
                // IBus engine 模式：连接 IBus D-Bus，接收按键事件
                fmt.Fprintf(os.Stderr, "[service] IBus engine mode (IBUS_ADDRESS detected)\n")
                ibusEng := ibus.NewEngine(eng, "samime", "/org/freedesktop/IBus/Samime")
                if err := ibusEng.Run(); err != nil {
                        fmt.Fprintf(os.Stderr, "[service] IBus engine error: %v\n", err)
                        os.Exit(1)
                }
                return
        }

        // TCP 服务模式（开发测试用）
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
                case "clipboard-history":
                        // 返回最近 50 条提交
                        if eng.Clipboard() != nil {
                                entries := eng.Clipboard().Recent(50)
                                // 序列化为 JSON 字符串放在 Error 字段（兼容协议）
                                data, _ := json.Marshal(entries)
                                writeServiceResp(writer, true, "", nil, string(data))
                        } else {
                                writeServiceResp(writer, false, "", nil, "clipboard not enabled")
                        }
                case "clipboard-clear":
                        if eng.Clipboard() != nil {
                                eng.Clipboard().Clear()
                                writeServiceResp(writer, true, "", nil, "")
                        } else {
                                writeServiceResp(writer, false, "", nil, "clipboard not enabled")
                        }
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

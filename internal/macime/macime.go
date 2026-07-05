//go:build darwin

// Package macime - macOS IMK 适配
// 通过 Unix Domain Socket 与 Swift IMK bundle 通信
package macime

import (
        "bufio"
        "encoding/json"
        "fmt"
        "log"
        "net"
        "os"
        "sync"
        "syscall"

        "github.com/zai/goime/internal/engine"
)

// Server Unix Domain Socket 服务
type Server struct {
        engine    *engine.Engine
        socketPath string
        listener  net.Listener
        mu        sync.Mutex
        connected int

        OnRequest  func(method string)
        OnResponse func(method string, success bool)
}

// New 创建服务
func New(eng *engine.Engine, socketPath string) (*Server, error) {
        if socketPath == "" {
                // macOS 标准 IMK socket 位置
                home, _ := os.UserHomeDir()
                socketPath = home + "/.samime/macime.sock"
        }
        return &Server{
                engine:     eng,
                socketPath: socketPath,
        }, nil
}

// ListenAndServe 启动服务
func (s *Server) ListenAndServe() error {
        // 删除旧 socket 文件
        if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
                return fmt.Errorf("remove old socket: %w", err)
        }

        // 设置 umask 确保 socket 可访问
        oldMask := syscall.Umask(0o077)
        defer syscall.Umask(oldMask)

        l, err := net.Listen("unix", s.socketPath)
        if err != nil {
                return fmt.Errorf("listen unix: %w", err)
        }
        s.listener = l
        log.Printf("[macime] listening on %s", s.socketPath)

        for {
                conn, err := l.Accept()
                if err != nil {
                        return err
                }
                s.mu.Lock()
                s.connected++
                s.mu.Unlock()
                go s.handleConn(conn)
        }
}

func (s *Server) handleConn(conn net.Conn) {
        defer conn.Close()
        defer func() {
                s.mu.Lock()
                s.connected--
                s.mu.Unlock()
        }()

        reader := bufio.NewReader(conn)
        writer := bufio.NewWriter(conn)
        defer writer.Flush()

        for {
                line, err := reader.ReadBytes('\n')
                if len(line) == 0 && err != nil {
                        return
                }
                line = trimNewline(line)
                if len(line) == 0 {
                        continue
                }

                var req serviceRequest
                if err := json.Unmarshal(line, &req); err != nil {
                        s.writeResp(writer, false, "", nil, err.Error())
                        continue
                }

                if s.OnRequest != nil {
                        s.OnRequest(req.Method)
                }

                resp := s.handleRequest(&req, conn)
                s.writeResp(writer, resp.OK, resp.Committed, resp.Candidates, resp.Error)

                if s.OnResponse != nil {
                        s.OnResponse(req.Method, resp.OK)
                }

                if req.Method == "shutdown" {
                        return
                }
        }
}

type serviceResponse struct {
        OK         bool
        Committed  string
        Candidates []engine.Candidate
        Error      string
}

func (s *Server) handleRequest(req *serviceRequest, _ net.Conn) *serviceResponse {
        switch req.Method {
        case "ping":
                return &serviceResponse{OK: true}
        case "search":
                cands := s.engine.Search(req.Preedit)
                return &serviceResponse{OK: true, Candidates: cands}
        case "commit":
                s.engine.Commit(req.Word, req.Pinyin)
                return &serviceResponse{OK: true, Committed: req.Word}
        case "reset":
                s.engine.ResetContext()
                return &serviceResponse{OK: true}
        case "status":
                st := s.engine.Dict().Stats()
                return &serviceResponse{OK: true, Error: fmt.Sprintf("entries=%d", st.TotalEntries)}
        default:
                return &serviceResponse{OK: false, Error: "unknown method: " + req.Method}
        }
}

type serviceRequest struct {
        Method  string `json:"method"`
        Preedit string `json:"preedit,omitempty"`
        Word    string `json:"word,omitempty"`
        Pinyin  string `json:"pinyin,omitempty"`
}

func (s *Server) writeResp(w *bufio.Writer, ok bool, committed string, cands []engine.Candidate, errMsg string) {
        resp := struct {
                OK         bool                  `json:"ok"`
                Committed  string                `json:"committed,omitempty"`
                Candidates []engine.Candidate    `json:"candidates,omitempty"`
                Error      string                `json:"error,omitempty"`
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

func trimNewline(b []byte) []byte {
        for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
                b = b[:len(b)-1]
        }
        return b
}

// Close 关闭
func (s *Server) Close() error {
        if s.listener != nil {
                err := s.listener.Close()
                os.Remove(s.socketPath)
                return err
        }
        return nil
}

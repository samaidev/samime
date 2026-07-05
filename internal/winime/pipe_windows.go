//go:build windows

// Package winime - 命名管道服务
// 接收 TSF proxy (C++) 的请求，返回候选词
//
// 协议（JSON over line-delimited）:
//   请求: {"method":"search","preedit":"nihao"}
//         {"method":"commit","word":"你好","pinyin":"nihao"}
//         {"method":"reset"}
//         {"method":"status"}
//   响应: {"candidates":[...],"committed":"...","ok":true}
package winime

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"

	"github.com/Microsoft/go-winio"
	"github.com/zai/goime/internal/engine"
)

// PipeServer 命名管道服务
type PipeServer struct {
	engine    *engine.Engine
	pipeName  string
	listener  net.Listener
	mu        sync.Mutex
	connected int

	OnRequest  func(method string)
	OnResponse func(method string, success bool)
}

// NewPipeServer 创建命名管道服务
func NewPipeServer(eng *engine.Engine, pipeName string) (*PipeServer, error) {
	if pipeName == "" {
		pipeName = `\\.\pipe\goime`
	}
	return &PipeServer{
		engine:   eng,
		pipeName: pipeName,
	}, nil
}

// ListenAndServe 启动服务（阻塞）
func (s *PipeServer) ListenAndServe() error {
	l, err := winio.ListenPipe(s.pipeName, nil)
	if err != nil {
		return fmt.Errorf("listen pipe %s: %w", s.pipeName, err)
	}
	s.listener = l
	log.Printf("[pipe] listening on %s", s.pipeName)

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

// handleConn 处理一个连接
func (s *PipeServer) handleConn(conn net.Conn) {
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

		var req PipeRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(writer, fmt.Sprintf("invalid json: %v", err))
			continue
		}

		if s.OnRequest != nil {
			s.OnRequest(req.Method)
		}

		resp := s.handleRequest(&req)
		s.writeResponse(writer, resp)

		if s.OnResponse != nil {
			s.OnResponse(req.Method, resp.OK)
		}

		if req.Method == "shutdown" {
			return
		}
	}
}

func (s *PipeServer) handleRequest(req *PipeRequest) *PipeResponse {
	switch req.Method {
	case "search":
		return s.handleSearch(req)
	case "commit":
		return s.handleCommit(req)
	case "reset":
		s.engine.ResetContext()
		return &PipeResponse{OK: true}
	case "status":
		return s.handleStatus()
	case "shutdown":
		return &PipeResponse{OK: true}
	case "ping":
		return &PipeResponse{OK: true}
	default:
		return &PipeResponse{OK: false, Error: fmt.Sprintf("unknown method: %s", req.Method)}
	}
}

func (s *PipeServer) handleSearch(req *PipeRequest) *PipeResponse {
	if req.Preedit == "" {
		return &PipeResponse{OK: false, Error: "missing preedit"}
	}
	cands := s.engine.Search(req.Preedit)
	out := make([]engine.Candidate, len(cands))
	copy(out, cands)
	return &PipeResponse{Candidates: out, OK: true}
}

func (s *PipeServer) handleCommit(req *PipeRequest) *PipeResponse {
	if req.Word == "" {
		return &PipeResponse{OK: false, Error: "missing word"}
	}
	s.engine.Commit(req.Word, req.Pinyin)
	return &PipeResponse{OK: true, Committed: req.Word}
}

func (s *PipeServer) handleStatus() *PipeResponse {
	stats := s.engine.Dict().Stats()
	return &PipeResponse{
		OK:    true,
		Error: fmt.Sprintf("entries=%d pinyins=%d", stats.TotalEntries, stats.UniquePinyin),
	}
}

func (s *PipeServer) writeError(w *bufio.Writer, msg string) {
	s.writeResponse(w, &PipeResponse{OK: false, Error: msg})
}

func (s *PipeServer) writeResponse(w *bufio.Writer, resp *PipeResponse) {
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

// ConnectionCount 当前连接数
func (s *PipeServer) ConnectionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.connected
}

// Close 关闭服务
func (s *PipeServer) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

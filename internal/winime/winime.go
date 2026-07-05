//go:build windows

// Package winime 提供 Windows TSF (Text Services Framework) 适配层
//
// 由于 Go 无法直接调用 COM 接口，本包仅提供：
// 1. Windows 平台初始化逻辑
// 2. 通过命名管道与外部 TSF proxy (C++) 通信的接口
// 3. 控制台交互模式（开发测试用）
//
// 实际的 TSF COM 实现需要 C++ 写薄壳，通过命名管道调用 Go 引擎
package winime

import (
	"fmt"
	"os"
	"strings"

	"github.com/zai/goime/internal/engine"
	"github.com/zai/goime/internal/ibus"
)

// Run Windows 上启动 IME
// 模式：
//   - "console": 控制台交互模式（开发测试用）
//   - "service": 作为命名管道服务（供 TSF proxy 调用）
//   - "auto": 优先 service，回退 console
func Run(eng *engine.Engine, mode string) error {
	switch strings.ToLower(mode) {
	case "console":
		return runConsole(eng)
	case "service":
		return runService(eng)
	case "auto", "":
		if os.Getenv("GOIME_PIPE_NAME") != "" {
			return runService(eng)
		}
		return runConsole(eng)
	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}
}

// runConsole 控制台模式（复用 ibus 的 stdin 实现）
func runConsole(eng *engine.Engine) error {
	ibusEng := ibus.New(eng)
	fmt.Fprintln(os.Stderr, "[goime] Windows console mode - type pinyin + Enter to commit")
	return ibusEng.Run()
}

// runService 命名管道服务模式
// TODO: 完整实现，监听 \\.\pipe\goime，接收 TSF proxy 的请求
func runService(eng *engine.Engine) error {
	pipeName := os.Getenv("GOIME_PIPE_NAME")
	if pipeName == "" {
		pipeName = `\\.\pipe\goime`
	}
	return fmt.Errorf("named pipe service not yet implemented (planned pipe: %s)", pipeName)
}

// PipeProtocol 定义与 TSF proxy 的通信协议（JSON over line-delimited）
//
// 请求 (TSF proxy -> Go engine):
//   {"method":"search","preedit":"nihao"}
//   {"method":"commit","word":"你好","pinyin":"nihao"}
//   {"method":"reset"}
//
// 响应 (Go engine -> TSF proxy):
//   {"candidates":[{"word":"你好","pinyin":"nihao","score":100.0}]}
//   {"committed":"你好"}
//   {"ok":true}
type PipeRequest struct {
	Method  string `json:"method"`
	Preedit string `json:"preedit,omitempty"`
	Word    string `json:"word,omitempty"`
	Pinyin  string `json:"pinyin,omitempty"`
}

type PipeResponse struct {
	Candidates []engine.Candidate `json:"candidates,omitempty"`
	Committed  string             `json:"committed,omitempty"`
	OK         bool               `json:"ok,omitempty"`
	Error      string             `json:"error,omitempty"`
}

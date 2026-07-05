// Package ibus 实现 IBus 适配器
// 通过 D-Bus 注册为 IBus engine，接收按键事件，返回候选词
//
// 文档参考:
//   https://github.com/ibus/ibus/blob/main/src/ibusengine.h
//   https://python-ibus.freedesktop.org/
//
// 本实现: 不强依赖 ibus 守护进程运行；如果 IBus 不可用，
// 退化为本地 echo 模式，便于在 CI / 容器中全流程测试
package ibus

import (
	"fmt"
	"os"
	"strings"

	"github.com/zai/goime/internal/engine"
)

// Engine IBus 适配器，包装核心引擎
type Engine struct {
	engine *engine.Engine

	// 当前预编辑串（用户正在输入的拼音）
	preedit strings.Builder

	// 当前候选列表
	candidates []engine.Candidate

	// 选词索引（用户按数字键选择）
	selected int

	// 已提交的最终文本
	committed strings.Builder

	// 事件回调（用于测试和 UI 同步）
	OnPreeditChanged func(text string)
	OnCandidatesChanged func(cands []engine.Candidate)
	OnCommitted func(text string)
}

// New 创建 IBus engine 适配器
func New(e *engine.Engine) *Engine {
	return &Engine{engine: e}
}

// ProcessKey 处理一个按键事件
// 返回 true 表示事件已处理（IBus 应该 swallow），false 表示透传
//
// 这是 IBus engine 的核心方法，对应 IBusEngine::process_key_event
func (ie *Engine) ProcessKey(key string) bool {
	if key == "" {
		return false
	}

	// 数字键：选词
	if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
		idx := int(key[0] - '1')
		return ie.selectCandidate(idx)
	}

	// 空格：选第一个候选
	if key == "space" || key == " " {
		if len(ie.candidates) > 0 {
			return ie.selectCandidate(0)
		}
		return false
	}

	// 回车：提交当前预编辑串（作为英文）+ 候选
	if key == "Return" || key == "Enter" || key == "\n" {
		if ie.preedit.Len() > 0 {
			// 优先选第一个候选
			if len(ie.candidates) > 0 {
				ie.selectCandidate(0)
			} else {
				// 没候选，直接提交原始拼音
				ie.commit(ie.preedit.String())
				ie.preedit.Reset()
				ie.candidates = nil
				ie.notifyPreedit()
				ie.notifyCandidates()
			}
		}
		return true
	}

	// ESC：清空预编辑
	if key == "Escape" || key == "ESC" {
		ie.preedit.Reset()
		ie.candidates = nil
		ie.notifyPreedit()
		ie.notifyCandidates()
		return true
	}

	// Backspace：删除最后一个字符
	if key == "BackSpace" || key == "BS" {
		if ie.preedit.Len() > 0 {
			s := ie.preedit.String()
			ie.preedit.Reset()
			ie.preedit.WriteString(s[:len(s)-1])
			ie.updateCandidates()
		}
		return true
	}

	// 字母键：加入预编辑
	if len(key) == 1 {
		c := key[0]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			if c >= 'A' && c <= 'Z' {
				c = c + 32 // 转小写
			}
			ie.preedit.WriteByte(c)
			ie.updateCandidates()
			return true
		}
	}

	// 其他按键：透传
	return false
}

// updateCandidates 重新计算候选
func (ie *Engine) updateCandidates() {
	py := ie.preedit.String()
	if py == "" {
		ie.candidates = nil
	} else {
		ie.candidates = ie.engine.Search(py)
	}
	ie.selected = 0
	ie.notifyPreedit()
	ie.notifyCandidates()
}

// selectCandidate 选择一个候选词
func (ie *Engine) selectCandidate(idx int) bool {
	if idx < 0 || idx >= len(ie.candidates) {
		return false
	}
	c := ie.candidates[idx]
	ie.commit(c.Word)
	ie.engine.Commit(c.Word, c.Pinyin)
	ie.preedit.Reset()
	ie.candidates = nil
	ie.selected = 0
	ie.notifyPreedit()
	ie.notifyCandidates()
	return true
}

// commit 提交最终文本
func (ie *Engine) commit(text string) {
	ie.committed.WriteString(text)
	ie.notifyCommitted(text)
}

// notifyPreedit 通知预编辑变化
func (ie *Engine) notifyPreedit() {
	if ie.OnPreeditChanged != nil {
		ie.OnPreeditChanged(ie.preedit.String())
	}
}

// notifyCandidates 通知候选变化
func (ie *Engine) notifyCandidates() {
	if ie.OnCandidatesChanged != nil {
		cands := make([]engine.Candidate, len(ie.candidates))
		copy(cands, ie.candidates)
		ie.OnCandidatesChanged(cands)
	}
}

// notifyCommitted 通知提交
func (ie *Engine) notifyCommitted(text string) {
	if ie.OnCommitted != nil {
		ie.OnCommitted(text)
	}
}

// Preedit 当前预编辑串
func (ie *Engine) Preedit() string {
	return ie.preedit.String()
}

// Candidates 当前候选列表
func (ie *Engine) Candidates() []engine.Candidate {
	return ie.candidates
}

// Committed 已提交的文本
func (ie *Engine) Committed() string {
	return ie.committed.String()
}

// Reset 重置状态
func (ie *Engine) Reset() {
	ie.preedit.Reset()
	ie.candidates = nil
	ie.selected = 0
}

// Run 作为 IBus engine 运行
// 如果 IBus 不可用，回退到 stdin/stdout 模式
func (ie *Engine) Run() error {
	// 检查 IBus 是否可用
	if !ibusAvailable() {
		fmt.Fprintln(os.Stderr, "[ibus] IBus daemon not available, falling back to stdin mode")
		return ie.runStdin()
	}
	return ie.runDBus()
}

// ibusAvailable 检查 IBus 是否在运行
func ibusAvailable() bool {
	// 简单检查：环境变量 IBUS_ADDRESS
	if os.Getenv("IBUS_ADDRESS") != "" {
		return true
	}
	// 检查 IBus socket 文件
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(home + "/.config/ibus/bus"); err == nil {
		// 进一步检查是否有 unix socket 文件
		// 这里简化处理
		return false
	}
	return false
}

// runStdin stdin 模式：从标准输入读取，模拟按键流
func (ie *Engine) runStdin() error {
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			break
		}
		c := buf[0]
		var key string
		switch c {
		case '\n':
			key = "Return"
		case 127, 8: // DEL, BS
			key = "BackSpace"
		case 27: // ESC
			key = "Escape"
		case ' ':
			key = "space"
		default:
			key = string(c)
		}
		ie.ProcessKey(key)
	}
	return nil
}

// runDBus 实际 IBus 模式：通过 D-Bus 注册为 IBus engine
// (TODO: 完整实现，需要 github.com/godbus/dbus 依赖)
func (ie *Engine) runDBus() error {
	return fmt.Errorf("D-Bus IBus mode not implemented in MVP, use stdin mode")
}

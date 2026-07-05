// Package fcitx5 implements a Fcitx5 addon adapter for GoIME.
//
// Fcitx5 通过 D-Bus 与 addon 通信，但 addon 模型与 IBus 不同：
//   - Fcitx5 addon 是 shared library（.so）或独立进程
//   - 通过 fcitx5 的 frontend 模块调用（不是直接 D-Bus）
//   - 可用 Go 实现 + IPC 桥接
//
// 本包提供两种集成方式：
//   1. Standalone: 独立进程，监听 D-Bus（与 IBus 类似）
//   2. IPC Bridge: 通过 Unix Socket 桥接（与 Windows TSF 模式相同）
//
// 推荐: Standalone 模式 + Fcitx5 D-Bus API
package fcitx5

import (
	"fmt"
	"log"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/zai/goime/internal/engine"
)

// Engine Fcitx5 输入法引擎
type Engine struct {
	engine *engine.Engine

	conn     *dbus.Conn
	busName  string
	objPath  dbus.ObjectPath
	mu       sync.Mutex
	running  bool

	// Fcitx5 输入法状态
	preedit string
	cands   []engine.Candidate

	// Fcitx5 输入法描述
	imName string
	imDesc string
}

// New 创建 Fcitx5 引擎
func New(eng *engine.Engine) *Engine {
	return &Engine{
		engine:  eng,
		busName: "org.fcitx.Fcitx5.Samime",
		objPath: "/samime",
		imName:  "Samime",
		imDesc:  "Samime Chinese Input Method (Go)",
	}
}

// Run 启动 Fcitx5 引擎服务
func (fe *Engine) Run() error {
	fe.mu.Lock()
	if fe.running {
		fe.mu.Unlock()
		return fmt.Errorf("already running")
	}
	fe.running = true
	fe.mu.Unlock()

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect session bus: %w", err)
	}
	fe.conn = conn
	defer conn.Close()

	reply, err := conn.RequestName(fe.busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("request name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("bus name already taken")
	}

	// 导出方法
	if err := conn.Export(fe, fe.objPath, "org.fcitx.Fcitx.Samime1"); err != nil {
		return fmt.Errorf("export: %w", err)
	}

	log.Printf("[fcitx5] Listening on %s %s", fe.busName, fe.objPath)
	log.Printf("[fcitx5] Restart fcitx5 to activate: fcitx5 -r")

	fe.loopSignals()
	return nil
}

func (fe *Engine) loopSignals() {
	c := make(chan *dbus.Signal, 100)
	fe.conn.Signal(c)
	for sig := range c {
		fe.handleSignal(sig)
	}
}

func (fe *Engine) handleSignal(sig *dbus.Signal) {
	log.Printf("[fcitx5] signal: %s", sig.Name)
}

// === Fcitx5 InputMethod 接口方法 ===

// ProcessKeyEvent 处理按键事件
// 参数: keyval (uint32), keycode (uint32), state (uint32), type (uint32)
// 返回: processed (bool)
func (fe *Engine) ProcessKeyEvent(keyval, keycode, state, keytype uint32) (bool, *dbus.Error) {
	fe.mu.Lock()
	defer fe.mu.Unlock()

	// 数字键 1-9
	if keyval >= '1' && keyval <= '9' {
		idx := int(keyval - '1')
		return fe.commitCandidate(idx), nil
	}

	// 空格
	if keyval == 32 { // space
		if fe.preedit != "" && len(fe.cands) > 0 {
			return fe.commitCandidate(0), nil
		}
		return false, nil
	}

	// 回车 (GDK_KEY_Return = 0xff0d = 65293)
	if keyval == 65293 {
		if fe.preedit != "" {
			if len(fe.cands) > 0 {
				return fe.commitCandidate(0), nil
			}
			fe.commitText(fe.preedit)
			fe.reset()
			return true, nil
		}
		return false, nil
	}

	// ESC
	if keyval == 65307 {
		if fe.preedit != "" {
			fe.reset()
			return true, nil
		}
		return false, nil
	}

	// 退格
	if keyval == 65288 {
		if fe.preedit != "" {
			fe.preedit = fe.preedit[:len(fe.preedit)-1]
			fe.updateCandidates()
			return true, nil
		}
		return false, nil
	}

	// 字母键
	if (keyval >= 'a' && keyval <= 'z') || (keyval >= 'A' && keyval <= 'Z') {
		ch := rune(keyval)
		if ch >= 'A' && ch <= 'Z' {
			ch = ch + 32
		}
		fe.preedit += string(ch)
		fe.updateCandidates()
		return true, nil
	}

	return false, nil
}

// Reset 重置
func (fe *Engine) Reset() *dbus.Error {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.reset()
	return nil
}

// FocusIn 焦点进入
func (fe *Engine) FocusIn() *dbus.Error { return nil }

// FocusOut 焦点离开
func (fe *Engine) FocusOut() *dbus.Error {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.reset()
	return nil
}

// SetCursorRect 设置光标矩形
func (fe *Engine) SetCursorRect(x, y, w, h int32) *dbus.Error {
	return nil
}

// CurrentIM 当前输入法信息
func (fe *Engine) CurrentIM() (string, string, *dbus.Error) {
	return fe.imName, "zh_CN", nil
}

// === 内部方法 ===

func (fe *Engine) updateCandidates() {
	fe.cands = fe.engine.Search(fe.preedit)
	fe.emitSignals()
}

func (fe *Engine) commitCandidate(idx int) bool {
	if idx < 0 || idx >= len(fe.cands) {
		return false
	}
	c := fe.cands[idx]
	fe.engine.Commit(c.Word, c.Pinyin)
	fe.commitText(c.Word)
	fe.reset()
	return true
}

func (fe *Engine) commitText(text string) {
	if fe.conn == nil {
		return
	}
	// Fcitx5 commit text signal
	fe.conn.Emit(fe.objPath, "org.fcitx.Fcitx.Samime1.CommitString", text)
}

func (fe *Engine) reset() {
	fe.preedit = ""
	fe.cands = nil
	fe.engine.ResetContext()
	fe.emitSignals()
}

func (fe *Engine) emitSignals() {
	if fe.conn == nil {
		return
	}
	// UpdatePreedit
	fe.conn.Emit(fe.objPath, "org.fcitx.Fcitx.Samime1.UpdatePreedit",
		fe.preedit, uint32(len(fe.preedit)))

	// UpdateCandidates
	if len(fe.cands) > 0 {
		words := make([]string, len(fe.cands))
		for i, c := range fe.cands {
			words[i] = c.Word
		}
		fe.conn.Emit(fe.objPath, "org.fcitx.Fcitx.Samime1.UpdateCandidates",
			words, true)
	} else {
		fe.conn.Emit(fe.objPath, "org.fcitx.Fcitx.Samime1.HideCandidates")
	}
}

// Stop 停止
func (fe *Engine) Stop() {
	fe.mu.Lock()
	defer fe.mu.Unlock()
	fe.running = false
	if fe.conn != nil {
		fe.conn.Close()
	}
}

// === Fcitx5 配置文件生成 ===

// WriteAddonConfig 写入 Fcitx5 addon 配置
// 路径: ~/.local/share/fcitx5/addon/samime.conf
func WriteAddonConfig(path string) error {
	conf := `[Addon]
Name=Samime
Category=InputMethod
Version=1.0.0
Library=samime
Type=SharedLibrary
OnDemand=True
Configurable=True

[Addon]
Name[samime]=Samime
Comment=Samime Chinese Input Method (Go)
Comment[zh_CN]=Samime 中文输入法（Go 实现）
`
	return os.WriteFile(path, []byte(conf), 0644)
}

// WriteIMConfig 写入输入法配置
// 路径: ~/.local/share/fcitx5/inputmethod/samime.conf
func WriteIMConfig(path string) error {
	conf := `[InputMethod]
Name=Samime
Icon=samime
Label=萨米
LangCode=zh_CN
Addon=samime
Configurable=True

[InputMethod]
Name[samime]=Samime
Comment=Samime Chinese Input Method (Go)
Comment[zh_CN]=Samime 中文输入法
`
	return os.WriteFile(path, []byte(conf), 0644)
}

// WriteProfileConfig 写入 profile 配置（启用 samime）
// 路径: ~/.config/fcitx5/profile
func WriteProfileConfig(path string) error {
	conf := `[Groups/0]
# Group Name
Name=Default
# Layout
Default Layout=us
# Default Input Method
DefaultIM=samime

[Groups/0/Items/0]
# Name
Name=keyboard-us
# Layout
Layout=

[Groups/0/Items/1]
# Name
Name=samime
# Layout
Layout=

[GroupOrder]
0=Default
`
	return os.WriteFile(path, []byte(conf), 0644)
}

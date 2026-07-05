// Package ibus implements an IBus engine adapter for GoIME.
//
// This registers as an IBus engine on the session D-Bus, so the Go engine
// is exposed as a real input method on Linux without any external wrapper.
//
// 参考:
//   - IBus D-Bus 协议: https://github.com/ibus/ibus/blob/main/src/ibusengine.h
//   - Python 实现: https://github.com/ibus/ibus/blob/main/bindings/python/ibus/engine.py
//
// 用法:
//   eng, _ := engine.NewWithUserStore(dict, "~/.samime/userdict")
//   ibusEng := ibus.NewEngine(eng, "samime", "/org/freedesktop/IBus/Samime")
//   ibusEng.Run()
package ibus

import (
	"fmt"
	"log"
	"os"
	"sync"
	"unicode"

	"github.com/godbus/dbus/v5"
	"github.com/zai/goime/internal/engine"
)

// IBusEngine IBus 引擎接口（Go 端实现）
type IBusEngine struct {
	engine *engine.Engine

	conn     *dbus.Conn
	busName  string
	objPath  dbus.ObjectPath
	mu       sync.Mutex
	running  bool

	// IBus 引擎状态
	preedit  string
	cands    []engine.Candidate
	cursorPos int

	// IBus 属性
	engineName string
	engineDesc string
	engineIcon string
}

// NewEngine 创建 IBus 引擎
// busName 例: "org.freedesktop.IBus.Samime"
// objPath 例: "/org/freedesktop/IBus/Samime"
func NewEngine(eng *engine.Engine, name, path string) *IBusEngine {
	return &IBusEngine{
		engine:     eng,
		busName:    "org.freedesktop.IBus.Samime",
		objPath:    dbus.ObjectPath(path),
		engineName: name,
		engineDesc: "Samime Chinese Input Method (Go)",
		engineIcon: "samime",
	}
}

// Run 连接到 D-Bus 并注册为 IBus engine
// 阻塞调用
func (ie *IBusEngine) Run() error {
	ie.mu.Lock()
	if ie.running {
		ie.mu.Unlock()
		return fmt.Errorf("already running")
	}
	ie.running = true
	ie.mu.Unlock()

	// 连接到 session bus
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect session bus: %w", err)
	}
	ie.conn = conn
	defer conn.Close()

	// 请求 bus name
	reply, err := conn.RequestName(ie.busName, dbus.NameFlagDoNotQueue)
	if err != nil {
		return fmt.Errorf("request name: %w", err)
	}
	if reply != dbus.RequestNameReplyPrimaryOwner {
		return fmt.Errorf("bus name already taken")
	}

	// 注册对象
	err = conn.Export(ie, ie.objPath, "org.freedesktop.IBus.Engine")
	if err != nil {
		return fmt.Errorf("export: %w", err)
	}

	// 通知 IBus 我们存在（注册为 IBus factory）
	// 实际 IBus 通过 ibus-daemon 进程交互，简化处理
	log.Printf("[ibus] Listening on %s %s", ie.busName, ie.objPath)
	log.Printf("[ibus] Run ibus restart to activate")

	// 监听信号
	ie.loopSignals()

	return nil
}

// loopSignals 处理 D-Bus 信号（被 IBus 调用）
func (ie *IBusEngine) loopSignals() {
	c := make(chan *dbus.Signal, 100)
	ie.conn.Signal(c)
	for sig := range c {
		ie.handleSignal(sig)
	}
}

func (ie *IBusEngine) handleSignal(sig *dbus.Signal) {
	log.Printf("[ibus] signal: %s %v", sig.Name, sig.Body)
}

// === IBus Engine 接口方法（被 IBus 调用） ===
// 这些方法导出为 D-Bus 方法

// ProcessKeyEvent 处理按键事件
// 参数: keyval (uint32), keycode (uint32), state (uint32)
// 返回: processed (bool)
func (ie *IBusEngine) ProcessKeyEvent(keyval, keycode, state uint32) (bool, *dbus.Error) {
	ie.mu.Lock()
	defer ie.mu.Unlock()

	// 数字键 1-9
	if keyval >= '1' && keyval <= '9' {
		idx := int(keyval - '1')
		return ie.commitCandidate(idx), nil
	}

	// 空格：选第一个候选
	if keyval == ' ' {
		if ie.preedit != "" && len(ie.cands) > 0 {
			return ie.commitCandidate(0), nil
		}
		return false, nil
	}

	// 回车
	if keyval == 65293 { // GDK_KEY_Return
		if ie.preedit != "" {
			if len(ie.cands) > 0 {
				return ie.commitCandidate(0), nil
			}
			// 没候选，提交原始拼音
			ie.commitText(ie.preedit)
			ie.reset()
			return true, nil
		}
		return false, nil
	}

	// ESC
	if keyval == 65307 { // GDK_KEY_Escape
		if ie.preedit != "" {
			ie.reset()
			return true, nil
		}
		return false, nil
	}

	// 退格
	if keyval == 65288 { // GDK_KEY_BackSpace
		if ie.preedit != "" {
			ie.preedit = ie.preedit[:len(ie.preedit)-1]
			ie.updateCandidates()
			return true, nil
		}
		return false, nil
	}

	// 字母键
	if unicode.IsLetter(rune(keyval)) {
		ch := rune(keyval)
		// 转小写
		if ch >= 'A' && ch <= 'Z' {
			ch = ch + 32
		}
		ie.preedit += string(ch)
		ie.updateCandidates()
		return true, nil
	}

	return false, nil
}

// SetCursorLocation 设置光标位置
func (ie *IBusEngine) SetCursorLocation(x, y, w, h int32) *dbus.Error {
	// 简化：用于候选窗位置（当前用日志代替）
	log.Printf("[ibus] cursor at (%d,%d,%d,%d)", x, y, w, h)
	return nil
}

// SetCapabilities 设置支持的能力
func (ie *IBusEngine) SetCapabilities(caps uint32) *dbus.Error {
	return nil
}

// FocusIn 焦点进入
func (ie *IBusEngine) FocusIn() *dbus.Error {
	return nil
}

// FocusOut 焦点离开
func (ie *IBusEngine) FocusOut() *dbus.Error {
	ie.mu.Lock()
	defer ie.mu.Unlock()
	ie.reset()
	return nil
}

// Reset 重置
func (ie *IBusEngine) Reset() *dbus.Error {
	ie.mu.Lock()
	defer ie.mu.Unlock()
	ie.reset()
	return nil
}

// Enable 启用
func (ie *IBusEngine) Enable() *dbus.Error {
	log.Printf("[ibus] Enabled")
	return nil
}

// Disable 禁用
func (ie *IBusEngine) Disable() *dbus.Error {
	log.Printf("[ibus] Disabled")
	return nil
}

// PageUp / PageDown / CursorUp / CursorDown 候选窗翻页
func (ie *IBusEngine) PageUp() *dbus.Error     { return nil }
func (ie *IBusEngine) PageDown() *dbus.Error   { return nil }
func (ie *IBusEngine) CursorUp() *dbus.Error   { return nil }
func (ie *IBusEngine) CursorDown() *dbus.Error { return nil }

// CandidateClicked 候选词被点击
func (ie *IBusEngine) CandidateClicked(idx uint32, button, state uint32) *dbus.Error {
	ie.mu.Lock()
	defer ie.mu.Unlock()
	ie.commitCandidate(int(idx))
	return nil
}

// PropertyActivate 属性激活（菜单等）
func (ie *IBusEngine) PropertyActivate(name string, state uint32) *dbus.Error {
	return nil
}

// PropertyShow 显示属性
func (ie *IBusEngine) PropertyShow(name string) *dbus.Error {
	return nil
}

// PropertyHide 隐藏属性
func (ie *IBusEngine) PropertyHide(name string) *dbus.Error {
	return nil
}

// === 内部辅助方法 ===

func (ie *IBusEngine) updateCandidates() {
	ie.cands = ie.engine.Search(ie.preedit)
	ie.emitSignals()
}

func (ie *IBusEngine) commitCandidate(idx int) bool {
	if idx < 0 || idx >= len(ie.cands) {
		return false
	}
	c := ie.cands[idx]
	ie.engine.Commit(c.Word, c.Pinyin)
	ie.commitText(c.Word)
	ie.reset()
	return true
}

func (ie *IBusEngine) commitText(text string) {
	// 通过 D-Bus 信号通知 IBus 提交文字
	// 实际需要发 "CommitText" 信号
	if ie.conn == nil {
		return
	}
	ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.CommitText", text)
}

func (ie *IBusEngine) reset() {
	ie.preedit = ""
	ie.cands = nil
	ie.cursorPos = 0
	ie.engine.ResetContext()
	ie.emitSignals()
}

func (ie *IBusEngine) emitSignals() {
	if ie.conn == nil {
		return
	}
	// UpdatePreeditText 信号
	ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.UpdatePreeditText",
		ie.preedit, uint32(len(ie.preedit)), true)

	// UpdateAuxiliaryText 信号（拼音提示）
	ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.UpdateAuxiliaryText",
		ie.preedit, true)

	// UpdateLookupTable 信号（候选词）
	if len(ie.cands) > 0 {
		// 转换为 IBus LookupTable 格式
		words := make([]string, len(ie.cands))
		for i, c := range ie.cands {
			words[i] = c.Word
		}
		ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.UpdateLookupTable",
			words, true)
	} else {
		ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.HideLookupTable")
	}
}

// Stop 停止
func (ie *IBusEngine) Stop() {
	ie.mu.Lock()
	defer ie.mu.Unlock()
	ie.running = false
	if ie.conn != nil {
		ie.conn.Close()
	}
}

// === IBus 注册辅助 ===

// EngineDesc IBus 引擎描述（用于注册到 IBus）
type EngineDesc struct {
	Name        string
	LongName    string
	Description string
	Language    string
	License     string
	Author      string
	Icon        string
	Layout      string
}

// DefaultEngineDesc 默认引擎描述
func DefaultEngineDesc() EngineDesc {
	return EngineDesc{
		Name:        "samime",
		LongName:    "Samime Pinyin",
		Description: "Samime Chinese Input Method (Go)",
		Language:    "zh_CN",
		License:     "MIT",
		Author:      "samime",
		Icon:        "samime",
		Layout:      "us",
	}
}

// WriteEngineConfig 写入 IBus 引擎配置文件
// 路径: /usr/share/ibus/component/samime.xml (系统) 或
//       ~/.config/ibus/component/samime.xml (用户)
func WriteEngineConfig(path string) error {
	desc := DefaultEngineDesc()
	xml := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<component>
    <name>org.freedesktop.IBus.Samime</name>
    <description>Samime Chinese Input Method (Go)</description>
    <exec>%s -mode=service</exec>
    <version>1.0.0</version>
    <author>samime</author>
    <license>MIT</license>
    <homepage>https://github.com/samaidev/samime</homepage>
    <textdomain>samime</textdomain>
    <engines>
        <engine>
            <name>%s</name>
            <longname>%s</longname>
            <description>%s</description>
            <language>%s</language>
            <license>%s</license>
            <author>%s</author>
            <icon>%s</icon>
            <layout>%s</layout>
            <rank>0</rank>
        </engine>
    </engines>
</component>
`, os.Args[0], desc.Name, desc.LongName, desc.Description,
		desc.Language, desc.License, desc.Author, desc.Icon, desc.Layout)

	return os.WriteFile(path, []byte(xml), 0644)
}

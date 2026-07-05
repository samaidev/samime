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
        "strings"
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

// Run 连接到 IBus D-Bus 并注册为 IBus engine factory
// 阻塞调用
//
// IBus engine 启动流程:
// 1. ibus-daemon 设置 IBUS_ADDRESS 环境变量后启动 engine 进程
// 2. engine 进程连接到 IBUS_ADDRESS 指定的 D-Bus 地址
// 3. engine 进程请求 bus name (如 org.freedesktop.IBus.Samime)
// 4. engine 进程注册 Factory 对象（实现 org.freedesktop.IBus.Factory 接口）
// 5. IBus daemon 检测到 Factory 后，调用 Factory.CreateEngine(engine_name)
// 6. Factory 创建 Engine 对象并返回 object path
// 7. IBus 通过 Engine 接口发送按键事件
func (ie *IBusEngine) Run() error {
        ie.mu.Lock()
        if ie.running {
                ie.mu.Unlock()
                return fmt.Errorf("already running")
        }
        ie.running = true
        ie.mu.Unlock()

        // 1. 读取 IBUS_ADDRESS 环境变量
        // 如果没有，从 IBus bus 文件中读取
        ibusAddr := os.Getenv("IBUS_ADDRESS")
        if ibusAddr == "" {
                ibusAddr = readIBusAddress()
        }
        var conn *dbus.Conn
        var err error

        if ibusAddr != "" {
                log.Printf("[ibus] Connecting to IBus D-Bus: %s", ibusAddr)
                conn, err = dbus.Dial(ibusAddr)
                if err != nil {
                        log.Printf("[ibus] Dial failed, trying session bus: %v", err)
                        conn, err = dbus.ConnectSessionBus()
                        if err != nil {
                                return fmt.Errorf("connect bus: %w", err)
                        }
                }
                if err = conn.Auth(nil); err != nil {
                        log.Printf("[ibus] Auth failed, trying session bus: %v", err)
                        conn.Close()
                        conn, err = dbus.ConnectSessionBus()
                        if err != nil {
                                return fmt.Errorf("connect session bus: %w", err)
                        }
                }
                if err = conn.Hello(); err != nil {
                        log.Printf("[ibus] Hello failed, trying session bus: %v", err)
                        conn.Close()
                        conn, err = dbus.ConnectSessionBus()
                        if err != nil {
                                return fmt.Errorf("connect session bus: %w", err)
                        }
                }
        } else {
                log.Printf("[ibus] No IBUS_ADDRESS, using session bus")
                conn, err = dbus.ConnectSessionBus()
                if err != nil {
                        return fmt.Errorf("connect session bus: %w", err)
                }
        }
        ie.conn = conn
        defer conn.Close()

        // 2. 请求 bus name
        reply, err := conn.RequestName(ie.busName, dbus.NameFlagDoNotQueue)
        if err != nil {
                return fmt.Errorf("request name: %w", err)
        }
        if reply != dbus.RequestNameReplyPrimaryOwner {
                return fmt.Errorf("bus name already taken")
        }
        log.Printf("[ibus] Bus name '%s' acquired", ie.busName)

        // 3. 注册 Factory 对象到 /org/freedesktop/IBus/Factory
	// IBus daemon 期望 factory 在这个固定路径上（IBus.PATH_FACTORY）。
	// 之前导出到 ie.objPath 是错误的：ibus 在 /org/freedesktop/IBus/Factory
	// 找不到 Factory 接口，导致 "Object does not implement the interface
	// 'org.freedesktop.IBus.Factory'" 错误，引擎无法被激活。
	factoryPath := dbus.ObjectPath("/org/freedesktop/IBus/Factory")
	factory := &IBusFactory{engine: ie}
	err = conn.Export(factory, factoryPath, "org.freedesktop.IBus.Factory")
	if err != nil {
		return fmt.Errorf("export factory: %w", err)
	}
	log.Printf("[ibus] Factory registered at %s", factoryPath)

	// 4. 预注册 Engine 接口到 ie.objPath
	// 当 ibus 调用 Factory.CreateEngine 时，我们返回 ie.objPath，
	// ibus 会通过该路径上的 org.freedesktop.IBus.Engine 接口发送按键事件。
	err = conn.Export(ie, ie.objPath, "org.freedesktop.IBus.Engine")
	if err != nil {
		return fmt.Errorf("export engine interface: %w", err)
	}
	log.Printf("[ibus] Engine interface registered at %s", ie.objPath)

	// 5. Component 发现：通过磁盘上的 XML 文件
	// ibus-daemon 启动时扫描 /usr/share/ibus/component/*.xml 和
	// ~/.config/ibus/component/*.xml 加载组件信息（包括 engine 描述、
	// 启动命令等）。不需要运行时调用 RegisterComponent —— 那个调用
	// 期望 IBusComponent variant（D-Bus 签名 v），而我们之前传的是
	// XML 字符串（签名 s），导致类型不匹配错误。XML 文件已足够。
	log.Printf("[ibus] Component discovered via on-disk XML file")

	log.Printf("[ibus] Waiting for IBus to create engine...")

        // 5. 监听 D-Bus 信号
        ie.loopSignals()

        return nil
}

// ReadIBusAddressFromBusFile 从 IBus bus 文件中读取 D-Bus 地址（导出版）
func ReadIBusAddressFromBusFile() string {
        return readIBusAddress()
}

// readIBusAddress 从 IBus bus 文件中读取 D-Bus 地址
// 当 IBUS_ADDRESS 环境变量未设置时使用
func readIBusAddress() string {
        home, err := os.UserHomeDir()
        if err != nil {
                return ""
        }
        busDir := home + "/.config/ibus/bus"
        entries, err := os.ReadDir(busDir)
        if err != nil {
                return ""
        }
        // 找最新的 bus 文件
        var newest os.DirEntry
        var newestTime int64
        for _, e := range entries {
                if e.IsDir() {
                        continue
                }
                info, err := e.Info()
                if err != nil {
                        continue
                }
                if info.ModTime().Unix() > newestTime {
                        newestTime = info.ModTime().Unix()
                        newest = e
                }
        }
        if newest == nil {
                return ""
        }
        data, err := os.ReadFile(busDir + "/" + newest.Name())
        if err != nil {
                return ""
        }
        // 解析 IBUS_ADDRESS=xxx
        for _, line := range strings.Split(string(data), "\n") {
                if strings.HasPrefix(line, "IBUS_ADDRESS=") {
                        addr := strings.TrimPrefix(line, "IBUS_ADDRESS=")
                        addr = strings.TrimSpace(addr)
                        log.Printf("[ibus] Read IBus address from bus file: %s", addr)
                        return addr
                }
        }
        return ""
}

// getComponentXML 返回 IBus 组件 XML
// IBus daemon 需要这个 XML 来了解 engine 的信息
func (ie *IBusEngine) getComponentXML() string {
        return `<?xml version="1.0" encoding="UTF-8"?>
<component>
    <name>org.freedesktop.IBus.Samime</name>
    <description>Samime Chinese Input Method (Go)</description>
    <exec>/usr/bin/samime -mode=service --ibus</exec>
    <version>1.0.0</version>
    <engines>
        <engine>
            <name>samime</name>
            <longname>Samime Pinyin</longname>
            <description>Samime Chinese Input Method (Go)</description>
            <language>zh_CN</language>
            <icon>samime</icon>
            <layout>us</layout>
        </engine>
    </engines>
</component>`
}

// IBusFactory 实现 org.freedesktop.IBus.Factory 接口
// IBus daemon 调用 CreateEngine 来创建 engine 实例
type IBusFactory struct {
        engine *IBusEngine
}

// CreateEngine 被 IBus daemon 调用，创建 engine 实例
// 参数: engine_name (string)
// 返回: engine_path (object path)
//
// Engine 接口已在 Run() 中预注册到 ie.objPath，所以这里只需返回该路径。
// ibus daemon 会通过该路径上的 org.freedesktop.IBus.Engine 接口发送
// 按键事件（ProcessKeyEvent、FocusIn、Reset 等）。
func (f *IBusFactory) CreateEngine(engineName string) (dbus.ObjectPath, *dbus.Error) {
	log.Printf("[ibus] CreateEngine called: %s", engineName)
	log.Printf("[ibus] Returning engine path: %s", f.engine.objPath)
	return f.engine.objPath, nil
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

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
        preedit   string
        cands     []engine.Candidate
        cursorPos int

        // 候选窗分页状态
        // expanded: false=折叠（只显示第 1 页 5 个），true=展开（可翻页看更多）
        // pageOffset: 当前页在 cands 中的起始索引（0, 5, 10, ...）
        expanded   bool
        pageOffset int

        // 中英文模式：false=中文（默认），true=英文（Shift 切换）
        // 英文模式下所有按键直接透传，不处理拼音
        englishMode bool

        // IBus 属性
        engineName string
        engineDesc string
        engineIcon string
}

// pageSize 每页显示的候选词数量
const pageSize = 5

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
//
// state 位掩码: IBUS_RELEASE_MASK = 0x40000000（按键释放事件）。
// 按下事件 state & 0x40000000 == 0，释放事件该位置 1。
// 只处理按下事件，避免一次按键触发两次（按下+释放都加字母导致重复）。
//
// Shift 切换中英文：
//   - 单独按下 Shift（左或右）切换模式
//   - 英文模式下所有按键直接透传
//   - 中文模式下正常处理拼音输入
func (ie *IBusEngine) ProcessKeyEvent(keyval, keycode, state uint32) (bool, *dbus.Error) {
        ie.mu.Lock()
        defer ie.mu.Unlock()

        release := (state & 0x40000000) != 0
        // Shift 单键切换中英文：只在释放事件检测（避免按下时其他键也被处理）
        // Shift_L = 0xffe1 (65505), Shift_R = 0xffe2 (65506)
        if release && (keyval == 65505 || keyval == 65506) {
                // 检查是否是单独按 Shift（没有其他修饰键）
                // state 在释放时除 release 位外不应有其他修饰（说明 Shift 是唯一按下的键）
                // 但 state 仍会包含 Shift 位，所以 mask 掉 release 和 shift 位
                otherMods := state & ^uint32(0x40000000) & ^uint32(0x0001)
                if otherMods == 0 {
                        ie.englishMode = !ie.englishMode
                        log.Printf("[ibus] Shift toggle -> englishMode=%v", ie.englishMode)
                        // 切换时清空 preedit
                        if ie.preedit != "" {
                                ie.reset()
                        }
                        // 通知 ibus 切换状态（可选：发 ForwardKeyEvent 不需要）
                        return true, nil
                }
        }
        if release {
                return false, nil
        }

        // 英文模式：所有按键直接透传
        if ie.englishMode {
                log.Printf("[ibus] EN mode: passthrough keyval=%d (%q)", keyval, string(rune(keyval)))
                return false, nil
        }

        log.Printf("[ibus] ProcessKeyEvent keyval=%d (%q) keycode=%d state=0x%08x preedit=%q cands=%d",
                keyval, string(rune(keyval)), keycode, state, ie.preedit, len(ie.cands))

        // 数字键 1-5：选当前页的候选（1=第1个，5=第5个）
        // 6-9 也支持（兼容候选数大于 5 但同页显示的情况）
        if keyval >= '1' && keyval <= '9' {
                idx := ie.pageOffset + int(keyval-'1')
                if idx >= len(ie.cands) {
                        // 超出范围，透传
                        log.Printf("[ibus]   digit %d -> idx %d out of range (cands=%d)", keyval, idx, len(ie.cands))
                        return false, nil
                }
                handled := ie.commitCandidate(idx)
                log.Printf("[ibus]   digit %d -> commitCandidate(pageOffset=%d + %d = %d)=%v",
                        keyval, ie.pageOffset, int(keyval-'1'), idx, handled)
                return handled, nil
        }

        // 空格：选第一个候选
        if keyval == ' ' || keyval == 32 {
                if ie.preedit != "" && len(ie.cands) > 0 {
                        handled := ie.commitCandidate(0)
                        log.Printf("[ibus]   space -> commitCandidate(0)=%v", handled)
                        return handled, nil
                }
                log.Printf("[ibus]   space -> passthrough (preedit=%q cands=%d)", ie.preedit, len(ie.cands))
                return false, nil
        }

        // 回车
        if keyval == 65293 || keyval == 65421 { // GDK_KEY_Return / KP_Enter
                if ie.preedit != "" {
                        if len(ie.cands) > 0 {
                                handled := ie.commitCandidate(0)
                                log.Printf("[ibus]   enter -> commitCandidate(0)=%v", handled)
                                return handled, nil
                        }
                        ie.commitText(ie.preedit)
                        ie.reset()
                        log.Printf("[ibus]   enter -> commit raw preedit")
                        return true, nil
                }
                log.Printf("[ibus]   enter -> passthrough")
                return false, nil
        }

        // ESC
        if keyval == 65307 { // GDK_KEY_Escape
                if ie.preedit != "" {
                        ie.reset()
                        log.Printf("[ibus]   esc -> reset")
                        return true, nil
                }
                return false, nil
        }

        // 退格
        if keyval == 65288 { // GDK_KEY_BackSpace
                if ie.preedit != "" {
                        ie.preedit = ie.preedit[:len(ie.preedit)-1]
                        log.Printf("[ibus]   backspace -> preedit=%q", ie.preedit)
                        ie.updateCandidates()
                        return true, nil
                }
                return false, nil
        }

        // 方向键：候选窗隐藏后 ibus 不再路由 CursorUp/CursorDown，
        // 这里自己捕获 Up/Down 实现展开/折叠/翻页。
        // 只在无修饰键（纯 Up/Down）时处理，Shift/Ctrl+方向键透传给应用。
        // GDK_KEY_Up = 65362, GDK_KEY_Down = 65364
        if ie.preedit != "" && len(ie.cands) > 0 &&
                (state&0x0001) == 0 && (state&0x0004) == 0 && (state&0x0008) == 0 {
                if keyval == 65362 { // Up
                        ie.cursorUpInternal()
                        return true, nil
                }
                if keyval == 65364 { // Down
                        ie.cursorDownInternal()
                        return true, nil
                }
        }

        // 标点符号智能转换：有 preedit 时先提交候选词，再输出标点。
        // 关键：合并成一次 commit（候选词+标点），避免两次 CommitText 信号
        // 被 ibus 合并/丢失。先清 preedit 状态再 commit，确保信号顺序正确。
        if punct, ok := punctMap[rune(keyval)]; ok {
                shift := (state & 0x0001) != 0
                // 基础标点（无需 Shift）在 Shift 时保持英文，方便输入 URL/邮箱/代码
                basePunct := keyval == '.' || keyval == ',' || keyval == '?' ||
                        keyval == '!' || keyval == ':' || keyval == ';' ||
                        keyval == '(' || keyval == ')' || keyval == '\\' ||
                        keyval == '/' || keyval == '\'' ||
                        keyval == '[' || keyval == ']' ||
                        keyval == '{' || keyval == '}'
                if shift && basePunct {
                        log.Printf("[ibus]   punct %q with Shift -> passthrough (english)", string(rune(keyval)))
                        return false, nil
                }

                // 计算要 commit 的完整文本：候选词（或原始拼音）+ 标点
                var commitStr string
                if ie.preedit != "" {
                        if len(ie.cands) > 0 {
                                c := ie.cands[0]
                                commitStr = c.Word
                                ie.engine.Commit(c.Word, c.Pinyin)
                                log.Printf("[ibus]   punct: commit candidate %q (pinyin=%q)", c.Word, c.Pinyin)
                        } else {
                                commitStr = ie.preedit
                                log.Printf("[ibus]   punct: commit raw preedit %q", ie.preedit)
                        }
                }
                commitStr += punct

                // 先清空 preedit 状态，发 Hide 信号清掉候选窗显示
                ie.preedit = ""
                ie.cands = nil
                ie.cursorPos = 0
                ie.emitSignals()
                // 一次性 commit 候选词 + 标点
                ie.commitText(commitStr)
                log.Printf("[ibus]   punct %q -> commit %q", string(rune(keyval)), commitStr)
                return true, nil
        }

        // Ctrl/Ctrl+letter 快捷键透传（Ctrl+A 全选、Ctrl+C 复制等）
        // state 位: 0x0004=Control, 0x0008=Mod1(Alt)
        ctrlMod := (state & 0x0004) != 0
        altMod := (state & 0x0008) != 0
        if ctrlMod || altMod {
                log.Printf("[ibus]   keyval=%d with Ctrl/Alt mod -> passthrough", keyval)
                return false, nil
        }

        // 字母键 (a-z, A-Z)
        if (keyval >= 'a' && keyval <= 'z') || (keyval >= 'A' && keyval <= 'Z') {
                ch := rune(keyval)
                if ch >= 'A' && ch <= 'Z' {
                        ch = ch + 32 // 转小写
                }
                ie.preedit += string(ch)
                log.Printf("[ibus]   letter %q -> preedit=%q", string(rune(keyval)), ie.preedit)
                ie.updateCandidates()
                log.Printf("[ibus]   after update: cands=%d", len(ie.cands))
                return true, nil
        }

        log.Printf("[ibus]   unhandled keyval=%d -> passthrough", keyval)
        return false, nil
}

// punctMap 英文标点 → 中文标点映射表。
// 用户按英文标点时（无 Shift），自动转成中文全角标点。
// Shift+标点保持英文符号不变，便于输入 URL、邮箱等。
var punctMap = map[rune]string{
        '.':  "。",  // 句号
        ',':  "，",  // 逗号
        '?':  "？",  // 问号
        '!':  "！",  // 感叹号
        ':':  "：",  // 冒号
        ';':  "；",  // 分号
        '(':  "（",  // 左括号
        ')':  "）",  // 右括号
        '<':  "《",  // 左书名号（Shift+,）
        '>':  "》",  // 右书名号（Shift+.）
        '"':  "“",  // 左双引号
        '\'': "‘",  // 左单引号
        '\\': "、",  // 顿号（反斜杠键）
        '/':  "、",  // 顿号（斜杠键，仅无 Shift 时）
        '^':  "……", // 省略号（Shift+6）
        '_':  "——", // 破折号（Shift+-）
        '$':  "￥",  // 人民币符号
        '~':  "～",  // 波浪号
        '[':  "【",  // 左方括号
        ']':  "】",  // 右方括号
        '{':  "「",  // 左花括号 → 直角引号
        '}':  "」",  // 右花括号 → 直角引号
}

// punctFlipQuotes 处理引号配对：连续按 " 时交替输出 “ ”
// （简化版：通过记录上一次是否输出了左引号来判断）
var lastLeftDoubleQuote = true
var lastLeftSingleQuote = true

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
// 下方向键展开候选窗（显示更多），上方向键在第一页时折叠，其他页翻页。
func (ie *IBusEngine) PageUp() *dbus.Error {
	ie.mu.Lock()
	defer ie.mu.Unlock()
	if len(ie.cands) == 0 {
		return nil
	}
	ie.expanded = true
	if ie.pageOffset >= pageSize {
		ie.pageOffset -= pageSize
	} else {
		// 循环到最后一页
		total := len(ie.cands)
		lastPageStart := ((total - 1) / pageSize) * pageSize
		ie.pageOffset = lastPageStart
	}
	log.Printf("[ibus] PageUp -> expanded=%v pageOffset=%d", ie.expanded, ie.pageOffset)
	ie.emitSignals()
	return nil
}
func (ie *IBusEngine) PageDown() *dbus.Error {
	ie.mu.Lock()
	defer ie.mu.Unlock()
	if len(ie.cands) == 0 {
		return nil
	}
	ie.expanded = true
	total := len(ie.cands)
	if ie.pageOffset+pageSize < total {
		ie.pageOffset += pageSize
	} else {
		// 循回第一页
		ie.pageOffset = 0
	}
	log.Printf("[ibus] PageDown -> expanded=%v pageOffset=%d", ie.expanded, ie.pageOffset)
	ie.emitSignals()
	return nil
}
func (ie *IBusEngine) CursorUp() *dbus.Error {
	ie.mu.Lock()
	defer ie.mu.Unlock()
	ie.cursorUpInternal()
	return nil
}
// cursorUpInternal 上方向键：第一页折叠，其他页翻页
// 调用方负责持锁
func (ie *IBusEngine) cursorUpInternal() {
	if len(ie.cands) == 0 {
		return
	}
	// 在第一页时按上 = 折叠
	if ie.pageOffset == 0 {
		ie.expanded = false
		log.Printf("[ibus] CursorUp -> fold (expanded=false)")
	} else {
		ie.expanded = true
		if ie.pageOffset >= pageSize {
			ie.pageOffset -= pageSize
		}
		log.Printf("[ibus] CursorUp -> pageOffset=%d", ie.pageOffset)
	}
	ie.emitSignals()
}
func (ie *IBusEngine) CursorDown() *dbus.Error {
	ie.mu.Lock()
	defer ie.mu.Unlock()
	ie.cursorDownInternal()
	return nil
}
// cursorDownInternal 下方向键：展开 + 翻页（最后一页循环回第一页）
// 调用方负责持锁
func (ie *IBusEngine) cursorDownInternal() {
	if len(ie.cands) == 0 {
		return
	}
	ie.expanded = true
	total := len(ie.cands)
	if ie.pageOffset+pageSize < total {
		ie.pageOffset += pageSize
	} else {
		ie.pageOffset = 0
	}
	log.Printf("[ibus] CursorDown -> expanded=%v pageOffset=%d", ie.expanded, ie.pageOffset)
	ie.emitSignals()
}

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
        // preedit 变化时重置分页状态（回到第一页、折叠）
        ie.pageOffset = 0
        ie.expanded = false
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

// makeIBusText 把字符串包装成 IBusText 的 D-Bus 表示。
//
// IBusText 在 D-Bus 上的签名是 (sa{sv}sv) —— 由 ibus_serializable_serialize_object
// 生成，字段顺序如下（见 ibustext.c::ibus_text_serialize）:
//   - name (s): 类名，固定 "IBusText"
//   - attachments (a{sv}): IBusSerializable 的 attachments dict（空）
//   - text (s): 文本本身
//   - attrs (v): IBusAttrList 序列化结果包在 variant 里（空 AttrList）
//
// 注意：IBusAttrList 本身也是 IBusSerializable，签名是 (sa{sv}av)，
// 这里用空数组占位（ibus_text_new_from_string 默认 attrs 就是空 AttrList）。
func makeIBusText(text string) interface{} {
	return struct {
		Name        string
		Attachments map[string]dbus.Variant
		Text        string
		Attrs       dbus.Variant
	}{
		Name:        "IBusText",
		Attachments: map[string]dbus.Variant{},
		Text:        text,
		Attrs:       dbus.MakeVariant(makeEmptyIBusAttrList()),
	}
}

// makeEmptyIBusAttrList 构造一个空的 IBusAttrList。
// 签名 (sa{sv}av): 类名 + attachments + 属性数组（空）。
func makeEmptyIBusAttrList() interface{} {
	return struct {
		Name        string
		Attachments map[string]dbus.Variant
		Attrs       []dbus.Variant
	}{
		Name:        "IBusAttrList",
		Attachments: map[string]dbus.Variant{},
		Attrs:       []dbus.Variant{},
	}
}

// makeIBusTextVariant 返回包装在 variant 里的 IBusText（信号参数期望 v 签名）。
func makeIBusTextVariant(text string) dbus.Variant {
	return dbus.MakeVariant(makeIBusText(text))
}

func (ie *IBusEngine) commitText(text string) {
	// 通过 D-Bus 信号通知 IBus 提交文字。
	// CommitText 信号签名: (v) —— 一个 IBusText variant。
	if ie.conn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ibus] PANIC in commitText: %v", r)
		}
	}()
	ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.CommitText",
		makeIBusTextVariant(text))
}

func (ie *IBusEngine) reset() {
	ie.preedit = ""
	ie.cands = nil
	ie.cursorPos = 0
	ie.pageOffset = 0
	ie.expanded = false
	ie.engine.ResetContext()
	ie.emitSignals()
}

func (ie *IBusEngine) emitSignals() {
	if ie.conn == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[ibus] PANIC in emitSignals: %v", r)
		}
	}()

	if ie.preedit == "" {
		// 清空预编辑
		ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.HidePreeditText")
	} else {
		// UpdatePreeditText: 拼音字母显示在光标位置的输入框（inline preedit）
		ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.UpdatePreeditText",
			makeIBusTextVariant(ie.preedit), uint32(len(ie.preedit)), true)
	}

	// UpdateAuxiliaryText: 候选词横向显示在下方独立一排（不含 preedit）
	// preedit 已由 UpdatePreeditText 显示在输入框，这里只放候选词
	if len(ie.cands) == 0 {
		ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.HideAuxiliaryText")
	} else {
		auxText := ie.buildHorizontalCandidates()
		ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.UpdateAuxiliaryText",
			makeIBusTextVariant(auxText), true)
	}

	// 隐藏 ibus 自带候选窗（GNOME Shell 强制竖排，用 auxiliary 横排替代）
	ie.conn.Emit(ie.objPath, "org.freedesktop.IBus.Engine.HideLookupTable")
}

// makeLookupTableVariant 构造 IBusLookupTable 的 variant。
//
// IBusLookupTable D-Bus 签名: (sa{sv}uubiavav) —— 由
// ibus_serializable_serialize_object + ibus_lookup_table_serialize 生成:
//   - name (s): "IBusLookupTable"
//   - attachments (a{sv}): 空
//   - page_size (u)
//   - cursor_pos (u)
//   - cursor_visible (b): 布尔，不是 int
//   - round (b): 布尔
//   - orientation (i): int32 (1=横向, 2=纵向)
//   - candidates (av): IBusText variant 数组
//   - labels (av): IBusText variant 数组
//
// 见 src/ibuslookuptable.c::ibus_lookup_table_serialize。
//
// 分页：只发送当前页（pageOffset 开始的 pageSize 个）候选词。
// 标签固定 1-5 对应当前页内的候选。
func (ie *IBusEngine) makeLookupTableVariant() dbus.Variant {
	type lookupTable struct {
		Name          string
		Attachments   map[string]dbus.Variant
		PageSize      uint32
		CursorPos     uint32
		CursorVisible bool
		Round         bool
		Orientation   int32
		Candidates    []dbus.Variant
		Labels        []dbus.Variant
	}

	total := len(ie.cands)
	// 折叠模式下只显示第一页；展开模式下按 pageOffset 翻页
	maxVisible := pageSize
	if !ie.expanded && total > pageSize {
		maxVisible = pageSize
	}

	start := ie.pageOffset
	if start >= total {
		start = 0
		ie.pageOffset = 0
	}
	end := start + maxVisible
	if end > total {
		end = total
	}

	candVariants := make([]dbus.Variant, 0, end-start)
	for i := start; i < end; i++ {
		candVariants = append(candVariants, makeIBusTextVariant(ie.cands[i].Word))
	}

	labelVariants := make([]dbus.Variant, 0, end-start)
	for i := start; i < end; i++ {
		// 标签 1-5 对应当前页内位置
		labelIdx := i - start
		labelVariants = append(labelVariants, makeIBusTextVariant(string(rune('1'+labelIdx))))
	}

	tbl := lookupTable{
		Name:          "IBusLookupTable",
		Attachments:   map[string]dbus.Variant{},
		PageSize:      uint32(pageSize),
		CursorPos:     0,
		CursorVisible: true,
		Round:         ie.expanded, // 展开时循环翻页
		Orientation:   1,            // 横向显示
		Candidates:    candVariants,
		Labels:        labelVariants,
	}
	return dbus.MakeVariant(tbl)
}

// buildHorizontalCandidates 构造横向候选词字符串，用于 auxiliary text 显示。
// 格式: "1.看看 2.可靠 3.开口 4.可可 5.开阔"
// 显示当前页（pageOffset 开始的 pageSize 个）候选，与 LookupTable 当前页一致。
// 折叠模式下只显示前 pageSize 个；展开模式下按 pageOffset 翻页。
func (ie *IBusEngine) buildHorizontalCandidates() string {
	total := len(ie.cands)
	maxVisible := pageSize
	if !ie.expanded && total > pageSize {
		maxVisible = pageSize
	}
	start := ie.pageOffset
	if start >= total {
		start = 0
		ie.pageOffset = 0
	}
	end := start + maxVisible
	if end > total {
		end = total
	}

	var sb strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			sb.WriteString("  ")
		}
		labelIdx := i - start
		sb.WriteString(string(rune('1' + labelIdx)))
		sb.WriteString(".")
		sb.WriteString(ie.cands[i].Word)
	}
	// 折叠模式下如果有更多候选，提示可按 Down 展开
	if !ie.expanded && total > end {
		sb.WriteString("  ↓更多")
	}
	return sb.String()
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

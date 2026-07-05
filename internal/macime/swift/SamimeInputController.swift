// SamimeInputController.swift
// macOS IMK 输入法完整实现
//
// 包含:
//   - IMKInputController 子类（按键事件处理）
//   - GoEngineClient（Unix Socket 通信）
//   - CandidateWindowController（候选词窗口 NSWindow）
//   - 服务端激活/停用

import Cocoa
import InputMethodKit
import Foundation
import Darwin

// MARK: - Go 引擎客户端

class GoEngineClient {
    private var socket: Int32 = -1
    private let socketPath: String
    private let queue = DispatchQueue(label: "com.samime.engine-client")
    private var connected = false

    init(socketPath: String? = nil) {
        self.socketPath = socketPath ?? (NSHomeDirectory() + "/.samime/macime.sock")
    }

    deinit {
        disconnect()
    }

    func connect() -> Bool {
        return queue.sync {
            if connected { return true }
            socket = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
            if socket < 0 { return false }

            var addr = sockaddr_un()
            addr.sun_family = sa_family_t(AF_UNIX)
            let pathBytes = self.socketPath.utf8CString
            withUnsafeMutablePointer(to: &addr.sun_path) {
                $0.withMemoryRebound(to: CChar.self, capacity: pathBytes.count) {
                    strcpy($0, pathBytes)
                }
            }

            let result = withUnsafePointer(to: &addr) {
                $0.withMemoryRebound(to: sockaddr.self, capacity: 1) {
                    Darwin.connect(socket, $0, socklen_t(MemoryLayout<sockaddr_un>.size))
                }
            }
            if result < 0 {
                Darwin.close(socket)
                socket = -1
                return false
            }
            connected = true
            return true
        }
    }

    func disconnect() {
        queue.sync {
            if socket >= 0 {
                Darwin.close(socket)
                socket = -1
            }
            connected = false
        }
    }

    func ensureConnected() -> Bool {
        if connected { return true }
        return connect()
    }

    private func sendRequest(_ req: [String: Any]) -> [String: Any]? {
        guard ensureConnected() else { return nil }

        guard let data = try? JSONSerialization.data(withJSONObject: req),
              let str = String(data: data, encoding: .utf8) else {
            return nil
        }
        let line = str + "\n"
        let bytes = [UInt8](line.utf8)
        let sent = bytes.withUnsafeBufferPointer {
            Darwin.send(socket, $0.baseAddress, $0.count, 0)
        }
        if sent < 0 {
            disconnect()
            return nil
        }

        // 读响应
        var buf = [UInt8](repeating: 0, count: 65536)
        var total = 0
        while total < buf.count - 1 {
            let n = Darwin.recv(socket, &buf + total, buf.count - 1 - total, 0)
            if n <= 0 { break }
            total += n
            if buf[total - 1] == UInt8(ascii: "\n") { break }
        }
        let line2 = String(bytes: buf[0..<total], encoding: .utf8)?
            .trimmingCharacters(in: .whitespacesAndNewlines)
        guard let l = line2,
              let d = l.data(using: .utf8),
              let json = try? JSONSerialization.jsonObject(with: d) as? [String: Any] else {
            return nil
        }
        return json
    }

    func search(_ preedit: String) -> [Candidate] {
        guard let resp = sendRequest(["method": "search", "preedit": preedit]),
              let cands = resp["candidates"] as? [[String: Any]] else {
            return []
        }
        return cands.compactMap { dict in
            guard let word = dict["Word"] as? String else { return nil }
            let pinyin = dict["Pinyin"] as? String ?? ""
            let score = dict["Score"] as? Double ?? 0
            let source = dict["Source"] as? String ?? ""
            return Candidate(word: word, pinyin: pinyin, score: score, source: source)
        }
    }

    func commit(_ word: String, pinyin: String) -> Bool {
        guard let resp = sendRequest([
            "method": "commit",
            "word": word,
            "pinyin": pinyin
        ]) else { return false }
        return resp["ok"] as? Bool ?? false
    }

    func reset() {
        _ = sendRequest(["method": "reset"])
    }

    func status() -> String {
        guard let resp = sendRequest(["method": "status"]) else { return "" }
        return resp["error"] as? String ?? ""
    }
}

// MARK: - 候选词数据

struct Candidate {
    let word: String
    let pinyin: String
    let score: Double
    let source: String
}

// MARK: - 候选词窗口

// MARK: - 现代化候选词窗口（毛玻璃效果 + 圆角 + 序号徽章）

class CandidateWindow: NSWindow {
    private let tableView = NSTableView()
    private var candidates: [Candidate] = []
    var onSelect: ((Int) -> Void)?

    // 现代配色（macOS Big Sur+ 风格）
    private let colorSelectedBg = NSColor.controlAccentColor
    private let colorNormalBg = NSColor.clear
    private let colorSelectedText = NSColor.white
    private let colorNormalText = NSColor.labelColor
    private let colorIndexText = NSColor.secondaryLabelColor
    private let colorPinyinText = NSColor.tertiaryLabelColor

    init() {
        let frame = NSRect(x: 0, y: 0, width: 240, height: 240)
        super.init(contentRect: frame,
                   styleMask: [.borderless, .fullSizeContentView],
                   backing: .buffered,
                   defer: false)

        // 现代窗口样式
        self.isOpaque = false
        self.backgroundColor = NSColor.clear
        self.hasShadow = true
        self.level = .popUpMenu
        self.isMovable = false
        self.hidesOnDeactivate = false
        self.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        self.isMovableByWindowBackground = false

        // 毛玻璃背景视图
        let visualEffect = NSVisualEffectView()
        visualEffect.material = .popover
        visualEffect.blendingMode = .behindWindow
        visualEffect.state = .active
        visualEffect.wantsLayer = true
        visualEffect.layer?.cornerRadius = 10
        visualEffect.layer?.masksToBounds = true

        // 内容视图
        let clipView = NSClipView()
        clipView.backgroundColor = .clear
        clipView.documentView = tableView
        clipView.drawsBackground = false

        visualEffect.addSubview(clipView)
        self.contentView = visualEffect

        // 表格配置（现代样式）
        let column = NSTableColumn(identifier: NSUserInterfaceItemIdentifier("cand"))
        column.width = 240
        tableView.addTableColumn(column)
        tableView.headerView = nil
        tableView.backgroundColor = .clear
        tableView.rowHeight = 28
        tableView.intercellSpacing = NSSize(width: 0, height: 4)
        tableView.style = .fullWidth
        tableView.dataSource = self
        tableView.delegate = self
        tableView.target = self
        tableView.doubleAction = #selector(onDoubleClick(_:))
        tableView.selectionHighlightStyle = .none  // 自定义选中样式
    }

    func setCandidates(_ cands: [Candidate], selected: Int = 0) {
        candidates = cands
        tableView.reloadData()
        if selected < candidates.count {
            tableView.selectRowIndexes(IndexSet(integer: selected), byExtendingSelection: false)
            tableView.scrollRowToVisible(selected)
        }
        // 调整窗口高度（自适应）
        let h = min(cands.count, 9) * 32 + 16
        self.setContentSize(NSSize(width: 240, height: h))
    }

    func setSelected(_ idx: Int) {
        guard idx >= 0 && idx < candidates.count else { return }
        tableView.selectRowIndexes(IndexSet(integer: idx), byExtendingSelection: false)
        tableView.scrollRowToVisible(idx)
    }

    func selected() -> Int {
        return tableView.selectedRow
    }

    @objc func onDoubleClick(_ sender: Any) {
        let idx = tableView.selectedRow
        if idx >= 0 {
            onSelect?(idx)
        }
    }
}

extension CandidateWindow: NSTableViewDataSource, NSTableViewDelegate {
    func numberOfRows(in tableView: NSTableView) -> Int {
        return min(candidates.count, 9)
    }

    func tableView(_ tableView: NSTableView, viewFor tableColumn: NSTableColumn?, row: Int) -> NSView? {
        // 自定义 cell：序号徽章 + 候选词 + 拼音提示
        let cell = NSTableCellView()
        cell.wantsLayer = true

        if row >= candidates.count {
            return cell
        }
        let c = candidates[row]
        let isSelected = (row == tableView.selectedRow)

        // 选中背景：圆角强调色
        if isSelected {
            cell.layer?.backgroundColor = colorSelectedBg.cgColor
            cell.layer?.cornerRadius = 6
        } else {
            cell.layer?.backgroundColor = NSColor.clear.cgColor
            cell.layer?.cornerRadius = 0
        }

        // 序号徽章
        let indexLabel = NSTextField(labelWithString: "\(row + 1)")
        indexLabel.font = NSFont.monospacedDigitSystemFont(ofSize: 12, weight: .medium)
        indexLabel.alignment = .center
        indexLabel.textColor = isSelected ? NSColor.white.withAlphaComponent(0.9) : colorIndexText
        indexLabel.frame = NSRect(x: 8, y: 6, width: 20, height: 16)
        // 序号背景圆
        if !isSelected {
            let badgeView = NSView(frame: NSRect(x: 6, y: 4, width: 22, height: 20))
            badgeView.wantsLayer = true
            badgeView.layer?.backgroundColor = NSColor.separatorColor.withAlphaComponent(0.3).cgColor
            badgeView.layer?.cornerRadius = 10
            cell.addSubview(badgeView)
            badgeView.addSubview(indexLabel)
            indexLabel.frame.origin = NSPoint(x: 0, y: 2)
            indexLabel.frame.size = NSSize(width: 22, height: 16)
        } else {
            cell.addSubview(indexLabel)
        }

        // 候选词文字
        let wordLabel = NSTextField(labelWithString: c.word)
        wordLabel.font = NSFont.systemFont(ofSize: 16, weight: isSelected ? .semibold : .regular)
        wordLabel.alignment = .left
        wordLabel.textColor = isSelected ? colorSelectedText : colorNormalText
        wordLabel.frame = NSRect(x: 36, y: 4, width: 140, height: 20)
        cell.addSubview(wordLabel)

        // 拼音提示（右对齐，浅色）
        if !c.pinyin.isEmpty && !isSelected {
            let pyLabel = NSTextField(labelWithString: c.pinyin)
            pyLabel.font = NSFont.systemFont(ofSize: 11)
            pyLabel.alignment = .right
            pyLabel.textColor = colorPinyinText
            pyLabel.frame = NSRect(x: 130, y: 6, width: 100, height: 16)
            cell.addSubview(pyLabel)
        }

        return cell
    }

    func tableView(_ tableView: NSTableView, shouldSelectRow row: Int) -> Bool {
        return true
    }

    func tableViewSelectionDidChange(_ notification: Notification) {
        // 重绘以更新选中色
        tableView.reloadData()
    }
}

// MARK: - 输入法主控制器

class SamimeInputController: IMKInputController {
    private let client = GoEngineClient()
    private var preeditBuffer = ""
    private var candidates: [Candidate] = []
    private var selectedIdx = 0
    private var candidateWindow: CandidateWindow?

    // 服务端激活时调用
    override init!(server: IMKServer!, delegate: Any!, client inputClient: Any!) {
        super.init(server: server, delegate: delegate, client: inputClient)
        client.connect()
    }

    // 处理按键事件
    override func handle(_ event: NSEvent!, client sender: Any!) -> Bool {
        guard let event = event else { return false }

        switch event.type {
        case .keyDown:
            return handleKeyDown(event, client: sender)
        default:
            return false
        }
    }

    private func handleKeyDown(_ event: NSEvent, client sender: Any) -> Bool {
        let chars = event.charactersIgnoringModifiers ?? ""
        guard let char = chars.unicodeScalars.first else { return false }

        // 数字键 1-9：选词
        if char >= "1" && char <= "9" {
            let idx = Int(char.value - Character("1").unicodeScalars.first!.value)
            return selectCandidate(idx, client: sender)
        }

        // 空格：选第一个候选
        if char == " " {
            if !preeditBuffer.isEmpty {
                return selectCandidate(0, client: sender)
            }
            return false
        }

        // 回车：提交当前预编辑（如有候选）或直接透传
        if event.keyCode == 36 { // Return
            if !preeditBuffer.isEmpty {
                return selectCandidate(0, client: sender)
            }
            return false
        }

        // ESC：清空预编辑
        if event.keyCode == 53 {
            if !preeditBuffer.isEmpty {
                preeditBuffer = ""
                client.reset()
                updatePreedit(client: sender)
                return true
            }
            return false
        }

        // Backspace：删除最后一个字符
        if event.keyCode == 51 {
            if !preeditBuffer.isEmpty {
                preeditBuffer.removeLast()
                updatePreedit(client: sender)
                return true
            }
            return false
        }

        // 字母键：加入预编辑
        if (char >= "a" && char <= "z") || (char >= "A" && char <= "Z") {
            let lower = String(char).lowercased()
            preeditBuffer += lower
            updatePreedit(client: sender)
            return true
        }

        return false
    }

    private func updatePreedit(client sender: Any) {
        if preeditBuffer.isEmpty {
            // 清空预编辑
            if let textInput = sender as? IMKTextInput {
                textInput.setMarkedText("",
                                         selectionRange: NSRange(location: 0, length: 0),
                                         replacementRange: NSRange(location: NSNotFound, length: NSNotFound))
            }
            hideCandidateWindow()
            return
        }

        // 调用 Go 引擎搜索
        candidates = client.search(preeditBuffer)

        if candidates.isEmpty {
            // 没候选，显示原始拼音
            if let textInput = sender as? IMKTextInput {
                textInput.setMarkedText(preeditBuffer,
                                         selectionRange: NSRange(location: 0, length: preeditBuffer.count),
                                         replacementRange: NSRange(location: NSNotFound, length: NSNotFound))
            }
            hideCandidateWindow()
            return
        }

        // 显示预编辑串（带下划线）
        let displayText = candidates.first?.word ?? preeditBuffer
        if let textInput = sender as? IMKTextInput {
            textInput.setMarkedText(displayText,
                                     selectionRange: NSRange(location: 0, length: displayText.count),
                                     replacementRange: NSRange(location: NSNotFound, length: NSNotFound))
        }

        // 显示候选窗
        showCandidateWindow(client: sender)
    }

    private func selectCandidate(_ idx: Int, client sender: Any) -> Bool {
        if idx < 0 || idx >= candidates.count {
            return false
        }
        let c = candidates[idx]
        _ = client.commit(c.word, pinyin: c.pinyin)

        // 插入文字到目标应用
        if let textInput = sender as? IMKTextInput {
            textInput.insertText(c.word,
                                  replacementRange: NSRange(location: NSNotFound, length: NSNotFound))
        }

        preeditBuffer = ""
        candidates = []
        selectedIdx = 0
        hideCandidateWindow()
        return true
    }

    private func showCandidateWindow(client sender: Any) {
        // 获取光标位置
        var cursorRect = NSRect.zero
        if let textInput = sender as? IMKTextInput {
            textInput.attributes(forCharacterIndex: 0, lineHeightRectangle: &cursorRect)
        }
        if cursorRect == .zero {
            // 用鼠标位置兜底
            cursorRect = NSRect(origin: NSEvent.mouseLocation, size: .zero)
        }

        if candidateWindow == nil {
            candidateWindow = CandidateWindow()
            candidateWindow?.onSelect = { [weak self] idx in
                guard let self = self else { return }
                _ = self.selectCandidate(idx, client: sender)
            }
        }

        // 显示窗口
        let origin = NSPoint(x: cursorRect.origin.x,
                              y: cursorRect.origin.y - 230)
        candidateWindow?.setFrameOrigin(origin)
        candidateWindow?.setCandidates(candidates, selected: 0)
        candidateWindow?.orderFrontRegardless()
    }

    private func hideCandidateWindow() {
        candidateWindow?.orderOut(nil)
    }
}

// MARK: - 服务端管理

class SamimeServer {
    static let shared = SamimeServer()
    private var client = GoEngineClient()

    func activate() {
        _ = client.connect()
    }

    func deactivate() {
        client.disconnect()
    }
}

// MARK: - 应用图标占位符

// 应用图标需要在 Xcode 项目中添加 .icns 文件
// 资源路径: Resources/icon.icns
// Info.plist 中 tsInputMethodIconFileKey = "icon"
//
// 简单生成 .icns 的方法：
//   1. 准备 1024x1024 PNG
//   2. 用 iconutil 命令生成
//      mkdir Samime.iconset
//      sips -z 16 16     icon.png --out Samime.iconset/icon_16x16.png
//      sips -z 32 32     icon.png --out Samime.iconset/icon_16x16@2x.png
//      sips -z 32 32     icon.png --out Samime.iconset/icon_32x32.png
//      sips -z 64 64     icon.png --out Samime.iconset/icon_32x32@2x.png
//      sips -z 128 128   icon.png --out Samime.iconset/icon_128x128.png
//      sips -z 256 256   icon.png --out Samime.iconset/icon_128x128@2x.png
//      sips -z 256 256   icon.png --out Samime.iconset/icon_256x256.png
//      sips -z 512 512   icon.png --out Samime.iconset/icon_256x256@2x.png
//      sips -z 512 512   icon.png --out Samime.iconset/icon_512x512.png
//      sips -z 1024 1024 icon.png --out Samime.iconset/icon_512x512@2x.png
//      iconutil -c icns Samime.iconset
//   3. 复制 icon.icns 到 bundle 的 Resources/

// SamimeInputController.swift
// macOS IMK 输入法控制器骨架
//
// 编译:
//   xcodebuild -project SamimeInputMethod.xcodeproj
// 或
//   swiftc -framework InputMethodKit -framework Cocoa \
//          -emit-module -emit-library \
//          SamimeInputController.swift -o SamimeInputMethod.bundle
//
// 安装:
//   1. 把编译好的 .bundle 复制到 ~/Library/Input Methods/SamimeInputMethod.bundle
//   2. 退出登录再重新登录
//   3. 系统偏好设置 -> 键盘 -> 输入源 -> 添加 SamimeInputMethod

import Cocoa
import InputMethodKit
import Foundation

// MARK: - Go 引擎客户端（通过 Unix Domain Socket）

class GoEngineClient {
    private var socket: Int32 = -1
    private let socketPath: String

    init(socketPath: String = NSHomeDirectory() + "/.samime/macime.sock") {
        self.socketPath = socketPath
    }

    deinit {
        disconnect()
    }

    func connect() -> Bool {
        socket = Darwin.socket(AF_UNIX, SOCK_STREAM, 0)
        if socket < 0 { return false }

        var addr = sockaddr_un()
        addr.sun_family = sa_family_t(AF_UNIX)
        let pathBytes = socketPath.utf8CString
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
        return true
    }

    func disconnect() {
        if socket >= 0 {
            Darwin.close(socket)
            socket = -1
        }
    }

    private func ensureConnected() -> Bool {
        if socket >= 0 { return true }
        return connect()
    }

    private func sendRequest(_ req: [String: Any]) -> [String: Any]? {
        if !ensureConnected() { return nil }

        guard let data = try? JSONSerialization.data(withJSONObject: req),
              let str = String(data: data, encoding: .utf8) else {
            return nil
        }
        let line = str + "\n"
        let bytes = [UInt8](line.utf8)
        let sent = bytes.withUnsafeBufferPointer {
            Darwin.send(socket, $0.baseAddress, $0.count, 0)
        }
        if sent < 0 { return nil }

        // 读响应（直到 \n）
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

    func search(_ preedit: String) -> [[String: Any]]? {
        guard let resp = sendRequest(["method": "search", "preedit": preedit]) else {
            return nil
        }
        return resp["candidates"] as? [[String: Any]]
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
}

// MARK: - IMK 输入法控制器

class SamimeInputController: IMKInputController {
    private let client = GoEngineClient()
    private var preeditBuffer = ""

    // 处理按键事件
    // 返回 true 表示已处理，false 表示透传
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
            return selectCandidate(idx)
        }

        // 空格：选第一个候选
        if char == " " {
            return selectCandidate(0)
        }

        // 回车：提交当前预编辑（如有候选）或直接透传
        if event.keyCode == 36 { // Return key
            if !preeditBuffer.isEmpty {
                return selectCandidate(0)
            }
            return false
        }

        // ESC：清空预编辑
        if event.keyCode == 53 { // ESC key
            preeditBuffer = ""
            client.reset()
            return true
        }

        // Backspace：删除最后一个字符
        if event.keyCode == 51 {
            if !preeditBuffer.isEmpty {
                preeditBuffer.removeLast()
                updatePreedit(client: sender as! IMKTextInput)
            }
            return true
        }

        // 字母键：加入预编辑
        if (char >= "a" && char <= "z") || (char >= "A" && char <= "Z") {
            let lower = String(char).lowercased()
            preeditBuffer += lower
            updatePreedit(client: sender as! IMKTextInput)
            return true
        }

        return false
    }

    private func updatePreedit(client: IMKTextInput) {
        if preeditBuffer.isEmpty {
            client.setMarkedText("", selectionRange: NSRange(location: 0, length: 0),
                                  replacementRange: NSRange(location: NSNotFound, length: NSNotFound))
            return
        }

        // 调用 Go 引擎搜索
        guard let candidates = client.search(preeditBuffer),
              !candidates.isEmpty else {
            // 没有候选，显示原始拼音
            client.setMarkedText(preeditBuffer,
                                  selectionRange: NSRange(location: 0, length: preeditBuffer.count),
                                  replacementRange: NSRange(location: NSNotFound, length: NSNotFound))
            return
        }

        // 显示候选词列表（预编辑区域）
        let displayText = candidates.prefix(9).enumerated().map { (i, cand) -> String in
            let word = cand["Word"] as? String ?? ""
            return "\(i + 1).\(word)"
        }.joined(separator: " ")

        client.setMarkedText(displayText,
                              selectionRange: NSRange(location: 0, length: displayText.count),
                              replacementRange: NSRange(location: NSNotFound, length: NSNotFound))
    }

    private func selectCandidate(_ idx: Int) -> Bool {
        guard let candidates = client.search(preeditBuffer),
              idx < candidates.count else {
            return false
        }
        let word = candidates[idx]["Word"] as? String ?? ""
        let pinyin = candidates[idx]["Pinyin"] as? String ?? ""
        _ = client.commit(word, pinyin: pinyin)

        // 插入到目标应用
        if let textInput = self.client() as? IMKTextInput {
            textInput.insertText(word, replacementRange: NSRange(location: NSNotFound, length: NSNotFound))
        }

        preeditBuffer = ""
        return true
    }
}

// MARK: - 服务端激活/停用

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

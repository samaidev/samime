// SamimeUpdateChecker.swift - macOS 自动更新（Sparkle 集成）
//
// Sparkle 是 macOS 事实标准的自动更新框架
// 安装: https://sparkle-project.org/documentation/programmatic-setup/
//
// 1. 在 Xcode 项目中添加 Sparkle.framework (via SPM 或 CocoaPods)
// 2. 生成 EdDSA 密钥: ./bin/sign_update your_update.zip
// 3. 发布 appcast.xml
// 4. 在 Info.plist 中设置 SUFeedURL

import Cocoa
// import Sparkle  // 取消注释以启用 Sparkle

class SamimeUpdateChecker {
    static let shared = SamimeUpdateChecker()

    #if canImport(Sparkle)
    // private let updaterController = SPUStandardUpdaterController(
    //     startingUpdater: true,
    //     updaterDelegate: nil,
    //     userDriverDelegate: nil
    // )
    #endif

    /// 启动时检查更新
    func startAutomaticChecks() {
        #if canImport(Sparkle)
        // updaterController.updater.automaticallyChecksForUpdates = true
        // updaterController.updater.updateCheckInterval = 86400  // 24 小时
        // updaterController.startUpdater()
        print("[Samime] Sparkle 自动更新已启用")
        #else
        // 回退: 用 GitHub API 手动检查
        checkForUpdatesViaGitHub()
        #endif
    }

    /// 手动检查更新（菜单触发）
    @objc func checkForUpdates(_ sender: Any?) {
        #if canImport(Sparkle)
        // updaterController.checkForUpdates(sender)
        #else
        checkForUpdatesViaGitHub()
        #endif
    }

    /// 通过 GitHub API 检查更新（无需 Sparkle 的回退方案）
    func checkForUpdatesViaGitHub() {
        let url = URL(string: "https://api.github.com/repos/samaidev/samime/releases/latest")!
        let task = URLSession.shared.dataTask(with: url) { data, _, error in
            guard let data = data, error == nil else {
                DispatchQueue.main.async {
                    self.showUpdateAlert(title: "检查更新失败",
                                          message: error?.localizedDescription ?? "网络错误")
                }
                return
            }

            do {
                if let json = try JSONSerialization.jsonObject(with: data) as? [String: Any],
                   let tagName = json["tag_name"] as? String,
                   let htmlURL = json["html_url"] as? String {

                    let currentVersion = "1.0.0"
                    let latestVersion = tagName.hasPrefix("v") ? String(tagName.dropFirst()) : tagName

                    DispatchQueue.main.async {
                        if latestVersion > currentVersion {
                            self.showUpdateAvailable(
                                version: tagName,
                                url: htmlURL,
                                body: json["body"] as? String ?? ""
                            )
                        } else {
                            self.showUpdateAlert(title: "已是最新版本",
                                                  message: "Samime \(currentVersion)")
                        }
                    }
                }
            } catch {
                DispatchQueue.main.async {
                    self.showUpdateAlert(title: "解析更新信息失败", message: error.localizedDescription)
                }
            }
        }
        task.resume()
    }

    private func showUpdateAvailable(version: String, url: String, body: String) {
        let alert = NSAlert()
        alert.alertStyle = .informational
        alert.messageText = "发现新版本 \(version)"
        alert.informativeText = body.prefix(500).description
        alert.addButton(withTitle: "下载更新")
        alert.addButton(withTitle: "稍后提醒")
        alert.addButton(withTitle: "跳过此版本")

        let response = alert.runModal()
        if response == .alertFirstButtonReturn {
            NSWorkspace.shared.open(URL(string: url)!)
        }
    }

    private func showUpdateAlert(title: String, message: String) {
        let alert = NSAlert()
        alert.alertStyle = .informational
        alert.messageText = title
        alert.informativeText = message
        alert.addButton(withTitle: "确定")
        alert.runModal()
    }
}

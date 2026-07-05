# macOS IMK 完整实现

## 文件结构

```
internal/macime/swift/
├── SamimeInputController.swift  # IMK 控制器 + GoEngineClient + CandidateWindow
├── Info.plist                   # Bundle 配置
├── build_imk.sh                 # 构建脚本
├── sign_and_notarize.sh         # 签名 + 公证脚本
└── README.md                    # 本文件
```

## 实现的组件

### 1. GoEngineClient
- 通过 Unix Domain Socket (`~/.samime/macime.sock`) 与 Go 引擎通信
- JSON over line-delimited 协议（与 Windows TSF 协议一致）
- 支持 `search` / `commit` / `reset` / `status`
- 线程安全（用 dispatch_queue）

### 2. CandidateWindow
- 继承 `NSWindow`
- 用 `NSTableView` 显示候选词列表
- 选中高亮（系统色 `selectedControlColor`）
- 双击选词（`onSelect` 回调）
- 自动调整窗口大小（最多 9 个候选）
- 窗口级别 `.popUpMenu`（始终在顶层）

### 3. SamimeInputController
- 继承 `IMKInputController`
- 处理按键：字母（拼音）/ 数字 1-9（选词）/ 空格 / 回车 / ESC / 退格
- 通过 `setMarkedText` 显示预编辑
- 通过 `insertText` 提交文字到目标应用
- 通过 `attributes(forCharacterIndex:lineHeightRectangle:)` 获取光标位置

### 4. 服务端管理
- `SamimeServer.shared` 单例
- 启动时连接 Go 引擎

## 构建步骤

### 1. 准备工作

```bash
# 安装 Xcode Command Line Tools
xcode-select --install

# 拉代码
cd ~/dev
git clone https://github.com/samaidev/samime.git
cd samime

# 交叉编译 Go 引擎（在 Linux 上）
GOOS=darwin GOARCH=arm64 go build -o bin/samime-darwin-arm64 ./cmd/ime-cli
# 或在 macOS 上直接编译
go build -o ~/bin/samime ./cmd/ime-cli
```

### 2. 编译 IMK Bundle

```bash
cd internal/macime/swift
bash build_imk.sh
```

输出：
```
SamimeInputMethod.bundle/
└── Contents/
    ├── Info.plist
    ├── MacOS/
    │   └── SamimeInputMethod
    └── Resources/
        └── icon.icns   (可选)
```

### 3. 启动 Go 引擎服务

```bash
# 前台运行（调试）
~/bin/samime -mode=service

# 或后台运行
nohup ~/bin/samime -mode=service > ~/.samime/samime.log 2>&1 &

# 开机自启动（用 launchd）
cat > ~/Library/LaunchAgents/com.samime.service.plist <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.samime.service</string>
    <key>ProgramArguments</key>
    <array>
        <string>/Users/YOU/bin/samime</string>
        <string>-mode=service</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
EOF
launchctl load ~/Library/LaunchAgents/com.samime.service.plist
```

### 4. 启用输入法

1. 注销并重新登录（让系统发现新 bundle）
2. 系统偏好设置 → 键盘 → 输入源
3. 点 `+`，搜索 "Samime"
4. 添加 "Samime Input Method"

### 5. （可选）签名与公证

如果不签名，macOS 11+ 会拒绝加载 bundle。

```bash
DEVELOPER_ID="Developer ID Application: Your Name (TEAMID)" \
APPLE_ID="you@example.com" \
TEAM_ID="TEAMID12345" \
APP_PASSWORD="xxxx-xxxx-xxxx-xxxx" \
bash sign_and_notarize.sh
```

## 生成应用图标

```bash
# 准备 1024x1024 PNG（命名为 icon.png）
mkdir Samime.iconset
sips -z 16 16     icon.png --out Samime.iconset/icon_16x16.png
sips -z 32 32     icon.png --out Samime.iconset/icon_16x16@2x.png
sips -z 32 32     icon.png --out Samime.iconset/icon_32x32.png
sips -z 64 64     icon.png --out Samime.iconset/icon_32x32@2x.png
sips -z 128 128   icon.png --out Samime.iconset/icon_128x128.png
sips -z 256 256   icon.png --out Samime.iconset/icon_128x128@2x.png
sips -z 256 256   icon.png --out Samime.iconset/icon_256x256.png
sips -z 512 512   icon.png --out Samime.iconset/icon_256x256@2x.png
sips -z 512 512   icon.png --out Samime.iconset/icon_512x512.png
sips -z 1024 1024 icon.png --out Samime.iconset/icon_512x512@2x.png
iconutil -c icns Samime.iconset
mv icon.icns internal/macime/swift/icon.icns
```

## 调试

### 查看日志

```bash
# 实时查看 IMK 日志
log stream --predicate 'subsystem == "com.apple.InputMethodKit"' --debug

# 查看 Go 引擎日志
tail -f ~/.samime/samime.log
```

### 重新加载 IMK

```bash
# 杀掉 IMK 服务进程
killall SamimeInputMethod

# 或重新登录
osascript -e 'tell application "System Events" to log out'
```

### 检查 bundle 是否被系统识别

```bash
# 列出所有已安装的输入法
ls ~/Library/Input\ Methods/

# 检查 Info.plist
plutil -p ~/Library/Input\ Methods/SamimeInputMethod.bundle/Contents/Info.plist
```

## 当前状态

- [x] **GoEngineClient**：Unix Socket + JSON 协议
- [x] **CandidateWindow**：NSTableView + 选中高亮 + 双击选词
- [x] **SamimeInputController**：按键处理 + 预编辑 + 提交
- [x] **build_imk.sh**：bundle 构建脚本
- [x] **sign_and_notarize.sh**：签名 + 公证脚本
- [x] **launchd 配置示例**：开机自启 Go 引擎
- [x] **图标生成说明**：iconutil 命令

## 已知限制

1. **光标位置**：`attributes(forCharacterIndex:lineHeightRectangle:)` 在某些应用中返回不准确
2. **预编辑样式**：未自定义下划线/背景色（应用 `IMKServer` 的属性字典）
3. **签名要求**：必须 Developer ID 签名才能在 macOS 11+ 加载
4. **未提供 .icns**：需用户自行生成图标

## 参考

- Apple IMK 完整文档：https://developer.apple.com/documentation/inputmethodkit
- IMK 示例：https://developer.apple.com/library/archive/samplecode/InputMethodKitDemonstration/
- 签名与公证：https://developer.apple.com/developer-id/

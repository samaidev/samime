# macOS IMK 输入法

## 文件结构

```
internal/macime/
├── macime.go           # Go 端 Unix Domain Socket 服务
└── swift/
    ├── SamimeInputController.swift  # Swift IMK 控制器
    ├── Info.plist                  # Bundle 配置
    └── README.md                   # 本文件
```

## 架构

```
+-------------------+    Unix Socket     +-------------------+
|  macOS 应用       |  <--------------   |  Go Engine        |
|  (任何 Cocoa 应用) |  JSON 请求/响应     |  (samime)         |
+-------------------+                    +-------------------+
        ^                                        |
        | insertText:                            |
        v                                        v
+-------------------+    Unix Socket     +-------------------+
|  IMK Bundle       |  ----------------> |  Go Engine        |
|  SamimeInputMethod|   search/commit    |  (samime)         |
|  .bundle (Swift)  |                    +-------------------+
+-------------------+
```

## 工作流程

1. 用户在 macOS 应用中打字
2. 系统通过 IMK 框架把按键事件传给 `SamimeInputController`
3. Swift 控制器把拼音发送给 Go 引擎（Unix Socket）
4. Go 引擎返回候选词
5. Swift 控制器通过 `setMarkedText` 显示候选
6. 用户选词后，调用 `insertText` 插入文字

## 编译 Go 引擎（macOS）

```bash
# 在 Linux 上交叉编译
GOOS=darwin GOARCH=arm64 go build -o bin/samime-darwin-arm64 ./cmd/ime-cli

# 或在 macOS 上直接编译
go build -o samime ./cmd/ime-cli
```

## 启动 Go 引擎服务

```bash
./samime -mode=service
# 监听 ~/.samime/macime.sock
```

## 创建 IMK Bundle

### 方式1: Xcode 项目

1. 打开 Xcode
2. File -> New -> Project -> macOS -> Bundle
3. 命名为 `SamimeInputMethod`
4. 添加 `SamimeInputController.swift`
5. 添加 `Info.plist`（见下方）
6. Link framework: `InputMethodKit.framework`, `Cocoa.framework`
7. Build

### 方式2: 命令行

```bash
mkdir -p SamimeInputMethod.bundle/Contents/MacOS
swiftc -framework InputMethodKit -framework Cocoa \
    -emit-library \
    SamimeInputController.swift \
    -o SamimeInputMethod.bundle/Contents/MacOS/SamimeInputMethod
cp Info.plist SamimeInputMethod.bundle/Contents/
```

## Info.plist

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>zh_CN</string>
    <key>CFBundleExecutable</key>
    <string>SamimeInputMethod</string>
    <key>CFBundleIdentifier</key>
    <string>com.samime.inputmethod</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>Samime Input Method</string>
    <key>CFBundlePackageType</key>
    <string>BNDL</string>
    <key>CFBundleShortVersionString</key>
    <string>1.0</string>
    <key>InputMethodConnectionName</key>
    <string>SamimeInputMethod_1_Connection</string>
    <key>InputMethodServerControllerClass</key>
    <string>SamimeInputController</string>
    <key>tsInputMethodCharacterRepertoireKey</key>
    <array>
        <string>Hant</string>
        <string>Hans</string>
    </array>
    <key>tsInputMethodIconFileKey</key>
    <string>icon.icns</string>
    <key>LSBackgroundOnly</key>
    <true/>
</dict>
</plist>
```

## 安装

```bash
# 复制到输入法目录
cp -R SamimeInputMethod.bundle ~/Library/Input\ Methods/

# 启动 Go 引擎服务（可加入登录项）
~/bin/samime -mode=service &

# 重新登录或注销
# 系统偏好设置 -> 键盘 -> 输入源 -> + -> Samime Input Method
```

## 调试

```bash
# 实时查看 IMK 日志
log stream --predicate 'subsystem == "com.apple.InputMethodKit"'

# 查看 Go 引擎日志
~/bin/samime -mode=service 2>&1 | tee ~/samime.log
```

## 当前状态

- [x] Go 端 Unix Domain Socket 服务
- [x] JSON 协议（与 Windows TSF 相同）
- [x] Swift IMK 控制器骨架
- [ ] 候选词窗口 UI（用 IMKCandidateController）
- [ ] Xcode 项目模板
- [ ] 应用图标
- [ ] 代码签名与公证（macOS 11+ 强制要求）

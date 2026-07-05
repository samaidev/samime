# Windows TSF Proxy 编译说明

## 文件结构

```
internal/winime/
├── winime.go              # Go 端入口（console/service）
├── pipe_windows.go        # Go 端命名管道服务（用 winio）
└── cpp/
    ├── samime_tsf.cpp     # C++ TSF 骨架
    ├── README.md          # 本文件
    └── BUILD.md           # 详细构建步骤
```

## 架构

```
+-------------------+        Named Pipe         +-------------------+
|   Windows 应用    |  Keyboard events as JSON  |   Go Engine       |
|   (any app)       |  <----------------------  |   (samime.exe)    |
+-------------------+                          +-------------------+
        ^                                              |
        | Insert text via TSF                          |
        |                                              v
+-------------------+        Named Pipe         +-------------------+
|   TSF Proxy DLL   |  ---------------------->  |   Go Engine       |
|  samime_tsf.dll   |   Requests/Responses       |   (samime.exe)    |
|  (C++ COM impl)   |                            +-------------------+
+-------------------+
```

## 工作流程

1. 用户在 Windows 应用中打字（如记事本、Word）
2. TSF 框架把按键事件转给 samime_tsf.dll
3. C++ proxy 把拼音发送给 Go 引擎（命名管道 `\\.\pipe\goime`）
4. Go 引擎返回候选词列表
5. C++ proxy 显示候选窗
6. 用户选词后，C++ proxy 通知 Go 引擎 commit，并通过 TSF 把文字插入应用

## 编译 Go 引擎

```bash
# 在 Linux 上交叉编译
GOOS=windows GOARCH=amd64 go build -o bin/samime-windows-amd64.exe ./cmd/ime-cli

# 或在 Windows 上
go build -o samime.exe ./cmd/ime-cli
```

## 启动 Go 引擎服务（命名管道模式）

```cmd
:: Windows 上运行
samime.exe -mode=service
```

## 编译 TSF Proxy DLL（C++）

需要 Visual Studio Build Tools 或 MinGW。

### 方式1: Visual Studio

```cmd
cl /EHsc /LD samime_tsf.cpp /link ole32.lib oleaut32.lib msctf.lib ws2_32.lib
```

### 方式2: MinGW

```bash
x86_64-w64-mingw32-g++ -shared -o samime_tsf.dll samime_tsf.cpp -lole32 -loleaut32 -lmsctf -lws2_32
```

## 安装 TSF 服务

```cmd
:: 复制 DLL
copy samime_tsf.dll "C:\Program Files\SamIME\"

:: 注册 COM 服务器
regsvr32 "C:\Program Files\SamIME\samime_tsf.dll"

:: 在注册表中注册为 TSF 文本服务（需要 .reg 文件）
:: 参见 Microsoft TSF 文档
regedit /s samime_tsf.reg
```

## 当前状态

- [x] Go 端命名管道服务（用 winio.ListenPipe）
- [x] JSON 协议（search/commit/reset/status）
- [x] C++ TSF proxy 骨架
- [ ] 完整 ITfKeyEventSink 实现（按键捕获）
- [ ] 候选词窗口 UI（C++ HWND）
- [ ] 注册表脚本 (.reg)
- [ ] 数字签名（Windows 11 强制要求）

## 调试

启动 Go 引擎时开启日志：
```cmd
set GOIME_LOG=debug
samime.exe -mode=service
```

C++ proxy 通过 `OutputDebugStringW` 输出日志，用 DebugView 查看。

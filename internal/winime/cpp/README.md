# Windows TSF Proxy 完整实现

## 文件结构

```
internal/winime/cpp/
├── samime_tsf.h              # 完整 TSF 接口声明
├── samime_tsf.cpp            # TSF TextService 实现（ITfKeyEventSink 等）
├── samime_reg.cpp            # 注册表写入逻辑
├── samime_tsf.def            # DLL 导出定义
├── build_tsf.bat             # 编译脚本（cl 或 g++）
├── samime_install.reg        # 手动安装注册表
├── samime_uninstall.reg      # 卸载注册表
└── README.md                 # 本文件
```

## 实现的 COM 接口

| 接口 | 用途 |
|------|------|
| `ITfTextInputProcessorEx` | TSF 主接口（Activate/Deactivate） |
| `ITfKeyEventSink` | 捕获所有按键事件 |
| `ITfCompositionSink` | 组合状态终止回调 |
| `ITfCandidateListUIElement` | 候选词列表 UI（系统候选窗） |
| `ITfUIElement` | UI 元素基接口 |
| `IClassFactory` | COM 类工厂 |

## 工作流程

```
用户在 Windows 应用打字
        ↓
TSF 框架把按键事件转给 samime_tsf.dll
        ↓
ITfKeyEventSink::OnKeyDown 被调用
        ↓
按键加入 preedit 缓冲区
        ↓
GoEngineClient::search()  ──→ 命名管道 \\.\pipe\goime
        ↓                              ↓
        ← JSON 响应 ←──────── Go 引擎返回候选词
        ↓
CandidateWindow 显示候选窗（自定义 HWND）
        ↓
用户按 1-9 选词
        ↓
GoEngineClient::commit() 通知 Go 引擎
        ↓
ITfInsertAtSelection::InsertTextAtSelection 插入到应用
```

## 编译

需要 Visual Studio Build Tools 或 MinGW。

### Visual Studio (推荐)

```cmd
:: 在 "x64 Native Tools Command Prompt" 中
cd C:\Users\Administrator\samime\internal\winime\cpp
build_tsf.bat
```

### MinGW

```bash
x86_64-w64-mingw32-g++ -shared -std=c++17 -O2 \
    -o samime_tsf.dll \
    samime_tsf.cpp samime_reg.cpp \
    -lole32 -loleaut32 -lmsctf -luser32 -lgdi32 -lws2_32 -lshlwapi
```

## 安装

### 自动安装（用 regsvr32）

```cmd
:: 1. 复制文件
mkdir "C:\Program Files\SamIME"
copy samime_tsf.dll "C:\Program Files\SamIME\"
copy C:\Users\Administrator\samime\bin\samime.exe "C:\Program Files\SamIME\"

:: 2. 启动 Go 引擎服务（用 schtasks 让其开机启动）
schtasks /Create /TN "SamimeService" /TR \
    "\"C:\Program Files\SamIME\samime.exe\" -mode=service" \
    /SC ONLOGON /RL HIGHEST /F

:: 3. 注册 TSF 服务
regsvr32 "C:\Program Files\SamIME\samime_tsf.dll"

:: 4. 在系统设置中启用
::    设置 -> 时间和语言 -> 语言 -> 中文(简体) -> 选项 -> 添加键盘 -> Samime
```

### 手动安装（用 .reg 文件）

```cmd
:: 1. 复制文件（同上）
:: 2. 双击 samime_install.reg 导入注册表
:: 3. 启动 samime.exe -mode=service
:: 4. 重新登录
```

## 卸载

```cmd
:: 1. 注销 COM 服务器
regsvr32 /u "C:\Program Files\SamIME\samime_tsf.dll"

:: 或双击 samime_uninstall.reg

:: 2. 停止 Go 引擎
schtasks /Delete /TN "SamimeService" /F
taskkill /IM samime.exe /F

:: 3. 删除文件
rmdir /S "C:\Program Files\SamIME"
```

## 测试

### 单元测试

```cmd
:: 编译后，先启动 Go 引擎
samime.exe -mode=service

:: 然后用 PowerShell 客户端测试
powershell -EncodedCommand <base64-encoded-script>
```

### 集成测试

打开记事本，在系统输入法中切换到 Samime，输入拼音应看到候选窗。

## 当前状态

- [x] **ITfTextInputProcessorEx**：激活/停用
- [x] **ITfKeyEventSink**：捕获字母、数字、空格、回车、退格、ESC
- [x] **ITfCompositionSink**：组合状态管理
- [x] **ITfCandidateListUIElement**：系统候选窗接口
- [x] **CandidateWindow**：自定义 GDI 候选窗（双缓冲、鼠标点击）
- [x] **GoEngineClient**：与 Go 引擎的命名管道/TCP 通信
- [x] **注册表写入**：DllRegisterServer + samime_install.reg
- [x] **构建脚本**：build_tsf.bat（cl + MinGW）
- [ ] 完整 ITfContextComposition 实现（组合区编辑）
- [ ] 候选词鼠标点击响应
- [ ] 应用图标 .ico 文件
- [ ] 数字签名（Windows 11 强制要求 EV 证书）
- [ ] UI 现代化（用 Direct2D 替代 GDI）

## 已知限制

1. **预编辑文本插入**：当前 `setPreeditText` 是简化实现，完整版需 `ITfContextComposition`
2. **候选窗位置**：使用鼠标位置，应改为光标位置（用 `ITfContextView::GetRangeFromPoint`）
3. **签名**：未签名 DLL 在 Windows 11 上会触发 SmartScreen 警告
4. **管理员权限**：注册到 `HKLM` 需要管理员权限

## 参考资源

- Microsoft TSF 完整文档：https://learn.microsoft.com/en-us/windows/win32/tsf/text-services-framework
- TSF 示例代码：https://github.com/microsoft/Windows-classic-samples/tree/main/Samples/Win7Samples/winui/input/tsf
- 候选词 UI：https://learn.microsoft.com/en-us/windows/win32/tsf/candidate-list

# Fcitx5 适配器

## 文件结构

```
internal/fcitx5/
├── fcitx5.go          # Fcitx5 D-Bus 引擎
└── README.md          # 本文件
```

## 工作原理

```
+-------------+   D-Bus signal    +-------------+
| Fcitx5      |  <--------------- | samime      |
| (fcitx5)    |                    | (Go engine) |
+-------------+                    +-------------+
        ^                                  ^
        | start/stop                       | JSON
        v                                  | Unix Socket
+-------------+                    +-------------+
| GTK/Qt app  |                    | samime.exe  |
|             |                    | (Win TSF)   |
+-------------+                    +-------------+
```

## 安装步骤

### 1. 安装 Fcitx5

```bash
# Debian/Ubuntu
sudo apt install fcitx5 fcitx5-frontend-gtk3 fcitx5-frontend-qt5

# Fedora
sudo dnf install fcitx5 fcitx5-gtk fcitx5-qt5

# Arch
sudo pacman -S fcitx5 fcitx5-gtk fcitx5-qt5
```

### 2. 编译 GoIME

```bash
cd ~/dev/samime
go build -o ~/bin/samime ./cmd/ime-cli
```

### 3. 注册为 Fcitx5 addon

```bash
# 创建 addon 配置目录
mkdir -p ~/.local/share/fcitx5/addon/
mkdir -p ~/.local/share/fcitx5/inputmethod/

# 写入 addon 配置
cat > ~/.local/share/fcitx5/addon/samime.conf <<'EOF'
[Addon]
Name=Samime
Category=InputMethod
Version=1.0.0
Library=samime
Type=SharedLibrary
OnDemand=True
Configurable=True
EOF

# 写入输入法配置
cat > ~/.local/share/fcitx5/inputmethod/samime.conf <<'EOF'
[InputMethod]
Name=Samime
Icon=samime
Label=萨米
LangCode=zh_CN
Addon=samime
Configurable=True
EOF
```

### 4. 启动 Go 引擎服务

```bash
# 前台运行（调试）
~/bin/samime -mode=service

# 后台运行
nohup ~/bin/samime -mode=service > ~/.samime/samime.log 2>&1 &

# 开机自启（systemd user unit）
mkdir -p ~/.config/systemd/user/
cat > ~/.config/systemd/user/samime.service <<'EOF'
[Unit]
Description=Samime Input Method Engine
After=graphical-session.target

[Service]
ExecStart=%h/bin/samime -mode=service
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF
systemctl --user enable --now samime.service
```

### 5. 重启 Fcitx5

```bash
fcitx5 -r &
```

### 6. 启用 Samime 输入法

```bash
# GUI 配置
fcitx5-configtool

# 或直接修改 profile
cat > ~/.config/fcitx5/profile <<'EOF'
[Groups/0]
Name=Default
Default Layout=us
DefaultIM=samime

[Groups/0/Items/0]
Name=keyboard-us
Layout=

[Groups/0/Items/1]
Name=samime
Layout=

[GroupOrder]
0=Default
EOF
```

### 7. 设置环境变量（让 GTK/Qt 用 Fcitx5）

```bash
# 添加到 ~/.bashrc 或 ~/.xprofile
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
export SDL_IM_MODULE=fcitx
export GLFW_IM_MODULE=ibus  # GLFW 用 ibus 协议
```

## 调试

### 查看日志

```bash
# Fcitx5 日志
journalctl --user -u fcitx5 -f

# samime 日志
tail -f ~/.samime/samime.log

# D-Bus 调试
dbus-monitor --session "type='signal',sender='org.fcitx.Fcitx5.Samime'"
```

### 验证 D-Bus 注册

```bash
dbus-send --session --print-reply \
    --dest=org.freedesktop.DBus \
    /org/freedesktop/DBus \
    org.freedesktop.DBus.ListNames | grep Samime
```

### 检查 addon 是否被识别

```bash
fcitx5-diagnose | grep -A 5 samime
```

## 实现的 Fcitx5 addon 方法

| 方法 | 说明 |
|------|------|
| `ProcessKeyEvent` | 处理按键事件 |
| `Reset` | 重置状态 |
| `FocusIn` / `FocusOut` | 焦点切换 |
| `SetCursorRect` | 设置光标矩形 |
| `CurrentIM` | 返回当前输入法信息 |

## 当前状态

- [x] D-Bus 连接（用 godbus）
- [x] Bus name 注册（`org.fcitx.Fcitx5.Samime`）
- [x] Addon 接口方法
- [x] Addon 配置文件生成
- [x] 输入法配置文件生成
- [x] Profile 配置文件生成
- [x] systemd user unit 模板
- [ ] 完整 Fcitx5 InputContext V2 协议
- [ ] Fcitx5 Action（菜单项）
- [ ] Fcitx5 Config UI（用 Qt）

## 已知限制

1. **Type=SharedLibrary 要求**: Fcitx5 标准 addon 是 .so 文件，Go 编译的 .so 不完全兼容
   - 解决方案：用 `Type=External` + 独立进程模式（推荐）
2. **D-Bus 协议版本**：当前实现是简化版
3. **候选窗 UI**：依赖 Fcitx5 classic UI 模块

## 替代方案：Fcitx5 Forwarder（用 .so 桥接）

如果 SharedLibrary 模式不工作，可以写一个 C/C++ 的桥接 addon：

```
samime_fcitx5_bridge.so  ←  Fcitx5 加载这个
       ↓ Unix Socket
samime (Go 引擎)
```

桥接 .so 用 C++ 编写（~100 行），转发调用到 Go 进程。详见
`internal/fcitx5/cpp/bridge.cpp`（待实现）。

## 参考

- Fcitx5 开发文档：https://fcitx5.org/manual/
- Fcitx5 addon 示例：https://github.com/fcitx/fcitx5/tree/master/src/modules
- D-Bus 协议：https://www.freedesktop.org/wiki/Software/dbus

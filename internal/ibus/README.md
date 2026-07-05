# IBus D-Bus 引擎安装说明

## 工作原理

```
+-------------+    D-Bus signal     +-------------+
| IBus daemon |  <---------------   | samime      |
| (ibus-daemon)                     | (Go engine) |
+-------------+                     +-------------+
        ^                                   ^
        | start/stop                        | JSON over
        v                                   | Unix Socket
+-------------+                     +-------------+
| GTK/Qt app  |                     | samime-tsf  |
|             |                     | (Win)       |
+-------------+                     +-------------+
```

## 安装步骤

### 1. 安装 IBus

```bash
# Debian/Ubuntu
sudo apt install ibus ibus-gtk ibus-gtk3 ibus-qt4

# Fedora
sudo dnf install ibus ibus-gtk2 ibus-gtk3 ibus-qt

# Arch
sudo pacman -S ibus
```

### 2. 编译 GoIME

```bash
cd ~/dev/samime
go build -o ~/bin/samime ./cmd/ime-cli
```

### 3. 注册为 IBus 组件

```bash
# 写入 IBus 组件配置（用户级，无需 root）
mkdir -p ~/.config/ibus/component/
cat > ~/.config/ibus/component/samime.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<component>
    <name>org.freedesktop.IBus.Samime</name>
    <description>Samime Chinese Input Method (Go)</description>
    <exec>/home/YOU/bin/samime -mode=service</exec>
    <version>1.0.0</version>
    <author>samime</author>
    <license>MIT</license>
    <homepage>https://github.com/samaidev/samime</homepage>
    <textdomain>samime</textdomain>
    <engines>
        <engine>
            <name>samime</name>
            <longname>Samime Pinyin</longname>
            <description>Samime Chinese Input Method (Go)</description>
            <language>zh_CN</language>
            <license>MIT</license>
            <author>samime</author>
            <icon>samime</icon>
            <layout>us</layout>
            <rank>0</rank>
        </engine>
    </engines>
</component>
EOF

# 或用 Go 命令生成
~/bin/samime -mode=install-ibus
```

### 4. 重启 IBus

```bash
ibus restart
# 或
ibus write-cache
```

### 5. 添加 Samime 输入法

```bash
# 命令行添加
gsettings set org.freedesktop.ibus.general preload-engines \
    "['xkb:us::eng', 'samime']"

# 或在 GUI 中:
# 设置 -> 区域和语言 -> 输入源 -> + -> 汉语 -> Samime
```

### 6. 启动 Go 引擎

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

## 调试

### 查看日志

```bash
# IBus 日志
journalctl --user -u ibus.service -f

# samime 日志
tail -f ~/.samime/samime.log

# D-Bus 调试
dbus-monitor --session "type='signal',sender='org.freedesktop.IBus'"
```

### 验证 D-Bus 注册

```bash
# 检查 bus name 是否注册
dbus-send --session --print-reply \
    --dest=org.freedesktop.DBus \
    /org/freedesktop/DBus \
    org.freedesktop.DBus.ListNames | grep Samime
```

### 检查 IBus 组件

```bash
# 列出所有 IBus 组件
ibus list-engine | grep samime

# 输出应为:
# samime - Samime Pinyin
```

## 实现的 IBus Engine 接口方法

| 方法 | 说明 |
|------|------|
| `ProcessKeyEvent` | 处理按键事件（字母/数字/空格/回车/退格/ESC） |
| `SetCursorLocation` | 接收光标位置（用于候选窗定位） |
| `SetCapabilities` | 接收能力声明 |
| `FocusIn` / `FocusOut` | 焦点切换 |
| `Reset` | 重置状态 |
| `Enable` / `Disable` | 启用/禁用 |
| `PageUp` / `PageDown` | 候选窗翻页 |
| `CursorUp` / `CursorDown` | 候选窗光标移动 |
| `CandidateClicked` | 候选词被点击 |
| `PropertyActivate` | 属性菜单激活 |

## 当前状态

- [x] D-Bus 连接（用 godbus）
- [x] Bus name 注册（`org.freedesktop.IBus.Samime`）
- [x] Engine 接口完整实现
- [x] IBus 组件 XML 配置生成
- [x] systemd user unit 模板
- [ ] 完整 IBus LookupTable 序列化
- [ ] IBus Property 菜单（设置面板）
- [ ] 候选窗主题适配（GNOME/KDE）
- [ ] 单元测试（需要 mock D-Bus）

## 已知限制

1. **D-Bus 协议版本**：当前实现是简化版，IBus 完整协议需要 IBusMessage 序列化
2. **LookupTable**：用字符串数组代替完整的 IBusText 结构
3. **候选窗 UI**：依赖 IBus 自带的候选窗（GNOME Shell / KDE Plasma 集成）

## 参考

- IBus D-Bus 协议：https://github.com/ibus/ibus/blob/main/src/ibusengine.h
- ibus-pinyin（参考实现）：https://github.com/ibus/ibus-pinyin
- godbus 文档：https://github.com/godbus/dbus

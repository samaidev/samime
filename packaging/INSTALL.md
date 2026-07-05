# Samime 安装指南

## 三平台安装方式

### Windows

#### 方式1: 安装包（推荐）

1. 下载 `samime-setup-1.0.0.exe`
2. 双击运行，按提示安装
3. 安装完成后，Samime 服务自动启动
4. 系统设置 → 时间和语言 → 语言 → 中文 → 选项 → 添加键盘 → 选择 **Samime**

#### 方式2: 免安装

1. 下载 `samime-windows-amd64.exe`
2. 双击运行（命令行模式）或用命令行启动服务模式:
   ```cmd
   samime-windows-amd64.exe -mode=service
   ```
3. TSF 集成需另行编译 `samime_tsf.dll`（见开发文档）

#### 卸载

- 控制面板 → 程序和功能 → Samime → 卸载
- 或运行 `uninstall.exe`

---

### macOS

#### 方式1: .dmg 安装包（推荐）

1. 下载 `samime-1.0.0.dmg`
2. 双击打开，拖动 **Samime.app** 到 **Applications**
3. 打开 Applications，双击 Samime（首次需右键 → 打开）
4. 系统偏好设置 → 键盘 → 输入源 → + → 选择 **Samime Input Method**

#### 方式2: 命令行

```bash
# 下载
curl -L https://github.com/samaidev/samime/releases/download/v1.0.0/samime-darwin-arm64 -o samime
chmod +x samime

# 启动服务
./samime -mode=service &

# 安装 IMK Bundle（需另行构建）
cd internal/macime/swift
bash build_imk.sh
```

#### 卸载

- 拖动 Samime.app 到废纸篓
- 移除 `~/Library/Input Methods/SamimeInputMethod.bundle`
- 移除 `~/.samime/`（用户数据）

---

### Linux

#### 方式1: .deb 包（Debian/Ubuntu）

```bash
# 下载
wget https://github.com/samaidev/samime/releases/download/v1.0.0/samime-1.0.0-amd64.deb

# 安装
sudo dpkg -i samime-1.0.0-amd64.deb
sudo apt-get install -f  # 解决依赖

# 重启 IBus
ibus restart

# 添加输入法
gsettings set org.freedesktop.ibus.general preload-engines \
    "['xkb:us::eng', 'samime']"

# 启用开机自启
systemctl --user enable --now samime.service
```

#### 方式2: .rpm 包（Fedora/RHEL）

```bash
sudo dnf install samime-1.0.0-1.x86_64.rpm
ibus restart
```

#### 方式3: AppImage（免安装）

```bash
chmod +x Samime-1.0.0-x86_64.AppImage
./Samime-1.0.0-x86_64.AppImage -mode=service &
```

#### 方式4: 二进制

```bash
sudo install samime-linux-amd64 /usr/bin/samime

# 启动
samime -mode=service &

# 注册 IBus
mkdir -p ~/.config/ibus/component/
cat > ~/.config/ibus/component/samime.xml <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<component>
    <name>org.freedesktop.IBus.Samime</name>
    <description>Samime Chinese Input Method</description>
    <exec>/usr/bin/samime -mode=service</exec>
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
</component>
EOF

ibus restart
```

#### 设置环境变量

添加到 `~/.bashrc` 或 `~/.xprofile`:

```bash
export GTK_IM_MODULE=ibus
export QT_IM_MODULE=ibus
export XMODIFIERS=@im=ibus
```

#### 卸载

```bash
# .deb
sudo dpkg -r samime

# .rpm
sudo dnf remove samime

# 手动
sudo rm /usr/bin/samime
sudo rm /etc/ibus/component/samime.xml
rm -rf ~/.samime/
```

---

## 验证安装

### 命令行验证

```bash
samime -mode=demo
```

应输出候选词列表。

### 服务模式验证

```bash
# Linux/macOS
samime -mode=service &
echo '{"method":"ping"}' | nc -q1 127.0.0.1 7788
# 应返回 {"ok":true}
```

```cmd
:: Windows
samime.exe -mode=service
:: 用 PowerShell 连接命名管道
```

---

## 数据存储位置

| 平台 | 用户词典 | 配置 |
|------|---------|------|
| Linux | `~/.samime/userdict/` | `~/.samime/` |
| macOS | `~/Library/Application Support/Samime/` | 同左 |
| Windows | `%APPDATA%\Samime\userdict\` | `%APPDATA%\Samime\` |

---

## 故障排查

### Q: 输入法没有出现在系统列表中

**Windows**: 运行 `regsvr32 samime_tsf.dll`，重启电脑

**macOS**: 注销重新登录，或 `killall SamimeInputMethod`

**Linux**: `ibus restart`，检查 `~/.config/ibus/component/samime.xml`

### Q: 候选词不出现

1. 确认服务在运行: `samime -mode=service`
2. 检查日志（stderr 输出）
3. Linux 检查环境变量: `echo $GTK_IM_MODULE`

### Q: 性能慢

- 首次启动需加载 136k 词典 + 22MB 2-gram 模型，约 500ms
- 后续查询 < 1ms
- 如仍慢，用 `-mode=bench` 测试

---

## 构建

如需自行构建，参见各平台打包脚本:

```bash
# 全平台二进制
bash packaging/build_all.sh 1.0.0

# Linux .deb/.rpm
bash packaging/linux/build_linux_package.sh 1.0.0 all

# Windows NSIS (在 Windows 上)
packaging\windows\build_windows_package.bat

# macOS .dmg (在 macOS 上)
bash packaging/macos/build_macos_package.sh 1.0.0
```

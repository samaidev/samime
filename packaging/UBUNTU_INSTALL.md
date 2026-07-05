# Samime 在 Ubuntu 上的安装指南

> 适用于 Ubuntu 20.04 / 22.04 / 24.04 及衍生版本（Linux Mint / Pop!_OS / Kubuntu 等）

---

## 方式一：.deb 包安装（推荐）

### 1. 下载安装包

从 GitHub Releases 下载最新 .deb 包：

```bash
# 下载（以 v1.0.0 为例）
wget https://github.com/samaidev/samime/releases/download/v1.0.0/samime-1.0.0-amd64.deb
```

或者用浏览器打开 https://github.com/samaidev/samime/releases 下载 `samime-1.0.0-amd64.deb`。

### 2. 安装依赖

Samime 依赖 IBus 输入法框架（Ubuntu 默认已装）：

```bash
# 确保 IBus 已安装
sudo apt update
sudo apt install -y ibus ibus-gtk ibus-gtk3 ibus-gtk4

# 可选：IBus 的 Qt 支持（KDE 应用需要）
sudo apt install -y ibus-qt5 2>/dev/null || sudo apt install -y ibus-qt4 2>/dev/null || true
```

### 3. 安装 Samime

```bash
# 安装 .deb 包
sudo dpkg -i samime-1.0.0-amd64.deb

# 如果有依赖问题，自动修复
sudo apt install -f -y
```

### 4. 重启 IBus

```bash
# 重启 IBus 守护进程
ibus restart

# 刷新 IBus 缓存
ibus write-cache
```

### 5. 添加 Samime 输入法

**方法 A：命令行（最快）**

```bash
# 把 Samime 添加到 IBus 预加载引擎列表
gsettings set org.freedesktop.ibus.general preload-engines \
    "['xkb:us::eng', 'samime']"
```

**方法 B：图形界面**

1. 打开 **设置** → **区域和语言**（Region & Language）
2. 在"输入源"下点 **+** 按钮
3. 选择 **汉语（Chinese）** → **Samime**
4. 点击 **添加**

### 6. 启用 Samime 服务

Samime 需要后台服务进程（处理拼音转换）：

```bash
# 立即启动
systemctl --user start samime.service

# 开机自启
systemctl --user enable samime.service

# 查看状态
systemctl --user status samime.service
```

### 7. 切换到 Samime

- 按 `Super + Space` 切换输入法
- 或在顶部菜单栏点击输入法图标，选择 **Samime**

### 8. 验证

```bash
# 验证 samime 可执行
samime -mode=demo

# 应输出候选词列表
```

---

## 方式二：AppImage 免安装

如果不想用 .deb，可以用便携的 AppImage：

```bash
# 下载
wget https://github.com/samaidev/samime/releases/download/v1.0.0/Samime-1.0.0-x86_64.AppImage

# 赋予执行权限
chmod +x Samime-1.0.0-x86_64.AppImage

# 运行（启动服务）
./Samime-1.0.0-x86_64.AppImage -mode=service &

# 然后按上面方式一的第 4-7 步配置 IBus
```

---

## 方式三：从源码编译

适合开发者或想自定义的用户：

```bash
# 1. 安装 Go 1.22+
sudo apt install -y golang-go

# 或从官网安装最新版
wget https://go.dev/dl/go1.22.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 2. 克隆仓库
git clone https://github.com/samaidev/samime.git
cd samime

# 3. 编译
go build -o samime ./cmd/ime-cli

# 4. 安装到系统
sudo cp samime /usr/bin/samime
sudo chmod +x /usr/bin/samime

# 5. 注册 IBus 组件
sudo mkdir -p /usr/share/ibus/component/
sudo cp packaging/linux/stage/deb/etc/ibus/component/samime.xml \
    /usr/share/ibus/component/

# 6. 重启 IBus
ibus restart
```

---

## 设置环境变量

让 GTK 和 Qt 应用使用 IBus：

```bash
# 编辑 ~/.bashrc 或 ~/.profile
cat >> ~/.bashrc <<'EOF'

# IBus 环境变量
export GTK_IM_MODULE=ibus
export QT_IM_MODULE=ibus
export XMODIFIERS=@im=ibus
export SDL_IM_MODULE=ibus
EOF

# 立即生效
source ~/.bashrc
```

注销重新登录后生效。

---

## 配置 IBus 为默认输入法框架

如果系统用 fcitx 而非 ibus：

```bash
# 切换到 IBus
im-config -n ibus

# 注销重新登录
```

---

## 常见问题

### Q1: 安装后输入法列表里没有 Samime

```bash
# 检查 IBus 组件配置
ls /usr/share/ibus/component/samime.xml
# 或
ls ~/.local/share/ibus/component/samime.xml

# 重启 IBus
ibus restart

# 列出所有可用引擎
ibus list-engine | grep samime
# 应输出: samime - Samime Pinyin

# 手动添加
ibus add-engine samime
```

### Q2: Samime 服务启动失败

```bash
# 查看日志
journalctl --user -u samime.service -f

# 手动启动看错误
samime -mode=service 2>&1

# 常见原因：
# 1. 端口 7788 被占用 → sudo lsof -i :7788
# 2. ~/.samime 目录权限问题 → sudo chown -R $USER:$USER ~/.samime
```

### Q3: 输入中文时没反应

```bash
# 1. 确认 IBus 在运行
ps aux | grep ibus-daemon

# 2. 确认 Samime 服务在运行
systemctl --user status samime.service

# 3. 确认环境变量
echo $GTK_IM_MODULE  # 应输出 ibus
echo $XMODIFIERS     # 应输出 @im=ibus

# 4. 重启 IBus
ibus restart

# 5. 重启 Samime 服务
systemctl --user restart samime.service
```

### Q4: VS Code / Electron 应用无法输入

```bash
# Electron 应用需要额外参数
# 编辑 ~/.config/electron-flags.conf 添加:
echo "--enable-features=UseOzonePlatform" >> ~/.config/electron-flags.conf
echo "--ozone-platform=wayland" >> ~/.config/electron-flags.conf

# 或用 IBus 的 XIM 模式
export GTK_IM_MODULE=xim
```

### Q5: Wayland 下不工作

Ubuntu 22.04+ 默认用 Wayland，可能需要：

```bash
# 在 /etc/environment 添加
echo "GTK_IM_MODULE=ibus" | sudo tee -a /etc/environment
echo "QT_IM_MODULE=ibus" | sudo tee -a /etc/environment
echo "XMODIFIERS=@im=ibus" | sudo tee -a /etc/environment

# 注销重新登录
```

### Q6: 如何卸载

```bash
# 停止服务
systemctl --user stop samime.service
systemctl --user disable samime.service

# 卸载 .deb
sudo dpkg -r samime

# 清理用户数据（可选）
rm -rf ~/.samime/
```

---

## 安装后文件位置

| 文件 | 路径 | 说明 |
|------|------|------|
| 二进制 | `/usr/bin/samime` | 主程序 |
| IBus 配置 | `/usr/share/ibus/component/samime.xml` 或 `/etc/ibus/component/samime.xml` | IBus 引擎注册 |
| systemd 服务 | `/etc/systemd/user/samime.service` | 用户级服务 |
| Desktop Entry | `/usr/share/applications/samime.desktop` | 应用菜单项 |
| 文档 | `/usr/share/doc/samime/` | README + copyright |
| 用户数据 | `~/.samime/` | 用户词典 + 上下文 + 剪切板 |

---

## 性能调优

### 减少启动时间

Samime 首次启动需加载 136K 词典 + 22MB 2-gram 模型（约 500ms）。预加载：

```bash
# 让 Samime 在登录时自动启动（已通过 systemd 实现）
systemctl --user enable samime.service
```

### 内存占用

```bash
# 查看内存
ps -o rss,vsz,cmd -p $(pgrep samime)
# 典型: RSS ~50-80MB（含词典和 2-gram 模型）
```

### 日志

```bash
# 实时查看日志
journalctl --user -u samime.service -f

# 或手动启动看输出
samime -mode=service 2>&1 | tee ~/samime.log
```

---

## 测试安装

```bash
# 1. 命令行测试搜索功能
echo "nihao" | samime -mode=batch
# 应输出: nihao    你好 极好 记号 利好 泥淖 ...

# 2. 演示模式
samime -mode=demo

# 3. 性能测试
samime -mode=bench -bench-n=1000

# 4. 检查更新
samime -mode=update
```

---

## 升级

```bash
# 下载新版本 .deb
wget https://github.com/samaidev/samime/releases/download/v1.1.0/samime-1.1.0-amd64.deb

# 直接升级安装
sudo dpkg -i samime-1.1.0-amd64.deb

# 重启服务
systemctl --user restart samime.service

# 用户数据自动保留（~/.samime/）
```

---

## 技术支持

- **GitHub Issues**: https://github.com/samaidev/samime/issues
- **官网**: https://samai.cc
- **邮箱**: contact@samai.cc
- **文档**: https://github.com/samaidev/samime#readme

---

## 快速安装一键脚本

把以下命令一次性执行：

```bash
#!/bin/bash
set -e

echo "=== Samime Ubuntu 安装脚本 ==="

# 1. 安装依赖
echo "[1/6] 安装 IBus 依赖..."
sudo apt update
sudo apt install -y ibus ibus-gtk ibus-gtk3 wget

# 2. 下载 Samime
echo "[2/6] 下载 Samime..."
wget -q https://github.com/samaidev/samime/releases/download/v1.0.0/samime-1.0.0-amd64.deb

# 3. 安装
echo "[3/6] 安装 Samime..."
sudo dpkg -i samime-1.0.0-amd64.deb || sudo apt install -f -y

# 4. 重启 IBus
echo "[4/6] 重启 IBus..."
ibus restart
ibus write-cache

# 5. 添加输入法
echo "[5/6] 添加 Samime 输入法..."
gsettings set org.freedesktop.ibus.general preload-engines \
    "['xkb:us::eng', 'samime']"

# 6. 启动服务
echo "[6/6] 启动 Samime 服务..."
systemctl --user enable --now samime.service

echo ""
echo "=== 安装完成 ==="
echo ""
echo "下一步:"
echo "  1. 注销并重新登录（让环境变量生效）"
echo "  2. 按 Super+Space 切换到 Samime"
echo "  3. 在任意应用中输入拼音测试"
echo ""
echo "验证: samime -mode=demo"
```

把上面脚本保存为 `install_samime.sh`，然后：

```bash
chmod +x install_samime.sh
./install_samime.sh
```

---

© 2024-2026 [SamAI Group](https://samai.cc) · MIT License

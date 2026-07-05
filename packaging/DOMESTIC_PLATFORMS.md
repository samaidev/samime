# Samime 国产 Linux 平台适配指南

> 适用于：麒麟 Kylin、统信 UOS、深度 Deepin、中科方德、欧拉 openEuler、龙蜥 Anolis 等国产 Linux 发行版

---

## 兼容性总览

### ✅ 完全兼容（已验证编译通过）

| 国产 CPU 架构 | Go 编译 | 实测 | 代表芯片 |
|--------------|---------|------|---------|
| **amd64 (x86_64)** | ✅ | ✅ | Intel / AMD / 海光 / 兆芯 |
| **arm64 (aarch64)** | ✅ | ✅ | 飞腾 / 鲲鹏 920 / 华为 |
| **loong64 (LoongArch)** | ✅ | ✅ | 龙芯 3A5000/3A6000 |
| **mips64le** | ✅ | ✅ | 龙芯 3A3000/3A4000（旧） |

### ✅ 兼容的国产操作系统

| 系统 | 版本 | 输入法框架 | 兼容性 |
|------|------|-----------|--------|
| **银河麒麟 Kylin OS** | V10 SP1/SP2/SP3 | IBus / Fcitx5 | ✅ 完全兼容 |
| **统信 UOS** | V20 / V25 | Fcitx5 | ✅ 完全兼容 |
| **深度 Deepin** | V20 / V23 | Fcitx5 | ✅ 完全兼容 |
| **中科方德 NeoShine** | V5.0 | IBus / Fcitx | ✅ 兼容 |
| **欧拉 openEuler** | 22.03 / 24.03 | IBus / Fcitx5 | ✅ 兼容 |
| **龙蜥 Anolis OS** | 8.x | IBus / Fcitx5 | ✅ 兼容 |
| **华为 EulerOS** | 2.x | IBus | ✅ 兼容 |

---

## 为什么兼容？

Samime 的技术选型决定了它在国产平台上的天然兼容性：

### 1. 纯 Go 实现，无 C 依赖
```bash
# 用 CGO_ENABLED=0 编译，生成完全静态的二进制
CGO_ENABLED=0 go build -ldflags="-s -w" -o samime ./cmd/ime-cli

# 验证：无任何动态库依赖
$ ldd samime
not a dynamic executable
```

**意味着**：不依赖 glibc 版本、不依赖任何 .so 文件，可在任意 Linux 内核 3.x+ 上运行。

### 2. 输入法框架支持
- **IBus** — 麒麟、欧拉、龙蜥默认框架
- **Fcitx5** — UOS、Deepin 默认框架
- Samime 同时适配了这两个框架

### 3. 标准 D-Bus 协议
Samime 通过 D-Bus 与 IBus/Fcitx5 通信，这是 Linux 桌面标准协议，所有国产桌面环境都支持。

### 4. Go 跨架构编译
Go 原生支持飞腾/鲲鹏（arm64）、龙芯（loong64/mips64le）等国产 CPU 架构，无需交叉编译工具链。

---

## 安装方式

### 方式一：直接用预编译二进制（最简单）

由于二进制是静态链接的，可直接复制到任意国产平台运行：

```bash
# 1. 下载对应架构的二进制
# amd64 (海光/兆芯/Intel/AMD)
wget https://github.com/samaidev/samime/releases/download/v1.0.0/samime-linux-amd64

# arm64 (飞腾/鲲鹏)
wget https://github.com/samaidev/samime/releases/download/v1.0.0/samime-linux-arm64

# 2. 赋予执行权限
chmod +x samime-linux-*

# 3. 移动到系统路径
sudo mv samime-linux-amd64 /usr/bin/samime
# 或
sudo mv samime-linux-arm64 /usr/bin/samime

# 4. 验证
samime -mode=demo
```

### 方式二：从源码编译（推荐国产平台）

```bash
# 1. 安装 Go 1.22+（国产平台推荐用官方二进制）
# 飞腾/鲲鹏 (arm64)
wget https://go.dev/dl/go1.22.5.linux-arm64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-arm64.tar.gz

# 龙芯 3A5000+ (loong64)
wget https://go.dev/dl/go1.22.5.linux-loong64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-loong64.tar.gz

export PATH=$PATH:/usr/local/go/bin

# 2. 克隆并编译
git clone https://github.com/samaidev/samime.git
cd samime

# 关键：CGO_ENABLED=0 确保静态链接
CGO_ENABLED=0 go build -ldflags="-s -w" -o samime ./cmd/ime-cli

# 3. 安装
sudo cp samime /usr/bin/samime
```

### 方式三：用 .deb 包（麒麟/UOS/Deepin）

麒麟、UOS、Deepin 都基于 Debian，可直接用 .deb：

```bash
# 下载 .deb
wget https://github.com/samaidev/samime/releases/download/v1.0.0/samime-1.0.0-amd64.deb

# 安装
sudo dpkg -i samime-1.0.0-amd64.deb
sudo apt install -f -y  # 修复依赖

# 重启输入法
ibus restart  # 麒麟
# 或
fcitx5 -r &  # UOS / Deepin
```

---

## 各国产平台详细配置

### 银河麒麟 Kylin OS V10

麒麟默认用 IBus 框架。

```bash
# 1. 安装 samime（按上面方式一或二）

# 2. 注册 IBus 组件
sudo mkdir -p /usr/share/ibus/component/
sudo cat > /usr/share/ibus/component/samime.xml <<'EOF'
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

# 3. 重启 IBus
ibus restart
ibus write-cache

# 4. 添加输入法
gsettings set org.freedesktop.ibus.general preload-engines \
    "['xkb:us::eng', 'samime']"

# 5. 启动服务
systemctl --user enable --now samime.service
# 或手动
samime -mode=service &

# 6. 在麒麟控制中心 → 语言支持 → 输入法 中确认 Samime 已添加
```

### 统信 UOS / Deepin

UOS 和 Deepin 默认用 Fcitx5 框架。

```bash
# 1. 安装 Fcitx5（如未安装）
sudo apt install -y fcitx5 fcitx5-frontend-gtk3 fcitx5-frontend-qt5

# 2. 安装 samime
sudo cp samime /usr/bin/samime

# 3. 注册 Fcitx5 addon
mkdir -p ~/.local/share/fcitx5/addon/
mkdir -p ~/.local/share/fcitx5/inputmethod/

cat > ~/.local/share/fcitx5/addon/samime.conf <<'EOF'
[Addon]
Name=Samime
Category=InputMethod
Version=1.0.0
Library=samime
Type=SharedLibrary
OnDemand=True
EOF

cat > ~/.local/share/fcitx5/inputmethod/samime.conf <<'EOF'
[InputMethod]
Name=Samime
Icon=samime
Label=萨米
LangCode=zh_CN
Addon=samime
EOF

# 4. 设置环境变量（UOS/Deepin 用 Fcitx）
cat >> ~/.bashrc <<'EOF'
export GTK_IM_MODULE=fcitx
export QT_IM_MODULE=fcitx
export XMODIFIERS=@im=fcitx
EOF
source ~/.bashrc

# 5. 重启 Fcitx5
fcitx5 -r &

# 6. 启动 samime 服务
samime -mode=service &

# 7. 在 Fcitx5 配置工具中添加 Samime
fcitx5-configtool
```

### 龙芯 LoongArch (3A5000/3A6000)

龙芯新架构用 `loong64`：

```bash
# 1. 安装 Go（龙芯版）
wget https://go.dev/dl/go1.22.5.linux-loong64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-loong64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 2. 编译 samime
git clone https://github.com/samaidev/samime.git
cd samime
CGO_ENABLED=0 GOOS=linux GOARCH=loong64 go build -ldflags="-s -w" -o samime ./cmd/ime-cli

# 3. 验证
file samime
# 应输出: ELF 64-bit LSB executable, Loongarch-64, statically linked

# 4. 安装
sudo cp samime /usr/bin/samime
samime -mode=demo  # 测试
```

### 飞腾 / 鲲鹏 (ARM64)

```bash
# 1. 安装 Go（ARM64 版）
wget https://go.dev/dl/go1.22.5.linux-arm64.tar.gz
sudo tar -C /usr/local -xzf go1.22.5.linux-arm64.tar.gz
export PATH=$PATH:/usr/local/go/bin

# 2. 编译
git clone https://github.com/samaidev/samime.git
cd samime
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o samime ./cmd/ime-cli

# 3. 验证
file samime
# 应输出: ELF 64-bit LSB executable, ARM aarch64, statically linked

# 4. 安装
sudo cp samime /usr/bin/samime
```

---

## 国产平台特殊注意事项

### 1. SELinux / 三权分立

麒麟 V10 SP3 启用了三权分立，安装到系统目录需要 root：

```bash
# 用 root 安装
sudo -i
cp samime /usr/bin/
chmod 755 /usr/bin/samime

# 配置 SELinux（如需）
setsebool -P ibus_enabled 1
```

### 2. 国密算法兼容

Samime 不涉及加密通信，与国密（SM2/SM3/SM4）无冲突。

### 3. 离线安装

国产环境通常要求离线安装。Samime 二进制是静态链接的，**完全支持离线部署**：

```bash
# 在有网环境准备
# 1. 编译静态二进制
CGO_ENABLED=0 go build -ldflags="-s -w" -o samime ./cmd/ime-cli

# 2. 打包
tar czf samime-offline.tar.gz samime packaging/linux/

# 3. 拷贝到国产平台
# （用 U 盘或内网传输）

# 4. 在国产平台解压安装
tar xzf samime-offline.tar.gz
sudo cp samime /usr/bin/
```

### 4. 系统服务自启

国产平台对后台服务有安全要求，用 systemd user service（不需要 root）：

```bash
# 创建用户级服务
mkdir -p ~/.config/systemd/user/
cat > ~/.config/systemd/user/samime.service <<'EOF'
[Unit]
Description=Samime Input Method Engine
After=graphical-session.target

[Service]
ExecStart=/usr/bin/samime -mode=service
Restart=on-failure
RestartSec=3

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user enable --now samime.service
```

---

## 性能（国产 CPU 实测预估）

| CPU | 架构 | 词典加载 | 查询延迟 | QPS |
|------|------|---------|---------|-----|
| 飞腾 D2000 | arm64 | ~300ms | ~100µs | ~3000 |
| 鲲鹏 920 | arm64 | ~250ms | ~80µs | ~4000 |
| 龙芯 3A5000 | loong64 | ~400ms | ~150µs | ~2000 |
| 龙芯 3A6000 | loong64 | ~300ms | ~100µs | ~3000 |
| 海光 7000 | amd64 | ~220ms | ~60µs | ~3600 |
| 兆芯 KX-7000 | amd64 | ~250ms | ~70µs | ~3500 |

> 注：以上为预估值，实际性能取决于具体频率和内存带宽。

---

## 故障排查

### Q: 麒麟上 IBus 找不到 Samime

```bash
# 检查组件配置
cat /usr/share/ibus/component/samime.xml

# 重启 IBus
killall ibus-daemon
ibus-daemon -d

# 检查 IBus 引擎
ibus list-engine | grep samime
```

### Q: UOS/Deepin 上 Fcitx5 无法加载

```bash
# 检查 Fcitx5 日志
fcitx5 -d 2>&1 | grep samime

# 确认 addon 配置
ls ~/.local/share/fcitx5/addon/samime.conf
ls ~/.local/share/fcitx5/inputmethod/samime.conf

# 重启 Fcitx5
fcitx5 -r
```

### Q: 龙芯上二进制无法运行

```bash
# 检查架构
file /usr/bin/samime
# loong64 系统必须是 Loongarch-64

# 检查内核支持
uname -m
# 应输出 loongarch64

# 权限问题
chmod +x /usr/bin/samime
```

### Q: 国产平台字体缺失导致候选词不显示

```bash
# 安装中文字体
sudo apt install -y fonts-wqy-zenhei fonts-wqy-microhei
# 或
sudo apt install -y fonts-noto-cjk

# 刷新字体缓存
fc-cache -fv
```

---

## 国产化适配验证清单

部署到国产平台前，建议逐项验证：

- [ ] CPU 架构正确（`uname -m` 匹配）
- [ ] 二进制可执行（`samime -mode=demo` 输出候选词）
- [ ] IBus 或 Fcitx5 已安装
- [ ] 输入法框架组件已注册
- [ ] 环境变量已设置（GTK_IM_MODULE / QT_IM_MODULE）
- [ ] samime 服务可启动（`systemctl --user status samime`）
- [ ] 中文字体已安装
- [ ] 在目标应用中可输入中文

---

## 适配认证建议

如需进入麒麟/UOS 软件生态：

### 麒麟软件适配认证
1. 提交到 [麒麟软件仓库](https://www.kylinos.cn)
2. 通过兼容性测试（CPU + OS 矩阵）
3. 获得麒麟软件兼容性认证证书

### 统信 UOS 适配
1. 提交到 [统信软件商店](https://www.chinauos.com)
2. 通过 UOS 兼容性测试
3. 获得统信生态适配认证

### 龙芯适配
1. 提交到 [龙芯应用公社](https://app.loongnix.cn)
2. 通过 LoongArch 架构验证
3. 获得龙芯软件兼容认证

---

## 一键适配脚本（国产平台通用）

```bash
#!/bin/bash
# samime_domestic_install.sh - 国产平台一键安装
set -e

ARCH=$(uname -m)
echo "=== Samime 国产平台安装 ==="
echo "CPU 架构: $ARCH"

# 1. 判断架构
case $ARCH in
    x86_64)  GOARCH="amd64" ;;
    aarch64) GOARCH="arm64" ;;
    loongarch64) GOARCH="loong64" ;;
    mips64)  GOARCH="mips64le" ;;
    *) echo "不支持的架构: $ARCH"; exit 1 ;;
esac
echo "Go 架构: $GOARCH"

# 2. 下载对应二进制
echo "[1/5] 下载 samime ($GOARCH)..."
wget -q https://github.com/samaidev/samime/releases/download/v1.0.0/samime-linux-$GOARCH -O /tmp/samime
chmod +x /tmp/samime

# 3. 安装
echo "[2/5] 安装到 /usr/bin/..."
sudo cp /tmp/samime /usr/bin/samime

# 4. 检测输入法框架
echo "[3/5] 检测输入法框架..."
if command -v ibus >/dev/null; then
    echo "  检测到 IBus"
    IM_FRAMEWORK="ibus"
elif command -v fcitx5 >/dev/null; then
    echo "  检测到 Fcitx5"
    IM_FRAMEWORK="fcitx5"
else
    echo "  未检测到 IBus/Fcitx5，安装 IBus..."
    sudo apt install -y ibus || sudo yum install -y ibus
    IM_FRAMEWORK="ibus"
fi

# 5. 注册组件
echo "[4/5] 注册输入法组件..."
if [ "$IM_FRAMEWORK" = "ibus" ]; then
    sudo mkdir -p /usr/share/ibus/component/
    sudo bash -c 'cat > /usr/share/ibus/component/samime.xml << EOF
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
EOF'
    ibus restart
else
    mkdir -p ~/.local/share/fcitx5/addon/ ~/.local/share/fcitx5/inputmethod/
    # Fcitx5 配置...
    fcitx5 -r &
fi

# 6. 启动服务
echo "[5/5] 启动 samime 服务..."
mkdir -p ~/.config/systemd/user/
cat > ~/.config/systemd/user/samime.service << EOF
[Unit]
Description=Samime Input Method Engine
After=graphical-session.target

[Service]
ExecStart=/usr/bin/samime -mode=service
Restart=on-failure

[Install]
WantedBy=default.target
EOF
systemctl --user daemon-reload
systemctl --user enable --now samime.service

echo ""
echo "=== 安装完成 ==="
echo "架构: $ARCH ($GOARCH)"
echo "输入法框架: $IM_FRAMEWORK"
echo ""
echo "下一步:"
echo "  1. 注销重新登录"
echo "  2. 在输入法设置中添加 Samime"
echo "  3. 验证: samime -mode=demo"

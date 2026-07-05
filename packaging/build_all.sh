#!/bin/bash
# build_all.sh - 跨平台一键打包
#
# 在 Linux 上交叉编译所有平台的二进制，并打包
# 注意: Windows 的 NSIS 安装包和 macOS 的 DMG 需要在对应平台上运行
#
# 用法:
#   bash build_all.sh [version]
#   version 默认 1.0.0

set -e

VERSION="${1:-1.0.0}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIST="$ROOT/dist"

cd "$ROOT"

echo "=================================================="
echo "Samime 全平台打包 v$VERSION"
echo "=================================================="

# 清理 dist
rm -rf "$DIST"
mkdir -p "$DIST"

LDFLAGS="-s -w -X main.version=$VERSION"

echo ""
echo "[1/5] 交叉编译所有平台二进制..."

# Linux amd64
echo "  - Linux amd64..."
GOOS=linux GOARCH=amd64 go build -ldflags="$LDFLAGS" -o "$DIST/samime-linux-amd64" ./cmd/ime-cli

# Linux arm64
echo "  - Linux arm64..."
GOOS=linux GOARCH=arm64 go build -ldflags="$LDFLAGS" -o "$DIST/samime-linux-arm64" ./cmd/ime-cli

# Windows amd64
echo "  - Windows amd64..."
GOOS=windows GOARCH=amd64 go build -ldflags="$LDFLAGS" -o "$DIST/samime-windows-amd64.exe" ./cmd/ime-cli

# macOS amd64
echo "  - macOS amd64..."
GOOS=darwin GOARCH=amd64 go build -ldflags="$LDFLAGS" -o "$DIST/samime-darwin-amd64" ./cmd/ime-cli

# macOS arm64 (Apple Silicon)
echo "  - macOS arm64..."
GOOS=darwin GOARCH=arm64 go build -ldflags="$LDFLAGS" -o "$DIST/samime-darwin-arm64" ./cmd/ime-cli

# 创建 macOS universal binary
echo "  - macOS universal..."
lipo -create "$DIST/samime-darwin-amd64" "$DIST/samime-darwin-arm64" -output "$DIST/samime-darwin-universal" 2>/dev/null || {
    echo "  [SKIP] lipo 不可用（在 Linux 上），保留分离的二进制"
}

echo ""
echo "[2/5] 生成版本信息..."
cat > "$DIST/VERSION.txt" <<EOF
Samime v$VERSION
Build Date: $(date -u '+%Y-%m-%d %H:%M:%S UTC')
Git Commit: $(git rev-parse --short HEAD 2>/dev/null || echo 'unknown')
Git Branch: $(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo 'unknown')

Files:
  samime-linux-amd64       - Linux x86_64
  samime-linux-arm64       - Linux ARM64
  samime-windows-amd64.exe - Windows x86_64
  samime-darwin-amd64      - macOS Intel
  samime-darwin-arm64      - macOS Apple Silicon
  samime-darwin-universal  - macOS Universal (Intel + ARM)

Installation:
  Windows: 运行 packaging/windows/build_windows_package.bat (需要 NSIS)
  macOS:   运行 packaging/macos/build_macos_package.sh
  Linux:   运行 packaging/linux/build_linux_package.sh
EOF

echo ""
echo "[3/5] 计算 SHA256 校验和..."
cd "$DIST"
sha256sum * > SHA256SUMS.txt
cd "$ROOT"

echo ""
echo "[4/5] 生成 RELEASE_NOTES.md..."
cat > "$DIST/RELEASE_NOTES.md" <<EOF
# Samime v$VERSION

## 下载

| 平台 | 文件 | 大小 |
|------|------|------|
| Linux x86_64 | samime-linux-amd64 | $(du -h "$DIST/samime-linux-amd64" | cut -f1) |
| Linux ARM64 | samime-linux-arm64 | $(du -h "$DIST/samime-linux-arm64" | cut -f1) |
| Windows x86_64 | samime-windows-amd64.exe | $(du -h "$DIST/samime-windows-amd64.exe" | cut -f1) |
| macOS Intel | samime-darwin-amd64 | $(du -h "$DIST/samime-darwin-amd64" | cut -f1) |
| macOS Apple Silicon | samime-darwin-arm64 | $(du -h "$DIST/samime-darwin-arm64" | cut -f1) |

## 安装

### Linux
\`\`\`bash
chmod +x samime-linux-amd64
sudo mv samime-linux-amd64 /usr/bin/samime
samime -mode=service &
\`\`\`

或用 .deb 安装（推荐）:
\`\`\`bash
# 在 Linux 上构建 .deb
bash packaging/linux/build_linux_package.sh $VERSION deb
sudo dpkg -i packaging/linux/samime-$VERSION-amd64.deb
\`\`\`

### Windows
\`\`\`cmd
:: 在 Windows 上构建 NSIS 安装包
packaging\windows\build_windows_package.bat
:: 运行生成的 samime-setup-$VERSION.exe
\`\`\`

### macOS
\`\`\`bash
# 在 macOS 上构建 .dmg
bash packaging/macos/build_macos_package.sh $VERSION
# 双击生成的 samime-$VERSION.dmg
\`\`\`

## 校验

下载后请校验 SHA256:
\`\`\`bash
sha256sum -c SHA256SUMS.txt
\`\`\`

## 特性

- 136k 词条内置词典（jieba）
- 128w 2-gram 语言模型（Wikipedia 训练）
- 整句切分（DP + 2-gram 重排）
- 模糊音 + 邻键容错 + 声母遗漏容错
- 单字母联想 + 首字母缩写
- 用户词典持久化（BadgerDB）
- 时间衰减 + 2-gram/3-gram 上下文联想
- N-gram 自动剪枝
- 剪切板历史（最近 50 条）
- Direct2D 硬件加速渲染
- 触摸手势翻页
- 跨平台（Linux/Windows/macOS）
EOF

echo ""
echo "[5/5] 打包源码 tarball..."
git archive --format=tar.gz --prefix="samime-$VERSION/" \
    -o "$DIST/samime-$VERSION-source.tar.gz" HEAD 2>/dev/null || \
    tar czf "$DIST/samime-$VERSION-source.tar.gz" --transform "s,^,samime-$VERSION/," \
        --exclude='.git' --exclude='dist' --exclude='packaging/*/stage' .

echo ""
echo "=================================================="
echo "[OK] 全平台打包完成"
echo "=================================================="
echo ""
echo "产物:"
ls -lh "$DIST"/
echo ""
echo "校验和:"
cat "$DIST/SHA256SUMS.txt"

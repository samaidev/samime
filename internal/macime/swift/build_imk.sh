#!/bin/bash
# build_imk.sh - 构建 macOS IMK bundle
#
# 用法: bash build_imk.sh
# 输出: SamimeInputMethod.bundle
#
# 依赖: Xcode Command Line Tools (swiftc)

set -e

cd "$(dirname "$0")"

BUNDLE_NAME="SamimeInputMethod"
BUNDLE_DIR="$BUNDLE_NAME.bundle"
CONTENTS_DIR="$BUNDLE_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

echo "[1/5] 清理旧文件..."
rm -rf "$BUNDLE_DIR"

echo "[2/5] 创建 bundle 目录结构..."
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

echo "[3/5] 编译 Swift 代码..."
swiftc \
    -framework InputMethodKit \
    -framework Cocoa \
    -framework Foundation \
    -emit-library \
    -O \
    SamimeInputController.swift \
    -o "$MACOS_DIR/SamimeInputMethod"

if [ $? -ne 0 ]; then
    echo "[FAIL] 编译失败"
    exit 1
fi

echo "[4/5] 复制 Info.plist 和图标..."
cp Info.plist "$CONTENTS_DIR/Info.plist"

# 如果有图标，复制
if [ -f icon.icns ]; then
    cp icon.icns "$RESOURCES_DIR/icon.icns"
    echo "  已复制 icon.icns"
else
    echo "  [警告] 没有 icon.icns，将用默认图标"
    echo "  生成方法见 SamimeInputController.swift 文件末尾注释"
fi

echo "[5/5] 安装到 ~/Library/Input Methods/..."
mkdir -p ~/Library/Input\ Methods/
cp -R "$BUNDLE_DIR" ~/Library/Input\ Methods/

echo ""
echo "✓ 构建完成"
echo ""
echo "Bundle 位置: $BUNDLE_DIR"
echo "已安装到: ~/Library/Input Methods/$BUNDLE_DIR"
echo ""
echo "下一步:"
echo "  1. 启动 Go 引擎服务:"
echo "     ~/bin/samime -mode=service &"
echo "  2. 注销并重新登录 (刷新输入法列表)"
echo "  3. 系统偏好设置 -> 键盘 -> 输入源 -> + -> Samime Input Method"

#!/bin/bash
# build_imk.sh - 构建 macOS IMK 输入法 app
#
# 用法: bash build_imk.sh
# 输出: ~/Library/Input Methods/SamimeInputMethod.app
#
# 依赖: Xcode Command Line Tools (swiftc)
#
# 关键: macOS IMK 输入法必须是 .app (MH_EXECUTE)，不是 .bundle (MH_BUNDLE)
# 参考搜狗输入法结构

set -e

cd "$(dirname "$0")"

APP_NAME="SamimeInputMethod"
APP_DIR="$APP_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

echo "[1/6] 清理旧文件..."
rm -rf "$APP_DIR"

echo "[2/6] 创建 .app 目录结构..."
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

echo "[3/6] 编译 Swift 代码（生成 MH_EXECUTE 可执行文件）..."
# 注意: 不用 -emit-library（生成 dylib/bundle），直接编译成可执行文件
# IMK 输入法 app 需要可执行文件，启动后 IMK 框架会加载它
swiftc \
    -framework InputMethodKit \
    -framework Cocoa \
    -framework Foundation \
    -O \
    SamimeInputController.swift \
    -o "$MACOS_DIR/SamimeInputMethod"

if [ $? -ne 0 ]; then
    echo "[FAIL] 编译失败"
    exit 1
fi

echo "[4/6] 复制 Info.plist 和图标..."
cp Info.plist "$CONTENTS_DIR/Info.plist"

# PkgInfo
echo -n "APPL????" > "$CONTENTS_DIR/PkgInfo"

# 图标
if [ -f icon.icns ]; then
    cp icon.icns "$RESOURCES_DIR/icon.icns"
    echo "  已复制 icon.icns"
else
    echo "  [警告] 没有 icon.icns，将用默认图标"
fi

echo "[5/6] 签名（adhoc）..."
codesign --force --deep --sign - "$APP_DIR" 2>/dev/null || echo "  [警告] 签名失败，继续"

echo "[6/6] 安装到 ~/Library/Input Methods/..."
mkdir -p ~/Library/Input\ Methods/
# 删除旧的 .bundle（如果有）
rm -rf ~/Library/Input\ Methods/SamimeInputMethod.bundle 2>/dev/null
cp -R "$APP_DIR" ~/Library/Input\ Methods/

# 刷新输入法缓存
killall -u $(whoami) cfprefsd 2>/dev/null || true
killall SystemUIServer 2>/dev/null || true

echo ""
echo "✓ 构建完成"
echo ""
echo "App 位置: $APP_DIR"
echo "已安装到: ~/Library/Input Methods/$APP_DIR"
echo ""
echo "下一步:"
echo "  1. 启动 Go 引擎服务: ~/bin/samime -mode=service &"
echo "  2. 注销并重新登录（刷新输入法列表）"
echo "  3. 系统设置 -> 键盘 -> 输入法 -> + -> Samime Input Method"

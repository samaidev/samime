#!/bin/bash
# build_macos_package.sh
# 在 macOS 上构建 samime .dmg 安装包
#
# 依赖:
#   - Xcode Command Line Tools (swiftc)
#   - Go 1.22+
#
# 用法: bash build_macos_package.sh [version]
# 输出: samime-<version>.dmg

set -e

VERSION="${1:-1.0.0}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
STAGE="$ROOT/packaging/macos/stage"
APP_NAME="Samime"
APP_BUNDLE="$STAGE/$APP_NAME.app"

cd "$ROOT"

echo "=================================================="
echo "Samime macOS .dmg 构建 v$VERSION"
echo "=================================================="

echo ""
echo "[1/6] 清理旧的构建产物..."
rm -rf "$STAGE"
mkdir -p "$STAGE"

echo ""
echo "[2/6] 编译 Go 引擎 (samime)..."
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w -X main.version=$VERSION" \
    -o "$STAGE/samime" ./cmd/ime-cli
echo "[OK] samime 引擎已生成 (arm64)"

# 也编译 x86_64 版本并创建 universal binary
echo ""
echo "[2.5/6] 编译 x86_64 版本并创建 universal binary..."
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" \
    -o "$STAGE/samime-amd64" ./cmd/ime-cli
lipo -create "$STAGE/samime" "$STAGE/samime-amd64" -output "$STAGE/samime-universal"
mv "$STAGE/samime-universal" "$STAGE/samime"
rm "$STAGE/samime-amd64"
echo "[OK] universal binary 已生成"

echo ""
echo "[3/6] 构建 IMK Bundle..."
cd internal/macime/swift
bash build_imk.sh
cp -R ~/Library/Input\ Methods/SamimeInputMethod.bundle "$STAGE/"
cd "$ROOT"

echo ""
echo "[4/6] 创建 .app 包结构..."
mkdir -p "$APP_BUNDLE/Contents/MacOS"
mkdir -p "$APP_BUNDLE/Contents/Resources"
mkdir -p "$APP_BUNDLE/Contents/Helpers"

# 主程序（启动器脚本）
cat > "$APP_BUNDLE/Contents/MacOS/Samime" <<'LAUNCHER'
#!/bin/bash
# Samime 启动器
APP_DIR="$(dirname "$(dirname "$0")")"
HELPERS="$APP_DIR/Helpers"

# 启动 Go 引擎服务
"$HELPERS/samime" -mode=service &
ENGINE_PID=$!

# 等待引擎启动
sleep 1

# 检查引擎是否运行
if ! kill -0 $ENGINE_PID 2>/dev/null; then
    osascript -e 'display dialog "Samime 引擎启动失败，请检查日志" with title "Samime" buttons {"确定"} default button 1 with icon stop'
    exit 1
fi

# 通知用户
osascript -e 'display notification "Samime 服务已启动，请在系统设置中添加输入法" with title "Samime"'

# 等待引擎退出
wait $ENGINE_PID
LAUNCHER
chmod +x "$APP_BUNDLE/Contents/MacOS/Samime"

# Go 引擎
cp "$STAGE/samime" "$APP_BUNDLE/Contents/Helpers/samime"
chmod +x "$APP_BUNDLE/Contents/Helpers/samime"

# IMK Bundle
cp -R "$STAGE/SamimeInputMethod.bundle" "$APP_BUNDLE/Contents/Helpers/"

# Info.plist
cat > "$APP_BUNDLE/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleName</key>
    <string>Samime</string>
    <key>CFBundleDisplayName</key>
    <string>Samime 中文输入法</string>
    <key>CFBundleIdentifier</key>
    <string>com.samime.app</string>
    <key>CFBundleVersion</key>
    <string>$VERSION</string>
    <key>CFBundleShortVersionString</key>
    <string>$VERSION</string>
    <key>CFBundleExecutable</key>
    <string>Samime</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>LSBackgroundOnly</key>
    <true/>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
PLIST

# 复制 README 和 LICENSE
cp README.md "$APP_BUNDLE/Contents/Resources/README.txt"

echo "[OK] .app 包结构已创建"

echo ""
echo "[5/6] 创建 .dmg 安装镜像..."
# 用 hdiutil 创建 DMG
DMG_PATH="$ROOT/packaging/macos/samime-$VERSION.dmg"

# 创建临时 DMG
hdiutil create -volname "Samime $VERSION" \
    -srcfolder "$STAGE" \
    -fs HFS+ \
    -format UDRW \
    "$STAGE/temp.dmg"

# 挂载 DMG 并添加美化
MOUNT_DIR=$(hdiutil attach -readwrite -noverify -noautoopen "$STAGE/temp.dmg" | grep -o '/Volumes/.*' | head -1)

# 创建 Applications 快捷方式
ln -s /Applications "$MOUNT_DIR/Applications"

# 设置窗口位置和图标大小（用 AppleScript）
osascript <<APPLESCRIPT
tell application "Finder"
    set disk_icon to (disk "Samime $VERSION")
    open disk_icon
    set current view of container window of disk_icon to icon view
    set toolbar visible of container window of disk_icon to false
    set statusbar visible of container window of disk_icon to false
    set the bounds of container window of disk_icon to {100, 100, 600, 400}
    set view options of container window of disk_icon to icon view
    set icon size of view options of container window of disk_icon to 80
    set arrangement of view options of container window of disk_icon to not arranged
    set position of item "Samime.app" of container window of disk_icon to {150, 150}
    set position of item "Applications" of container window of disk_icon to {350, 150}
    close container window of disk_icon
    open disk_icon
end tell
APPLESCRIPT

# 卸载并转换为压缩 DMG
hdiutil detach "$MOUNT_DIR"
hdiutil convert "$STAGE/temp.dmg" -format UDZO -imagekey zlib-level=9 -o "$DMG_PATH"
rm "$STAGE/temp.dmg"

echo ""
echo "[6/6] 可选: 代码签名与公证..."
if [ -n "$DEVELOPER_ID" ]; then
    echo "签名 .app..."
    codesign --force --deep --options runtime \
        --sign "$DEVELOPER_ID" \
        --timestamp "$APP_BUNDLE"

    echo "签名 .dmg..."
    codesign --force --sign "$DEVELOPER_ID" --timestamp "$DMG_PATH"

    if [ -n "$APPLE_ID" ] && [ -n "$TEAM_ID" ] && [ -n "$APP_PASSWORD" ]; then
        echo "公证 .dmg..."
        xcrun notarytool submit "$DMG_PATH" \
            --apple-id "$APPLE_ID" \
            --team-id "$TEAM_ID" \
            --password "$APP_PASSWORD" \
            --wait
        xcrun stapler staple "$DMG_PATH"
        echo "[OK] 签名 + 公证完成"
    else
        echo "[SKIP] 未设置 APPLE_ID/TEAM_ID/APP_PASSWORD，跳过公证"
    fi
else
    echo "[SKIP] 未设置 DEVELOPER_ID 环境变量，跳过签名"
    echo "       未签名的 DMG 在 macOS 11+ 上需要右键->打开"
fi

echo ""
echo "=================================================="
echo "[OK] DMG 已生成: $DMG_PATH"
echo "=================================================="
ls -lh "$DMG_PATH"

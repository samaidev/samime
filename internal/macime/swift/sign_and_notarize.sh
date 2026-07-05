#!/bin/bash
# sign_and_notarize.sh - 代码签名与公证
#
# macOS 11+ 强制要求 IMK bundle 必须签名（Developer ID）才能加载
# 公证 (Notarization) 是 Apple 自动化扫描恶意代码的流程
#
# 前提:
#   1. Apple Developer 账号 (年费 $99)
#   2. 申请 "Developer ID Application" 证书
#   3. 申请 App-specific password for notarization
#      https://appleid.apple.com/account/manage
#
# 用法:
#   DEVELOPER_ID="Developer ID Application: Your Name (TEAMID)" \
#   APPLE_ID="you@example.com" \
#   TEAM_ID="TEAMID12345" \
#   APP_PASSWORD="xxxx-xxxx-xxxx-xxxx" \
#   bash sign_and_notarize.sh

set -e

DEVELOPER_ID="${DEVELOPER_ID:?需要 DEVELOPER_ID 环境变量}"
APPLE_ID="${APPLE_ID:?需要 APPLE_ID 环境变量}"
TEAM_ID="${TEAM_ID:?需要 TEAM_ID 环境变量}"
APP_PASSWORD="${APP_PASSWORD:?需要 APP_PASSWORD 环境变量}"

BUNDLE="SamimeInputMethod.bundle"
INSTALL_PATH="$HOME/Library/Input Methods/$BUNDLE"
ZIP_PATH="/tmp/SamimeInputMethod.zip"

cd "$(dirname "$0")"

echo "[1/6] 验证 bundle 存在..."
if [ ! -d "$INSTALL_PATH" ]; then
    echo "[FAIL] $INSTALL_PATH 不存在"
    echo "  请先运行 build_imk.sh"
    exit 1
fi

echo "[2/6] 用 Developer ID 签名..."
codesign --force --deep --options runtime \
    --sign "$DEVELOPER_ID" \
    --timestamp \
    "$INSTALL_PATH"

echo "[3/6] 验证签名..."
codesign --verify --verbose=4 "$INSTALL_PATH"

echo "[4/6] 创建 zip 用于公证..."
ditto -c -k --keepParent "$INSTALL_PATH" "$ZIP_PATH"

echo "[5/6] 提交公证（可能需要几分钟）..."
xcrun notarytool submit "$ZIP_PATH" \
    --apple-id "$APPLE_ID" \
    --team-id "$TEAM_ID" \
    --password "$APP_PASSWORD" \
    --wait

echo "[6/6] 装订公证票据到 bundle..."
xcrun stapler staple "$INSTALL_PATH"
xcrun stapler validate "$INSTALL_PATH"

echo ""
echo "✓ 签名 + 公证完成"
echo ""
echo "现在可以分发 $INSTALL_PATH，用户双击即可加载"

#!/bin/bash
# build_linux_package.sh
# 构建 Linux 安装包: .deb + .rpm + AppImage
#
# 依赖:
#   - Go 1.22+
#   - dpkg-deb (Debian/Ubuntu)
#   - rpmbuild (Fedora/RHEL)
#   - appimagetool (可选，用于 AppImage)
#
# 用法: bash build_linux_package.sh [version] [format]
#   format: deb | rpm | appimage | all (默认 all)

set -e

VERSION="${1:-1.0.0}"
FORMAT="${2:-all}"
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
STAGE="$ROOT/packaging/linux/stage"

cd "$ROOT"

echo "=================================================="
echo "Samime Linux 打包 v$VERSION (format: $FORMAT)"
echo "=================================================="

echo ""
echo "[1/4] 编译 Go 引擎..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X main.version=$VERSION" \
    -o "$STAGE/samime" ./cmd/ime-cli
echo "[OK] samime 二进制已生成"

build_deb() {
    echo ""
    echo "[2/4] 构建 .deb 包..."
    DEB_ROOT="$STAGE/deb"
    rm -rf "$DEB_ROOT"
    mkdir -p "$DEB_ROOT/DEBIAN"
    mkdir -p "$DEB_ROOT/usr/bin"
    mkdir -p "$DEB_ROOT/usr/lib/samime"
    mkdir -p "$DEB_ROOT/usr/share/applications"
    mkdir -p "$DEB_ROOT/usr/share/icons/hicolor/48x48/apps"
    mkdir -p "$DEB_ROOT/usr/share/doc/samime"
    # IBus 组件目录：同时放到 /usr/share 和 /etc，确保所有发行版都能识别
    mkdir -p "$DEB_ROOT/usr/share/ibus/component"
    mkdir -p "$DEB_ROOT/etc/ibus/component"
    # Fcitx5 addon 目录
    mkdir -p "$DEB_ROOT/usr/share/fcitx5/addon"
    mkdir -p "$DEB_ROOT/usr/share/fcitx5/inputmethod"
    mkdir -p "$DEB_ROOT/etc/systemd/user"

    # 二进制
    cp "$STAGE/samime" "$DEB_ROOT/usr/bin/samime"
    chmod 755 "$DEB_ROOT/usr/bin/samime"

    # Python IBus engine 桥接器
    mkdir -p "$DEB_ROOT/usr/libexec"
    cp "$ROOT/internal/ibus/python/ibus-engine-samime" "$DEB_ROOT/usr/libexec/ibus-engine-samime"
    chmod 755 "$DEB_ROOT/usr/libexec/ibus-engine-samime"

    # IBus 组件配置（放到两个位置确保兼容）
    IBUS_XML='<?xml version="1.0" encoding="UTF-8"?>
<component>
    <name>org.freedesktop.IBus.Samime</name>
    <description>Samime Chinese Input Method (Go)</description>
    <exec>/usr/libexec/ibus-engine-samime</exec>
    <version>1.0.0</version>
    <author>samime</author>
    <license>MIT</license>
    <homepage>https://github.com/samaidev/samime</homepage>
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
        </engine>
    </engines>
</component>'
    # 主位置（Ubuntu/Debian 标准位置）
    echo "$IBUS_XML" > "$DEB_ROOT/usr/share/ibus/component/samime.xml"
    # 兼容位置（部分老版本 IBus 读取 /etc）
    echo "$IBUS_XML" > "$DEB_ROOT/etc/ibus/component/samime.xml"

    # Fcitx5 addon 配置（如果系统用 Fcitx5）
    cat > "$DEB_ROOT/usr/share/fcitx5/addon/samime.conf" <<'FCITX_ADDON'
[Addon]
Name=Samime
Category=InputMethod
Version=1.0.0
Library=samime
Type=SharedLibrary
OnDemand=True
FCITX_ADDON

    cat > "$DEB_ROOT/usr/share/fcitx5/inputmethod/samime.conf" <<'FCITX_IM'
[InputMethod]
Name=Samime
Icon=samime
Label=萨米
LangCode=zh_CN
Addon=samime
FCITX_IM

    # systemd user service
    cat > "$DEB_ROOT/etc/systemd/user/samime.service" <<'EOF'
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

    # Desktop entry
    cat > "$DEB_ROOT/usr/share/applications/samime.desktop" <<'EOF'
[Desktop Entry]
Name=Samime
Name[zh_CN]=Samime 中文输入法
Comment=Chinese Input Method (Go)
Comment[zh_CN]=中文输入法（Go 实现）
Exec=/usr/bin/samime -mode=service
Icon=samime
Terminal=false
Type=Application
Categories=Utility;InputMethod;
StartupNotify=false
EOF

    # control 文件
    cat > "$DEB_ROOT/DEBIAN/control" <<EOF
Package: samime
Version: $VERSION
Architecture: amd64
Maintainer: Samime Project <noreply@samime.dev>
Depends: ibus | fcitx5
Recommends: ibus-gtk3, ibus-qt5
Section: utils
Priority: optional
Description: Samime Chinese Input Method (Go)
 Samime is a cross-platform Chinese input method written in Go.
 Features:
  * 136k word dictionary (jieba)
  * 2-gram language model (Wikipedia-trained)
  * Fuzzy pinyin + typo tolerance
  * Sentence segmentation
  * User dictionary persistence (BadgerDB)
  * Time-decay frequency + context association
  * Clipboard history (last 50)
 Homepage: https://github.com/samaidev/samime
EOF

    # postinst - 自动完成所有配置
    cat > "$DEB_ROOT/DEBIAN/postinst" <<'POSTINST'
#!/bin/bash
set -e

# 确保 IBus 组件在标准位置（双重保险）
if [ -f /etc/ibus/component/samime.xml ] && [ ! -f /usr/share/ibus/component/samime.xml ]; then
    mkdir -p /usr/share/ibus/component
    cp /etc/ibus/component/samime.xml /usr/share/ibus/component/samime.xml
fi

# 启动 samime 服务（用户级 systemd）
mkdir -p /etc/systemd/user
if [ -f /etc/systemd/user/samime.service ]; then
    # 找到当前登录用户
    REAL_USER=$(who | awk '{print $1}' | head -1)
    if [ -n "$REAL_USER" ]; then
        USER_UID=$(id -u "$REAL_USER")
        # 用 machinectl 或 runuser 启动用户级服务
        XDG_RUNTIME_DIR="/run/user/$USER_UID"
        if [ -d "$XDG_RUNTIME_DIR" ]; then
            runuser -u "$REAL_USER" -- systemctl --user daemon-reload 2>/dev/null || true
            runuser -u "$REAL_USER" -- systemctl --user enable samime.service 2>/dev/null || true
            runuser -u "$REAL_USER" -- systemctl --user start samime.service 2>/dev/null || true
        fi
    fi
fi

# 添加 samime 到输入源（支持 GNOME 和 IBus 两种配置）
if command -v ibus >/dev/null 2>&1; then
    # 刷新 IBus 缓存
    ibus write-cache --system 2>/dev/null || true

    # 找到当前用户并添加输入源
    REAL_USER=$(who | awk '{print $1}' | head -1)
    if [ -n "$REAL_USER" ]; then
        USER_UID=$(id -u "$REAL_USER")
        XDG_RUNTIME_DIR="/run/user/$USER_UID"
        DBUS_ADDR="unix:path=$XDG_RUNTIME_DIR/bus"

        if [ -d "$XDG_RUNTIME_DIR" ]; then
            # 方式 1: GNOME 桌面用 org.gnome.desktop.input-sources（Ubuntu/Debian 标准方式）
            # 这是"设置 → 键盘 → 输入源"实际读取的配置
            CURRENT_SOURCES=$(runuser -u "$REAL_USER" -- env DBUS_SESSION_BUS_ADDRESS="$DBUS_ADDR" \
                gsettings get org.gnome.desktop.input-sources sources 2>/dev/null || echo "[]")

            # 检查是否已有 samime
            if echo "$CURRENT_SOURCES" | grep -q "samime"; then
                : # 已有，跳过
            else
                # 添加 ('ibus', 'samime') 到现有输入源
                if echo "$CURRENT_SOURCES" | grep -q "\["; then
                    # 在最后一个 ) 后面添加
                    NEW_SOURCES=$(echo "$CURRENT_SOURCES" | sed "s/\]$/, ('ibus', 'samime')]/" | sed "s/\[\]/[('ibus', 'samime')]/")
                else
                    NEW_SOURCES="[('ibus', 'samime')]"
                fi
                runuser -u "$REAL_USER" -- env DBUS_SESSION_BUS_ADDRESS="$DBUS_ADDR" \
                    gsettings set org.gnome.desktop.input-sources sources "$NEW_SOURCES" 2>/dev/null || true
            fi

            # 方式 2: 非 GNOME 桌面用 IBus 的 preload-engines（兼容方式）
            CURRENT_ENGINES=$(runuser -u "$REAL_USER" -- env DBUS_SESSION_BUS_ADDRESS="$DBUS_ADDR" \
                gsettings get org.freedesktop.ibus.general preload-engines 2>/dev/null || echo "[]")
            if ! echo "$CURRENT_ENGINES" | grep -q "samime"; then
                runuser -u "$REAL_USER" -- env DBUS_SESSION_BUS_ADDRESS="$DBUS_ADDR" \
                    gsettings set org.freedesktop.ibus.general preload-engines \
                    "['xkb:us::eng', 'samime']" 2>/dev/null || true
            fi

            # 重启 IBus 让组件生效
            runuser -u "$REAL_USER" -- env DBUS_SESSION_BUS_ADDRESS="$DBUS_ADDR" \
                ibus restart 2>/dev/null || true
        fi
    fi
fi

# 如果用 Fcitx5
if command -v fcitx5 >/dev/null 2>&1; then
    REAL_USER=$(who | awk '{print $1}' | head -1)
    if [ -n "$REAL_USER" ]; then
        USER_UID=$(id -u "$REAL_USER")
        XDG_RUNTIME_DIR="/run/user/$USER_UID"
        if [ -d "$XDG_RUNTIME_DIR" ]; then
            runuser -u "$REAL_USER" -- env DBUS_SESSION_BUS_ADDRESS="unix:path=$XDG_RUNTIME_DIR/bus" \
                fcitx5 -r -d 2>/dev/null || true
        fi
    fi
fi

echo ""
echo "============================================"
echo "  Samime 已安装并自动配置完成！"
echo "============================================"
echo ""
echo "已完成："
echo "  ✓ 二进制安装到 /usr/bin/samime"
echo "  ✓ IBus 组件注册到 /usr/share/ibus/component/"
echo "  ✓ Fcitx5 组件注册到 /usr/share/fcitx5/"
echo "  ✓ systemd 用户服务已启动"
echo "  ✓ IBus 缓存已刷新"
echo "  ✓ Samime 已添加到输入源"
echo ""
echo "使用方法："
echo "  1. 按 Super+Space 切换输入法到 Samime"
echo "  2. 或在 设置 → 键盘 → 输入源 中查看"
echo ""
echo "验证："
echo "  samime -mode=demo    # 演示模式"
echo "  samime -mode=update  # 检查更新"
echo ""
exit 0
POSTINST
    chmod 755 "$DEB_ROOT/DEBIAN/postinst"

    # prerm
    cat > "$DEB_ROOT/DEBIAN/prerm" <<'EOF'
#!/bin/bash
set -e
# 停止服务
systemctl --user stop samime.service 2>/dev/null || true
# 移除 IBus 引擎
gsettings set org.freedesktop.ibus.general preload-engines \
    "$(gsettings get org.freedesktop.ibus.general preload-engines | \
    sed 's/, '"'"'samime'"'"'//g; s/'"'"'samime'"'"', //g; s/'"'"'samime'"'"'//g')" \
    2>/dev/null || true
exit 0
EOF
    chmod 755 "$DEB_ROOT/DEBIAN/prerm"

    # README
    cp README.md "$DEB_ROOT/usr/share/doc/samime/README"
    cat > "$DEB_ROOT/usr/share/doc/samime/copyright" <<'EOF'
Samime is licensed under the MIT License.
Copyright (c) 2024 Samime Project
EOF

    # 构建 .deb
    DEB_FILE="$ROOT/packaging/linux/samime-$VERSION-amd64.deb"
    dpkg-deb --build --root-owner-group "$DEB_ROOT" "$DEB_FILE"
    echo "[OK] .deb 已生成: $DEB_FILE"
}

build_rpm() {
    echo ""
    echo "[3/4] 构建 .rpm 包..."
    if ! command -v rpmbuild >/dev/null 2>&1; then
        echo "[SKIP] 未安装 rpmbuild，跳过 .rpm"
        echo "       安装: sudo apt install rpm  或  sudo dnf install rpm-build"
        return 0
    fi

    RPM_TOP="$STAGE/rpmbuild"
    rm -rf "$RPM_TOP"
    mkdir -p "$RPM_TOP"/{BUILD,RPMS,SOURCES,SPECS,SRPMS}

    # 准备源码 tar
    TAR_DIR="$STAGE/samime-$VERSION"
    mkdir -p "$TAR_DIR/usr/bin" "$TAR_DIR/etc/ibus/component" "$TAR_DIR/etc/systemd/user"
    cp "$STAGE/samime" "$TAR_DIR/usr/bin/samime"
    cp "$STAGE/deb/etc/ibus/component/samime.xml" "$TAR_DIR/etc/ibus/component/"
    cp "$STAGE/deb/etc/systemd/user/samime.service" "$TAR_DIR/etc/systemd/user/"
    tar czf "$RPM_TOP/SOURCES/samime-$VERSION.tar.gz" -C "$STAGE" "samime-$VERSION"

    # spec 文件
    cat > "$RPM_TOP/SPECS/samime.spec" <<SPEC
Name:           samime
Version:        $VERSION
Release:        1%{?dist}
Summary:        Samime Chinese Input Method (Go)
License:        MIT
URL:            https://github.com/samaidev/samime
Source0:        %{name}-%{version}.tar.gz
Requires:       ibus
BuildArch:      noarch

%description
Samime is a cross-platform Chinese input method written in Go.

%prep
%setup -q

%install
mkdir -p %{buildroot}/usr/bin
mkdir -p %{buildroot}/etc/ibus/component
mkdir -p %{buildroot}/etc/systemd/user
cp usr/bin/samime %{buildroot}/usr/bin/samime
cp etc/ibus/component/samime.xml %{buildroot}/etc/ibus/component/
cp etc/systemd/user/samime.service %{buildroot}/etc/systemd/user/

%files
/usr/bin/samime
/etc/ibus/component/samime.xml
/etc/systemd/user/samime.service

%post
ibus write-cache --system >/dev/null 2>&1 || true

%postun
systemctl --user stop samime.service 2>/dev/null || true

%changelog
* $(date '+%a %b %d %Y') Samime Project - $VERSION-1
- Initial package
SPEC

    rpmbuild -bb --define "_topdir $RPM_TOP" "$RPM_TOP/SPECS/samime.spec"

    # 复制 .rpm 到输出
    RPM_FILE=$(find "$RPM_TOP/RPMS" -name "*.rpm" -print -quit)
    if [ -n "$RPM_FILE" ]; then
        cp "$RPM_FILE" "$ROOT/packaging/linux/"
        echo "[OK] .rpm 已生成: $ROOT/packaging/linux/$(basename "$RPM_FILE")"
    fi
}

build_appimage() {
    echo ""
    echo "[4/4] 构建 AppImage..."
    if ! command -v appimagetool >/dev/null 2>&1 && [ ! -f "$STAGE/appimagetool" ]; then
        echo "[SKIP] 未安装 appimagetool，跳过 AppImage"
        echo "       安装: wget https://github.com/AppImage/AppImageKit/releases/download/continuous/appimagetool-x86_64.AppImage"
        echo "             -O /usr/local/bin/appimagetool && chmod +x /usr/local/bin/appimagetool"
        return 0
    fi

    APPDIR="$STAGE/Samime.AppDir"
    rm -rf "$APPDIR"
    mkdir -p "$APPDIR/usr/bin" "$APPDIR/usr/share/applications" "$APPDIR/usr/share/icons/hicolor/48x48/apps"

    cp "$STAGE/samime" "$APPDIR/usr/bin/samime"

    # .desktop
    cat > "$APPDIR/samime.desktop" <<'EOF'
[Desktop Entry]
Name=Samime
Comment=Chinese Input Method
Exec=samime -mode=service
Icon=samime
Terminal=false
Type=Application
Categories=Utility;
EOF
    cp "$APPDIR/samime.desktop" "$APPDIR/usr/share/applications/"

    # AppRun
    cat > "$APPDIR/AppRun" <<'EOF'
#!/bin/bash
SELF="$(readlink -f "$0")"
HERE="$(dirname "$SELF")"
exec "$HERE/usr/bin/samime" "$@"
EOF
    chmod +x "$APPDIR/AppRun"

    # 图标占位（用文本生成简单图标）
    cat > "$APPDIR/samime.svg" <<'EOF'
<svg xmlns="http://www.w3.org/2000/svg" width="48" height="48">
  <rect width="48" height="48" rx="8" fill="#0078D4"/>
  <text x="24" y="32" font-size="24" text-anchor="middle" fill="white" font-family="sans-serif">S</text>
</svg>
EOF

    APPIMAGE_FILE="$ROOT/packaging/linux/Samime-$VERSION-x86_64.AppImage"
    if command -v appimagetool >/dev/null 2>&1; then
        appimagetool "$APPDIR" "$APPIMAGE_FILE"
    else
        "$STAGE/appimagetool" "$APPDIR" "$APPIMAGE_FILE"
    fi

    if [ -f "$APPIMAGE_FILE" ]; then
        chmod +x "$APPIMAGE_FILE"
        echo "[OK] AppImage 已生成: $APPIMAGE_FILE"
    fi
}

case "$FORMAT" in
    deb)
        build_deb
        ;;
    rpm)
        build_deb  # rpm 也需要 deb 的中间产物
        build_rpm
        ;;
    appimage)
        build_appimage
        ;;
    all)
        build_deb
        build_rpm
        build_appimage
        ;;
    *)
        echo "未知格式: $FORMAT"
        echo "支持: deb | rpm | appimage | all"
        exit 1
        ;;
esac

echo ""
echo "=================================================="
echo "[OK] Linux 打包完成"
echo "=================================================="
ls -lh "$ROOT"/packaging/linux/*.deb "$ROOT"/packaging/linux/*.rpm "$ROOT"/packaging/linux/*.AppImage 2>/dev/null

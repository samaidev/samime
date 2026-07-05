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
    mkdir -p "$DEB_ROOT/etc/ibus/component"
    mkdir -p "$DEB_ROOT/etc/systemd/user"

    # 二进制
    cp "$STAGE/samime" "$DEB_ROOT/usr/bin/samime"
    chmod 755 "$DEB_ROOT/usr/bin/samime"

    # IBus 组件配置
    cat > "$DEB_ROOT/etc/ibus/component/samime.xml" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<component>
    <name>org.freedesktop.IBus.Samime</name>
    <description>Samime Chinese Input Method (Go)</description>
    <exec>/usr/bin/samime -mode=service</exec>
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
</component>
EOF

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

    # postinst
    cat > "$DEB_ROOT/DEBIAN/postinst" <<'EOF'
#!/bin/bash
set -e
# 更新 IBus 缓存
if command -v ibus write-cache >/dev/null 2>&1; then
    ibus write-cache --system >/dev/null 2>&1 || true
fi
# 提示用户
echo ""
echo "Samime 已安装。请执行以下步骤启用："
echo "  1. 重启 IBus: ibus restart"
echo "  2. 添加输入法: ibus add-engine samime"
echo "  3. 或在系统设置 -> 键盘 -> 输入源中添加"
echo ""
exit 0
EOF
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

# 代码签名与公证指南

Samime 需要数字签名才能在三大平台上正常运行（不触发安全警告）。

---

## Windows 签名

### 为什么需要签名

- **Windows SmartScreen**：未签名的 .exe 会显示"Windows 已保护你的电脑"警告
- **Windows 11 强制**：内核模式驱动必须签名
- **TSF 注册**：部分 Windows 版本要求 TSF DLL 签名

### 证书类型

| 类型 | 价格 | 验证 | 适用场景 |
|------|------|------|---------|
| 自签名 | 免费 | 无 | 开发测试 |
| OV (Organization Validation) | ~$200/年 | 组织验证 | 小团队 |
| **EV (Extended Validation)** | ~$400/年 | 扩展验证 | **推荐**，SmartScreen 立即信任 |

### 申请 EV 证书

1. **选择证书供应商**：
   - DigiCert: https://www.digicert.com/code-signing/
   - Sectigo: https://sectigo.com/ssl-certificates-tls/code-signing
   - GlobalSign: https://www.globalsign.com/en/code-signing-certificate

2. **准备材料**（以 samai.cc 为例）：
   ```
   组织名称: SamAI Group
   注册地址: [公司注册地址]
   营业执照: [扫描件]
   联系电话: [公司电话]
   域名: samai.cc
   ```

3. **验证流程**（约 3-7 个工作日）：
   - 供应商验证公司注册信息
   - 电话回访确认
   - 域名所有权验证
   - 签发证书（USB Token 或云签名）

### 签名配置

```bash
# 用 signtool 签名（Windows SDK 自带）
signtool sign /fd SHA256 \
    /tr http://timestamp.digicert.com \
    /td SHA256 \
    /sha1 <证书指纹> \
    /d "Samime Chinese Input Method" \
    /du "https://samai.cc" \
    packaging\windows\samime-setup-1.0.0.exe

# 验证签名
signtool verify /pa /v packaging\windows\samime-setup-1.0.0.exe
```

### 在 build_windows_package.bat 中集成

```bat
REM 签名安装包
set SIGNTOOL="C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\signtool.exe"
if exist %SIGNTOOL% (
    %SIGNTOOL% sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 ^
        /sha1 %CERT_THUMBPRINT% ^
        /d "Samime" /du "https://samai.cc" ^
        packaging\windows\samime-setup-1.0.0.exe
    echo [OK] 安装包已签名
)
```

---

## macOS 签名与公证

### 为什么需要签名

- **macOS 11+**：未签名的应用无法运行（Gatekeeper 拦截）
- **IMK Bundle**：必须签名才能被系统加载
- **公证 (Notarization)**：Apple 自动扫描恶意代码，通过后才能分发

### 申请 Developer ID

1. **加入 Apple Developer Program**：
   - 费用：$99/年
   - 注册：https://developer.apple.com/programs/
   - 使用 samai.cc 邮箱注册

2. **创建 Developer ID 证书**：
   - Apple Developer → Certificates, IDs & Profiles
   - 创建 "Developer ID Application" 证书（用于签名应用）
   - 创建 "Developer ID Installer" 证书（用于签名 .pkg）

3. **创建 App-Specific Password**（用于公证）：
   - https://appleid.apple.com → 登录 → App 专用密码

### 签名与公证

```bash
# 环境变量
export DEVELOPER_ID="Developer ID Application: SamAI Group (TEAMID123)"
export APPLE_ID="dev@samai.cc"
export TEAM_ID="TEAMID123"
export APP_PASSWORD="xxxx-xxxx-xxxx-xxxx"

# 1. 签名 .app
codesign --force --deep --options runtime \
    --sign "$DEVELOPER_ID" \
    --timestamp \
    Samime.app

# 2. 创建 .dmg
hdiutil create -volname "Samime" -srcfolder Samime.app -fs HFS+ \
    -format UDZO samime-1.0.0.dmg

# 3. 签名 .dmg
codesign --force --sign "$DEVELOPER_ID" --timestamp samime-1.0.0.dmg

# 4. 提交公证
xcrun notarytool submit samime-1.0.0.dmg \
    --apple-id "$APPLE_ID" \
    --team-id "$TEAM_ID" \
    --password "$APP_PASSWORD" \
    --wait

# 5. 装订公证票据
xcrun stapler staple samime-1.0.0.dmg
xcrun stapler validate samime-1.0.0.dmg
```

### 自动化签名脚本

已集成在 `packaging/macos/build_macos_package.sh` 中：

```bash
DEVELOPER_ID="Developer ID Application: SamAI Group (TEAMID)" \
APPLE_ID="dev@samai.cc" \
TEAM_ID="TEAMID" \
APP_PASSWORD="xxxx-xxxx-xxxx-xxxx" \
bash packaging/macos/build_macos_package.sh 1.0.0
```

---

## Linux 签名

Linux 不强制签名，但推荐：

### GPG 签名

```bash
# 生成 GPG 密钥（如果没有）
gpg --gen-key

# 签名 .deb
dpkg-sig --sign builder packaging/linux/samime-1.0.0-amd64.deb

# 验证签名
dpkg-sig --verify packaging/linux/samime-1.0.0-amd64.deb
```

### APT 仓库签名

```bash
# 生成仓库元数据签名
apt-ftparchive packages . > Packages
apt-ftparchive release . > Release
gpg --batch --yes --detach-sign --armor -o Release.gpg Release
```

### 分发 GPG 公钥

```bash
# 导出公钥
gpg --armor --export dev@samai.cc > samime-key.asc

# 用户导入
sudo apt-key add samime-key.asc
# 或
wget -qO - https://samai.cc/samime-key.asc | sudo apt-key add -
```

---

## samai.cc 证书申请清单

### 准备文件

```
samai.cc/
├── company/
│   ├── business-license.pdf      # 营业执照
│   ├── organization-code.pdf     # 组织机构代码证
│   └── tax-registration.pdf      # 税务登记证
├── contact/
│   ├── dev@samai.cc              # 开发者邮箱
│   ├── legal@samai.cc            # 法务联系人
│   └── phone-verification.txt    # 电话验证记录
└── certificates/
    ├── windows-ev-cert.pfx       # Windows EV 证书
    ├── macos-developer-id.cer    # macOS Developer ID
    └── gpg-public-key.asc        # Linux GPG 公钥
```

### 费用预算

| 项目 | 费用 | 周期 |
|------|------|------|
| Windows EV 证书 | ~$400 | 年 |
| Apple Developer | $99 | 年 |
| 域名 samai.cc | ~$20 | 年 |
| 总计 | ~$520 | 年 |

---

## 签名验证流程

### 用户验证（三平台）

```bash
# Windows
# 右键 .exe → 属性 → 数字签名

# macOS
codesign --verify --verbose=4 Samime.app
spctl --assess --verbose=4 Samime.app

# Linux
dpkg-sig --verify samime-1.0.0-amd64.deb
```

### CI/CD 自动验证

在 `.github/workflows/build-release.yml` 中添加签名验证步骤：

```yaml
- name: Verify Windows signature
  if: runner.os == 'Windows'
  run: signtool verify /pa /v samime-setup-*.exe

- name: Verify macOS signature
  if: runner.os == 'macOS'
  run: |
    codesign --verify --verbose=4 Samime.app
    spctl --assess --verbose=4 Samime.app
```

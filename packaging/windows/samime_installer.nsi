; samime_installer.nsi - NSIS 安装脚本
; 编译: makensis samime_installer.nsi
; 输出: samime-setup-x.y.z.exe
;
; 需要 NSIS 3.x: https://nsis.sourceforge.io/

!define APP_NAME "Samime"
!define APP_NAME_ZH "Samime 中文输入法"
!define APP_VERSION "1.0.0"
!define APP_PUBLISHER "Samime Project"
!define APP_URL "https://github.com/samaidev/samime"
!define APP_REGKEY "Software\Samime"
!define APP_UNINSTKEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\Samime"

Unicode true
ManifestDPIAware true

Name "${APP_NAME} ${APP_VERSION}"
OutFile "samime-setup-${APP_VERSION}.exe"
InstallDir "$PROGRAMFILES64\Samime"
InstallDirRegKey HKLM "${APP_REGKEY}" "InstallDir"
RequestExecutionLevel admin
ShowInstDetails show
ShowUnInstDetails show

; === Version Info ===
VIProductVersion "1.0.0.0"
VIAddVersionKey "ProductName" "${APP_NAME}"
VIAddVersionKey "ProductVersion" "${APP_VERSION}"
VIAddVersionKey "CompanyName" "${APP_PUBLISHER}"
VIAddVersionKey "FileDescription" "${APP_NAME_ZH} 安装程序"
VIAddVersionKey "FileVersion" "${APP_VERSION}"
VIAddVersionKey "LegalCopyright" "MIT License"

; === Modern UI ===
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"

!define MUI_ABORTWARNING
; 图标和 banner 可选（如果存在则使用，不存在用默认）
; !define MUI_ICON "samime.ico"
; !define MUI_UNICON "samime.ico"
; !define MUI_WELCOMEFINISHPAGE_BITMAP "samime-banner.bmp"

; Welcome page
!insertmacro MUI_PAGE_WELCOME
; License page
!insertmacro MUI_PAGE_LICENSE "stage\LICENSE.txt"
; Components page
!insertmacro MUI_PAGE_COMPONENTS
; Directory page
!insertmacro MUI_PAGE_DIRECTORY
; Install files page
!insertmacro MUI_PAGE_INSTFILES
; Finish page (with option to start service)
!define MUI_FINISHPAGE_RUN "$INSTDIR\samime.exe"
!define MUI_FINISHPAGE_RUN_PARAMETERS "-mode=service"
!define MUI_FINISHPAGE_RUN_TEXT "启动 Samime 服务（推荐）"
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\README.txt"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "查看安装说明"
!define MUI_FINISHPAGE_LINK "访问项目主页"
!define MUI_FINISHPAGE_LINK_LOCATION "${APP_URL}"
!insertmacro MUI_PAGE_FINISH

; Uninstaller pages
!insertmacro MUI_UNPAGE_WELCOME
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_UNPAGE_FINISH

; Languages
!insertmacro MUI_LANGUAGE "SimpChinese"
!insertmacro MUI_LANGUAGE "English"

; === Sections ===

Section "Samime 核心引擎" SecCore
    SectionIn RO  ; 必选
    SetOutPath "$INSTDIR"

    ; 主程序（从 stage 目录读）
    File "stage\samime.exe"
    File "stage\README.txt"
    File "stage\LICENSE.txt"

    ; 写注册表
    WriteRegStr HKLM "${APP_REGKEY}" "InstallDir" "$INSTDIR"
    WriteRegStr HKLM "${APP_REGKEY}" "Version" "${APP_VERSION}"
    WriteRegStr HKLM "${APP_UNINSTKEY}" "DisplayName" "${APP_NAME_ZH}"
    WriteRegStr HKLM "${APP_UNINSTKEY}" "DisplayVersion" "${APP_VERSION}"
    WriteRegStr HKLM "${APP_UNINSTKEY}" "Publisher" "${APP_PUBLISHER}"
    WriteRegStr HKLM "${APP_UNINSTKEY}" "DisplayIcon" "$INSTDIR\samime.exe"
    WriteRegStr HKLM "${APP_UNINSTKEY}" "URLInfoAbout" "${APP_URL}"
    WriteRegStr HKLM "${APP_UNINSTKEY}" "InstallLocation" "$INSTDIR"
    WriteRegStr HKLM "${APP_UNINSTKEY}" "UninstallString" '"$INSTDIR\uninstall.exe"'
    WriteRegDWORD HKLM "${APP_UNINSTKEY}" "NoModify" 1
    WriteRegDWORD HKLM "${APP_UNINSTKEY}" "NoRepair" 1

    ; 卸载程序
    WriteUninstaller "$INSTDIR\uninstall.exe"

    ; 创建开始菜单快捷方式
    CreateDirectory "$SMPROGRAMS\Samime"
    CreateShortcut "$SMPROGRAMS\Samime\Samime 服务.lnk" "$INSTDIR\samime.exe" "-mode=service"
    CreateShortcut "$SMPROGRAMS\Samime\Samime 演示.lnk" "$INSTDIR\samime.exe" "-mode=demo"
    CreateShortcut "$SMPROGRAMS\Samime\卸载 Samime.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "TSF 输入法集成" SecTSF
    SetOutPath "$INSTDIR"
    File "stage\samime_tsf.dll"

    ; 注册 TSF 服务
    ExecWait 'regsvr32 /s "$INSTDIR\samime_tsf.dll"'
    DetailPrint "已注册 TSF 输入法服务"
SectionEnd

Section "开机自启动" SecAutoStart
    ; 用计划任务实现开机自启
    ExecWait 'schtasks /Create /TN "SamimeService" /TR "\"$INSTDIR\samime.exe\" -mode=service" /SC ONLOGON /RL HIGHEST /F'
    DetailPrint "已设置开机自启动"
SectionEnd

Section "创建桌面快捷方式" SecDesktop
    CreateShortcut "$DESKTOP\Samime.lnk" "$INSTDIR\samime.exe" "-mode=service"
SectionEnd

; === Section Descriptions ===
LangString DESC_SecCore ${LANG_SIMPCHINESE} "Samime 核心引擎（必选）"
LangString DESC_SecTSF ${LANG_SIMPCHINESE} "注册 TSF 输入法服务，让 Samime 成为系统输入法"
LangString DESC_SecAutoStart ${LANG_SIMPCHINESE} "开机自动启动 Samime 服务"
LangString DESC_SecDesktop ${LANG_SIMPCHINESE} "在桌面创建快捷方式"

LangString DESC_SecCore ${LANG_ENGLISH} "Samime core engine (required)"
LangString DESC_SecTSF ${LANG_ENGLISH} "Register TSF input method service"
LangString DESC_SecAutoStart ${LANG_ENGLISH} "Start Samime service on boot"
LangString DESC_SecDesktop ${LANG_ENGLISH} "Create desktop shortcut"

!insertmacro MUI_FUNCTION_DESCRIPTION_BEGIN
!insertmacro MUI_DESCRIPTION_TEXT ${SecCore} $(DESC_SecCore)
!insertmacro MUI_DESCRIPTION_TEXT ${SecTSF} $(DESC_SecTSF)
!insertmacro MUI_DESCRIPTION_TEXT ${SecAutoStart} $(DESC_SecAutoStart)
!insertmacro MUI_DESCRIPTION_TEXT ${SecDesktop} $(DESC_SecDesktop)
!insertmacro MUI_FUNCTION_DESCRIPTION_END

; === Uninstall ===
Section "Uninstall"
    ; 停止服务
    ExecWait 'taskkill /IM samime.exe /F'

    ; 注销 TSF
    ExecWait 'regsvr32 /u /s "$INSTDIR\samime_tsf.dll"'

    ; 删除计划任务
    ExecWait 'schtasks /Delete /TN "SamimeService" /F'

    ; 删除文件
    Delete "$INSTDIR\samime.exe"
    Delete "$INSTDIR\samime_tsf.dll"
    Delete "$INSTDIR\README.txt"
    Delete "$INSTDIR\LICENSE.txt"
    Delete "$INSTDIR\uninstall.exe"
    RMDir /r "$INSTDIR"

    ; 删除快捷方式
    RMDir /r "$SMPROGRAMS\Samime"
    Delete "$DESKTOP\Samime.lnk"

    ; 清理注册表
    DeleteRegKey HKLM "${APP_UNINSTKEY}"
    DeleteRegKey HKLM "${APP_REGKEY}"

    DetailPrint "Samime 已完全卸载"
SectionEnd

; === 检查管理员权限 ===
Function .onInit
    ${IfNot} ${RunningX64}
        MessageBox MB_OK|MB_ICONSTOP "Samime 需要 64 位 Windows 系统。"
        Abort
    ${EndIf}
FunctionEnd

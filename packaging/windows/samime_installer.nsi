; samime_installer.nsi - NSIS installer script for Samime
; Build: makensis samime_installer.nsi
; Output: samime-setup-x.y.z.exe
; Requires NSIS 3.x: https://nsis.sourceforge.io/

!define APP_NAME "Samime"
!define APP_NAME_ZH "Samime Chinese Input Method"
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
VIAddVersionKey "FileDescription" "${APP_NAME_ZH} Installer"
VIAddVersionKey "FileVersion" "${APP_VERSION}"
VIAddVersionKey "LegalCopyright" "MIT License"

; === Modern UI ===
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "x64.nsh"

!define MUI_ABORTWARNING
; Icons and banner are optional (use NSIS defaults if not specified)
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
!define MUI_FINISHPAGE_RUN_TEXT "Start Samime service (recommended)"
!define MUI_FINISHPAGE_SHOWREADME "$INSTDIR\README.txt"
!define MUI_FINISHPAGE_SHOWREADME_TEXT "View installation guide"
!define MUI_FINISHPAGE_LINK "Visit project homepage"
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

Section "Samime Core Engine" SecCore
    SectionIn RO  ; Required
    SetOutPath "$INSTDIR"

    ; Main executable (from stage directory)
    File "stage\samime.exe"
    File "stage\README.txt"
    File "stage\LICENSE.txt"

    ; Registry entries
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

    ; Uninstaller
    WriteUninstaller "$INSTDIR\uninstall.exe"

    ; Start menu shortcuts
    CreateDirectory "$SMPROGRAMS\Samime"
    CreateShortcut "$SMPROGRAMS\Samime\Samime Service.lnk" "$INSTDIR\samime.exe" "-mode=service"
    CreateShortcut "$SMPROGRAMS\Samime\Samime Demo.lnk" "$INSTDIR\samime.exe" "-mode=demo"
    CreateShortcut "$SMPROGRAMS\Samime\Uninstall Samime.lnk" "$INSTDIR\uninstall.exe"
SectionEnd

Section "TSF Input Method Integration" SecTSF
    SetOutPath "$INSTDIR"
    File "stage\samime_tsf.dll"

    ; Register TSF service
    ExecWait 'regsvr32 /s "$INSTDIR\samime_tsf.dll"'
    DetailPrint "Registered TSF input method service"
SectionEnd

Section "Start on Boot" SecAutoStart
    ; Use scheduled task for auto-start
    ExecWait 'schtasks /Create /TN "SamimeService" /TR "\"$INSTDIR\samime.exe\" -mode=service" /SC ONLOGON /RL HIGHEST /F'
    DetailPrint "Set up auto-start on boot"
SectionEnd

Section "Create Desktop Shortcut" SecDesktop
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
    ; Stop service
    ExecWait 'taskkill /IM samime.exe /F'

    ; Unregister TSF
    ExecWait 'regsvr32 /u /s "$INSTDIR\samime_tsf.dll"'

    ; Remove scheduled task
    ExecWait 'schtasks /Delete /TN "SamimeService" /F'

    ; Delete files
    Delete "$INSTDIR\samime.exe"
    Delete "$INSTDIR\samime_tsf.dll"
    Delete "$INSTDIR\README.txt"
    Delete "$INSTDIR\LICENSE.txt"
    Delete "$INSTDIR\uninstall.exe"
    RMDir /r "$INSTDIR"

    ; Delete shortcuts
    RMDir /r "$SMPROGRAMS\Samime"
    Delete "$DESKTOP\Samime.lnk"

    ; Clean registry
    DeleteRegKey HKLM "${APP_UNINSTKEY}"
    DeleteRegKey HKLM "${APP_REGKEY}"

    DetailPrint "Samime has been completely uninstalled"
SectionEnd

; === Check admin privileges ===
Function .onInit
    ${IfNot} ${RunningX64}
        MessageBox MB_OK|MB_ICONSTOP "Samime requires 64-bit Windows."
        Abort
    ${EndIf}
FunctionEnd

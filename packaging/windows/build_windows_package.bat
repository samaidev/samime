@echo off
REM build_windows_package.bat
REM 在 Windows 上构建 samime 安装包
REM
REM 依赖:
REM   - Go 1.22+
REM   - NSIS 3.x (https://nsis.sourceforge.io/)
REM   - Visual Studio Build Tools (可选，用于编译 TSF DLL)

setlocal enabledelayedexpansion

set VERSION=1.0.0
set ROOT=%~dp0..\..
set STAGE=%ROOT%\packaging\windows\stage

cd /d %ROOT%

echo ==================================================
echo Samime Windows 安装包构建 v%VERSION%
echo ==================================================

echo.
echo [1/5] 清理旧的构建产物...
rmdir /s /q %STAGE% 2>nul
mkdir %STAGE%

echo.
echo [2/5] 编译 Go 引擎 (samime.exe)...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w -X main.version=%VERSION%" -o %STAGE%\samime.exe ./cmd/ime-cli
if errorlevel 1 (
    echo [FAIL] Go 编译失败
    exit /b 1
)
echo [OK] samime.exe 已生成

echo.
echo [3/5] 编译 TSF DLL (samime_tsf.dll)...
where cl >nul 2>nul
if %ERRORLEVEL% == 0 (
    pushd internal\winime\cpp
    cl /EHsc /std:c++17 /O2 /LD ^
       samime_tsf.cpp samime_reg.cpp d2d_renderer.cpp ^
       /link ole32.lib oleaut32.lib msctf.lib user32.lib gdi32.lib ws2_32.lib shlwapi.lib ^
             d2d1.lib dwrite.lib ^
       /DEF:samime_tsf.def /OUT:samime_tsf.dll 2>nul
    if exist samime_tsf.dll (
        copy samime_tsf.dll %STAGE%\
        echo [OK] samime_tsf.dll 已生成
    ) else (
        echo [WARN] TSF DLL 编译失败，跳过（安装包将不包含 TSF 集成）
    )
    popd
) else (
    echo [SKIP] 未找到 cl.exe，跳过 TSF DLL 编译
    echo        如需 TSF 集成，请安装 Visual Studio Build Tools
)

echo.
echo [4/5] 准备打包文件...
copy README.md %STAGE%\README.txt >nul
echo MIT License > %STAGE%\LICENSE.txt

echo.
echo [5/5] 用 NSIS 生成安装包...
set MAKENSIS=
where makensis >nul 2>nul && set MAKENSIS=makensis
if not defined MAKENSIS (
    if exist "C:\Program Files (x86)\NSIS\makensis.exe" set MAKENSIS="C:\Program Files (x86)\NSIS\makensis.exe"
)
if not defined MAKENSIS (
    if exist "C:\Program Files\NSIS\makensis.exe" set MAKENSIS="C:\Program Files\NSIS\makensis.exe"
)

if defined MAKENSIS (
    pushd packaging\windows
    %MAKENSIS% samime_installer.nsi
    if exist samime-setup-%VERSION%.exe (
        echo.
        echo ==================================================
        echo [OK] 安装包已生成: packaging\windows\samime-setup-%VERSION%.exe
        echo ==================================================
        dir samime-setup-%VERSION%.exe
    ) else (
        echo [FAIL] NSIS 打包失败
    )
    popd
) else (
    echo [FAIL] 未找到 makensis，请安装 NSIS 3.x
    echo        下载: https://nsis.sourceforge.io/Download
    echo.
    echo 已暂存文件:
    dir %STAGE%
)

endlocal

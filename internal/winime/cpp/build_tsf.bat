@echo off
REM build_tsf.bat - 编译 samime_tsf.dll
REM
REM 需要 Visual Studio Build Tools (cl.exe) 或 MinGW

setlocal

cd /d %~dp0

REM === 方式1: Visual Studio (推荐) ===
where cl >nul 2>nul
if %ERRORLEVEL% == 0 (
    echo [1/3] 用 cl.exe 编译...
    cl /EHsc /std:c++17 /O2 /LD ^
       samime_tsf.cpp samime_reg.cpp ^
       /link ole32.lib oleaut32.lib msctf.lib user32.lib gdi32.lib ws2_32.lib shlwapi.lib ^
       /DEF:samime_tsf.def /OUT:samime_tsf.dll
    if exist samime_tsf.dll (
        echo [2/3] 编译成功: samime_tsf.dll
        dir samime_tsf.dll
        echo [3/3] 安装步骤:
        echo   1. 复制 samime_tsf.dll 到 C:\Program Files\SamIME\
        echo   2. 启动 Go 引擎: samime.exe -mode=service
        echo   3. 注册: regsvr32 "C:\Program Files\SamIME\samime_tsf.dll"
        echo   4. 系统设置 -> 时间和语言 -> 语言 -> 中文 -> 选项 -> 添加 Samime
    ) else (
        echo [FAIL] 编译失败
        exit /b 1
    )
    goto :eof
)

REM === 方式2: MinGW ===
where g++ >nul 2>nul
if %ERRORLEVEL% == 0 (
    echo [1/3] 用 g++ 编译...
    g++ -shared -std=c++17 -O2 -o samime_tsf.dll ^
        samime_tsf.cpp samime_reg.cpp ^
        -lole32 -loleaut32 -lmsctf -luser32 -lgdi32 -lws2_32 -lshlwapi ^
        -Wl,--output-def,samime_tsf.def
    if exist samime_tsf.dll (
        echo [2/3] 编译成功: samime_tsf.dll
        dir samime_tsf.dll
    ) else (
        echo [FAIL] 编译失败
        exit /b 1
    )
    goto :eof
)

echo [FAIL] 找不到 cl.exe 或 g++
echo 请安装 Visual Studio Build Tools 或 MinGW
exit /b 1

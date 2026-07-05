@echo off
REM GoIME Windows 全流程测试脚本
REM 在 Windows 上运行: scripts\test_win.bat

setlocal
cd /d %~dp0\..

echo ==================================================
echo GoIME Windows 全流程测试
echo ==================================================

REM 0. 环境检查
echo.
echo [0/5] 环境检查
go version
echo OS: %OS% %PROCESSOR_ARCHITECTURE%

REM 1. 构建
echo.
echo [1/5] 构建
go build -o bin\samime.exe ./cmd/ime-cli
if errorlevel 1 (
    echo BUILD FAILED
    exit /b 1
)
echo BUILD OK
dir bin\samime.exe

REM 2. 单元测试
echo.
echo [2/5] 单元测试
go test ./internal/...
if errorlevel 1 (
    echo TEST FAILED
    exit /b 1
)

REM 3. 性能基准
echo.
echo [3/5] 性能基准
go test -bench=. -benchmem -run=^$ ./internal/engine/

REM 4. E2E 集成测试
echo.
echo [4/5] 端到端集成测试
go test -tags=integration ./test/

REM 5. Demo 演示
echo.
echo [5/5] CLI 演示
bin\samime.exe -mode=demo

echo.
echo ==================================================
echo Windows 测试完成
echo ==================================================
endlocal

#!/bin/bash
# 交叉编译：在 Linux 上为三大平台编译 GoIME
set -e

export PATH=$PATH:/home/z/.local/go/bin
ROOT=/home/z/my-project/go-ime
cd "$ROOT"
mkdir -p bin

VERSION=$(git rev-parse --short HEAD 2>/dev/null || echo "dev")
echo "[1/4] 编译 Linux amd64"
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/samime-linux-amd64 ./cmd/ime-cli

echo "[2/4] 编译 Windows amd64"
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H windowsgui" -o bin/samime-windows-amd64.exe ./cmd/ime-cli

echo "[3/4] 编译 macOS amd64"
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/samime-darwin-amd64 ./cmd/ime-cli

echo "[4/4] 编译 macOS arm64 (Apple Silicon)"
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/samime-darwin-arm64 ./cmd/ime-cli

echo ""
echo "构建完成:"
ls -lh bin/samime-* 2>/dev/null
echo ""
echo "版本: $VERSION"
echo "Linux:   bin/samime-linux-amd64"
echo "Windows: bin/samime-windows-amd64.exe"
echo "macOS:   bin/samime-darwin-amd64 / bin/samime-darwin-arm64"

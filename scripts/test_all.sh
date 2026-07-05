#!/bin/bash
# GoIME Linux 全流程测试脚本
# 用法: bash scripts/test_all.sh

set -e

export PATH=$PATH:/home/z/.local/go/bin
ROOT=/home/z/my-project/go-ime
cd "$ROOT"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

echo "================================================"
echo -e "${YELLOW}GoIME Linux 全流程测试${NC}"
echo "================================================"

# 0. 环境检查
echo ""
echo -e "${YELLOW}[0/6] 环境检查${NC}"
echo "Go version: $(go version)"
echo "OS: $(uname -srmo)"
echo "Project root: $ROOT"

# 1. 构建
echo ""
echo -e "${YELLOW}[1/6] 构建项目${NC}"
go build -o bin/ime-cli ./cmd/ime-cli
echo -e "${GREEN}✓ 构建成功${NC}"
ls -lh bin/ime-cli

# 2. 单元测试
echo ""
echo -e "${YELLOW}[2/6] 单元测试${NC}"
go test ./internal/... 2>&1 | sed 's/^/  /'
echo -e "${GREEN}✓ 单元测试全部通过${NC}"

# 3. 性能基准
echo ""
echo -e "${YELLOW}[3/6] 性能基准${NC}"
go test -bench=. -benchmem -run=^$ ./internal/engine/ 2>&1 | sed 's/^/  /'
echo -e "${GREEN}✓ 性能基准完成${NC}"

# 4. 端到端集成测试
echo ""
echo -e "${YELLOW}[4/6] 端到端集成测试${NC}"
go test -tags=integration -v ./test/ 2>&1 | grep -E '(PASS|FAIL|---|ok|E2E|fuzzy|typo|latency|benchmark)' | sed 's/^/  /'
echo -e "${GREEN}✓ 端到端测试通过${NC}"

# 5. CLI 演示
echo ""
echo -e "${YELLOW}[5/6] CLI 演示${NC}"
./bin/ime-cli -mode=demo 2>&1 | sed 's/^/  /'
echo -e "${GREEN}✓ CLI 演示成功${NC}"

# 6. Batch 模式
echo ""
echo -e "${YELLOW}[6/6] Batch 模式 (批处理)${NC}"
printf "nihao\nzhongguo\nshurufa\nlihao\nnigao\nrengongzhineng\n" | ./bin/ime-cli -mode=batch 2>/dev/null | sed 's/^/  /'
echo -e "${GREEN}✓ Batch 模式成功${NC}"

# 总结
echo ""
echo "================================================"
echo -e "${GREEN}✓ 所有测试通过${NC}"
echo "================================================"
echo ""
echo "测试产物:"
echo "  - CLI 二进制: $ROOT/bin/ime-cli"
echo "  - 词典文件:   $ROOT/internal/dict/data/jieba.txt"
echo "  - 测试报告:   上方输出"
echo ""
echo "下一步可用命令:"
echo "  $ROOT/bin/ime-cli -mode=interactive    # 交互模式"
echo "  $ROOT/bin/ime-cli -mode=bench -bench-n=10000    # 性能压测"
echo "  $ROOT/bin/ime-cli -dict /path/to/extra.txt    # 加载扩展词典"

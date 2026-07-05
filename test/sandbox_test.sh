#!/bin/bash
# sandbox_test.sh - 沙盒测试脚本
# 在隔离环境中验证安装包的功能
#
# 用法:
#   bash sandbox_test.sh <installer_path>
#   bash sandbox_test.sh packaging/linux/samime-1.0.0-amd64.deb
#
# 测试内容:
#   1. 安装包结构验证
#   2. 二进制可执行性
#   3. 服务模式启动
#   4. 搜索功能验证
#   5. 容错功能验证
#   6. 性能验证
#   7. 清理

set -e

INSTALLER="${1:-}"
VERSION="${2:-1.0.0}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m'

PASS=0
FAIL=0

log_pass() { echo -e "${GREEN}[PASS]${NC} $1"; PASS=$((PASS+1)); }
log_fail() { echo -e "${RED}[FAIL]${NC} $1"; FAIL=$((FAIL+1)); }
log_info() { echo -e "${YELLOW}[INFO]${NC} $1"; }

if [ -z "$INSTALLER" ]; then
    echo "用法: bash sandbox_test.sh <installer_path> [version]"
    echo ""
    echo "支持格式:"
    echo "  .deb   - Debian/Ubuntu 安装包"
    echo "  .exe   - Windows 安装包（需在 Windows 上运行）"
    echo "  .dmg   - macOS 安装包（需在 macOS 上运行）"
    echo "  binary - 单独的二进制文件"
    exit 1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SANDBOX="$ROOT/test/sandbox"
mkdir -p "$SANDBOX"

echo "=================================================="
echo "Samime 沙盒测试 v$VERSION"
echo "安装包: $INSTALLER"
echo "=================================================="

# === 测试 1: 安装包结构 ===
echo ""
echo "=== [1/7] 安装包结构验证 ==="

if [[ "$INSTALLER" == *.deb ]]; then
    if [ ! -f "$INSTALLER" ]; then
        log_fail "安装包不存在: $INSTALLER"
        exit 1
    fi
    log_info "检查 .deb 包结构..."
    if dpkg-deb -I "$INSTALLER" | grep -q "Package: samime"; then
        log_pass ".deb 包元数据正确"
    else
        log_fail ".deb 包元数据不正确"
    fi
    if dpkg-deb -c "$INSTALLER" | grep -q "usr/bin/samime"; then
        log_pass "包含 samime 二进制"
    else
        log_fail "缺少 samime 二进制"
    fi
    if dpkg-deb -c "$INSTALLER" | grep -q "ibus/component/samime.xml"; then
        log_pass "包含 IBus 配置"
    else
        log_fail "缺少 IBus 配置"
    fi

    # 解压到沙盒
    log_info "解压到沙盒: $SANDBOX"
    dpkg-deb -x "$INSTALLER" "$SANDBOX"
    BINARY="$SANDBOX/usr/bin/samime"

elif [[ "$INSTALLER" == *.exe ]]; then
    log_info "Windows 安装包，请在 Windows 上运行此测试"
    log_info "检查文件大小..."
    if [ -f "$INSTALLER" ]; then
        SIZE=$(stat -c%s "$INSTALLER" 2>/dev/null || stat -f%z "$INSTALLER")
        if [ "$SIZE" -gt 1000000 ]; then
            log_pass "安装包大小合理 ($SIZE bytes)"
        else
            log_fail "安装包太小 ($SIZE bytes)"
        fi
    fi
    exit 0

elif [[ "$INSTALLER" == *.dmg ]]; then
    log_info "macOS 安装包，请在 macOS 上运行此测试"
    exit 0

else
    # 假设是二进制文件
    log_info "直接测试二进制文件"
    cp "$INSTALLER" "$SANDBOX/samime"
    BINARY="$SANDBOX/samime"
fi

# === 测试 2: 二进制可执行性 ===
echo ""
echo "=== [2/7] 二进制可执行性 ==="
chmod +x "$BINARY"
if "$BINARY" -mode=demo 2>&1 | grep -q "GoIME"; then
    log_pass "二进制可执行"
else
    log_fail "二进制无法执行"
    exit 1
fi

# === 测试 3: 词典加载 ===
echo ""
echo "=== [3/7] 词典加载验证 ==="
OUTPUT=$("$BINARY" -mode=demo 2>&1)
if echo "$OUTPUT" | grep -q "136004 条"; then
    log_pass "词典加载正确（136004 条）"
else
    log_fail "词典加载异常"
fi

# === 测试 4: 搜索功能 ===
echo ""
echo "=== [4/7] 搜索功能验证 ==="
test_search() {
    local input="$1"
    local expected="$2"
    local desc="$3"
    local result=$(echo "$input" | "$BINARY" -mode=batch 2>/dev/null | head -1)
    if echo "$result" | grep -q "$expected"; then
        log_pass "$desc: '$input' -> '$expected'"
    else
        log_fail "$desc: '$input' -> 期望 '$expected', 实际 '$result'"
    fi
}

test_search "nihao" "你好" "基础搜索"
test_search "zhongguo" "中国" "基础搜索"
test_search "shurufa" "输入法" "基础搜索"
test_search "woaixuexi" "我爱学习" "整句切分"

# === 测试 5: 容错功能 ===
echo ""
echo "=== [5/7] 容错功能验证 ==="
test_tolerance() {
    local input="$1"
    local expected="$2"
    local desc="$3"
    local result=$(echo "$input" | "$BINARY" -mode=batch 2>/dev/null | head -1)
    if echo "$result" | grep -q "$expected"; then
        log_pass "$desc"
    else
        log_fail "$desc: '$input' 期望包含 '$expected', 实际 '$result'"
    fi
}

test_tolerance "lihao" "你好" "模糊音 n/l (lihao→你好)"
test_tolerance "nigao" "你好" "拼写错误 h/g (nigao→你好)"

# === 测试 6: 单字母联想 ===
echo ""
echo "=== [6/7] 单字母联想验证 ==="
test_single() {
    local input="$1"
    local desc="$2"
    local result=$(echo "$input" | "$BINARY" -mode=batch 2>/dev/null | head -1)
    if [ -n "$result" ]; then
        log_pass "$desc: '$input' -> 有候选"
    else
        log_fail "$desc: '$input' -> 无候选"
    fi
}

test_single "n" "单字母 n"
test_single "w" "单字母 w"
test_single "z" "单字母 z"

# === 测试 7: 性能验证 ===
echo ""
echo "=== [7/7] 性能验证 ==="
PERF=$("$BINARY" -mode=bench -bench-n=1000 2>&1)
if echo "$PERF" | grep -q "bench"; then
    log_pass "性能测试通过"
    echo "  $PERF"
else
    log_fail "性能测试异常"
fi

# === 测试更新检查 ===
echo ""
echo "=== 额外: 更新检查 ==="
UPDATE_OUTPUT=$("$BINARY" -mode=update 2>&1)
if echo "$UPDATE_OUTPUT" | grep -q "update"; then
    log_pass "更新检查功能正常"
else
    log_fail "更新检查功能异常"
fi

# === 清理 ===
echo ""
echo "=== 清理沙盒 ==="
rm -rf "$SANDBOX"
log_info "沙盒已清理"

# === 汇总 ===
echo ""
echo "=================================================="
echo "测试结果汇总"
echo "=================================================="
echo -e "通过: ${GREEN}$PASS${NC}  失败: ${RED}$FAIL${NC}"
echo ""

if [ $FAIL -eq 0 ]; then
    echo -e "${GREEN}✓ 全部测试通过${NC}"
    exit 0
else
    echo -e "${RED}✗ 有 $FAIL 个测试失败${NC}"
    exit 1
fi

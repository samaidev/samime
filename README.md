# GoIME - 跨平台中文输入法（Go 实现）

> MVP 版本：Linux 全流程已验证 ✅

## 项目目标

用 Go 开发一个支持 Windows / macOS / Linux 三平台的中文拼音输入法，要求：
- **基础词库丰富**：内置 13.6 万词条（来自结巴词典）
- **容错能力强**：模糊音 + 邻键容错 + 编辑距离
- **跨平台**：核心引擎纯 Go，平台层薄壳
- **可二次开发**：模块化，开源协议

## 架构

```
┌──────────────────────────────────────────────────────┐
│                平台 UI 层（薄壳）                      │
│  Windows TSF (C++)  │  macOS IMK (Swift)  │  Linux IBus (Go) │
└───────────┬───────────────────┬──────────────────┬────┘
            │                   │                  │
            └─────────┬─────────┴──────────────────┘
                      │  gRPC + Unix Domain Socket (JSON)
┌─────────────────────▼─────────────────────────────────┐
│              GoIME Engine (纯 Go，跨平台)               │
│ ┌──────────┬──────────┬──────────┬─────────┬────────┐ │
│ │ Pinyin   │ Dict     │ Ranker   │ Fuzzy   │ User   │ │
│ │ Segment  │ Lookup   │ (n-gram) │ Engine  │ Dict   │ │
│ └──────────┴──────────┴──────────┴─────────┴────────┘ │
└──────────────────────────────────────────────────────┘
```

## 模块说明

| 路径 | 说明 | 关键技术 |
|------|------|---------|
| `internal/pinyin` | 拼音切分、声韵母识别、DP 最优切分 | AC 自动机 + DP |
| `internal/dict` | 词典加载与前缀检索 | `go:embed` + Trie |
| `internal/fuzzy` | 模糊音、邻键容错、编辑距离 | 规则引擎 + BK-Tree 思路 |
| `internal/engine` | 核心引擎，组合各模块 | 综合打分排序 |
| `internal/ibus` | IBus 适配器 | D-Bus（待完善）+ stdin 测试 |
| `cmd/ime-cli` | CLI 测试入口 | 4 种模式 |

## 容错能力（三层模型）

### Layer 1: 模糊音
默认开启，兼容 Rime 方案：
- `z/zh`, `c/ch`, `s/sh`
- `n/l`
- `an/ang`, `en/eng`, `in/ing`
- `ian/iang`, `uan/uang`, `ou/uo`

**示例**：`lihao → 你好`（n/l 模糊）

### Layer 2: 拼写错误容错
- **邻键替换**：QWERTY 键盘邻键图，每位置可替换为邻键
- **长度一致**：变体必须与原输入等长
- **音节数一致**：segment 后音节数必须相同

**示例**：`nigao → 你好`（h 错按成 g）；`nohao → 你好`（i 错按成 o）

### Layer 3: 用户词典自学习
- 用户每次提交候选词，对应 (word, pinyin) 频次 +1
- 下次相同输入时该候选优先级提升

## 词库策略

| 层级 | 来源 | 规模 | 加载方式 |
|------|------|------|---------|
| 内置基础词库 | jieba 词典 | 13.6w | `go:embed` |
| 扩展词库 | Rime / THUOCL | 任意 | 运行时加载 |
| 用户词典 | 用户输入历史 | 动态 | 内存中（MVP）/ BadgerDB（待实现） |

## 性能指标（Linux 容器实测）

| 指标 | 数值 | 备注 |
|------|------|------|
| 词典加载时间 | 244ms | 13.6w 词条 |
| 短输入查询（2 音节） | 60µs | 远低于 16ms 阈值 |
| 长输入查询（4+ 音节） | 571µs | 仍 < 1ms |
| 模糊音查询 | 145µs | 含笛卡尔积展开 |
| QPS | 3669 | 2000 次混合查询平均 |
| 16ms 延迟达标率 | 100% | 1000 次查询 0 次超时 |
| 二进制大小 | 5.4MB | 静态编译，无外部依赖 |

## 测试覆盖

| 类型 | 测试数 | 状态 |
|------|--------|------|
| 单元测试 - pinyin | 4 | ✅ |
| 单元测试 - dict | 5 | ✅ |
| 单元测试 - fuzzy | 6 | ✅ |
| 单元测试 - engine | 6 | ✅ |
| 集成测试 - E2E | 9 | ✅ |
| 性能基准 | 4 | ✅ |

**典型测试用例**：

| 输入 | Top1 候选 | 容错类型 |
|------|----------|---------|
| `nihao` | 你好 | 精确匹配 |
| `zhongguo` | 中国 | 精确匹配 |
| `shurufa` | 输入法 | 精确匹配 |
| `rengongzhineng` | 人工智能 | 长词匹配 |
| `lihao` | 厉害 (Top3 含你好) | n/l 模糊音 |
| `zongguo` | 中国 | zh/z 模糊音 |
| `nigao` | 你好 | h/g 邻键容错 |
| `nohao` | 你好 | i/o 邻键容错 |

## 快速开始

### 构建
```bash
export PATH=$PATH:/home/z/.local/go/bin
cd /home/z/my-project/go-ime
go build -o bin/ime-cli ./cmd/ime-cli
```

### 演示模式
```bash
./bin/ime-cli -mode=demo
```

### 交互模式
```bash
./bin/ime-cli -mode=interactive
# 输入拼音 + Enter 选第一个候选
# 输入 1-9 选对应候选
# ESC 清空；Ctrl+C 退出
```

### 批处理模式（适合脚本化测试）
```bash
printf "nihao\nzhongguo\nshurufa\n" | ./bin/ime-cli -mode=batch
# 输出: nihao	你好 极好 记号 利好 泥淖 ...
```

### 性能压测
```bash
./bin/ime-cli -mode=bench -bench-n=10000
```

### 加载扩展词典
```bash
./bin/ime-cli -dict /path/to/extra.txt
# 格式：每行 `汉字 拼音 词频`
```

### 全流程测试
```bash
bash scripts/test_all.sh
```

## 后续开发路线

### Phase 1（已完成 ✅）
- [x] 拼音切分
- [x] 词典检索（13.6w 词条）
- [x] 模糊音 / 邻键容错
- [x] 候选排序
- [x] 用户词典（内存）
- [x] CLI 全流程测试

### Phase 2（计划中）
- [ ] 整句切分（当前仅整词匹配，长句无法组合）
- [ ] 2-gram 语言模型（提升排序质量）
- [ ] 用户词典持久化（BadgerDB）
- [ ] IBus D-Bus 完整适配
- [ ] Fcitx5 适配

### Phase 3（规划中）
- [ ] Windows TSF 适配（C++ 薄壳 + gRPC）
- [ ] macOS IMK 适配（Swift 薄壳 + gRPC）
- [ ] 配置文件（兼容 Rime YAML）
- [ ] 云端词库同步
- [ ] 主题 / 候选窗 UI

## 二次开发指南

### 添加新词典
1. 准备文本文件，每行 `汉字 拼音 词频`，例如：
   ```
   你好 nihao 1000
   世界 shijie 800
   ```
2. 用 `-dict` 参数加载，或放到 `internal/dict/data/` 重新编译

### 调整容错规则
编辑 `internal/fuzzy/fuzzy.go` 中的 `DefaultPairs`：
```go
var DefaultPairs = []FuzzyPair{
    {"z", "zh"},
    {"c", "ch"},
    // 添加你的规则
}
```

### 调整排序权重
编辑 `internal/engine/engine.go` 中的 `DefaultConfig`：
```go
return Config{
    WPinyinMatch: 100.0,  // 精确匹配权重
    WFreq:        1.0,    // 词频权重
    WUserFreq:    50.0,   // 用户频次权重
    WFuzzy:       0.7,    // 模糊音折扣
    WTypo:        0.5,    // 拼写错误折扣
}
```

## 开源依赖

- 结巴词典：https://github.com/fxsjy/jieba (MIT)
- 拼音转换参考：pypinyin（仅词库生成阶段使用，运行时无 Python 依赖）

## 许可证

MIT

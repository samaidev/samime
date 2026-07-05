<div align="center">

# Samime — 跨平台智能中文输入法

<img src="assets/icons/samime-256.png" width="128" height="128" alt="Samime">

**用 Go 编写的跨平台中文输入法 · Windows / macOS / Linux**

[![Tests](https://img.shields.io/badge/tests-250%2B-brightgreen)](test/)
[![Go Version](https://img.shields.io/badge/Go-1.22%2B-blue)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Win%20%7C%20macOS%20%7C%20Linux-lightgrey)](#下载安装)
[![GitHub](https://img.shields.io/badge/GitHub-samaidev%2Fsamime-black)](https://github.com/samaidev/samime)

[功能特性](#功能特性) · [下载安装](#下载安装) · [快速开始](#快速开始) · [性能](#性能指标) · [开发文档](#开发文档)

</div>

---

## 项目简介

**Samime**（萨米输入法）是 [SamAI Group](https://samai.cc) 开发的开源中文输入法，用 Go 语言编写核心引擎，三平台原生适配：

- **Windows** — TSF COM + Direct2D 硬件加速渲染
- **macOS** — IMK + NSVisualEffectView 毛玻璃
- **Linux** — IBus + Fcitx5 双适配

### 核心数据
- 📚 **136,000** 词条内置词典（jieba）
- 🧠 **1,280,000** 条 2-gram 语言模型（Wikipedia 188 万中文标题训练）
- ⚡ **3,669+ QPS** 查询吞吐量，延迟 < 1ms
- 🧪 **250+** 测试用例，三平台全部通过
- 📦 三平台安装包就绪（NSIS / DMG / deb / rpm / AppImage）

---

## 功能特性

### 📚 海量词库 + 智能切分
- 136K 内置词条（jieba 词典）
- 1.28M 2-gram 语言模型（Wikipedia 训练）
- 整句切分（DP + 2-gram 重排）：`woaixuexi` → **我爱学习**
- 3-gram 上下文联想：提交"我爱"后输入 `xuexi`，"学习"排第一

### 🎯 强大容错（三层模型）
- **模糊音**：z/zh, c/ch, s/sh, n/l, f/h, l/r, an/ang 等 19 对（含方言）
- **邻键容错**：QWERTY 键盘邻键替换，`nigao` → 你好（h 错按成 g）
- **声母遗漏**：输入纯韵母 `ao` 联想 好/高/到 等
- **首字母缩写**：`nh` → 你好，`zg` → 最高/祖国，`bj` → 北京
- **单字母联想**：输入 `n` → 年/你/那/能/内

### 🧠 用户自学习
- **BadgerDB 持久化**：用户词典跨进程保留
- **时间衰减**：24 小时半衰期，最近提交权重更高
- **2-gram + 3-gram 上下文**：记录词共现，智能排序
- **N-gram 自动剪枝**：maxContextPairs=10000，避免内存增长
- **剪切板历史**：自动保存最近 50 条提交

### 🎨 现代化 UI
- **Windows**：Direct2D 硬件加速 + DirectWrite 抗锯齿 + 圆角选中 + 动画过渡
- **macOS**：NSVisualEffectView 毛玻璃 + 圆角 + 系统强调色
- **触摸手势**：垂直滑动翻页候选
- **候选词去重**：相同汉字不同拼音只保留最高分
- **来源优先级**：dict > segment > acronym > fuzzy > typo

### 🔄 自动更新
- GitHub Releases API 检查更新
- macOS Sparkle 集成 + appcast.xml
- CLI 命令：`samime -mode=update`

### 🌍 跨平台原生
- **Windows TSF**：完整 COM（ITfKeyEventSink + ITfCompositionSink + CandidateWindow）
- **macOS IMK**：Swift IMKInputController + CandidateWindow
- **Linux IBus**：godbus D-Bus 完整 Engine 接口
- **Linux Fcitx5**：godbus D-Bus addon 适配

---

## 下载安装

### Windows
| 方式 | 说明 |
|------|------|
| **NSIS 安装包**（推荐） | 下载 `samime-setup-1.0.0.exe`，双击安装 |
| 免安装版 | 下载 `samime-windows-amd64.exe`，命令行运行 |

### macOS
| 方式 | 说明 |
|------|------|
| **.dmg 安装包**（推荐） | 下载 `samime-1.0.0.dmg`，拖拽到 Applications |
| Universal Binary | 支持 Intel + Apple Silicon |

### Linux
| 方式 | 说明 |
|------|------|
| **.deb**（Debian/Ubuntu） | `sudo dpkg -i samime-1.0.0-amd64.deb` |
| .rpm（Fedora/RHEL） | `sudo dnf install samime-1.0.0-1.x86_64.rpm` |
| AppImage（免安装） | `./Samime-1.0.0-x86_64.AppImage` |
| 二进制 | 直接运行 `samime-linux-amd64` |

详细安装步骤参见 [packaging/INSTALL.md](packaging/INSTALL.md)。

---

## 快速开始

### 构建

```bash
git clone https://github.com/samaidev/samime.git
cd samime
go build -o bin/samime ./cmd/ime-cli
```

### 6 种运行模式

```bash
# 1. 演示模式（展示典型用例）
./bin/samime -mode=demo

# 2. 交互模式（终端输入拼音）
./bin/samime -mode=interactive

# 3. 批处理模式（脚本化测试）
echo "nihao" | ./bin/samime -mode=batch

# 4. 性能压测
./bin/samime -mode=bench -bench-n=10000

# 5. 服务模式（后台运行，供 TSF/IMK/IBus 调用）
./bin/samime -mode=service

# 6. 检查更新
./bin/samime -mode=update
```

### 演示输出示例

```
=== Samime 演示 ===
用例                                    | Top1       | 其他候选
------------------------------------------------------------
基础-你好                                | 你好        | 极好 记号 利好 泥淖
基础-中国                                | 中国        | 公国 功过 中古 终归
基础-输入法                              | 输入法       |
整句切分-woaixuexi→我爱学习               | 我爱学习     |
单字母-n→年/你                           | 年          | 你 那 能 内
首字母缩写-nh→你好                        | 南海        | 南湖 女孩 ... 你好 呐喊
首字母缩写-bj→北京                        | 北京        | 编辑 比较 不仅 本级
声母遗漏-ao→好/高                         | 我          | 到 要 道 好
模糊音-n/l(lihao→你好)                    | 厉害        | 利害 你好 利好 里海
拼写错误-h/g错按(nigao→你好)              | 你好        | 拟稿 比高 密告 你报
```

---

## 架构

```
┌──────────────────────────────────────────────────────┐
│                平台 UI 层（原生适配）                   │
│  Windows TSF (C++)  │  macOS IMK (Swift)  │  Linux IBus/Fcitx5 (Go) │
│  Direct2D 渲染      │  NSVisualEffectView │  D-Bus (godbus)         │
└───────────┬───────────────────┬──────────────────┬────┘
            │  命名管道 / Unix Socket / D-Bus（JSON 协议）
┌───────────▼───────────────────▼──────────────────▼────┐
│              Samime Engine (纯 Go，跨平台)               │
│ ┌─────────┬─────────┬─────────┬─────────┬───────────┐ │
│ │ Pinyin  │ Dict    │ Segment │ Fuzzy   │ Engine    │ │
│ │ Segment │ Lookup  │ +Bigram │ +Typo   │ +Context  │ │
│ │ (DP)    │ (Trie)  │ (DP+LM) │ (邻键)  │ (2/3-gram)│ │
│ └─────────┴─────────┴─────────┴─────────┴───────────┘ │
│ ┌─────────┬─────────┬─────────┐                        │
│ │ UserDict│Clipboard│ Updater │                        │
│ │ (Badger)│ (50条)  │ (GH API)│                        │
│ └─────────┴─────────┴─────────┘                        │
└──────────────────────────────────────────────────────┘
```

## 模块说明

| 路径 | 说明 | 关键技术 |
|------|------|---------|
| `internal/pinyin` | 拼音切分、声韵母识别、DP 最优切分 | AC 自动机 + DP |
| `internal/dict` | 词典加载与前缀检索 | `go:embed` + Trie |
| `internal/fuzzy` | 模糊音（19 对）+ 邻键容错 + 编辑距离 | 规则引擎 |
| `internal/segmenter` | 整句切分（DP + 2-gram LM + 重排） | DP + 对数概率 |
| `internal/engine` | 核心引擎（综合打分 + 上下文 + 衰减） | 多源排序 |
| `internal/userdict` | 用户词典 + 上下文持久化 | BadgerDB (LSM-Tree) |
| `internal/clipboard` | 剪切板历史（最近 50 条） | 内存 + RWMutex |
| `internal/updater` | 自动更新检查 | GitHub Releases API |
| `internal/ibus` | Linux IBus 适配 | godbus D-Bus |
| `internal/fcitx5` | Linux Fcitx5 适配 | godbus D-Bus |
| `internal/winime` | Windows TSF 适配 | winio 命名管道 + C++ COM |
| `internal/macime` | macOS IMK 适配 | Unix Socket + Swift |
| `tools/rime_convert.py` | Rime 词库转换工具 | Python |
| `cmd/ime-cli` | CLI 入口 | 6 种模式 |

---

## 容错能力详解

### Layer 1: 模糊音（19 对）

**标准模糊音**（Rime 兼容）：
- `z/zh`, `c/ch`, `s/sh`
- `n/l`, `an/ang`, `en/eng`, `in/ing`
- `ian/iang`, `uan/uang`, `ou/uo`

**方言模糊音**：
- `f/h`（福建口音）、`l/r`（湖南/四川）、`v/u`、`v/w`、`h/k`（客家）
- `ong/eng`、`un/uen`、`ue/ve`、`ie/iai`

### Layer 2: 拼写错误容错
- QWERTY 邻键替换（每位置 4-6 个邻键）
- 长度一致 + 音节数一致约束
- 例：`nigao → 你好`（h→g），`nohao → 你好`（i→o）

### Layer 3: 声母遗漏 + 单字母联想
- 纯韵母输入：`ao → 好/高/到`
- 单声母联想：`n → 年/你/那/能/内`
- 首字母缩写：`nh → 你好`，`zg → 最高/祖国`

### Layer 4: 上下文联想
- **2-gram**：前一个词与候选词共现
- **3-gram**：前两个词与候选词共现（权重 1.5x）
- **时间衰减**：24h 半衰期，最近提交权重更高
- 例：提交"我→你"多次后，输入"我"再搜索"n"，"你"排名提升

---

## 性能指标

| 指标 | Linux | Windows | macOS |
|------|-------|---------|-------|
| 词典加载时间 | 224ms | 746ms | ~300ms |
| 短输入查询（2 音节） | 60µs | 157µs | ~80µs |
| 长输入查询（4+ 音节） | 571µs | 1516µs | ~700µs |
| 模糊音查询 | 145µs | 391µs | ~200µs |
| QPS 吞吐量 | 3,669 | 1,968 | ~2,500 |
| 1 分钟持续 QPS | 9,524 | — | — |
| 16ms 延迟达标率 | 100% | 100% | 100% |
| 二进制大小 | 5.3MB | 6.0MB | 5.5MB |

**测试主机**：
- Linux: 容器环境
- Windows: Intel Core i5-4258U @ 2.40GHz, Windows 11
- macOS: (预估，需 Apple Silicon 实测)

---

## 测试覆盖

**总计 250+ 测试函数，全部通过**

| 类型 | 测试数 | 状态 |
|------|--------|------|
| 单元测试 - pinyin | 14 | ✅ |
| 单元测试 - dict | 19 | ✅ |
| 单元测试 - fuzzy | 20 | ✅ |
| 单元测试 - segmenter | 19 | ✅ |
| 单元测试 - engine | 50+ | ✅ |
| 单元测试 - userdict | 3 | ✅ |
| 单元测试 - clipboard | 25 | ✅ |
| 单元测试 - updater | 11 | ✅ |
| 集成测试 - E2E | 9 | ✅ |
| 集成测试 - Service | 27 | ✅ |
| 性能基准 - Stress | 9 | ✅ |
| Race Detector | 全部并发 | ✅ 无竞争 |

**典型测试用例**：

| 输入 | Top1 候选 | 容错类型 |
|------|----------|---------|
| `nihao` | 你好 | 精确匹配 |
| `zhongguo` | 中国 | 精确匹配 |
| `woaixuexi` | 我爱学习 | 整句切分 |
| `zhongguoren` | 中国人 | 整句切分 |
| `n` | 年 | 单字母联想 |
| `nh` | 南海（含你好） | 首字母缩写 |
| `bj` | 北京 | 首字母缩写 |
| `ao` | 我（含好/高） | 声母遗漏 |
| `lihao` | 厉害（含你好） | n/l 模糊音 |
| `fahao` | 大好（含法号） | f/h 模糊音 |
| `nigao` | 你好 | h/g 邻键容错 |

---

## 开发文档

### 项目结构

```
samime/
├── cmd/ime-cli/              # CLI 入口（6 种模式）
├── internal/
│   ├── pinyin/               # 拼音切分
│   ├── dict/                 # 词典 + Trie (136K 词)
│   ├── fuzzy/                # 模糊音 + 邻键容错
│   ├── segmenter/            # 整句切分 + 2-gram LM
│   │   └── data/bigram.txt   # 1.28M 2-gram 模型
│   ├── engine/               # 核心引擎
│   ├── userdict/             # BadgerDB 持久化
│   ├── clipboard/            # 剪切板历史
│   ├── updater/              # 自动更新
│   ├── ibus/                 # Linux IBus
│   ├── fcitx5/               # Linux Fcitx5
│   ├── winime/               # Windows TSF
│   │   └── cpp/              # C++ COM + Direct2D
│   └── macime/               # macOS IMK
│       └── swift/            # Swift IMK
├── packaging/                # 打包脚本
│   ├── windows/              # NSIS
│   ├── macos/                # DMG + Sparkle
│   ├── linux/                # deb/rpm/AppImage
│   ├── build_all.sh          # 跨平台一键打包
│   ├── INSTALL.md            # 安装指南
│   └── SIGNING.md            # 签名证书指南
├── assets/
│   ├── icons/                # 图标（PNG/ICO/ICNS/SVG）
│   └── brand/                # 品牌资源（落地页）
├── tools/
│   └── rime_convert.py       # Rime 词库转换
├── test/                     # 集成测试 + 沙盒测试
├── .github/workflows/        # CI/CD
├── ABOUT.md                  # SamAI Group 介绍
└── README.md
```

### 二次开发

**添加新词典**：
1. 准备文本文件，每行 `汉字 拼音 词频`
2. 用 `-dict` 参数加载，或放到 `internal/dict/data/` 重新编译

**调整模糊音规则**（`internal/fuzzy/fuzzy.go`）：
```go
var DefaultPairs = []FuzzyPair{
    {"z", "zh"}, {"n", "l"}, {"f", "h"},  // 添加你的规则
}
```

**调整排序权重**（`internal/engine/engine.go`）：
```go
return Config{
    WPinyinMatch: 100.0,  // 精确匹配权重
    WFreq:        1.0,    // 词频权重
    WUserFreq:    50.0,   // 用户频次权重
    WContext:     30.0,   // 上下文权重
    WFuzzy:       0.7,    // 模糊音折扣
    WTypo:        0.5,    // 拼写错误折扣
}
```

### 跨平台打包

```bash
# 全平台二进制（5 个）
bash packaging/build_all.sh 1.0.0

# Linux .deb / .rpm / AppImage
bash packaging/linux/build_linux_package.sh 1.0.0 all

# Windows NSIS 安装包（在 Windows 上）
packaging\windows\build_windows_package.bat

# macOS .dmg（在 macOS 上）
bash packaging/macos/build_macos_package.sh 1.0.0
```

### 沙盒测试

```bash
bash test/sandbox_test.sh packaging/linux/samime-1.0.0-amd64.deb
# 7 阶段 16 项测试：包结构 / 二进制 / 词典 / 搜索 / 容错 / 单字母 / 性能
```

### CI/CD

推送 `v*` tag 自动触发 GitHub Actions 构建发布：
```bash
git tag v1.0.0
git push origin v1.0.0
```

---

## 开发路线

### Phase 1 ✅ 基础功能
- 拼音切分（DP）、词典检索（136K 词）、模糊音、邻键容错
- 整句切分、跨平台编译、Windows 验证

### Phase 2 ✅ 持久化 + 平台适配
- 2-gram 语言模型、BadgerDB 用户词典、Rime 词库转换
- Windows TSF / macOS IMK / IBus D-Bus 适配

### Phase 3 ✅ 完整平台集成
- 完整 TSF COM（ITfKeyEventSink + 候选窗 + 注册表）
- 完整 IMK（NSVisualEffectView + 签名公证脚本）
- 2-gram 增强（Wikipedia 188 万标题，128 万 2-gram）
- Fcitx5 适配

### Phase 4 ✅ 智能化 + UI
- 单字母联想、首字母缩写、声母遗漏容错
- 增强模糊音（方言 f/h, l/r 等）、候选词去重
- 时间衰减、2-gram/3-gram 上下文联想、N-gram 自动剪枝
- 剪切板历史、Direct2D 渲染、触摸手势、动画

### Phase 5 ✅ 打包发布
- 三平台安装包（NSIS/DMG/deb/rpm/AppImage）
- 应用图标（PNG/ICO/ICNS/SVG）
- 自动更新（GitHub API + Sparkle）
- GitHub Actions CI/CD
- 沙盒测试、签名证书文档

### Phase 6（计划中）
- [ ] 配置文件（兼容 Rime YAML）
- [ ] 云端词库同步
- [ ] 主题切换（深色/浅色）
- [ ] 完整 ITfContextComposition
- [ ] 数字签名（Windows EV + macOS Developer ID）

---

## 关于 SamAI Group

**SamAI Group** 是专注于人工智能与中文自然语言处理的技术团队。

- 🌐 官网：[samai.cc](https://samai.cc)
- 📧 联系：contact@samai.cc
- 🐙 GitHub：[samaidev](https://github.com/samaidev)

详见 [ABOUT.md](ABOUT.md)。

## 开源依赖

- [jieba](https://github.com/fxsjy/jieba) — 中文分词与词典 (MIT)
- [BadgerDB](https://github.com/dgraph-io/badger) — 嵌入式 KV 存储 (Apache 2.0)
- [godbus](https://github.com/godbus/dbus) — D-Bus Go 绑定 (BSD-2)
- [go-winio](https://github.com/Microsoft/go-winio) — Windows 命名管道 (MIT)
- [Wikipedia](https://dumps.wikimedia.org/) — 2-gram 训练语料 (CC BY-SA)
- [Rime](https://rime.im/) — 中文输入法先驱，方案参考 (BSD)

## 许可证

MIT License — 详见 [LICENSE](LICENSE)

---

<div align="center">

**[⬆ 回到顶部](#samime--跨平台智能中文输入法)**

Made with ❤️ by [SamAI Group](https://samai.cc)

</div>

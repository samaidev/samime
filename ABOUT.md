# SamAI Group

> **samai.cc** — 人工智能驱动的中文输入解决方案

---

## 关于我们

**SamAI Group** 是一家专注于人工智能与中文自然语言处理的技术团队。我们的使命是**让中文输入更智能、更高效、更自然**。

Samime（萨米输入法）是我们的开源旗舰项目——一个用 Go 语言编写的跨平台中文输入法，覆盖 Windows、macOS 和 Linux 三大平台。

---

## 核心项目

### Samime 中文输入法
- **仓库**: https://github.com/samaidev/samime
- **官网**: https://samai.cc
- **协议**: MIT 开源
- **特性**:
  - 136,000 词条内置词典（jieba）
  - 1,280,000 条 2-gram 语言模型（Wikipedia 训练）
  - 整句切分 + 模糊音 + 拼写容错
  - 用户词典持久化（BadgerDB）
  - 2-gram/3-gram 上下文联想
  - 时间衰减频次模型
  - 剪切板历史（最近 50 条）
  - Direct2D 硬件加速渲染
  - 触摸手势支持
  - 三平台原生适配（TSF/IMK/IBus/Fcitx5）

### 技术栈
- **核心引擎**: Go 1.22+（跨平台编译）
- **Windows TSF**: C++ + Direct2D + DirectWrite
- **macOS IMK**: Swift + NSVisualEffectView
- **Linux IBus**: Go + D-Bus (godbus)
- **存储**: BadgerDB（嵌入式 LSM-Tree）
- **CI/CD**: GitHub Actions

---

## 团队

| 角色 | 职责 | 联系 |
|------|------|------|
| 核心开发 | Go 引擎 + 跨平台架构 | dev@samai.cc |
| Windows 开发 | TSF COM + Direct2D | windows@samai.cc |
| macOS 开发 | IMK + Swift | macos@samai.cc |
| Linux 开发 | IBus + Fcitx5 | linux@samai.cc |
| NLP 算法 | 2-gram 模型 + 整句切分 | nlp@samai.cc |
| 产品/设计 | UI/UX + 图标设计 | design@samai.cc |

---

## 联系方式

- **官网**: https://samai.cc
- **GitHub**: https://github.com/samaidev
- **邮箱**: contact@samai.cc
- **问题反馈**: https://github.com/samaidev/samime/issues

---

## 开源贡献

Samime 是 MIT 开源项目，欢迎社区贡献：

1. **Fork** 仓库
2. 创建特性分支: `git checkout -b feature/amazing-feature`
3. 提交代码: `git commit -m 'Add amazing feature'`
4. 推送: `git push origin feature/amazing-feature`
5. 发起 **Pull Request**

### 贡献指南

- 代码需通过所有测试: `go test ./...`
- 新功能需附带测试用例
- 遵循现有代码风格
- 提交信息用英文，描述清晰

---

## 许可证

Samime 使用 **MIT License** 开源，可自由用于商业和非商业项目。

---

## 致谢

- [jieba](https://github.com/fxsjy/jieba) - 中文分词与词典
- [BadgerDB](https://github.com/dgraph-io/badger) - 嵌入式 KV 存储
- [godbus](https://github.com/godbus/dbus) - D-Bus Go 绑定
- [go-winio](https://github.com/Microsoft/go-winio) - Windows 命名管道
- [Wikipedia](https://dumps.wikimedia.org/) - 2-gram 训练语料
- [Rime](https://rime.im/) - 中文输入法先驱，方案参考

---

© 2024-2026 SamAI Group. All rights reserved.

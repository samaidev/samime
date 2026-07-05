// ime-cli: 命令行测试入口
// 用于在不依赖 IBus 的情况下，全流程测试输入法引擎
package main

import (
        "bufio"
        "flag"
        "fmt"
        "os"
        "strings"
        "time"

        "github.com/zai/goime/internal/dict"
        "github.com/zai/goime/internal/engine"
        "github.com/zai/goime/internal/ibus"
)

func main() {
        mode := flag.String("mode", "interactive", "运行模式: interactive | batch | bench | demo | service")
        dictPath := flag.String("dict", "", "额外加载词典文件路径")
        benchN := flag.Int("bench-n", 1000, "bench 模式查询次数")
        userDictPath := flag.String("user-dict", "", "用户词典持久化路径（空=不持久化）")
        flag.Parse()

        // 加载词典
        t0 := time.Now()
        d, err := dict.LoadEmbedded()
        if err != nil {
                fmt.Fprintf(os.Stderr, "load embedded dict failed: %v\n", err)
                os.Exit(1)
        }
        if *dictPath != "" {
                if _, err := dict.LoadFromFile(*dictPath); err != nil {
                        fmt.Fprintf(os.Stderr, "load extra dict failed: %v\n", err)
                }
        }
        loadDur := time.Since(t0)
        s := d.Stats()
        fmt.Fprintf(os.Stderr, "[goime] 词典加载: %d 条 | %d 个不同拼音 | 用时 %v\n",
                s.TotalEntries, s.UniquePinyin, loadDur)

        // 创建引擎（可选持久化）
        var eng *engine.Engine
        if *userDictPath != "" || *mode == "service" {
                path := *userDictPath
                if path == "" {
                        // 默认路径
                        home, _ := os.UserHomeDir()
                        path = home + "/.samime/userdict"
                }
                e, err := engine.NewWithUserStore(d, path)
                if err != nil {
                        fmt.Fprintf(os.Stderr, "[goime] 持久化用户词典初始化失败，降级到内存模式: %v\n", err)
                        eng = engine.NewDefault(d)
                } else {
                        eng = e
                        fmt.Fprintf(os.Stderr, "[goime] 用户词典持久化: %s\n", path)
                        defer eng.Close()
                }
        } else {
                eng = engine.NewDefault(d)
        }

        switch *mode {
        case "interactive":
                runInteractive(eng)
        case "batch":
                runBatch(eng)
        case "bench":
                runBench(eng, *benchN)
        case "demo":
                runDemo(eng)
        case "service":
                runService(eng)
        default:
                fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
                os.Exit(1)
        }
}

// runInteractive 交互模式
func runInteractive(eng *engine.Engine) {
        ibusEng := ibus.New(eng)
        ibusEng.OnPreeditChanged = func(text string) {
                fmt.Printf("\r[预编辑] %s | 候选: ", text)
                cands := ibusEng.Candidates()
                for i, c := range cands {
                        if i >= 9 {
                                break
                        }
                        fmt.Printf("%d.%s ", i+1, c.Word)
                }
                fmt.Print("        \r") // 清行尾
                fmt.Printf("\r[预编辑] %s | 候选: ", text)
                for i, c := range cands {
                        if i >= 9 {
                                break
                        }
                        fmt.Printf("%d.%s ", i+1, c.Word)
                }
        }
        ibusEng.OnCommitted = func(text string) {
                fmt.Printf("\r[提交] %s\n", text)
        }

        fmt.Println("=== GoIME 交互模式 ===")
        fmt.Println("输入拼音 + Enter 选第一个候选；输入 1-9 选对应候选；ESC 清空；Ctrl+C 退出")
        fmt.Println()

        reader := bufio.NewReader(os.Stdin)
        for {
                fmt.Print("> ")
                line, err := reader.ReadString('\n')
                if err != nil {
                        break
                }
                line = strings.TrimRight(line, "\n\r")

                if line == "quit" || line == "exit" {
                        break
                }

                if line == "" {
                        continue
                }

                // 数字：选候选
                if len(line) == 1 && line[0] >= '1' && line[0] <= '9' {
                        ibusEng.ProcessKey(line)
                        continue
                }

                // 把整行作为按键流处理
                for _, c := range line {
                        ibusEng.ProcessKey(string(c))
                }
                // 回车提交
                ibusEng.ProcessKey("Return")
        }
        fmt.Println("\n[goime] 已退出")
}

// runBatch 批量模式：从 stdin 读取多行拼音，输出候选
func runBatch(eng *engine.Engine) {
        scanner := bufio.NewScanner(os.Stdin)
        for scanner.Scan() {
                py := strings.TrimSpace(scanner.Text())
                if py == "" {
                        continue
                }
                cands := eng.Search(py)
                fmt.Printf("%s\t", py)
                for i, c := range cands {
                        if i >= 10 {
                                break
                        }
                        if i > 0 {
                                fmt.Print(" ")
                        }
                        fmt.Printf("%s", c.Word)
                }
                fmt.Println()
        }
}

// runBench 性能测试
func runBench(eng *engine.Engine, n int) {
        queries := []string{
                "nihao", "zhongguo", "shurufa", "pinyin", "xuesheng",
                "laoshi", "diannao", "shouji", "pengyou", "gongzuo",
                "nihaozhongguo", "wome", "shijie", "jiandan", "yixia",
        }
        t0 := time.Now()
        for i := 0; i < n; i++ {
                q := queries[i%len(queries)]
                _ = eng.Search(q)
        }
        dur := time.Since(t0)
        avg := dur / time.Duration(n)
        fmt.Printf("[bench] %d 次查询 | 总耗时 %v | 平均 %v/次\n", n, dur, avg)
}

// runDemo 演示模式：跑一组典型用例
func runDemo(eng *engine.Engine) {
        cases := []struct {
                name string
                py   string
        }{
                {"基础-你好", "nihao"},
                {"基础-中国", "zhongguo"},
                {"基础-输入法", "shurufa"},
                {"长词-人工智能", "rengongzhineng"},
                {"长词-机器学习", "jiqixuexi"},
                {"整句切分-woaixuexi→我爱学习", "woaixuexi"},
                {"整句切分-zhongguoren→中国人", "zhongguoren"},
                {"模糊音-n/l(lihao→你好)", "lihao"},
                {"模糊音-zh/z(zongguo→中国)", "zongguo"},
                {"拼写错误-h/g错按(nigao→你好)", "nigao"},
                {"拼写错误-i/o错按(nohao→你好)", "nohao"},
                {"短输入-ni", "ni"},
        }

        fmt.Println("=== GoIME 演示 ===")
        fmt.Printf("%-40s | %-10s | %s\n", "用例", "Top1", "其他候选 (Top 5)")
        fmt.Println(strings.Repeat("-", 100))

        for _, c := range cases {
                cands := eng.Search(c.py)
                top1 := "-"
                others := ""
                if len(cands) > 0 {
                        top1 = cands[0].Word
                        for i := 1; i < len(cands) && i < 5; i++ {
                                if i > 1 {
                                        others += " "
                                }
                                others += cands[i].Word
                        }
                }
                fmt.Printf("%-40s | %-10s | %s\n", c.name, top1, others)
        }
}

// runService 服务模式：作为后台服务运行（Windows 命名管道 / Linux TCP）
// 接收来自 TSF proxy 或 IBus 的请求
func runService(eng *engine.Engine) {
        fmt.Fprintln(os.Stderr, "[goime] Service mode: 等待 TSF/IBus proxy 连接...")
        // Windows 上启动命名管道服务（见 internal/winime/pipe_windows.go）
        // Linux/macOS 上启动 TCP 服务（开发用）
        runServicePlatform(eng)
}

// 远端延迟测试：精确测量 Search 延迟
package main

import (
	"fmt"
	"sort"
	"time"

	"github.com/zai/goime/internal/dict"
	"github.com/zai/goime/internal/engine"
)

func main() {
	d, err := dict.LoadEmbedded()
	if err != nil {
		fmt.Printf("load dict failed: %v\n", err)
		return
	}
	eng := engine.NewDefault(d)

	cases := []string{
		"nihao", "shurufa", "woaixuexi", "woyaochif", "woyaochifan",
		"rengongzhin", "rengongzhineng", "zhongguoren",
	}

	const runs = 50
	fmt.Printf("%-18s | %8s | %8s | %8s | %s\n", "input", "p50_us", "p90_us", "p99_us", "top1")
	fmt.Println("--------------------------------------------------------")

	for _, inp := range cases {
		eng.Search(inp) // warmup
		var times []time.Duration
		top1 := ""
		for i := 0; i < runs; i++ {
			t1 := time.Now()
			cands := eng.Search(inp)
			times = append(times, time.Since(t1))
			if top1 == "" && len(cands) > 0 {
				top1 = cands[0].Word
			}
		}
		sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
		fmt.Printf("%-18s | %8.0f | %8.0f | %8.0f | %s\n",
			inp,
			float64(times[runs*50/100].Nanoseconds())/1000,
			float64(times[runs*90/100].Nanoseconds())/1000,
			float64(times[runs*99/100].Nanoseconds())/1000,
			top1)
	}
}

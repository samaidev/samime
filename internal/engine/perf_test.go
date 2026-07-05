package engine

import (
	"fmt"
	"testing"
	"time"

	"github.com/zai/goime/internal/dict"
)

func TestSearchPerf(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	eng := NewDefault(d)

	cases := []string{
		"n",       // 单声母
		"k",
		"kk",      // 双声母
		"nihao",   // 短词
		"shibushizheyang", // 长句
		"wohenxiangni",
		"jiniantianqi",
		"abcdefgh", // 无效输入
	}

	for _, input := range cases {
		// warmup
		eng.Search(input)
		
		t0 := time.Now()
		const N = 10
		for i := 0; i < N; i++ {
			eng.Search(input)
		}
		dur := time.Since(t0) / N
		cands := eng.Search(input)
		fmt.Printf("Search(%q) = %d cands, avg %v\n", input, len(cands), dur)
	}
}

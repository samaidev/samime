package dict

import (
	"fmt"
	"testing"
)

func TestKKAcronym(t *testing.T) {
	d, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	entries := d.LookupByAcronym("kk")
	fmt.Printf("LookupByAcronym('kk') = %d entries:\n", len(entries))
	for i, e := range entries {
		fmt.Printf("  %d. %s (%s, %.1f)\n", i+1, e.Word, e.Pinyin, e.Freq)
	}

	// 直接查 kankan 在不在 byPinyin
	if ent := d.Lookup("kankan"); ent != nil {
		fmt.Printf("Lookup('kankan') = %d entries, first: %s %.1f\n", len(ent), ent[0].Word, ent[0].Freq)
		// 检查每个 kankan 词条的词长
		for _, e := range ent {
			fmt.Printf("  word=%q runelen=%d\n", e.Word, len([]rune(e.Word)))
		}
	}

	// 检查 acronymIndex 里有没有"看看"
	for _, e := range entries {
		if e.Word == "看看" {
			fmt.Printf("看看 FOUND in acronymIndex: %s %.1f\n", e.Pinyin, e.Freq)
		}
	}

	// 测试 pinyinToAcronym
	for _, py := range []string{"kankan", "nihao", "zhongguo"} {
		acr := pinyinToAcronym(py)
		fmt.Printf("pinyinToAcronym(%q) = %q\n", py, acr)
	}
}

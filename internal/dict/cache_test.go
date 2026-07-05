package dict

import (
	"fmt"
	"testing"
)

func TestInitialCache(t *testing.T) {
	d, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, ini := range []string{"n", "k", "w", "z"} {
		entries := d.LookupByInitial(ini)
		fmt.Printf("LookupByInitial(%q) = %d entries:", ini, len(entries))
		for i, e := range entries {
			if i >= 5 {
				break
			}
			fmt.Printf(" %s(%s,%.0f)", e.Word, e.Pinyin, e.Freq)
		}
		fmt.Println()
	}
	// 测试 acronymIndex
	for _, acr := range []string{"kk", "nh", "zg", "sh"} {
		entries := d.LookupByAcronym(acr)
		fmt.Printf("LookupByAcronym(%q) = %d entries:", acr, len(entries))
		for i, e := range entries {
			if i >= 5 {
				break
			}
			fmt.Printf(" %s(%s,%.0f)", e.Word, e.Pinyin, e.Freq)
		}
		fmt.Println()
	}
}

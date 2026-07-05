package engine

import (
	"fmt"
	"testing"

	"github.com/zai/goime/internal/dict"
	"github.com/zai/goime/internal/pinyin"
)

func TestDebugN(t *testing.T) {
	d, err := dict.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	eng := NewDefault(d)
	
	syls := pinyin.Segment("n")
	fmt.Printf("Segment('n') = %d syls: %v\n", len(syls), syls)
	
	cands := eng.Search("n")
	fmt.Printf("Search('n') = %d cands\n", len(cands))
	for i, c := range cands {
		if i >= 10 {
			break
		}
		fmt.Printf("  %d. %s (%s) score=%.1f source=%s\n", i+1, c.Word, c.Pinyin, c.Score, c.Source)
	}
}

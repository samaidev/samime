package dict

import (
	"fmt"
	"testing"
	"time"
)

func TestLoadTime(t *testing.T) {
	t0 := time.Now()
	d, err := LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("加载 %d 条，用时 %v\n", d.Size(), time.Since(t0))
}

package engine

import (
        "os"
        "testing"

        "github.com/zai/goime/internal/dict"
)

// BenchmarkSearchSingle 短输入
func BenchmarkSearchSingle(b *testing.B) {
        d, _ := dict.LoadEmbedded()
        e := NewDefault(d)
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
                _ = e.Search("ni")
        }
}

// BenchmarkSearchShort 双音节
func BenchmarkSearchShort(b *testing.B) {
        d, _ := dict.LoadEmbedded()
        e := NewDefault(d)
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
                _ = e.Search("nihao")
        }
}

// BenchmarkSearchLong 长输入
func BenchmarkSearchLong(b *testing.B) {
        d, _ := dict.LoadEmbedded()
        e := NewDefault(d)
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
                _ = e.Search("rengongzhineng")
        }
}

// BenchmarkSearchWithFuzzy 带模糊音
func BenchmarkSearchWithFuzzy(b *testing.B) {
        d, _ := dict.LoadEmbedded()
        e := NewDefault(d)
        b.ResetTimer()
        for i := 0; i < b.N; i++ {
                _ = e.Search("zongguo")
        }
}

// 防止编译器报未使用
var _ = os.Stdout

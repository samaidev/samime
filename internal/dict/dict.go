// Package dict 实现词典加载与前缀检索
package dict

import (
	"bufio"
	"embed"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed data/*.txt
var embeddedDict embed.FS

// Entry 词典条目
type Entry struct {
	Word   string  // 汉字
	Pinyin string  // 拼音串（无分隔符，如 "nihao"）
	Freq   float64 // 词频
}

// Dict 词典
type Dict struct {
	mu         sync.RWMutex
	byPinyin   map[string][]*Entry // 拼音索引
	prefixTrie *Trie               // 前缀 trie
	all        []*Entry            // 全部词条
	totalFreq  float64
}

// New 新建空词典
func New() *Dict {
	return &Dict{
		byPinyin:   make(map[string][]*Entry),
		prefixTrie: NewTrie(),
	}
}

// LoadEmbedded 加载内嵌词库
func LoadEmbedded() (*Dict, error) {
	d := New()
	entries, err := embeddedDict.ReadDir("data")
	if err != nil {
		return nil, fmt.Errorf("read embedded dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		f, err := embeddedDict.Open("data/" + e.Name())
		if err != nil {
			return nil, err
		}
		err = d.LoadFromReader(f, e.Name())
		f.Close()
		if err != nil {
			return nil, err
		}
	}
	return d, nil
}

// LoadFromFile 从文件加载
// 格式：每行 `word pinyin freq` 或 `word pinyin`（freq 默认 1）
func LoadFromFile(path string) (*Dict, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d := New()
	return d, d.LoadFromReader(f, path)
}

// LoadFromReader 从 io.Reader 加载
func (d *Dict) LoadFromReader(r io.Reader, source string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		word := parts[0]
		py := strings.ToLower(parts[1])
		freq := 1.0
		if len(parts) >= 3 {
			if v, err := strconv.ParseFloat(parts[2], 64); err == nil {
				freq = v
			}
		}
		d.addEntry(&Entry{Word: word, Pinyin: py, Freq: freq})
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", source, err)
	}
	return nil
}

// addEntry 添加词条
func (d *Dict) addEntry(e *Entry) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.byPinyin[e.Pinyin] = append(d.byPinyin[e.Pinyin], e)
	d.prefixTrie.Insert(e.Pinyin)
	d.all = append(d.all, e)
	d.totalFreq += e.Freq
}

// Lookup 精确拼音查找
func (d *Dict) Lookup(pinyin string) []*Entry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	entries := d.byPinyin[pinyin]
	if len(entries) == 0 {
		return nil
	}
	out := make([]*Entry, len(entries))
	copy(out, entries)
	sort.Slice(out, func(i, j int) bool {
		return out[i].Freq > out[j].Freq
	})
	return out
}

// LookupPrefix 前缀匹配，返回所有以 prefix 开头的拼音串
func (d *Dict) LookupPrefix(prefix string) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.prefixTrie.PrefixMatch(prefix)
}

// LookupMulti 多音节联合查找
func (d *Dict) LookupMulti(syllables []string) []*Entry {
	if len(syllables) == 0 {
		return nil
	}
	joined := strings.Join(syllables, "")
	if entries := d.Lookup(joined); len(entries) > 0 {
		return entries
	}
	return d.Lookup(syllables[0])
}

// Size 词条总数
func (d *Dict) Size() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.all)
}

// Stats 统计信息
type Stats struct {
	TotalEntries int
	UniquePinyin int
	TotalFreq    float64
}

func (d *Dict) Stats() Stats {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return Stats{
		TotalEntries: len(d.all),
		UniquePinyin: len(d.byPinyin),
		TotalFreq:    d.totalFreq,
	}
}

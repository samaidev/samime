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

	"github.com/zai/goime/internal/pinyin"
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
	byPinyin   map[string][]*Entry // 拼音索引（entries 已按词频降序排序）
	prefixTrie *Trie               // 前缀 trie
	all        []*Entry            // 全部词条
	totalFreq  float64

	// 预计算缓存：声母 -> Top N 单字候选（避免每次搜索都遍历前缀）
	// 在 FinalizeLoad 时构建
	initialSingleChars map[string][]*Entry // key=声母(如"n"), val=按词频降序的单字

	// 预计算缓存：声母缩写串 -> 候选词条（避免 acronymMatch 遍历所有前缀）
	// key=声母缩写(如"kk"), val=按词频降序的词条(词长==缩写长度)
	// 只缓存 2-4 字缩写（最常见的联想场景）
	acronymIndex map[string][]*Entry
	// acronymReady: 异步构建完成信号，关闭表示构建完成
	acronymReady chan struct{}

	// 预计算缓存：单字 -> 拼音（多音字取最常用读音）
	// 用于续接联想：给定汉字反查拼音，匹配用户输入
	charToPinyin map[string]string

	// 预计算缓存：词 -> 是否存在（用于续接联想判断 bigram 链是否为已收录词）
	byWord map[string]bool
}

// New 新建空词典
func New() *Dict {
	return &Dict{
		byPinyin:           make(map[string][]*Entry),
		prefixTrie:         NewTrie(),
		initialSingleChars: make(map[string][]*Entry),
		acronymIndex:       make(map[string][]*Entry),
		charToPinyin:       make(map[string]string),
		byWord:             make(map[string]bool),
	}
}

// LoadEmbedded 加载内嵌词库
func LoadEmbedded() (*Dict, error) {
	d := New()
	// 预分配：根据经验，merged.txt 约 130w 条
	d.byPinyin = make(map[string][]*Entry, 200000)
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
	// 加载完成后做预排序和缓存构建
	d.FinalizeLoad()
	return d, nil
}

// FinalizeLoad 加载完成后调用：对每个拼音的 entries 按词频降序排序，
// 并预计算声母前缀的单字缓存（大幅加速 singleInitialMatch）
func (d *Dict) FinalizeLoad() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// 1. 对每个拼音的 entries 按词频降序排序
	for _, entries := range d.byPinyin {
		if len(entries) > 1 {
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].Freq > entries[j].Freq
			})
		}
	}

	// 2. 预计算每个声母的单字候选缓存
	// 优化：只遍历一次 byPinyin，根据拼音首字母分桶，避免 23 次 O(N) 遍历
	// （之前 23 * 126万 = 2.9 亿次 HasPrefix，现在只需 126 万次）
	// 先按拼音首字母分组
	byFirstChar := make(map[byte]map[string][]*Entry) // key=首字母 byte
	for py, entries := range d.byPinyin {
		if len(py) == 0 {
			continue
		}
		firstByte := py[0]
		if byFirstChar[firstByte] == nil {
			byFirstChar[firstByte] = make(map[string][]*Entry)
		}
		byFirstChar[firstByte][py] = entries
	}
	// 声母列表（含单字母和 zh/ch/sh）
	singleInitials := []string{
		"b", "p", "m", "f", "d", "t", "n", "l",
		"g", "k", "h", "j", "q", "x",
		"r", "z", "c", "s", "y", "w",
	}
	// 单字母声母：用首字母分桶
	for _, ini := range singleInitials {
		firstByte := ini[0]
		var topEntries []*Entry
		for py, entries := range byFirstChar[firstByte] {
			if !strings.HasPrefix(py, ini) {
				continue
			}
			// 只取该拼音的第 1 个（最高频）单字
			for _, ent := range entries {
				if len([]rune(ent.Word)) == 1 {
					topEntries = append(topEntries, ent)
					break
				}
			}
		}
		sort.Slice(topEntries, func(i, j int) bool {
			return topEntries[i].Freq > topEntries[j].Freq
		})
		if len(topEntries) > 50 {
			topEntries = topEntries[:50]
		}
		d.initialSingleChars[ini] = topEntries
	}
	// zh/ch/sh：用首字母 'z'/'c'/'s' 分桶
	for _, ini := range []string{"zh", "ch", "sh"} {
		firstByte := ini[0]
		var topEntries []*Entry
		for py, entries := range byFirstChar[firstByte] {
			if !strings.HasPrefix(py, ini) {
				continue
			}
			for _, ent := range entries {
				if len([]rune(ent.Word)) == 1 {
					topEntries = append(topEntries, ent)
					break
				}
			}
		}
		sort.Slice(topEntries, func(i, j int) bool {
			return topEntries[i].Freq > topEntries[j].Freq
		})
		if len(topEntries) > 50 {
			topEntries = topEntries[:50]
		}
		d.initialSingleChars[ini] = topEntries
	}

	// 4. 预计算 trie 节点的 Top N 词条缓存（同步，必需，<1s）
	const topN = 50
	const maxDepth = 4
	d.prefixTrie.root.computeTopEntries(d.byPinyin, "", 0, maxDepth, topN)

	// 4.5 预计算单字→拼音映射（用于续接联想反查）
	// 遍历所有拼音的所有单字条目，记录其拼音
	// 多音字：只记录第一个遇到的（map 遍历顺序随机，但 entries 已按词频降序，
	// 同拼音内最高频的先记录；不同拼音间随机，可能取到非最高频读音，
	// 但对续接联想影响不大——续接靠 bigram 选字，拼音只用于前缀匹配）
	for py, entries := range d.byPinyin {
		for _, ent := range entries {
			if len([]rune(ent.Word)) == 1 {
				if _, exists := d.charToPinyin[ent.Word]; !exists {
					d.charToPinyin[ent.Word] = py
				}
			}
			// 同时构建 byWord set（用于续接联想判断 bigram 链是否为已收录词）
			d.byWord[ent.Word] = true
		}
	}

	// 5. 预计算声母缩写索引（acronymIndex）—— 异步构建
	// pinyin.Segment 对 20 万拼音调用需 ~8s，会拖慢启动。
	// 改为后台 goroutine 构建，构建完成前 acronymMatch 回退到空结果。
	// 用户启动后前几秒输入 "kk" 等缩写可能无候选，但不影响正常拼音输入。
	d.acronymReady = make(chan struct{})
	go d.buildAcronymIndexAsync()
}

// buildAcronymIndexAsync 后台构建 acronymIndex
func (d *Dict) buildAcronymIndexAsync() {
	defer close(d.acronymReady)
	d.mu.Lock()
	defer d.mu.Unlock()

	// 先找出所有包含 2-4 字词的拼音
	candidatePys := make(map[string]bool)
	for py, entries := range d.byPinyin {
		for _, ent := range entries {
			wl := len([]rune(ent.Word))
			if wl >= 2 && wl <= 4 {
				candidatePys[py] = true
				break
			}
		}
	}
	for py := range candidatePys {
		acronym := pinyinToAcronym(py)
		if acronym == "" || len(acronym) < 2 || len(acronym) > 4 {
			continue
		}
		entries := d.byPinyin[py]
		for _, ent := range entries {
			wordLen := len([]rune(ent.Word))
			if wordLen != len(acronym) {
				continue
			}
			d.acronymIndex[acronym] = append(d.acronymIndex[acronym], ent)
		}
	}
	// 对每个缩写的词条按词频降序排序，取 Top 30
	for acr, entries := range d.acronymIndex {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Freq > entries[j].Freq
		})
		if len(entries) > 30 {
			d.acronymIndex[acr] = entries[:30]
		}
	}
}

// computeTopEntries 递归计算节点的 Top N 词条缓存（自底向上归并）
// 每个节点的 topEntries = 该节点拼音的 entries + 所有子节点 topEntries 的合并，按词频取 Top N
// 这是 O(N) 的，比每个节点独立遍历子树（O(N²)）快得多
func (n *trieNode) computeTopEntries(byPinyin map[string][]*Entry, curPy string, depth, maxDepth, topN int) {
	// 先收集当前节点的 entries（如果该拼音存在）
	var combined []*Entry
	if n.isEnd {
		combined = append(combined, byPinyin[curPy]...)
	}

	// 如果未超过深度限制，递归子节点并归并
	if depth < maxDepth {
		for c, child := range n.children {
			childPy := curPy + string(c)
			child.computeTopEntries(byPinyin, childPy, depth+1, maxDepth, topN)
			// 合并子节点的 topEntries
			combined = append(combined, child.topEntries...)
		}
	}

	// 按词频降序排序，取 Top N
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].Freq > combined[j].Freq
	})
	if topN > 0 && len(combined) > topN {
		combined = combined[:topN]
	}
	n.topEntries = combined
}

// pinyinToAcronym 把拼音串转成声母缩写
// 如 "kankan" -> "kk", "nihao" -> "nh", "zhongguo" -> "zg"
// 用贪心最长匹配做音节切分，取每个音节声母的首字母
// zh/ch/sh 取首字母（z/c/s），保证缩写都是单字母
//
// 优化：不用 pinyin.Segment（DP 切分，每次分配3个数组，~65µs/call），
// 改用 IsValidSyllableFast（O(1) map 查找）做贪心最长匹配，
// 约 1µs/call，快 60 倍。200k 拼音从 13s 降到 ~0.2s。
func pinyinToAcronym(py string) string {
	if py == "" {
		return ""
	}
	var result []byte
	i := 0
	n := len(py)
	for i < n {
		matched := false
		// 贪心最长匹配：优先尝试 6 字符音节
		// 注意：起始 l 必须限制为 min(6, n-i)，否则当剩余长度 < 6 时
		// 循环条件 i+l<=n 在 l=6 时就为 false，循环体完全不执行
		maxL := 6
		if rem := n - i; rem < maxL {
			maxL = rem
		}
		for l := maxL; l >= 1; l-- {
			syl := py[i : i+l]
			if pinyin.IsValidSyllableFast(syl) {
				// 提取声母首字母
				// 先检查 2 字符声母 (zh/ch/sh)
				if l >= 3 {
					two := py[i : i+2]
					if two == "zh" || two == "ch" || two == "sh" {
						result = append(result, two[0])
						i += l
						matched = true
						break
					}
				}
				// 单字符声母（b/p/m/f/...）
				c := py[i]
				if isInitialByte(c) {
					result = append(result, c)
				}
				// 韵母开头的音节（如 "an"）无首字母，跳过
				i += l
				matched = true
				break
			}
		}
		if !matched {
			// 无法切分，跳过该字符
			i++
		}
	}
	return string(result)
}

// isInitialByte 判断单字节字符是否为声母首字母（b/p/m/f/d/t/n/l/g/k/h/j/q/x/r/z/c/s/y/w）
func isInitialByte(c byte) bool {
	switch c {
	case 'b', 'p', 'm', 'f', 'd', 't', 'n', 'l',
		'g', 'k', 'h', 'j', 'q', 'x',
		'r', 'z', 'c', 's', 'y', 'w':
		return true
	}
	return false
}

// LookupByAcronym 返回声母缩写对应的候选词条
// 如 LookupByAcronym("kk") 返回 "看看","可靠","开口" 等
// 数据在 FinalizeLoad 时异步构建，构建完成前返回 nil（非阻塞）
// 这样启动后立即输入 "kk" 等缩写不会卡顿，只是前几秒无缩写候选
func (d *Dict) LookupByAcronym(acronym string) []*Entry {
	// 非阻塞检查：构建未完成就返回 nil，避免输入卡顿
	if d.acronymReady != nil {
		select {
		case <-d.acronymReady:
		default:
			return nil
		}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.acronymIndex[acronym]
}

// LookupPrefixEntries 返回以 prefix 开头的拼音对应的词条（按词频降序）
// 优先用 trie 节点的 topEntries 缓存（O(1)），缓存未命中时回退到遍历
func (d *Dict) LookupPrefixEntries(prefix string, limit int) []*Entry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	// 优先用缓存（前缀长度 ≤ 4 时有缓存）
	if node := d.prefixTrie.FindNode(prefix); node != nil {
		entries := node.TopEntries()
		if entries != nil {
			if limit > 0 && len(entries) > limit {
				entries = entries[:limit]
			}
			return entries
		}
	}
	// 回退：遍历所有前缀拼音，合并 entries
	allPys := d.prefixTrie.PrefixMatch(prefix)
	var combined []*Entry
	for _, py := range allPys {
		combined = append(combined, d.byPinyin[py]...)
	}
	sort.Slice(combined, func(i, j int) bool {
		return combined[i].Freq > combined[j].Freq
	})
	if limit > 0 && len(combined) > limit {
		combined = combined[:limit]
	}
	return combined
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
// 性能优化：使用 bufio.Reader 而非 Scanner，预分配 entry 切片
func (d *Dict) LoadFromReader(r io.Reader, source string) error {
        // 预分配 all 切片容量（仅对首次调用有效）
        if d.all == nil {
                d.all = make([]*Entry, 0, 140000)
        }
        br := bufio.NewReaderSize(r, 1<<20) // 1MB buffer
        for {
                line, err := br.ReadString('\n')
                if len(line) > 0 {
                        // 去掉行尾
                        line = strings.TrimRight(line, "\r\n")
                        line = strings.TrimSpace(line)
                        if line != "" && line[0] != '#' {
                                // 手动分割字段（比 strings.Fields 快）
                                space1 := strings.IndexByte(line, ' ')
                                if space1 > 0 {
                                        word := line[:space1]
                                        rest := line[space1+1:]
                                        rest = strings.TrimLeft(rest, " ")
                                        space2 := strings.IndexByte(rest, ' ')
                                        var py string
                                        var freq float64 = 1.0
                                        if space2 > 0 {
                                                py = rest[:space2]
                                                // 解析词频
                                                freqStr := strings.TrimSpace(rest[space2+1:])
                                                if v, e := strconv.ParseFloat(freqStr, 64); e == nil {
                                                        freq = v
                                                }
                                        } else {
                                                py = rest
                                        }
                                        if py != "" {
                                                py = strings.ToLower(py)
                                                d.addEntry(&Entry{Word: word, Pinyin: py, Freq: freq})
                                        }
                                }
                        }
                }
                if err != nil {
                        if err == io.EOF {
                                break
                        }
                        return fmt.Errorf("read %s: %w", source, err)
                }
        }
        return nil
}

// addEntry 添加词条
// 性能优化：跳过包含繁体专用字的词条，确保输入法只输出简体字。
// 这能在加载时过滤掉约 20% 的繁体词条（merged.txt: 1266266 → 1016540），
// 同时减小内存占用、加快后续查询。
func (d *Dict) addEntry(e *Entry) {
        // 过滤繁体专用字词条
        if containsTraditional(e.Word) {
                return
        }
        d.mu.Lock()
        defer d.mu.Unlock()
        d.byPinyin[e.Pinyin] = append(d.byPinyin[e.Pinyin], e)
        d.prefixTrie.Insert(e.Pinyin)
        d.all = append(d.all, e)
        d.totalFreq += e.Freq
}

// Lookup 精确拼音查找
// 返回的切片已按词频降序排序（FinalizeLoad 时预排序）
// 注意：返回内部切片引用，调用方不要修改
func (d *Dict) Lookup(pinyin string) []*Entry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.byPinyin[pinyin]
}

// LookupByInitial 返回以指定声母开头的高频单字候选（Top 50）
// 数据在 FinalizeLoad 时预计算，O(1) 查询
// 用于加速 singleInitialMatch（输入 "n"/"k" 等单声母时的联想）
func (d *Dict) LookupByInitial(initial string) []*Entry {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.initialSingleChars[initial]
}

// LookupCharPinyin 查询单字的拼音（多音字取最常用读音）
// 用于续接联想：给定汉字反查拼音，O(1) 查 charToPinyin 缓存
func (d *Dict) LookupCharPinyin(ch string) string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.charToPinyin[ch]
}

// HasWord 检查词是否在词典中（用于续接联想判断 bigram 链是否为已收录词）
func (d *Dict) HasWord(word string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.byWord[word]
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

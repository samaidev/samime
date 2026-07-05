package dict

// Trie 拼音前缀 trie
type Trie struct {
        root *trieNode
}

type trieNode struct {
        children map[byte]*trieNode
        isEnd    bool
}

// NewTrie 新建 trie
func NewTrie() *Trie {
        return &Trie{root: newTrieNode()}
}

func newTrieNode() *trieNode {
        return &trieNode{children: make(map[byte]*trieNode)}
}

// Insert 插入拼音串
func (t *Trie) Insert(s string) {
        if s == "" {
                return
        }
        node := t.root
        for i := 0; i < len(s); i++ {
                c := s[i]
                child, ok := node.children[c]
                if !ok {
                        child = newTrieNode()
                        node.children[c] = child
                }
                node = child
        }
        node.isEnd = true
}

// PrefixMatch 返回所有以 prefix 开头的字符串
// 不限制数量，调用方需自行截断
func (t *Trie) PrefixMatch(prefix string) []string {
        if prefix == "" {
                return nil
        }
        node := t.root
        for i := 0; i < len(prefix); i++ {
                c := prefix[i]
                child, ok := node.children[c]
                if !ok {
                        return nil
                }
                node = child
        }
        var result []string
        t.dfs(node, prefix, &result)
        return result
}

func (t *Trie) dfs(node *trieNode, cur string, result *[]string) {
        if node.isEnd {
                *result = append(*result, cur)
        }
        for k := range node.children {
                t.dfs(node.children[k], cur+string(k), result)
        }
}

// Contains 是否包含
func (t *Trie) Contains(s string) bool {
        node := t.root
        for i := 0; i < len(s); i++ {
                c := s[i]
                child, ok := node.children[c]
                if !ok {
                        return false
                }
                node = child
        }
        return node.isEnd
}

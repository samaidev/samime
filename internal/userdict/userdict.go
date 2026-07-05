// Package userdict 用户词典持久化（BadgerDB）
//
// 存储用户提交的 (word, pinyin) -> freq 映射
// 跨进程保留用户输入习惯
package userdict

import (
        "fmt"
        "os"
        "path/filepath"
        "strconv"
        "strings"
        "sync"
        "time"

        "github.com/dgraph-io/badger/v4"
)

// Store 用户词典存储
type Store struct {
        db *badger.DB
        mu sync.RWMutex

        // 内存缓存（避免每次查询都读磁盘）
        cache map[string]float64
}

// New 创建或打开用户词典
// path 为空时使用默认路径 ~/.samime/userdict
func New(path string) (*Store, error) {
        if path == "" {
                home, err := os.UserHomeDir()
                if err != nil {
                        return nil, fmt.Errorf("get home dir: %w", err)
                }
                path = filepath.Join(home, ".samime", "userdict")
        }
        if err := os.MkdirAll(path, 0755); err != nil {
                return nil, fmt.Errorf("mkdir: %w", err)
        }
        opts := badger.DefaultOptions(path).
                WithLogger(nil).
                WithValueLogFileSize(16 << 20) // 16MB
        db, err := badger.Open(opts)
        if err != nil {
                return nil, fmt.Errorf("open badger: %w", err)
        }
        s := &Store{
                db:    db,
                cache: make(map[string]float64),
        }
        // 加载缓存
        if err := s.loadCache(); err != nil {
                // 不致命，继续
                fmt.Fprintf(os.Stderr, "[userdict] warn: load cache: %v\n", err)
        }
        // 启动 GC 协程
        go s.gcLoop()
        return s, nil
}

// makeKey 生成存储键
// 格式: "uf:word|pinyin"
func makeKey(word, py string) []byte {
        return []byte("uf:" + word + "|" + py)
}

// loadCache 加载所有用户频次到内存
func (s *Store) loadCache() error {
        return s.db.View(func(txn *badger.Txn) error {
                opts := badger.DefaultIteratorOptions
                opts.Prefix = []byte("uf:")
                it := txn.NewIterator(opts)
                defer it.Close()
                for it.Rewind(); it.Valid(); it.Next() {
                        item := it.Item()
                        err := item.Value(func(v []byte) error {
                                freq, err := strconv.ParseFloat(string(v), 64)
                                if err != nil {
                                        return nil // 跳过坏数据
                                }
                                key := string(item.Key())
                                // 去掉 "uf:" 前缀
                                key = strings.TrimPrefix(key, "uf:")
                                s.cache[key] = freq
                                return nil
                        })
                        if err != nil {
                                return err
                        }
                }
                return nil
        })
}

// Incr 增加用户频次（不存在则设为 1）
func (s *Store) Incr(word, py string) error {
        s.mu.Lock()
        defer s.mu.Unlock()

        key := word + "|" + py
        s.cache[key]++

        // 异步写入 Badger
        val := strconv.FormatFloat(s.cache[key], 'f', -1, 64)
        return s.db.Update(func(txn *badger.Txn) error {
                e := badger.NewEntry(makeKey(word, py), []byte(val))
                return txn.SetEntry(e)
        })
}

// Get 获取用户频次
func (s *Store) Get(word, py string) float64 {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return s.cache[word+"|"+py]
}

// All 返回所有用户频次（用于引擎初始化）
func (s *Store) All() map[string]float64 {
        s.mu.RLock()
        defer s.mu.RUnlock()
        out := make(map[string]float64, len(s.cache))
        for k, v := range s.cache {
                out[k] = v
        }
        return out
}

// LoadBatch 批量加载（用于引擎初始化）
func (s *Store) LoadBatch(m map[string]float64) {
        s.mu.Lock()
        defer s.mu.Unlock()
        for k, v := range m {
                s.cache[k] = v
        }
}

// Clear 清空用户词典
func (s *Store) Clear() error {
        s.mu.Lock()
        defer s.mu.Unlock()
        s.cache = make(map[string]float64)
        return s.db.Update(func(txn *badger.Txn) error {
                opts := badger.DefaultIteratorOptions
                opts.Prefix = []byte("uf:")
                it := txn.NewIterator(opts)
                defer it.Close()
                var keys [][]byte
                for it.Rewind(); it.Valid(); it.Next() {
                        keys = append(keys, it.Item().KeyCopy(nil))
                }
                for _, k := range keys {
                        if err := txn.Delete(k); err != nil {
                                return err
                        }
                }
                return nil
        })
}

// Close 关闭
func (s *Store) Close() error {
        return s.db.Close()
}

// gcLoop 定期 GC
func (s *Store) gcLoop() {
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
        again:
                err := s.db.RunValueLogGC(0.5)
                if err == nil {
                        goto again // 持续 GC 直到没有可回收
                }
        }
}

// Stats 统计
type Stats struct {
        Entries int
        Path    string
}

func (s *Store) Stats() Stats {
        s.mu.RLock()
        defer s.mu.RUnlock()
        return Stats{
                Entries: len(s.cache),
        }
}

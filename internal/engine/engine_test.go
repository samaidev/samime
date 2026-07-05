package engine

import (
        "testing"

        "github.com/zai/goime/internal/dict"
)

func loadTestDict() (*dict.Dict, error) {
        return dict.LoadEmbedded()
}

func TestSearchNihao(t *testing.T) {
        d, err := loadTestDict()
        if err != nil {
                t.Fatalf("load dict: %v", err)
        }
        e := NewDefault(d)
        cands := e.Search("nihao")
        if len(cands) == 0 {
                t.Fatal("no candidates for nihao")
        }
        t.Logf("nihao -> top 5:")
        for i, c := range cands {
                if i >= 5 {
                        break
                }
                t.Logf("  %d. %s (%s) score=%.2f source=%s", i+1, c.Word, c.Pinyin, c.Score, c.Source)
        }
        found := false
        for _, c := range cands {
                if c.Word == "你好" {
                        found = true
                        break
                }
        }
        if !found {
                t.Errorf("'你好' not in candidates")
        }
}

func TestSearchZhongguo(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        cands := e.Search("zhongguo")
        if len(cands) == 0 {
                t.Fatal("no candidates for zhongguo")
        }
        t.Logf("zhongguo -> top 5:")
        for i, c := range cands {
                if i >= 5 {
                        break
                }
                t.Logf("  %d. %s (%s) score=%.2f source=%s", i+1, c.Word, c.Pinyin, c.Score, c.Source)
        }
        found := false
        for _, c := range cands {
                if c.Word == "中国" {
                        found = true
                        break
                }
        }
        if !found {
                t.Errorf("'中国' not in candidates")
        }
}

func TestSearchFuzzyZL(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        cands := e.Search("lihao")
        t.Logf("lihao -> top 5:")
        for i, c := range cands {
                if i >= 5 {
                        break
                }
                t.Logf("  %d. %s (%s) score=%.2f source=%s", i+1, c.Word, c.Pinyin, c.Score, c.Source)
        }
}

func TestSearchTypo(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        cands := e.Search("nigao")
        t.Logf("nigao -> top 5:")
        for i, c := range cands {
                if i >= 5 {
                        break
                }
                t.Logf("  %d. %s (%s) score=%.2f source=%s", i+1, c.Word, c.Pinyin, c.Score, c.Source)
        }
}

func TestSearchShurufa(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        cands := e.Search("shurufa")
        if len(cands) == 0 {
                t.Fatal("no candidates for shurufa")
        }
        t.Logf("shurufa -> top 5:")
        for i, c := range cands {
                if i >= 5 {
                        break
                }
                t.Logf("  %d. %s (%s) score=%.2f source=%s", i+1, c.Word, c.Pinyin, c.Score, c.Source)
        }
}

func TestSearchWoAiXueXi(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        cands := e.Search("woaixuexi")
        if len(cands) == 0 {
                t.Fatal("no candidates for woaixuexi")
        }
        t.Logf("woaixuexi -> top 5:")
        for i, c := range cands {
                if i >= 5 {
                        break
                }
                t.Logf("  %d. %s (%s) score=%.2f source=%s", i+1, c.Word, c.Pinyin, c.Score, c.Source)
        }
        // Top1 应该是 "我爱学习"
        if cands[0].Word != "我爱学习" {
                t.Errorf("expected '我爱学习', got %q", cands[0].Word)
        }
}

func TestSearchZhongguoRen(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        cands := e.Search("zhongguoren")
        if len(cands) == 0 {
                t.Fatal("no candidates for zhongguoren")
        }
        t.Logf("zhongguoren -> top 5:")
        for i, c := range cands {
                if i >= 5 {
                        break
                }
                t.Logf("  %d. %s (%s) score=%.2f source=%s", i+1, c.Word, c.Pinyin, c.Score, c.Source)
        }
        // 应该能识别为 "中国人"
        found := false
        for _, c := range cands {
                if c.Word == "中国人" {
                        found = true
                        break
                }
        }
        if !found {
                t.Errorf("'中国人' not in candidates: %v", cands[:min(5, len(cands))])
        }
}

func TestCommitUpdatesUserFreq(t *testing.T) {
        d, _ := loadTestDict()
        e := NewDefault(d)
        e.Commit("你好", "nihao")
        uf := e.UserFreq()
        if uf["你好|nihao"] != 1 {
                t.Errorf("user freq not updated: %v", uf)
        }
        cands := e.Search("nihao")
        if len(cands) == 0 {
                t.Fatal("no candidates")
        }
        if cands[0].Word != "你好" {
                t.Errorf("你好 should be #1 after commit, got %s", cands[0].Word)
        }
}

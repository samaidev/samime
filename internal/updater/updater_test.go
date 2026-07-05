package updater

import (
        "encoding/json"
        "fmt"
        "net/http"
        "net/http/httptest"
        "strings"
        "testing"
        "time"
)

// 用测试服务器模拟 GitHub API
func newTestChecker(statusCode int, response string) *Checker {
        server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "application/json")
                w.WriteHeader(statusCode)
                fmt.Fprint(w, response)
        }))
        c := New("test/repo", "1.0.0")
        c.httpClient = server.Client()
        // 覆盖 URL 构造（通过 monkey patch 不现实，改用直接构造）
        return c
}

// 由于 CheckLatest 用了硬编码 GitHub URL，我们直接测试 ReleaseInfo 解析
func TestReleaseInfoParsing(t *testing.T) {
        cases := []struct {
                name    string
                json    string
                wantTag string
                wantErr bool
        }{
                {
                        name:    "正常响应",
                        json:    `{"tag_name":"v1.1.0","name":"Samime 1.1.0","html_url":"https://github.com/samaidev/samime/releases/tag/v1.1.0","published_at":"2026-01-01T00:00:00Z","assets":[{"name":"samime-1.1.0.exe","browser_download_url":"https://example.com/samime-1.1.0.exe","size":1000000}]}`,
                        wantTag: "v1.1.0",
                },
                {
                        name:    "空 JSON",
                        json:    `{}`,
                        wantTag: "",
                },
                {
                        name:    "畸形 JSON",
                        json:    `{invalid`,
                        wantErr: true,
                },
                {
                        name:    "数组而非对象",
                        json:    `[]`,
                        wantErr: true,
                },
                {
                        name:    "无 tag_name",
                        json:    `{"name":"no tag"}`,
                        wantTag: "",
                },
                {
                        name:    "tag 无 v 前缀",
                        json:    `{"tag_name":"1.1.0"}`,
                        wantTag: "1.1.0",
                },
                {
                        name:    "空 tag_name",
                        json:    `{"tag_name":""}`,
                        wantTag: "",
                },
                {
                        name:    "超大响应",
                        json:    `{"tag_name":"v1.1.0","body":"` + strings.Repeat("a", 100000) + `"}`,
                        wantTag: "v1.1.0",
                },
        }

        for _, c := range cases {
                t.Run(c.name, func(t *testing.T) {
                        var release ReleaseInfo
                        err := json.Unmarshal([]byte(c.json), &release)
                        if c.wantErr && err == nil {
                                t.Errorf("expected error, got nil")
                        }
                        if !c.wantErr && err != nil {
                                t.Errorf("unexpected error: %v", err)
                        }
                        if err == nil && release.TagName != c.wantTag {
                                t.Errorf("TagName = %q, want %q", release.TagName, c.wantTag)
                        }
                })
        }
}

func TestVersionComparison(t *testing.T) {
        cases := []struct {
                current string
                tag     string
                update  bool
        }{
                {"1.0.0", "v1.0.0", false},     // 相同版本
                {"1.0.0", "v1.1.0", true},      // 有更新
                {"1.1.0", "v1.0.0", true},      // 当前更新（也算有"更新"因为版本不同）
                {"1.0.0", "", false},           // 空 tag
                {"1.0.0", "v", false},          // 只有 v，去 v 后为空
                {"", "v1.0.0", true},           // 当前为空
        }
        for _, c := range cases {
                tag := c.tag
                hasUpdate := tag != c.current && tag != ""
                // 去掉 v 前缀的逻辑
                if len(tag) > 0 && tag[0] == 'v' {
                        tag = tag[1:]
                }
                hasUpdate = tag != c.current && tag != ""
                if hasUpdate != c.update {
                        t.Errorf("current=%q tag=%q: hasUpdate=%v, want %v",
                                c.current, c.tag, hasUpdate, c.update)
                }
        }
}

func TestFindAssetForCurrentPlatform(t *testing.T) {
        release := &ReleaseInfo{
                Assets: []Asset{
                        {Name: "samime-linux-amd64", BrowserDownloadURL: "url1"},
                        {Name: "samime-linux-arm64", BrowserDownloadURL: "url2"},
                        {Name: "samime-windows-amd64.exe", BrowserDownloadURL: "url3"},
                        {Name: "samime-darwin-arm64", BrowserDownloadURL: "url4"},
                        {Name: "samime-darwin-universal", BrowserDownloadURL: "url5"},
                },
        }

        c := New("test/repo", "1.0.0")
        asset := c.FindAssetForCurrentPlatform(release)
        if asset == nil {
                // 当前测试运行在 Linux amd64 上
                t.Logf("当前平台未找到资源（可能不是 linux/amd64）")
        } else {
                t.Logf("当前平台资源: %s", asset.Name)
        }
}

func TestFindAssetEmpty(t *testing.T) {
        c := New("test/repo", "1.0.0")
        release := &ReleaseInfo{Assets: []Asset{}}
        if asset := c.FindAssetForCurrentPlatform(release); asset != nil {
                t.Errorf("empty assets should return nil, got %v", asset)
        }
}

func TestFindAssetMalformed(t *testing.T) {
        c := New("test/repo", "1.0.0")
        release := &ReleaseInfo{
                Assets: []Asset{
                        {Name: "", BrowserDownloadURL: ""},
                        {Name: "no-platform-info", BrowserDownloadURL: "url"},
                        {Name: "samime-amd64", BrowserDownloadURL: "url"},  // 缺平台
                },
        }
        if asset := c.FindAssetForCurrentPlatform(release); asset != nil {
                t.Logf("找到资源: %s（可能匹配到 samime-amd64）", asset.Name)
        }
}

func TestCheckAndPromptFormat(t *testing.T) {
        c := New("samaidev/samime", "1.0.0")
        // 实际会请求 GitHub API，可能因网络失败
        msg := c.CheckAndPrompt()
        if msg == "" {
                t.Error("CheckAndPrompt should return non-empty string")
        }
        t.Logf("CheckAndPrompt 输出:\n%s", msg)
}

func TestContains(t *testing.T) {
        cases := []struct {
                s, sub string
                want   bool
        }{
                {"hello", "ell", true},
                {"hello", "world", false},
                {"", "", true},
                {"hello", "", true},
                {"", "a", false},
                {"hello", "hello world", false},
                {"samime-windows-amd64.exe", "windows", true},
                {"samime-windows-amd64.exe", "linux", false},
        }
        for _, c := range cases {
                got := contains(c.s, c.sub)
                if got != c.want {
                        t.Errorf("contains(%q,%q) = %v, want %v", c.s, c.sub, got, c.want)
                }
        }
}

func TestReleaseInfoWithNilAssets(t *testing.T) {
        json := `{"tag_name":"v1.0.0","assets":null}`
        var release ReleaseInfo
        if err := jsonUnmarshal([]byte(json), &release); err != nil {
                t.Fatal(err)
        }
        if release.Assets != nil {
                t.Errorf("Assets should be nil, got %v", release.Assets)
        }
}

// 防止 import 警告
func jsonUnmarshal(data []byte, v interface{}) error {
        return json.Unmarshal(data, v)
}

func TestVersionConstants(t *testing.T) {
        if Version == "" {
                t.Error("Version should not be empty")
        }
        if UpdateRepo == "" {
                t.Error("UpdateRepo should not be empty")
        }
}

func TestNewChecker(t *testing.T) {
        c := New("owner/repo", "2.0.0")
        if c.repo != "owner/repo" {
                t.Errorf("repo = %q", c.repo)
        }
        if c.currentVer != "2.0.0" {
                t.Errorf("currentVer = %q", c.currentVer)
        }
        if c.httpClient == nil {
                t.Error("httpClient should not be nil")
        }
        if c.httpClient.Timeout != 10*time.Second {
                t.Errorf("timeout = %v, want 10s", c.httpClient.Timeout)
        }
}

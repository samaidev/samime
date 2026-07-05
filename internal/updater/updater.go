// Package updater 自动更新检查
// 从 GitHub Releases 检查最新版本，提示用户下载
package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"time"
)

// Version 当前版本
const Version = "1.0.0"

// UpdateRepo GitHub 仓库 (owner/repo)
var UpdateRepo = "samaidev/samime"

// ReleaseInfo GitHub Release 信息
type ReleaseInfo struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []Asset   `json:"assets"`
}

// Asset Release 附件
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// Checker 更新检查器
type Checker struct {
	repo       string
	currentVer string
	httpClient *http.Client
}

// New 创建检查器
func New(repo, currentVer string) *Checker {
	return &Checker{
		repo:       repo,
		currentVer: currentVer,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// CheckLatest 检查最新版本
func (c *Checker) CheckLatest() (*ReleaseInfo, bool, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", c.repo)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("check update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, false, fmt.Errorf("github API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, false, err
	}

	var release ReleaseInfo
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, false, err
	}

	latestVer := release.TagName
	if len(latestVer) > 0 && latestVer[0] == 'v' {
		latestVer = latestVer[1:]
	}
	hasUpdate := latestVer != c.currentVer && latestVer != ""

	return &release, hasUpdate, nil
}

// DownloadAsset 下载某个资源到指定路径
func (c *Checker) DownloadAsset(asset *Asset, destPath string) error {
	resp, err := c.httpClient.Get(asset.BrowserDownloadURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// FindAssetForCurrentPlatform 找到当前平台的安装包
func (c *Checker) FindAssetForCurrentPlatform(release *ReleaseInfo) *Asset {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	for i, asset := range release.Assets {
		name := asset.Name
		switch goos {
		case "windows":
			if goarch == "amd64" && contains(name, "windows") && contains(name, "amd64") {
				return &release.Assets[i]
			}
		case "darwin":
			if goarch == "arm64" && contains(name, "darwin") && (contains(name, "arm64") || contains(name, "universal")) {
				return &release.Assets[i]
			}
			if goarch == "amd64" && contains(name, "darwin") && (contains(name, "amd64") || contains(name, "universal")) {
				return &release.Assets[i]
			}
		case "linux":
			if goarch == "amd64" && contains(name, "linux") && contains(name, "amd64") {
				return &release.Assets[i]
			}
			if goarch == "arm64" && contains(name, "linux") && contains(name, "arm64") {
				return &release.Assets[i]
			}
		}
	}
	return nil
}

// CheckAndPrompt 检查更新并返回提示信息
func (c *Checker) CheckAndPrompt() string {
	release, hasUpdate, err := c.CheckLatest()
	if err != nil {
		return fmt.Sprintf("[update] 检查失败: %v", err)
	}
	if !hasUpdate {
		return fmt.Sprintf("[update] 已是最新版本 (%s)", c.currentVer)
	}

	asset := c.FindAssetForCurrentPlatform(release)
	msg := fmt.Sprintf("[update] 发现新版本 %s (当前 %s)\n", release.TagName, c.currentVer)
	msg += fmt.Sprintf("  发布时间: %s\n", release.PublishedAt.Format("2006-01-02"))
	if release.Body != "" {
		body := release.Body
		if len(body) > 200 {
			body = body[:200] + "..."
		}
		msg += fmt.Sprintf("  更新内容:\n%s\n", body)
	}
	if asset != nil {
		msg += fmt.Sprintf("  下载: %s\n", asset.BrowserDownloadURL)
	} else {
		msg += fmt.Sprintf("  手动下载: %s\n", release.HTMLURL)
	}
	return msg
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

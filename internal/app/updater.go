package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// 从 GitHub Releases 检查新版、一键更新。查版本、下载、校验两个平台一样；
// 挑哪个资产、怎么装各自实现（updater_windows.go / updater_darwin.go）。

const releaseAPI = "https://api.github.com/repos/margbug01/mami/releases/latest"

const updateFlag = "--mami-update"

type release struct {
	version string
	name    string // 资产文件名
	url     string
	size    int64
	sha256  string // GitHub 给资产算的 digest，旧版 release 可能没有
}

type updateState struct {
	mu       sync.Mutex
	checking bool
	checked  bool     // 至少成功查过一次
	latest   *release // 比当前版本新的那个；nil = 已是最新
	err      string
	busy     bool    // 正在下载/准备重启
	progress float64 // 下载进度 0..1
}

func (u *updateState) snapshot() (checking, checked bool, latest *release, err string, busy bool, progress float64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.checking, u.checked, u.latest, u.err, u.busy, u.progress
}

// newerVersion 比较 x.y.z，b 比 a 新就返回 true；带 v 前缀、预发布后缀（-beta）都只看数字部分
func newerVersion(a, b string) bool {
	pa, pb := versionParts(a), versionParts(b)
	for i := range 3 {
		if pb[i] != pa[i] {
			return pb[i] > pa[i]
		}
	}
	return false
}

func versionParts(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	v, _, _ = strings.Cut(v, "-")
	var out [3]int
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

func updateDir() string { return filepath.Join(os.TempDir(), "Mami-update") }

func (a *App) updateClient(timeout time.Duration) *http.Client {
	cfg, _ := a.config()
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if p := strings.TrimSpace(cfg.Proxy); p != "" {
		if u, err := url.Parse(p); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

// fetchLatest 读最新 release，挑出这台机器要用的资产（isUpdateAsset）
func (a *App) fetchLatest(ctx context.Context, portable bool) (*release, error) {
	api := releaseAPI
	if v := os.Getenv("MAMI_UPDATE_API"); v != "" { // 测试用假接口
		api = v
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", api, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Mami/"+mygo.App.Version())
	resp, err := a.updateClient(15 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}
	var body struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name   string `json:"name"`
			URL    string `json:"browser_download_url"`
			Size   int64  `json:"size"`
			Digest string `json:"digest"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body); err != nil {
		return nil, err
	}
	r := &release{version: strings.TrimPrefix(body.Tag, "v")}
	for _, as := range body.Assets {
		if isUpdateAsset(strings.ToLower(as.Name), portable) {
			r.name, r.url, r.size = as.Name, as.URL, as.Size
			if h, ok := strings.CutPrefix(as.Digest, "sha256:"); ok {
				r.sha256 = h
			}
			break
		}
	}
	if r.url == "" {
		return nil, fmt.Errorf(T("%s 里没有可下载的安装文件"), body.Tag)
	}
	return r, nil
}

// checkUpdate 查一次最新版。manual=false 是后台自动查，失败只记日志不显示
func (a *App) checkUpdate(manual bool) {
	cur := mygo.App.Version()
	u := &a.upd
	u.mu.Lock()
	if u.checking || u.busy {
		u.mu.Unlock()
		return
	}
	u.checking = true
	if manual {
		u.err = ""
	}
	u.mu.Unlock()
	a.refreshSettings()

	exe, _ := os.Executable()
	r, err := a.fetchLatest(context.Background(), portableInstall(exe))
	u.mu.Lock()
	u.checking = false
	switch {
	case err != nil:
		log.Println("update check:", err)
		if manual {
			u.err = T("检查失败：") + err.Error()
		}
	case cur == "":
		u.err = T("开发版没有版本号，不能自动更新")
	default:
		u.checked = true
		u.latest = nil
		if newerVersion(cur, r.version) {
			u.latest = r
			log.Printf("update: %s → %s available", cur, r.version)
		}
	}
	u.mu.Unlock()
	a.refreshSettings()
}

// updateLoop 启动 20 秒后查一次，之后每天一次；有新版只在托盘菜单和设置页里提示，不弹窗
func (a *App) updateLoop() {
	if mygo.App.Version() == "" {
		return
	}
	time.Sleep(20 * time.Second)
	for {
		a.checkUpdate(false)
		time.Sleep(24 * time.Hour)
	}
}

func (a *App) refreshSettings() {
	if w := a.setWin; w != nil {
		w.Update(func() {})
	}
}

// startUpdate 下载新版、校验，然后交给更新助手并退出
func (a *App) startUpdate() {
	u := &a.upd
	u.mu.Lock()
	r := u.latest
	if r == nil || u.busy {
		u.mu.Unlock()
		return
	}
	u.busy, u.progress, u.err = true, 0, ""
	u.mu.Unlock()
	a.refreshSettings()
	go func() {
		err := a.downloadAndHandOff(r)
		if err == nil {
			log.Printf("update: handing off to helper for %s", r.version)
			a.quitting = true
			mygo.App.Quit()
			return
		}
		log.Println("update:", err)
		u.mu.Lock()
		u.busy, u.err = false, T("更新失败：")+err.Error()
		u.mu.Unlock()
		a.refreshSettings()
	}()
}

func (a *App) download(r *release, file string) error {
	req, _ := http.NewRequest("GET", r.url, nil)
	req.Header.Set("User-Agent", "Mami/"+mygo.App.Version())
	resp, err := a.updateClient(10 * time.Minute).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf(T("下载失败：HTTP %d"), resp.StatusCode)
	}
	f, err := os.Create(file)
	if err != nil {
		return err
	}
	h := sha256.New()
	pw := &progressWriter{total: r.size, on: func(p float64) {
		a.upd.mu.Lock()
		a.upd.progress = p
		a.upd.mu.Unlock()
		a.refreshSettings()
	}}
	n, err := io.Copy(io.MultiWriter(f, h, pw), resp.Body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if r.size > 0 && n != r.size {
		return fmt.Errorf(T("下载不完整（%d / %d 字节）"), n, r.size)
	}
	if r.sha256 != "" && !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), r.sha256) {
		return errors.New(T("文件校验没通过，可能下载被篡改了"))
	}
	return nil
}

// progressWriter 每涨 2% 报一次，免得每个包都重画设置窗口
type progressWriter struct {
	total, n int64
	last     float64
	on       func(float64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	p.n += int64(len(b))
	if p.total > 0 {
		if v := float64(p.n) / float64(p.total); v-p.last >= 0.02 || v >= 1 {
			p.last = v
			p.on(v)
		}
	}
	return len(b), nil
}

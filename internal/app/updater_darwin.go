//go:build darwin

package app

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// macOS 的更新：下载 Mami-mac.zip（里面是 Mami.app），解到旁边，把旧的 .app 改名、新的换上，
// 再起一个 sh 等 Mami 退出后 open 新的。正在跑的程序的文件可以直接改名，不用更新助手。

const macUpdateAsset = "mami-mac.zip"

func portableInstall(string) bool { return false }

func isUpdateAsset(n string, _ bool) bool { return n == macUpdateAsset }

// appBundle 正在跑的 Mami.app 的路径；不是从 .app 里跑的（go run、测试）返回 ""
func appBundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if p, err := filepath.EvalSymlinks(exe); err == nil {
		exe = p
	}
	macos := filepath.Dir(exe)
	if filepath.Base(macos) != "MacOS" || filepath.Base(filepath.Dir(macos)) != "Contents" {
		return ""
	}
	bundle := filepath.Dir(filepath.Dir(macos))
	if !strings.HasSuffix(bundle, ".app") {
		return ""
	}
	return bundle
}

func (a *App) downloadAndHandOff(r *release) error {
	bundle := appBundle()
	if bundle == "" {
		return errors.New(T("不是从 Mami.app 启动的，不能自动更新"))
	}
	if strings.Contains(bundle, "/AppTranslocation/") {
		// 没拖进「应用程序」就直接打开的：系统把它放在只读的临时位置
		return errors.New(T("先把 Mami 拖进「应用程序」文件夹，再打开它更新"))
	}
	dir := updateDir()
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file := filepath.Join(dir, filepath.Base(r.name))
	if err := a.download(r, file); err != nil {
		return err
	}
	out := filepath.Join(dir, "unzipped")
	if err := exec.Command("/usr/bin/ditto", "-x", "-k", file, out).Run(); err != nil {
		return err
	}
	fresh := filepath.Join(out, "Mami.app")
	if _, err := os.Stat(filepath.Join(fresh, "Contents", "MacOS")); err != nil {
		return errors.New(T("更新包里没有 Mami.app"))
	}
	staged, old := bundle+".new", bundle+".old"
	os.RemoveAll(staged)
	os.RemoveAll(old)
	if err := exec.Command("/usr/bin/ditto", fresh, staged).Run(); err != nil {
		os.RemoveAll(staged)
		return err
	}
	if err := os.Rename(bundle, old); err != nil {
		os.RemoveAll(staged)
		return err
	}
	if err := os.Rename(staged, bundle); err != nil {
		os.Rename(old, bundle)
		return err
	}
	// Mami 退出后再打开新的；setsid 让它不跟着 Mami 一起被关掉
	cmd := exec.Command("/bin/sh", "-c", `while kill -0 "$0" 2>/dev/null; do sleep 0.2; done; open "$1"`, strconv.Itoa(os.Getpid()), bundle)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

// cleanupUpdate 启动后删掉上次更新留下的临时目录和旧的 .app
func cleanupUpdate() {
	time.Sleep(30 * time.Second)
	os.RemoveAll(updateDir())
	if b := appBundle(); b != "" {
		os.RemoveAll(b + ".old")
	}
}

// runUpdateHelper macOS 不用更新助手
func runUpdateHelper([]string) {}

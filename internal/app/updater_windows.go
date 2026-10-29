//go:build windows

package app

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// 安装包不管 Mami 是不是还在跑，覆盖被占用的 Mami.exe 会失败，所以得先退出再装、装完再拉起来。
// 做法：把自己拷到临时目录当「更新助手」启动（--mami-update），Mami 退出；
// 助手等 Mami 进程结束 → 安装版跑安装包 /S /D=原目录、绿色版直接换 exe → 启动新的 Mami。

// portableInstall 绿色版（旁边没有 Uninstall.exe）换 exe，安装版跑安装包
func portableInstall(exe string) bool {
	_, err := os.Stat(filepath.Join(filepath.Dir(exe), "Uninstall.exe"))
	return err != nil
}

// isUpdateAsset 安装版要安装包，绿色版要 Mami.exe（n 已转小写）
func isUpdateAsset(n string, portable bool) bool {
	setup := strings.HasPrefix(n, "mami") && strings.Contains(n, "setup") && strings.HasSuffix(n, ".exe")
	return (portable && n == "mami.exe") || (!portable && setup)
}

func (a *App) downloadAndHandOff(r *release) error {
	dir := updateDir()
	os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file := filepath.Join(dir, filepath.Base(r.name))
	if err := a.download(r, file); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	helper := filepath.Join(dir, "Mami-updater.exe")
	if err := copyFile(self, helper); err != nil {
		return err
	}
	kind := "setup"
	if portableInstall(self) {
		kind = "portable"
	}
	cmd := exec.Command(helper, updateFlag, strconv.Itoa(os.Getpid()), kind, file, self, a.logPath)
	cmd.Dir = dir
	return cmd.Start()
}

// cleanupUpdate 启动后删掉上次更新留下的临时目录和绿色版的旧 exe（助手可能还没退，晚点再删）
func cleanupUpdate() {
	time.Sleep(30 * time.Second)
	os.RemoveAll(updateDir())
	if exe, err := os.Executable(); err == nil {
		os.Remove(exe + ".old")
	}
}

// runUpdateHelper 是更新助手：args = pid kind file target logPath
func runUpdateHelper(args []string) {
	if len(args) < 5 {
		return
	}
	if f := openLog(args[4]); f != nil {
		log.SetOutput(f)
		defer f.Close()
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	pid, _ := strconv.Atoi(args[0])
	kind, file, target := args[1], args[2], args[3]
	waitExit(uint32(pid), 20*time.Second)
	var err error
	if kind == "portable" {
		err = replaceExe(file, target)
	} else {
		err = runSetup(file, filepath.Dir(target))
	}
	if err != nil {
		log.Println("updater:", err)
	} else {
		log.Println("updater: installed", filepath.Base(file))
	}
	// 失败也把 Mami 拉起来：旧版还在，总比托盘里什么都没有强
	cmd := exec.Command(target)
	cmd.Dir = filepath.Dir(target)
	if err := cmd.Start(); err != nil {
		log.Println("updater: restart:", err)
	}
}

func waitExit(pid uint32, timeout time.Duration) {
	const SYNCHRONIZE = 0x00100000
	h, err := syscall.OpenProcess(SYNCHRONIZE, false, pid)
	if err != nil {
		return // 已经退了
	}
	defer syscall.CloseHandle(h)
	syscall.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
	time.Sleep(300 * time.Millisecond) // 等系统放开 exe 的文件锁
}

// runSetup 静默跑安装包，装回原来的目录。/D= 必须放最后、不能加引号，所以命令行自己拼
func runSetup(setup, dir string) error {
	cmd := exec.Command(setup)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `"` + setup + `" /S /D=` + dir}
	return cmd.Run()
}

// replaceExe 绿色版：旧 exe 改名成 .old，新的放进来；失败就换回去
func replaceExe(src, target string) error {
	old := target + ".old"
	os.Remove(old)
	var err error
	for range 20 { // 进程刚退时文件可能还锁着
		if err = os.Rename(target, old); err == nil {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil {
		return err
	}
	if err := copyFile(src, target); err != nil {
		os.Remove(target)
		os.Rename(old, target)
		return err
	}
	return nil
}

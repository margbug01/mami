//go:build windows

package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/egoist/mygo"
)

// 2.1.0 起项目从 Huaci 改名 Mami，数据目录也从 %AppData%\Huaci 换到 %AppData%\Mami。
// migrateFromHuaci 首次启动时把旧配置（含备份）拷过来，并把旧的开机自启项换成新的。
// 旧目录原样留着，不删。
func migrateFromHuaci(newDir string) {
	oldDir := filepath.Join(filepath.Dir(newDir), "Huaci")
	if _, err := os.Stat(filepath.Join(newDir, "config.json")); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(oldDir, "config.json")); err == nil {
			names, _ := filepath.Glob(filepath.Join(oldDir, "config.json*"))
			for _, src := range names {
				if err := copyFile(src, filepath.Join(newDir, filepath.Base(src))); err != nil {
					log.Printf("migrate: copy %s: %v", filepath.Base(src), err)
				}
			}
			log.Printf("migrate: copied %d config file(s) from %s", len(names), oldDir)
		}
	}
	if deleteRunValue("dev.margbug.huaci") {
		if err := mygo.App.SetOpenAtLogin(true); err != nil {
			log.Printf("migrate: open at login: %v", err)
		} else {
			log.Println("migrate: moved open-at-login from Huaci to Mami")
		}
	}
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

var (
	advapi32         = syscall.NewLazyDLL("advapi32.dll")
	pRegOpenKeyExW   = advapi32.NewProc("RegOpenKeyExW")
	pRegDeleteValueW = advapi32.NewProc("RegDeleteValueW")
	pRegCloseKey     = advapi32.NewProc("RegCloseKey")
)

// deleteRunValue 删掉 HKCU\...\Run 下的一项；删成功（原来有）才返回 true
func deleteRunValue(name string) bool {
	const hkcu, keySetValue = 0x80000001, 0x0002
	path, _ := syscall.UTF16PtrFromString(`Software\Microsoft\Windows\CurrentVersion\Run`)
	var k uintptr
	if r, _, _ := pRegOpenKeyExW.Call(hkcu, uintptr(unsafe.Pointer(path)), 0, keySetValue, uintptr(unsafe.Pointer(&k))); r != 0 {
		return false
	}
	defer pRegCloseKey.Call(k)
	n, _ := syscall.UTF16PtrFromString(name)
	r, _, _ := pRegDeleteValueW.Call(k, uintptr(unsafe.Pointer(n)))
	return r == 0
}

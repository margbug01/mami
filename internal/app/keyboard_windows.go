//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

var (
	pGetWindowTextW    = user32.NewProc("GetWindowTextW")
	pGetKeyboardLayout = user32.NewProc("GetKeyboardLayout")
	pVkKeyScanExW      = user32.NewProc("VkKeyScanExW")
)

// windowTitle 窗口标题（按程序换润色档位时认浏览器里开的是不是 AI 网站）
func windowTitle(h uintptr) string {
	buf := make([]uint16, 512)
	n, _, _ := pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf[:n])
}

// rightAltTaken 当前键盘布局里右 Alt 另有用处：韩文键盘是韩/英切换，
// 欧洲键盘的 AltGr 要用来打 @ € { } 这些字（这些字要按 Ctrl+Alt 才出来就是 AltGr 布局）
func rightAltTaken() bool {
	hkl, _, _ := pGetKeyboardLayout.Call(0)
	if hkl&0x3ff == 0x12 { // LANG_KOREAN
		return true
	}
	for _, ch := range `@€{}[]\|~` {
		r, _, _ := pVkKeyScanExW.Call(uintptr(ch), hkl)
		if r != 0xffff && (r>>8)&6 == 6 {
			return true
		}
	}
	return false
}

package app

import (
	"runtime"
	"testing"
)

// nativeApp 前台程序在这个系统上报出来的样子：Windows 是 exe 的完整路径，macOS 是 .app 的路径
func nativeApp(windows, mac string) string {
	if runtime.GOOS == "windows" {
		return windows
	}
	return mac
}

func TestPasteKeysFor(t *testing.T) {
	for exe, want := range map[string]int{
		nativeApp(`C:\Program Files\Git\usr\bin\mintty.exe`, "/opt/homebrew/bin/mintty.exe"): pasteShiftIns,
		"wezterm-gui.exe": pasteCtrlShiftV, "Alacritty.exe": pasteCtrlShiftV,
		"WindowsTerminal.exe": pasteCtrlV, "Cursor.exe": pasteCtrlV, "": pasteCtrlV,
		nativeApp(`C:\Users\x\AppData\Local\Programs\cursor\Cursor.exe`, "/Applications/Cursor.app"): pasteCtrlV,
	} {
		if got := pasteKeysFor(exe); got != want {
			t.Errorf("%s → %d, want %d", exe, got, want)
		}
	}
}

func TestPasteBlind(t *testing.T) {
	for exe, want := range map[string]bool{
		nativeApp(`C:\Program Files\Tencent\Weixin\Weixin.exe`, "/Applications/WeChat.app"): true, "WeChat.exe": true,
		"Cursor.exe": false, "": false, nativeApp(`C:\Windows\notepad.exe`, "/System/Applications/TextEdit.app"): false,
	} {
		if got := pasteBlind(exe); got != want {
			t.Errorf("%s → %v, want %v", exe, got, want)
		}
	}
}

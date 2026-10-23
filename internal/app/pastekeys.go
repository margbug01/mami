package app

import (
	"path/filepath"
	"strings"
)

const (
	pasteCtrlV      = iota // 绝大多数程序，含 Windows Terminal、cmd/PowerShell 窗口
	pasteShiftIns          // mintty（Git Bash）：Ctrl+V 会原样发给程序
	pasteCtrlShiftV        // WezTerm、Alacritty、Tabby、Warp 默认的粘贴键
)

// pasteKeysFor 前台程序用哪个键粘贴
func pasteKeysFor(exe string) int {
	switch strings.ToLower(filepath.Base(exe)) {
	case "mintty.exe":
		return pasteShiftIns
	case "wezterm-gui.exe", "alacritty.exe", "tabby.exe", "warp.exe":
		return pasteCtrlShiftV
	}
	return pasteCtrlV
}

// pasteBlind 焦点在输入框里，但 UIA / 辅助功能认不出、也没有系统光标的程序（微信 4.x 是 Qt 自绘）：照样贴
func pasteBlind(exe string) bool {
	switch strings.ToLower(filepath.Base(exe)) {
	case "weixin.exe", "wechat.exe", "wechat.app":
		return true
	}
	return false
}

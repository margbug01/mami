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

package app

import "testing"

func TestPasteKeysFor(t *testing.T) {
	for exe, want := range map[string]int{
		`C:\Program Files\Git\usr\bin\mintty.exe`: pasteShiftIns,
		"wezterm-gui.exe":                         pasteCtrlShiftV, "Alacritty.exe": pasteCtrlShiftV,
		"WindowsTerminal.exe": pasteCtrlV, "Cursor.exe": pasteCtrlV, "": pasteCtrlV,
	} {
		if got := pasteKeysFor(exe); got != want {
			t.Errorf("%s → %d, want %d", exe, got, want)
		}
	}
}

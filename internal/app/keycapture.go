package app

import (
	"fmt"
	"strings"
)

// keyCapture 是设置页「点一下框，直接按键」的录制状态，只由钩子线程驱动。
// 组合键模式录 Alt+T 这类全局快捷键；语音模式录单按一下的右 Alt / 右 Ctrl / 右 Shift。
type keyCapture struct {
	voice     bool
	mods      [4]bool // Ctrl Alt Shift Win
	candidate uint32  // 语音模式：按下了、还没松开的右侧修饰键
	hint      string
}

const (
	captureGoing = iota
	captureDone
	captureCancel
)

// step 喂一个按键事件。captureDone 时 acc 是录到的结果（组合键模式清空是 ""，语音模式是 "Off"）；
// captureGoing 时 hint 是框里要显示的提示，"" 表示显示默认提示。
func (k *keyCapture) step(vk uint32, down bool) (state int, acc, hint string) {
	if k.voice {
		return k.stepVoice(vk, down)
	}
	if m := modIndex(vk); m >= 0 {
		k.mods[m] = down
		k.hint = ""
		if mods := k.modText(); mods != "" {
			k.hint = keysLabel(mods + "+…")
		}
		return captureGoing, "", k.hint
	}
	if !down {
		return captureGoing, "", k.hint
	}
	none := k.modText() == ""
	switch {
	case none && vk == 0x1B:
		return captureCancel, "", ""
	case none && (vk == 0x08 || vk == 0x2E):
		return captureDone, "", ""
	}
	name := keyName(vk)
	switch {
	case name == "":
		k.hint = T("不支持这个键")
	case none && (vk < 0x70 || vk > 0x87):
		k.hint = T("要带上 Ctrl、Alt 或 Shift")
		if onMac {
			k.hint = T("要带上 ⌘、⌃、⌥ 或 ⇧")
		}
	case none:
		return captureDone, name, ""
	default:
		return captureDone, k.modText() + "+" + name, ""
	}
	return captureGoing, "", k.hint
}

// stepVoice 语音触发键：单按一下修饰键（分左右，松开才算）、单按 F 键 / CapsLock，或者一个组合键
func (k *keyCapture) stepVoice(vk uint32, down bool) (int, string, string) {
	if m := modIndex(vk); m >= 0 {
		wasNone := k.modText() == ""
		k.mods[m] = down
		switch {
		case down && wasNone && voiceKeyName(vk) != "":
			k.candidate = vk
		case down && vk == 0xA5 && k.candidate == 0xA2: // AltGr 先带出一个左 Ctrl
			k.candidate = vk
		case down:
			k.candidate = 0 // 两个修饰键一起按，等后面的键凑组合
		case vk == k.candidate:
			return captureDone, voiceKeyName(vk), ""
		}
		return captureGoing, "", k.hint
	}
	if !down {
		return captureGoing, "", k.hint
	}
	k.candidate = 0
	if k.modText() != "" {
		if name := keyName(vk); name != "" {
			return captureDone, k.modText() + "+" + name, ""
		}
		k.hint = T("不支持这个键")
		return captureGoing, "", k.hint
	}
	switch name := voiceKeyName(vk); {
	case vk == 0x1B:
		return captureCancel, "", ""
	case vk == 0x08 || vk == 0x2E:
		return captureDone, "Off", ""
	case name != "":
		return captureDone, name, ""
	}
	k.hint = T("单按只能用 Alt/Ctrl/Shift、F 键或 CapsLock")
	if onMac {
		k.hint = T("单按只能用 ⌥/⌃/⇧/⌘、fn 或 F 键")
	}
	return captureGoing, "", k.hint
}

func (k *keyCapture) modText() string {
	var parts []string
	for i, name := range []string{"Ctrl", "Alt", "Shift", "Win"} {
		if k.mods[i] {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, "+")
}

func modIndex(vk uint32) int {
	switch vk {
	case 0x11, 0xA2, 0xA3:
		return 0
	case 0x12, 0xA4, 0xA5:
		return 1
	case 0x10, 0xA0, 0xA1:
		return 2
	case 0x5B, 0x5C:
		return 3
	}
	return -1
}

// keyLabel 给人看的键名。macOS 上换成 ⌘⌥⌃⇧ 和左右；配置里存的还是 Ctrl/Alt/Shift/Win（mygo 认 Win=⌘）
func keyLabel(part string) string {
	if !onMac {
		return part
	}
	side := map[byte]string{'L': T("左"), 'R': T("右")}
	sym := map[string]string{"Ctrl": "⌃", "Alt": "⌥", "Shift": "⇧", "Win": "⌘", "Fn": "fn",
		"Enter": "↩", "Backspace": "⌫", "Delete": "⌦", "Tab": "⇥", "Up": "↑", "Down": "↓", "Left": "←", "Right": "→"}
	if s, ok := sym[part]; ok {
		return s
	}
	if len(part) > 1 && side[part[0]] != "" { // LCtrl / RAlt / RWin 这种分左右的单键
		if s, ok := sym[part[1:]]; ok {
			return side[part[0]] + " " + s
		}
	}
	return part
}

// keysLabel 一整串组合键给人看：Windows 上原样，macOS 上连成「⌃⌥V」
func keysLabel(acc string) string {
	if !onMac {
		return acc
	}
	parts := strings.Split(acc, "+")
	for i, p := range parts {
		parts[i] = keyLabel(p)
	}
	return strings.Join(parts, "")
}

func voiceKeyName(vk uint32) string {
	for name, v := range voiceKeyNames {
		if v == vk {
			return name
		}
	}
	return ""
}

// captureKeyNames 是 mygo 认得的键名（RegisterHotKey 能用的），其余的键不收
var captureKeyNames = map[uint32]string{
	0x0D: "Enter", 0x09: "Tab", 0x20: "Space", 0x08: "Backspace", 0x2E: "Delete", 0x2D: "Insert",
	0x26: "Up", 0x28: "Down", 0x25: "Left", 0x27: "Right", 0x24: "Home", 0x23: "End",
	0x21: "PageUp", 0x22: "PageDown", 0x2C: "PrintScreen",
}

func keyName(vk uint32) string {
	switch {
	case vk >= 'A' && vk <= 'Z', vk >= '0' && vk <= '9':
		return string(rune(vk))
	case vk >= 0x70 && vk <= 0x87:
		return fmt.Sprintf("F%d", vk-0x70+1)
	}
	return captureKeyNames[vk]
}

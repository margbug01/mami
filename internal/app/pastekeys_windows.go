//go:build windows

package app

const (
	vkInsert              = 0x2D
	KEYEVENTF_EXTENDEDKEY = 0x0001 // Insert 是扩展键，不带这个会被当成小键盘 0
)

// sendPaste 按前台程序认的粘贴键发一次粘贴
func sendPaste() {
	switch pasteKeysFor(processName(windowPID(foreground()))) {
	case pasteShiftIns:
		sendKeys(
			KEYBDINPUT{WVk: VK_SHIFT},
			KEYBDINPUT{WVk: vkInsert, DwFlags: KEYEVENTF_EXTENDEDKEY},
			KEYBDINPUT{WVk: vkInsert, DwFlags: KEYEVENTF_EXTENDEDKEY | KEYEVENTF_KEYUP},
			KEYBDINPUT{WVk: VK_SHIFT, DwFlags: KEYEVENTF_KEYUP},
		)
	case pasteCtrlShiftV:
		sendKeys(
			KEYBDINPUT{WVk: VK_CONTROL},
			KEYBDINPUT{WVk: VK_SHIFT},
			KEYBDINPUT{WVk: 'V'},
			KEYBDINPUT{WVk: 'V', DwFlags: KEYEVENTF_KEYUP},
			KEYBDINPUT{WVk: VK_SHIFT, DwFlags: KEYEVENTF_KEYUP},
			KEYBDINPUT{WVk: VK_CONTROL, DwFlags: KEYEVENTF_KEYUP},
		)
	default:
		sendCtrl('V')
	}
}

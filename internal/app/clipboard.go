//go:build windows

package app

import (
	"log"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"
)

type clipItem struct {
	format uint32
	data   []byte
}

// 这些格式是 GDI 句柄而不是 HGLOBAL，没法按字节复制；位图会由 CF_DIB 自动合成回来
var skipFormats = map[uint32]bool{2: true, 3: true, 9: true, 14: true, 0x80: true, 0x82: true, 0x83: true, 0x8E: true}

var cfExcludeHistory uint32

func init() {
	r, _, _ := pRegisterClipboardFormatW.Call(uintptr(unsafe.Pointer(u16("ExcludeClipboardContentFromMonitorProcessing"))))
	cfExcludeHistory = uint32(r)
}

// pumpSleep 等待期间处理别的线程 SendMessage 过来的消息。
// 我们还原过剪贴板后就是 owner，别的程序 EmptyClipboard 时会同步给我们发
// WM_DESTROYCLIPBOARD；UI 线程要是干睡，对方的 Ctrl+C 就会卡到我们超时为止。
func pumpSleep(d time.Duration) {
	deadline := time.Now().Add(d)
	var m MSG
	for {
		pPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, 0) // PM_NOREMOVE：只处理 sent message
		left := time.Until(deadline)
		if left <= 0 {
			return
		}
		if left > 5*time.Millisecond {
			left = 5 * time.Millisecond
		}
		time.Sleep(left)
	}
}

func openClipboard(owner uintptr) bool {
	for i := 0; i < 30; i++ {
		if r, _, _ := pOpenClipboard.Call(owner); r != 0 {
			return true
		}
		pumpSleep(10 * time.Millisecond)
	}
	return false
}

func clipSeq() uint32 {
	r, _, _ := pGetClipboardSequenceNumber.Call()
	return uint32(r)
}

func snapshotClipboard(owner uintptr) ([]clipItem, bool) {
	if !openClipboard(owner) {
		return nil, false
	}
	defer pCloseClipboard.Call()
	var items []clipItem
	f := uintptr(0)
	for {
		r, _, _ := pEnumClipboardFormats.Call(f)
		if r == 0 {
			break
		}
		f = r
		if skipFormats[uint32(f)] {
			continue
		}
		h, _, _ := pGetClipboardData.Call(f)
		if h == 0 {
			continue
		}
		size, _, _ := pGlobalSize.Call(h)
		if size == 0 || size > 64<<20 {
			continue
		}
		p, _, _ := pGlobalLock.Call(h)
		if p == 0 {
			continue
		}
		buf := make([]byte, size)
		copy(buf, unsafe.Slice((*byte)(unsafe.Pointer(p)), size))
		pGlobalUnlock.Call(h)
		items = append(items, clipItem{uint32(f), buf})
	}
	return items, true
}

func setGlobal(format uint32, data []byte) {
	n := len(data)
	if n == 0 {
		n = 1
	}
	h, _, _ := pGlobalAlloc.Call(GMEM_MOVEABLE, uintptr(n))
	if h == 0 {
		return
	}
	p, _, _ := pGlobalLock.Call(h)
	if p != 0 {
		copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), n), data)
		pGlobalUnlock.Call(h)
	}
	if r, _, _ := pSetClipboardData.Call(uintptr(format), h); r == 0 {
		pGlobalFree.Call(h)
	}
}

func restoreClipboard(owner uintptr, items []clipItem) {
	if !openClipboard(owner) {
		return
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	for _, it := range items {
		setGlobal(it.format, it.data)
	}
	if cfExcludeHistory != 0 {
		setGlobal(cfExcludeHistory, nil)
	}
}

// setClipboardText 写入文本；temp=true 时不进 Win+V 剪贴板历史
func setClipboardText(owner uintptr, s string, temp bool) bool {
	if !openClipboard(owner) {
		return false
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	s = strings.ReplaceAll(s, "\x00", "")
	u := append(utf16.Encode([]rune(s)), 0)
	setGlobal(CF_UNICODETEXT, unsafe.Slice((*byte)(unsafe.Pointer(&u[0])), len(u)*2))
	if temp && cfExcludeHistory != 0 {
		setGlobal(cfExcludeHistory, nil)
	}
	return true
}

func getClipboardText(owner uintptr) string {
	if !openClipboard(owner) {
		return ""
	}
	defer pCloseClipboard.Call()
	h, _, _ := pGetClipboardData.Call(CF_UNICODETEXT)
	if h == 0 {
		return ""
	}
	size, _, _ := pGlobalSize.Call(h)
	p, _, _ := pGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer pGlobalUnlock.Call(h)
	u := unsafe.Slice((*uint16)(unsafe.Pointer(p)), size/2)
	for i, c := range u {
		if c == 0 {
			u = u[:i]
			break
		}
	}
	return string(utf16.Decode(u))
}

func waitClipChange(seq uint32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if clipSeq() != seq {
			pumpSleep(20 * time.Millisecond) // 有的程序分几次写入多种格式
			return true
		}
		pumpSleep(10 * time.Millisecond)
	}
	// 卡顿时一次循环就能睡过 deadline：出门前再看一眼，晚到的复制也算
	if clipSeq() != seq {
		pumpSleep(20 * time.Millisecond)
		return true
	}
	return false
}

func sendKeys(keys ...KEYBDINPUT) {
	in := make([]INPUT, len(keys))
	for i, k := range keys {
		k.DwExtraInfo = ourKeyTag // 键盘钩子认出是自己发的，不当成用户按键
		in[i] = INPUT{Type: INPUT_KEYBOARD, Ki: k}
	}
	pSendInput.Call(uintptr(len(in)), uintptr(unsafe.Pointer(&in[0])), unsafe.Sizeof(INPUT{}))
}

func sendCtrl(key uint16) {
	sendKeys(
		KEYBDINPUT{WVk: VK_CONTROL},
		KEYBDINPUT{WVk: key},
		KEYBDINPUT{WVk: key, DwFlags: KEYEVENTF_KEYUP},
		KEYBDINPUT{WVk: VK_CONTROL, DwFlags: KEYEVENTF_KEYUP},
	)
}

// waitModifiersUp 等用户松开快捷键里的修饰键，不然模拟的 Ctrl+C 会变成 Ctrl+Alt+C
func waitModifiersUp(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !keyDown(VK_SHIFT) && !keyDown(VK_CONTROL) && !keyDown(VK_MENU) && !keyDown(VK_LWIN) && !keyDown(VK_RWIN) {
			return
		}
		pumpSleep(10 * time.Millisecond)
	}
}

// grabSelection 用模拟 Ctrl+C 取当前选中的文字，取完把剪贴板原样还回去。
func grabSelection(owner uintptr, selectAllIfEmpty bool) (text string, selectedAll bool) {
	snap, _ := snapshotClipboard(owner)
	snapSeq := clipSeq()
	changed := false

	seq := clipSeq()
	start := time.Now()
	if selectAllIfEmpty && selectionKnownEmpty() {
		// 输入框明确没选中：Ctrl+C 不会改剪贴板，等它就是白等 600ms，直接走下面的全选
		log.Printf("grab: nothing selected (uia, %v), select all", time.Since(start).Round(time.Millisecond))
	} else {
		sendCtrl('C')
		sent := time.Since(start)
		if waitClipChange(seq, 600*time.Millisecond) {
			changed = true
			text = getClipboardText(owner)
		}
		log.Printf("grab: ctrl+c changed=%v after %v (sendinput %v), %d chars, seq %d→%d", changed, time.Since(start).Round(time.Millisecond), sent.Round(time.Millisecond), len([]rune(text)), seq, clipSeq())
	}
	if strings.TrimSpace(text) == "" && selectAllIfEmpty {
		sendCtrl('A')
		selectedAll = true
		pumpSleep(40 * time.Millisecond)
		seq = clipSeq()
		sendCtrl('C')
		if waitClipChange(seq, 600*time.Millisecond) {
			changed = true
			text = getClipboardText(owner)
		}
	}
	// 超时之后剪贴板才被写（程序卡了一下）也得还原，不然用户原来的剪贴板就被顶掉了
	if changed || clipSeq() != snapSeq {
		restoreClipboard(owner, snap)
	}
	return text, selectedAll
}

// collapseSelection 按一下右方向键：全选留下的选区收到末尾，光标回到文字最后
func collapseSelection() {
	sendKeys(KEYBDINPUT{WVk: 0x27}, KEYBDINPUT{WVk: 0x27, DwFlags: KEYEVENTF_KEYUP}) // VK_RIGHT
}

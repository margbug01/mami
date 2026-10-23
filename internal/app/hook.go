//go:build windows

package app

import (
	"fmt"
	"log"
	"runtime"
	"syscall"
	"unsafe"
)

var (
	ibeamCursor uintptr
	isOursFn    func(POINT) bool
)

func hookProc(nCode, wp, lp uintptr) uintptr {
	if int32(nCode) == 0 {
		kind := 0
		switch wp {
		case WM_LBUTTONDOWN:
			kind = mouseDown
		case WM_LBUTTONUP:
			kind = mouseUp
		case WM_RBUTTONDOWN, WM_MBUTTONDOWN, WM_XBUTTONDOWN:
			kind = mouseOther
		case WM_MOUSEWHEEL, WM_MOUSEHWHEEL:
			kind = mouseWheel
		}
		if kind != 0 {
			ms := (*MSLLHOOKSTRUCT)(nativePointer(lp))
			ev := mouseEvent{kind: kind, pt: ms.Pt, time: ms.Time, ibeam: cursorIsIBeam()}
			if isOursFn != nil {
				ev.ours = isOursFn(ms.Pt)
			}
			select {
			case mouseEvents <- ev:
			default: // 处理不过来就丢，钩子绝不能阻塞
			}
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, nCode, wp, lp)
	return r
}

const (
	whKeyboardLL = 13
	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104
	ourKeyTag    = 0x48554143 // "HUAC"：sendKeys 发的按键带这个标记
)

func kbdProc(nCode, wp, lp uintptr) uintptr {
	if int32(nCode) == 0 && (wp == wmKeyDown || wp == wmSysKeyDown || wp == 0x0101 || wp == 0x0105) {
		k := (*kbdLLHook)(nativePointer(lp))
		down := wp == wmKeyDown || wp == wmSysKeyDown
		if k.ExtraInfo != ourKeyTag && k.Flags&0x10 == 0 && (captureKey(k.VkCode, down) || voiceHookKey(k, down)) {
			return 1
		}
	}
	if int32(nCode) == 0 && (wp == wmKeyDown || wp == wmSysKeyDown) {
		k := (*kbdLLHook)(nativePointer(lp))
		if wp == wmKeyDown && k.ExtraInfo != ourKeyTag && cardWantsKey(k.VkCode) {
			go cardKeyFn(k.VkCode)
			return 1 // 吞掉：别让 Enter 把没翻译的原文发出去
		}
		if k.ExtraInfo != ourKeyTag && !isModifierVK(k.VkCode) {
			select {
			case mouseEvents <- mouseEvent{kind: keyPress, time: k.Time, vk: k.VkCode}:
			default:
			}
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, nCode, wp, lp)
	return r
}

func cursorIsIBeam() bool {
	ci := CURSORINFO{CbSize: uint32(unsafe.Sizeof(CURSORINFO{}))}
	if r, _, _ := pGetCursorInfo.Call(uintptr(unsafe.Pointer(&ci))); r == 0 {
		return false
	}
	return ci.HCursor == ibeamCursor
}

// startHook 在独立线程上挂低级鼠标钩子：钩子回调超时系统会直接摘掉它，
// 所以它所在的线程除了收消息什么都不干。
func startHook(isOurs func(POINT) bool, onCardKey func(vk uint32)) error {
	ibeamCursor, _, _ = pLoadCursorW.Call(0, IDC_IBEAM)
	isOursFn = isOurs
	cardKeyFn = onCardKey
	ready := make(chan error)
	go func() {
		runtime.LockOSThread()
		inst, _, _ := pGetModuleHandleW.Call(0)
		hk, _, e := pSetWindowsHookExW.Call(WH_MOUSE_LL, syscall.NewCallback(hookProc), inst, 0)
		if hk == 0 {
			ready <- fmt.Errorf("SetWindowsHookEx: %v", e)
			return
		}
		// 键盘钩子挂同一条线程；挂不上只是按钮收得慢，不算致命
		if kh, _, e := pSetWindowsHookExW.Call(whKeyboardLL, syscall.NewCallback(kbdProc), inst, 0); kh == 0 {
			log.Printf("keyboard hook: %v", e)
		}
		ready <- nil
		var m MSG
		for {
			r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				log.Println("hook thread exit")
				return
			}
		}
	}()
	return <-ready
}

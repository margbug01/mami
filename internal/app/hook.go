//go:build windows

package app

import (
	"fmt"
	"log"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	mouseDown = iota + 1
	mouseUp
	mouseOther
	mouseWheel
	keyPress // 键盘钩子报的：用户真按了键（不含我们自己模拟的、也不含单按修饰键）
)

// mouseEvent 是钩子线程报出来的一次鼠标动作。ours/ibeam 必须在钩子里当场判断：
// 等事件被处理时，指针可能已经移开了。
type mouseEvent struct {
	kind  int
	pt    POINT
	time  uint32
	ours  bool
	ibeam bool
}

var (
	mouseEvents = make(chan mouseEvent, 256)
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

// KBDLLHOOKSTRUCT
type kbdLLHook struct {
	VkCode, ScanCode, Flags, Time uint32
	ExtraInfo                     uintptr
}

const (
	whKeyboardLL = 13
	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104
	ourKeyTag    = 0x48554143 // "HUAC"：sendKeys 发的按键带这个标记
)

// 卡片开着时拦 Enter / Esc：cardKeyMode 0=不拦 1=只拦 Esc 2=Enter 和 Esc 都拦；
// 只在焦点还在原来那个窗口（cardKeyFg）、没按着修饰键时拦，Shift+Enter 换行之类照常
var (
	cardKeyMode atomic.Int32
	cardKeyFg   atomic.Uintptr
	cardKeyFn   func(vk uint32)
)

const (
	vkReturn = 0x0D
	vkEscape = 0x1B
)

func cardWantsKey(vk uint32) bool {
	mode := cardKeyMode.Load()
	if mode == 0 || cardKeyFn == nil || (vk != vkEscape && (vk != vkReturn || mode < 2)) {
		return false
	}
	for _, m := range []uintptr{0x10, 0x11, 0x12, 0x5B, 0x5C} {
		if keyDown(m) {
			return false
		}
	}
	return foreground() == cardKeyFg.Load()
}

func kbdProc(nCode, wp, lp uintptr) uintptr {
	if int32(nCode) == 0 && (wp == wmKeyDown || wp == wmSysKeyDown || wp == 0x0101 || wp == 0x0105) {
		k := (*kbdLLHook)(nativePointer(lp))
		if k.ExtraInfo != ourKeyTag && k.Flags&0x10 == 0 && voiceHookKey(k, wp == wmKeyDown || wp == wmSysKeyDown) {
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
			case mouseEvents <- mouseEvent{kind: keyPress, time: k.Time}:
			default:
			}
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, nCode, wp, lp)
	return r
}

// isModifierVK Shift / Ctrl / Alt（含左右）、Win、CapsLock：单按这些不算改了选区
func isModifierVK(vk uint32) bool {
	switch vk {
	case 0x10, 0x11, 0x12, 0x14, 0x5B, 0x5C, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5:
		return true
	}
	return false
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

package app

import "sync/atomic"

// 两个平台共用的坐标和键码。Windows 上 POINT/RECT 是物理像素；macOS 上是屏幕坐标的点
// （主屏左上角为原点，和 mygo、CGEvent、辅助功能 API 一致），dpiAt 恒为 96，scale 不缩放。
// 键码一律用 Windows 虚拟键（VK_*）：macOS 的事件监听把按键换算成同一套码，下面的状态机两边共用。

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }

func (r RECT) w() int32 { return r.Right - r.Left }
func (r RECT) h() int32 { return r.Bottom - r.Top }

// packPt 把 POINT 塞进一个 uintptr（存进 atomic；Windows 上按值传参时也正好是这个布局）
func packPt(p POINT) uintptr { return uintptr(uint32(p.X)) | uintptr(uint32(p.Y))<<32 }

func scale(v int32, dpi uint32) int32 { return int32((int64(v)*int64(dpi) + 48) / 96) }

// clampInto 让 w×h 的窗口落在 (x,y)，放不下就挪回工作区内；
// 下方放不下时翻到 flipY 之上。
func clampInto(x, y, w, h, flipY int32, wa RECT) (int32, int32) {
	if x+w > wa.Right {
		x = wa.Right - w
	}
	if x < wa.Left {
		x = wa.Left
	}
	if y+h > wa.Bottom {
		y = flipY - h
	}
	if y < wa.Top {
		y = wa.Top
	}
	return x, y
}

const (
	VK_LBUTTON = 0x01
	VK_RBUTTON = 0x02
	VK_SHIFT   = 0x10
	VK_CONTROL = 0x11
	VK_MENU    = 0x12
	VK_LWIN    = 0x5B // macOS 上是左 ⌘
	VK_RWIN    = 0x5C // macOS 上是右 ⌘

	vkReturn = 0x0D
	vkEscape = 0x1B
	vkFn     = 0x0E // Windows 没用的码，借给 macOS 的 fn / 🌐 键
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
	time  uint32 // 毫秒，只拿来比两次松开的间隔
	ours  bool
	ibeam bool
	vk    uint32 // keyPress：按的哪个键
}

var mouseEvents = make(chan mouseEvent, 256)

// kbdLLHook 一次按键（字段和 Windows 的 KBDLLHOOKSTRUCT 一样，macOS 上由事件监听填）
type kbdLLHook struct {
	VkCode, ScanCode, Flags, Time uint32
	ExtraInfo                     uintptr
}

// 卡片开着时拦 Enter / Esc：cardKeyMode 0=不拦 1=只拦 Esc 2=Enter 和 Esc 都拦；
// 只在焦点还在原来那个窗口（cardKeyFg）、没按着修饰键时拦，Shift+Enter 换行之类照常
var (
	cardKeyMode atomic.Int32
	cardKeyFg   atomic.Uintptr
	cardKeyFn   func(vk uint32)
)

func cardWantsKey(vk uint32) bool {
	mode := cardKeyMode.Load()
	if mode == 0 || cardKeyFn == nil || (vk != vkEscape && (vk != vkReturn || mode < 2)) {
		return false
	}
	for _, m := range []uintptr{VK_SHIFT, VK_CONTROL, VK_MENU, VK_LWIN, VK_RWIN} {
		if keyDown(m) {
			return false
		}
	}
	return foreground() == cardKeyFg.Load()
}

// isModifierVK Shift / Ctrl / Alt（含左右）、Win、CapsLock：单按这些不算改了选区
func isModifierVK(vk uint32) bool {
	switch vk {
	case 0x10, 0x11, 0x12, 0x14, 0x5B, 0x5C, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5:
		return true
	}
	return false
}

// keyCaptureJob 设置页正在录的一个快捷键框；report 在钩子线程调用，自己别阻塞
type keyCaptureJob struct {
	keyCapture
	hwnd   uintptr // 只在设置窗口在前台时接管键盘，切到别的程序照常打字
	report func(state int, acc, hint string)
}

var (
	keyCapturing atomic.Pointer[keyCaptureJob]
	captureHeld  [256]bool // 录制时按下的键：录完之后它们的松开也吞掉，别漏给前台窗口（仅钩子线程）
)

// captureKey 录制中把按键交给 keyCaptureJob 并吞掉：录 Alt+T 时现有的 Alt+T 不会触发，语音键也不会开始录音
func captureKey(vk uint32, down bool) bool {
	if !down && vk < 256 && captureHeld[vk] {
		captureHeld[vk] = false
		if job := keyCapturing.Load(); job == nil || foreground() != job.hwnd {
			return true
		}
	}
	job := keyCapturing.Load()
	if job == nil || foreground() != job.hwnd {
		return false
	}
	if down && vk < 256 {
		captureHeld[vk] = true
	}
	state, acc, hint := job.step(vk, down)
	if state != captureGoing {
		keyCapturing.CompareAndSwap(job, nil)
	}
	go job.report(state, acc, hint)
	return true
}

// focusInfo：焦点是不是有选中文字的输入框；sel 是选区最后一行在屏幕上的框
type focusInfo struct {
	ok     bool
	sel    RECT
	hasSel bool
	blind  bool // 有输入光标但位置是假的、选区也问不出（微信 4.x 这类 Qt 程序）：分不清输入框和聊天记录
}

// inputState Alt+T 没选中时用：焦点是不是能打字的输入框、里面是不是明确什么都没选
type inputState struct {
	input bool // 可写的输入框（Windows：可写的 ValuePattern，或者不是整页 Document 的 TextPattern）
	empty bool // 输入框里选区明确是空的：Ctrl+C 不会改剪贴板，不用白等 600ms
}

const (
	selUnknown = iota // 问不出来就放行，宁可多弹也别在能用的地方不弹
	selEmpty
	selText
)

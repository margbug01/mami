//go:build darwin

package app

import (
	"errors"
	"log"
	"math"
	"runtime"
	"time"

	"github.com/ebitengine/purego"
)

// macOS 的全局鼠标/键盘监听：CGEventTap 挂在自己的线程和 run loop 上。
// 能吞按键的 tap 要「辅助功能」权限；没给就先报错，后台每秒看一次，给了就挂上，不用重启。

const (
	evLeftDown     = 1
	evLeftUp       = 2
	evRightDown    = 3
	evKeyDown      = 10
	evKeyUp        = 11
	evFlagsChanged = 12
	evScroll       = 22
	evOtherDown    = 25

	evTapDisabledTimeout = 0xFFFFFFFE
	evTapDisabledUser    = 0xFFFFFFFF

	fieldAutorepeat = 8
	fieldKeycode    = 9
	fieldUserData   = 42 // kCGEventSourceUserData：sendKeys 发的事件在这里带 ourKeyTag

	ourKeyTag = 0x48554143
)

var (
	tapPort     uintptr // 只在钩子线程上写一次
	tapCallback = purego.NewCallback(tapProc)
	isOursFn    func(POINT) bool
	tapEpoch    = time.Now()
)

func nowMs() uint32 { return uint32(time.Since(tapEpoch).Milliseconds()) }

func tapProc(proxy, typ, ev, user uintptr) uintptr {
	switch uint32(typ) {
	case evTapDisabledTimeout, evTapDisabledUser:
		// 回调太慢或系统暂停了监听：马上接着听，不然按键全漏
		cgEventTapEnable(tapPort, true)
		return ev
	case evKeyDown, evKeyUp, evFlagsChanged:
		return tapKey(uint32(typ), ev)
	}
	kind := 0
	switch uint32(typ) {
	case evLeftDown:
		kind = mouseDown
	case evLeftUp:
		kind = mouseUp
	case evRightDown, evOtherDown:
		kind = mouseOther
	case evScroll:
		kind = mouseWheel
	}
	if kind == 0 {
		return ev
	}
	p := cgEventGetLocation(ev)
	me := mouseEvent{kind: kind, pt: POINT{int32(math.Round(p.X)), int32(math.Round(p.Y))}, time: nowMs()}
	withPool(func() {
		if kind == mouseDown || kind == mouseUp {
			me.ibeam = cursorIsIBeam()
		}
		if isOursFn != nil {
			me.ours = isOursFn(me.pt)
		}
	})
	select {
	case mouseEvents <- me:
	default: // 处理不过来就丢，回调绝不能卡住
	}
	return ev
}

func tapKey(typ uint32, ev uintptr) uintptr {
	if cgEventGetIntegerField(ev, fieldUserData) == ourKeyTag {
		return ev
	}
	code := uint16(cgEventGetIntegerField(ev, fieldKeycode))
	vk, down := macKeyVK(typ, code, cgEventGetFlags(ev))
	if vk == 0 {
		return ev
	}
	k := &kbdLLHook{VkCode: vk, Time: nowMs()}
	if captureKey(vk, down) || voiceHookKey(k, down) {
		return 0 // 吞掉
	}
	if typ == evKeyDown {
		if cardWantsKey(vk) {
			go cardKeyFn(vk)
			return 0 // 吞掉：别让 Enter 把没翻译的原文发出去
		}
		if !isModifierVK(vk) {
			select {
			case mouseEvents <- mouseEvent{kind: keyPress, time: k.Time, vk: vk}:
			default:
			}
		}
	}
	return ev
}

// modifierBits flagsChanged 时靠「这个键自己那一位」判断是按下还是松开（区分左右）
var modifierBits = map[uint16]uint64{
	0x3B: 0x1, 0x3E: 0x2000, // ⌃ 左右
	0x38: 0x2, 0x3C: 0x4, // ⇧ 左右
	0x3A: 0x20, 0x3D: 0x40, // ⌥ 左右
	0x37: 0x8, 0x36: 0x10, // ⌘ 左右
	0x3F: flagFn, // fn / 🌐
}

// macKeyVK 把一次 macOS 按键换成 VK 码；认不出（含 CapsLock：它只报切换不报松开）返回 0
func macKeyVK(typ uint32, code uint16, flags uint64) (vk uint32, down bool) {
	vk = macToVK[code]
	if typ != evFlagsChanged {
		return vk, typ == evKeyDown
	}
	bit, ok := modifierBits[code]
	if !ok {
		return 0, false
	}
	return vk, flags&bit != 0
}

// cursorIsIBeam 系统当前光标是不是文本 I 形：比热点和图片大小（比图片内容太贵）
func cursorIsIBeam() bool {
	cur := send(class("NSCursor"), "currentSystemCursor")
	if cur == 0 {
		return false
	}
	ib := send(class("NSCursor"), "IBeamCursor")
	if msgPoint(cur, sel("hotSpot")) != msgPoint(ib, sel("hotSpot")) {
		return false
	}
	return msgSize(send(cur, "image"), sel("size")) == msgSize(send(ib, "image"), sel("size"))
}

func startHook(isOurs func(POINT) bool, onCardKey func(vk uint32)) error {
	isOursFn = isOurs
	cardKeyFn = onCardKey
	ready := make(chan error, 2)
	go func() {
		runtime.LockOSThread()
		if !axIsProcessTrusted() {
			// 不在启动时弹系统对话框：新手引导的权限那屏（或设置「通用」页）点「去打开」才弹
			ready <- errNeedAccessibility
			for !axIsProcessTrusted() {
				time.Sleep(time.Second)
			}
			log.Println("hook: accessibility granted")
		}
		var mask uint64
		for _, t := range []uint{evLeftDown, evLeftUp, evRightDown, evOtherDown, evScroll, evKeyDown, evKeyUp, evFlagsChanged} {
			mask |= 1 << t
		}
		// kCGSessionEventTap, kCGHeadInsertEventTap, kCGEventTapOptionDefault（能吞事件）
		port := cgEventTapCreate(1, 0, 0, mask, tapCallback, 0)
		if port == 0 {
			ready <- errors.New("CGEventTapCreate failed")
			return
		}
		tapPort = port
		cfRunLoopAddSource(cfRunLoopGetCurrent(), cfMachPortCreateRLS(0, port, 0), kCFRunLoopCommonModes)
		cgEventTapEnable(port, true)
		log.Println("hook: event tap on")
		ready <- nil
		cfRunLoopRun()
		log.Println("hook thread exit")
	}()
	return <-ready
}

// postKey 发一次按下+松开；flags 是这次按键要带的修饰键（不带用户手上按着的）。
// 带 ourKeyTag，自己的监听认得出来，不当成用户按键
func postKey(code uint16, flags uint64) {
	for _, down := range []bool{true, false} {
		ev := cgEventCreateKeyboard(0, code, down)
		if ev == 0 {
			return
		}
		cgEventSetFlags(ev, flags)
		cgEventSetIntegerField(ev, fieldUserData, ourKeyTag)
		cgEventPost(0, ev) // kCGHIDEventTap
		cfRelease(ev)
	}
}

// macToVK / vkToMac：macOS 的虚拟键码（按 ANSI 键位）和 Windows VK 码互换
var (
	macToVK = map[uint16]uint32{}
	vkToMac = map[uint32]uint16{}
)

func init() {
	pairs := [][2]uint32{
		// 字母、数字
		{0x00, 'A'}, {0x0B, 'B'}, {0x08, 'C'}, {0x02, 'D'}, {0x0E, 'E'}, {0x03, 'F'}, {0x05, 'G'}, {0x04, 'H'},
		{0x22, 'I'}, {0x26, 'J'}, {0x28, 'K'}, {0x25, 'L'}, {0x2E, 'M'}, {0x2D, 'N'}, {0x1F, 'O'}, {0x23, 'P'},
		{0x0C, 'Q'}, {0x0F, 'R'}, {0x01, 'S'}, {0x11, 'T'}, {0x20, 'U'}, {0x09, 'V'}, {0x0D, 'W'}, {0x07, 'X'},
		{0x10, 'Y'}, {0x06, 'Z'},
		{0x1D, '0'}, {0x12, '1'}, {0x13, '2'}, {0x14, '3'}, {0x15, '4'}, {0x17, '5'}, {0x16, '6'}, {0x1A, '7'}, {0x1C, '8'}, {0x19, '9'},
		// 标点（VK_OEM_*）
		{0x29, 0xBA}, {0x18, 0xBB}, {0x2B, 0xBC}, {0x1B, 0xBD}, {0x2F, 0xBE}, {0x2C, 0xBF}, {0x32, 0xC0},
		{0x21, 0xDB}, {0x2A, 0xDC}, {0x1E, 0xDD}, {0x27, 0xDE},
		// 编辑、方向
		{0x24, 0x0D}, {0x4C, 0x0D}, {0x30, 0x09}, {0x31, 0x20}, {0x33, 0x08}, {0x35, 0x1B}, {0x75, 0x2E}, {0x72, 0x2D},
		{0x73, 0x24}, {0x77, 0x23}, {0x74, 0x21}, {0x79, 0x22}, {0x7B, 0x25}, {0x7E, 0x26}, {0x7C, 0x27}, {0x7D, 0x28},
		// 修饰键：左右分开，fn 借 vkFn
		{0x38, 0xA0}, {0x3C, 0xA1}, {0x3B, 0xA2}, {0x3E, 0xA3}, {0x3A, 0xA4}, {0x3D, 0xA5}, {0x37, VK_LWIN}, {0x36, VK_RWIN}, {0x3F, vkFn},
		// 小键盘
		{0x52, 0x60}, {0x53, 0x61}, {0x54, 0x62}, {0x55, 0x63}, {0x56, 0x64}, {0x57, 0x65}, {0x58, 0x66}, {0x59, 0x67}, {0x5B, 0x68}, {0x5C, 0x69},
		{0x43, 0x6A}, {0x45, 0x6B}, {0x4E, 0x6D}, {0x41, 0x6E}, {0x4B, 0x6F},
		// F1–F20
		{0x7A, 0x70}, {0x78, 0x71}, {0x63, 0x72}, {0x76, 0x73}, {0x60, 0x74}, {0x61, 0x75}, {0x62, 0x76}, {0x64, 0x77},
		{0x65, 0x78}, {0x6D, 0x79}, {0x67, 0x7A}, {0x6F, 0x7B}, {0x69, 0x7C}, {0x6B, 0x7D}, {0x71, 0x7E}, {0x6A, 0x7F},
		{0x40, 0x80}, {0x4F, 0x81}, {0x50, 0x82}, {0x5A, 0x83},
		// 音量
		{0x4A, 0xAD}, {0x49, 0xAE}, {0x48, 0xAF},
	}
	for _, p := range pairs {
		code, vk := uint16(p[0]), p[1]
		macToVK[code] = vk
		if _, dup := vkToMac[vk]; !dup { // 回车有主键盘和小键盘两个，发键用主键盘那个
			vkToMac[vk] = code
		}
	}
}

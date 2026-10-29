//go:build darwin

package app

import (
	_ "embed"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// macOS 上的「窗口句柄」：Mami 自己的窗口是 NSWindow*（mygo 的 NativeHandle），
// 别的程序的窗口是 CGWindowID（很小的整数），前台程序没有窗口时用 pidHandle|pid。
// 三种值不会撞：CGWindowID < 2^32，NSWindow* 是 64 位进程里的堆地址（> 2^32 且 < 2^47）。
const pidHandle = 1 << 62

var (
	cgMainDisplayID func() uint32
	cgDisplayBounds func(display uint32) cgRect
	cgRectFromDict  func(dict uintptr, r *cgRect) bool
	msgSetFloat     func(obj objc.ID, sel objc.SEL, v float64)

	kWinOwnerPID, kWinLayer, kWinNumber, kWinBounds uintptr
)

func init() {
	purego.RegisterLibFunc(&cgMainDisplayID, libCG, "CGMainDisplayID")
	purego.RegisterLibFunc(&cgDisplayBounds, libCG, "CGDisplayBounds")
	purego.RegisterLibFunc(&cgRectFromDict, libCG, "CGRectMakeWithDictionaryRepresentation")
	purego.RegisterFunc(&msgSetFloat, msgSendAddr)
	kWinOwnerPID = dlvar(libCG, "kCGWindowOwnerPID")
	kWinLayer = dlvar(libCG, "kCGWindowLayer")
	kWinNumber = dlvar(libCG, "kCGWindowNumber")
	kWinBounds = dlvar(libCG, "kCGWindowBounds")
}

const onMac = true

const excludeExample = "Steam.app, Terminal.app" // 设置里「不弹的程序」框的示例

//go:embed assets/tray_mac.png
var trayTemplatePNG []byte

// trayIcon 菜单栏用黑白模板图，系统按菜单栏深浅自己上色
func trayIcon() (png []byte, template bool) { return trayTemplatePNG, true }

func init() {
	// macOS 上单按左右 ⌘ 或 fn 也能当语音触发键（Windows 上单按 Win 会弹开始菜单，不给用）
	voiceKeyNames["LWin"] = VK_LWIN
	voiceKeyNames["RWin"] = VK_RWIN
	voiceKeyNames["Fn"] = vkFn
}

func nsApp() objc.ID { return send(class("NSApplication"), "sharedApplication") }

// primaryHeight 主屏高度（点）：Cocoa 坐标（左下角原点）和屏幕坐标（左上角原点）互换用。
// 只用 CoreGraphics，哪个线程都能调
func primaryHeight() float64 { return cgDisplayBounds(cgMainDisplayID()).Size.H }

func toCocoa(x, y, w, h int32) cgRect {
	return cgRect{cgPoint{float64(x), primaryHeight() - float64(y+h)}, cgSize{float64(w), float64(h)}}
}

func round32(v float64) int32 { return int32(math.Round(v)) }

// cursorPos 鼠标位置（屏幕坐标，点）
func cursorPos() POINT {
	ev := cgEventCreate(0)
	if ev == 0 {
		return POINT{}
	}
	defer cfRelease(ev)
	p := cgEventGetLocation(ev)
	return POINT{round32(p.X), round32(p.Y)}
}

func dpiAt(POINT) uint32 { return 96 }

func workArea(p POINT) RECT {
	wa, _, _ := dockMonitor(p)
	return wa
}

// windowRect Mami 自己窗口的位置（屏幕坐标）
func windowRect(h uintptr) RECT {
	if h == 0 {
		return RECT{}
	}
	f := msgRect(objc.ID(h), sel("frame"))
	top := primaryHeight() - f.Origin.Y - f.Size.H
	return RECT{round32(f.Origin.X), round32(top), round32(f.Origin.X + f.Size.W), round32(top + f.Size.H)}
}

const (
	nsStatusWindowLevel = 25 // 菜单栏那一层：盖过普通窗口和程序坞

	collectionCanJoinAllSpaces    = 1 << 0
	collectionTransient           = 1 << 3
	collectionIgnoresCycle        = 1 << 6
	collectionFullScreenAuxiliary = 1 << 8
)

// floatAbove 浮窗跟着每个桌面空间走（含全屏程序），压在别的程序的窗口上面，不进 ⌘Tab
func floatAbove(win objc.ID) {
	send(win, "setLevel:", nsStatusWindowLevel)
	send(win, "setCollectionBehavior:", collectionCanJoinAllSpaces|collectionTransient|collectionIgnoresCycle|collectionFullScreenAuxiliary)
	send(win, "setHidesOnDeactivate:", 0)
}

// makeFloating mygo 的 Flyout 本来就是不激活的 NSPanel，这里只把它抬到菜单栏那一层
func makeFloating(h uintptr) {
	mygo.RunOnMain(func() { floatAbove(objc.ID(h)) })
}

// makeAnchor 浮窗显示时会挂成锚点窗口的子窗口，子窗口跟着父窗口的层级、父窗口藏着它也出不来。
// 所以锚点在 macOS 上是「看得见」的：1×1、全透明、不接鼠标，放在主屏左上角菜单栏底下
func makeAnchor(h uintptr) {
	mygo.RunOnMain(func() {
		win := objc.ID(h)
		floatAbove(win)
		msgSetFloat(win, sel("setAlphaValue:"), 0)
		send(win, "setIgnoresMouseEvents:", 1)
		msgSetRectBool(win, sel("setFrame:display:"), toCocoa(0, 0, 1, 1), false)
		send(win, "orderFrontRegardless")
	})
}

// moveTo / placeAt 改窗口位置必须在主线程（AppKit 在别的线程改会直接抛异常）
func moveTo(h uintptr, x, y int32) {
	mygo.RunOnMain(func() {
		f := msgRect(objc.ID(h), sel("frame"))
		r := toCocoa(x, y, round32(f.Size.W), round32(f.Size.H))
		msgSetRectBool(objc.ID(h), sel("setFrame:display:"), r, true)
	})
}

func placeAt(h uintptr, x, y, w, ht int32) {
	mygo.RunOnMain(func() {
		msgSetRectBool(objc.ID(h), sel("setFrame:display:"), toCocoa(x, y, w, ht), true)
	})
}

// rootWindowAt 这一点最上层的窗口：是 Mami 自己的就返回它的 NSWindow*，别人的返回 0。
// 钩子线程上调，只读 AppKit，不等主线程
func rootWindowAt(p POINT) uintptr {
	n := msgPointInt(class("NSWindow"), sel("windowNumberAtPoint:belowWindowWithWindowNumber:"),
		cgPoint{float64(p.X), primaryHeight() - float64(p.Y)}, 0)
	if n <= 0 {
		return 0
	}
	return uintptr(send(nsApp(), "windowWithWindowNumber:", uintptr(n)))
}

func doubleClickTime() uint32 {
	return uint32(msgFloat(class("NSEvent"), sel("doubleClickInterval")) * 1000)
}

// cgWindow CGWindowListCopyWindowInfo 里的一个窗口（不需要屏幕录制权限的那几项）
type cgWindow struct {
	number uint32
	pid    int32
	layer  int
	bounds RECT
}

// listWindows option 见 CGWindowListOption；从前往后排
func listWindows(option, relativeTo uint32) []cgWindow {
	var out []cgWindow
	withPool(func() {
		arr := cgWindowListCopyInfo(option, relativeTo)
		if arr == 0 {
			return
		}
		defer cfRelease(arr)
		a := objc.ID(arr)
		for i := 0; i < int(send(a, "count")); i++ {
			d := send(a, "objectAtIndex:", uintptr(i))
			w := cgWindow{
				number: uint32(send(send(d, "objectForKey:", kWinNumber), "unsignedIntValue")),
				pid:    int32(send(send(d, "objectForKey:", kWinOwnerPID), "intValue")),
				layer:  int(int32(send(send(d, "objectForKey:", kWinLayer), "intValue"))),
			}
			var r cgRect
			if cgRectFromDict(uintptr(send(d, "objectForKey:", kWinBounds)), &r) {
				w.bounds = RECT{round32(r.Origin.X), round32(r.Origin.Y), round32(r.Origin.X + r.Size.W), round32(r.Origin.Y + r.Size.H)}
			}
			out = append(out, w)
		}
	})
	return out
}

const (
	cgListOnScreen       = 1 << 0
	cgListIncludingWin   = 1 << 3
	cgListExcludeDesktop = 1 << 4
)

// frontWindowOf pid 最前面的普通窗口（第 0 层）
func frontWindowOf(pid int32) (cgWindow, bool) {
	for _, w := range listWindows(cgListOnScreen|cgListExcludeDesktop, 0) {
		if w.pid == pid && w.layer == 0 {
			return w, true
		}
	}
	return cgWindow{}, false
}

func frontmostPID() int32 {
	var pid int32
	withPool(func() {
		if app := send(send(class("NSWorkspace"), "sharedWorkspace"), "frontmostApplication"); app != 0 {
			pid = int32(send(app, "processIdentifier"))
		}
	})
	return pid
}

// foreground 前台窗口。前台是 Mami 自己时返回它的主窗口（设置窗口），和 setHwnd 比得上
func foreground() uintptr {
	pid := frontmostPID()
	if pid <= 0 {
		return 0
	}
	if int(pid) == os.Getpid() {
		if w := send(nsApp(), "keyWindow"); w != 0 {
			return uintptr(w)
		}
	}
	if w, ok := frontWindowOf(pid); ok {
		return uintptr(w.number)
	}
	return pidHandle | uintptr(uint32(pid))
}

func windowPID(h uintptr) uint32 {
	switch {
	case h == 0:
		return 0
	case h&pidHandle != 0:
		return uint32(h)
	case h < 1<<32:
		if ws := listWindows(cgListIncludingWin, uint32(h)); len(ws) > 0 {
			return uint32(ws[0].pid)
		}
		return 0
	}
	return uint32(os.Getpid())
}

// processName 程序的 .app 路径（如 /Applications/WeChat.app），没有包就是可执行文件路径。
// 调用方取 filepath.Base 和按程序名的表比，所以表里 macOS 的程序写成 "wechat.app" 这样
func processName(pid uint32) string {
	var s string
	withPool(func() {
		app := send(class("NSRunningApplication"), "runningApplicationWithProcessIdentifier:", uintptr(int32(pid)))
		if app == 0 {
			return
		}
		for _, key := range []string{"bundleURL", "executableURL"} {
			if u := send(app, key); u != 0 {
				if s = goString(send(u, "path")); s != "" {
					return
				}
			}
		}
	})
	return s
}

// windowTitle 前台程序焦点窗口的标题，走辅助功能（CGWindowList 里的标题要屏幕录制权限）
func windowTitle(h uintptr) string {
	pid := windowPID(h)
	if pid == 0 {
		return ""
	}
	app := axCreateApplication(int32(pid))
	if app == 0 {
		return ""
	}
	defer cfRelease(app)
	axSetMessagingTimeout(app, axTimeout)
	win := axCopy(app, "AXFocusedWindow")
	if win == 0 {
		return ""
	}
	defer cfRelease(win)
	return axString(win, "AXTitle")
}

// isTerminal 前台是不是终端：终端里 ⌘C 没选中时不出事，但全选/替换都不合适，和 Windows 一样只给卡片
func isTerminal(fg uintptr) bool {
	if fg == 0 {
		return false
	}
	switch strings.ToLower(filepath.Base(processName(windowPID(fg)))) {
	case "terminal.app", "iterm.app", "warp.app", "ghostty.app", "wezterm.app", "alacritty.app", "kitty.app", "hyper.app", "tabby.app":
		return true
	}
	return false
}

const (
	hidState = 1 // kCGEventSourceStateHIDSystemState：键盘真实的按下状态

	flagShift = 0x20000
	flagCtrl  = 0x40000
	flagAlt   = 0x80000
	flagCmd   = 0x100000
	flagFn    = 0x800000
)

// keyDown 按 VK 码问键现在是不是按着
func keyDown(vk uintptr) bool {
	switch vk {
	case VK_LBUTTON:
		return cgEventSourceButton(hidState, 0)
	case VK_RBUTTON:
		return cgEventSourceButton(hidState, 1)
	case VK_SHIFT:
		return cgEventSourceFlagsState(hidState)&flagShift != 0
	case VK_CONTROL:
		return cgEventSourceFlagsState(hidState)&flagCtrl != 0
	case VK_MENU:
		return cgEventSourceFlagsState(hidState)&flagAlt != 0
	case vkFn:
		return cgEventSourceFlagsState(hidState)&flagFn != 0
	}
	if code, ok := vkToMac[uint32(vk)]; ok {
		return cgEventSourceKeyState(hidState, code)
	}
	return false
}

// rightAltTaken macOS 上右 Option 单按没有别的用处（输入法切换走 CapsLock / ⌃空格 / 🌐），默认触发键照用
func rightAltTaken() bool { return false }

// muteAltMenu 单按 Option 不会激活菜单栏，不用补键
func muteAltMenu() {}

// migrateFromHuaci 改名前的 Huaci 只有 Windows 版
func migrateFromHuaci(string) {}

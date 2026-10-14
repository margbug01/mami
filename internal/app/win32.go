//go:build windows

package app

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

// MyGo 管窗口、托盘和快捷键；这里只剩它管不到的：全局鼠标钩子、
// 别的程序里的选区（剪贴板 + 模拟按键）、以及给浮窗补「不抢焦点」。

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")
	shcore   = syscall.NewLazyDLL("shcore.dll")

	pRegisterClassExW           = user32.NewProc("RegisterClassExW")
	pCreateWindowExW            = user32.NewProc("CreateWindowExW")
	pDefWindowProcW             = user32.NewProc("DefWindowProcW")
	pGetMessageW                = user32.NewProc("GetMessageW")
	pPeekMessageW               = user32.NewProc("PeekMessageW")
	pTranslateMessage           = user32.NewProc("TranslateMessage")
	pDispatchMessageW           = user32.NewProc("DispatchMessageW")
	pPostMessageW               = user32.NewProc("PostMessageW")
	pSetWindowPos               = user32.NewProc("SetWindowPos")
	pGetWindowRect              = user32.NewProc("GetWindowRect")
	pGetWindowLongPtrW          = user32.NewProc("GetWindowLongPtrW")
	pSetWindowLongPtrW          = user32.NewProc("SetWindowLongPtrW")
	pSetWindowsHookExW          = user32.NewProc("SetWindowsHookExW")
	pCallNextHookEx             = user32.NewProc("CallNextHookEx")
	pGetCursorPos               = user32.NewProc("GetCursorPos")
	pGetCursorInfo              = user32.NewProc("GetCursorInfo")
	pLoadCursorW                = user32.NewProc("LoadCursorW")
	pWindowFromPoint            = user32.NewProc("WindowFromPoint")
	pGetAncestor                = user32.NewProc("GetAncestor")
	pGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	pMonitorFromPoint           = user32.NewProc("MonitorFromPoint")
	pGetMonitorInfoW            = user32.NewProc("GetMonitorInfoW")
	pOpenClipboard              = user32.NewProc("OpenClipboard")
	pCloseClipboard             = user32.NewProc("CloseClipboard")
	pEmptyClipboard             = user32.NewProc("EmptyClipboard")
	pGetClipboardData           = user32.NewProc("GetClipboardData")
	pSetClipboardData           = user32.NewProc("SetClipboardData")
	pGetClipboardSequenceNumber = user32.NewProc("GetClipboardSequenceNumber")
	pEnumClipboardFormats       = user32.NewProc("EnumClipboardFormats")
	pRegisterClipboardFormatW   = user32.NewProc("RegisterClipboardFormatW")
	pSendInput                  = user32.NewProc("SendInput")
	pGetClassNameW              = user32.NewProc("GetClassNameW")
	pGetAsyncKeyState           = user32.NewProc("GetAsyncKeyState")
	pGetDoubleClickTime         = user32.NewProc("GetDoubleClickTime")
	pGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")

	pGlobalAlloc                = kernel32.NewProc("GlobalAlloc")
	pGlobalLock                 = kernel32.NewProc("GlobalLock")
	pGlobalUnlock               = kernel32.NewProc("GlobalUnlock")
	pGlobalSize                 = kernel32.NewProc("GlobalSize")
	pGlobalFree                 = kernel32.NewProc("GlobalFree")
	pGetModuleHandleW           = kernel32.NewProc("GetModuleHandleW")
	pOpenProcess                = kernel32.NewProc("OpenProcess")
	pCloseHandle                = kernel32.NewProc("CloseHandle")
	pQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")

	pDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	pSetWindowRgn          = user32.NewProc("SetWindowRgn")
	pCreateRoundRectRgn    = gdi32.NewProc("CreateRoundRectRgn")
	pGetDpiForWindow       = user32.NewProc("GetDpiForWindow")
	pGetDpiForMonitor      = shcore.NewProc("GetDpiForMonitor")
)

const (
	WM_LBUTTONDOWN = 0x0201
	WM_LBUTTONUP   = 0x0202
	WM_RBUTTONDOWN = 0x0204
	WM_MBUTTONDOWN = 0x0207
	WM_MOUSEWHEEL  = 0x020A
	WM_XBUTTONDOWN = 0x020B
	WM_MOUSEHWHEEL = 0x020E
	WM_APP         = 0x8000

	WH_MOUSE_LL     = 14
	GA_ROOT         = 2
	IDC_IBEAM       = 32513
	MONITOR_NEAREST = 2
	CF_UNICODETEXT  = 13
	GMEM_MOVEABLE   = 0x0002
	INPUT_KEYBOARD  = 1
	KEYEVENTF_KEYUP = 0x0002
	VK_LBUTTON      = 0x01
	VK_SHIFT        = 0x10
	VK_CONTROL      = 0x11
	VK_MENU         = 0x12
	VK_LWIN         = 0x5B
	VK_RWIN         = 0x5C
	VK_MASK         = 0xE8 // 未分配的虚拟键：松开 Alt 时不让菜单栏被激活

	GWL_EXSTYLE              = ^uintptr(19)                                                   // -20
	GWL_STYLE                = ^uintptr(15)                                                   // -16
	WS_FRAME_BITS            = 0x00C00000 | 0x00080000 | 0x00040000 | 0x00020000 | 0x00010000 // CAPTION|SYSMENU|THICKFRAME|MINIMIZEBOX|MAXIMIZEBOX
	WS_EX_TOOLWINDOW         = 0x00000080
	WS_EX_NOACTIVATE         = 0x08000000
	HWND_TOPMOST             = ^uintptr(0) // -1
	HWND_MESSAGE             = ^uintptr(2) // -3
	SWP_NOSIZE               = 0x0001
	SWP_NOACTIVATE           = 0x0010
	SWP_FRAMECHANGED         = 0x0020
	SWP_NOMOVE               = 0x0002
	DWMWA_CORNER_PREF        = 33
	DWMWA_NCRENDERING_POLICY = 2
)

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }

type MSG struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type MSLLHOOKSTRUCT struct {
	Pt          POINT
	MouseData   uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type CURSORINFO struct {
	CbSize      uint32
	Flags       uint32
	HCursor     uintptr
	PtScreenPos POINT
}

type MONITORINFO struct {
	CbSize    uint32
	RcMonitor RECT
	RcWork    RECT
	DwFlags   uint32
}

type KEYBDINPUT struct {
	WVk         uint16
	WScan       uint16
	DwFlags     uint32
	Time        uint32
	DwExtraInfo uintptr
}

// INPUT 在 amd64 上是 40 字节：type(4)+pad(4)+union(32)
type INPUT struct {
	Type uint32
	_    uint32
	Ki   KEYBDINPUT
	_    [8]byte
}

func u16(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		p, _ = syscall.UTF16PtrFromString("")
	}
	return p
}

// POINT 按值传参时在 amd64 上就是一个 8 字节整数
func packPt(p POINT) uintptr { return uintptr(uint32(p.X)) | uintptr(uint32(p.Y))<<32 }

func cursorPos() POINT {
	var p POINT
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

func foreground() uintptr {
	r, _, _ := pGetForegroundWindow.Call()
	return r
}

func dpiAt(p POINT) uint32 {
	mon, _, _ := pMonitorFromPoint.Call(packPt(p), MONITOR_NEAREST)
	var dx, dy uint32
	if r, _, _ := pGetDpiForMonitor.Call(mon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r != 0 || dx == 0 {
		return 96
	}
	return dx
}

func workArea(p POINT) RECT {
	mon, _, _ := pMonitorFromPoint.Call(packPt(p), MONITOR_NEAREST)
	mi := MONITORINFO{CbSize: uint32(unsafe.Sizeof(MONITORINFO{}))}
	pGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	return mi.RcWork
}

func scale(v int32, dpi uint32) int32 { return int32((int64(v)*int64(dpi) + 48) / 96) }

func windowRect(h uintptr) RECT {
	var r RECT
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	return r
}

// makeFloating 把 MyGo 的无边框窗口改成浮窗：点它不会激活它，
// 原来窗口里的选区和光标都还在；也不进 Alt+Tab。
func makeFloating(h uintptr) {
	defer roundCorners(h)
	// MyGo 的无边框窗口仍带 WS_CAPTION，系统据此强制最小尺寸（150% 下约 50×47），小按钮缩不下去
	st, _, _ := pGetWindowLongPtrW.Call(h, GWL_STYLE)
	pSetWindowLongPtrW.Call(h, GWL_STYLE, st&^WS_FRAME_BITS)
	ex, _, _ := pGetWindowLongPtrW.Call(h, GWL_EXSTYLE)
	pSetWindowLongPtrW.Call(h, GWL_EXSTYLE, ex|WS_EX_NOACTIVATE|WS_EX_TOOLWINDOW)
	pSetWindowPos.Call(h, HWND_TOPMOST, 0, 0, 0, 0, SWP_NOMOVE|SWP_NOSIZE|SWP_NOACTIVATE|SWP_FRAMECHANGED)
	pref := int32(1) // 不用系统圆角：Win11 会给圆角窗口配一圈投影，圆角改用 SetWindowRgn 自己裁
	pDwmSetWindowAttribute.Call(h, DWMWA_CORNER_PREF, uintptr(unsafe.Pointer(&pref)), 4)
	// 关掉系统给无边框窗口加的阴影（按钮和卡片四周那圈暗影）
	nc := int32(1) // DWMNCRP_DISABLED
	pDwmSetWindowAttribute.Call(h, DWMWA_NCRENDERING_POLICY, uintptr(unsafe.Pointer(&nc)), 4)
}

// moveTo 按物理像素摆窗口（钩子和 GetCursorPos 给的都是物理像素）
func moveTo(h uintptr, x, y int32) {
	defer roundCorners(h)
	pSetWindowPos.Call(h, HWND_TOPMOST, uintptr(x), uintptr(y), 0, 0, SWP_NOSIZE|SWP_NOACTIVATE)
}

// placeAt 连尺寸一起设（物理像素）
func placeAt(h uintptr, x, y, w, ht int32) {
	defer roundCorners(h)
	pSetWindowPos.Call(h, HWND_TOPMOST, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), SWP_NOACTIVATE)
}

func (r RECT) w() int32 { return r.Right - r.Left }
func (r RECT) h() int32 { return r.Bottom - r.Top }

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

func windowPID(h uintptr) uint32 {
	var pid uint32
	pGetWindowThreadProcessId.Call(h, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func processName(pid uint32) string {
	ph, _, _ := pOpenProcess.Call(0x1000, 0, uintptr(pid))
	if ph == 0 {
		return ""
	}
	defer pCloseHandle.Call(ph)
	buf := make([]uint16, 520)
	n := uint32(len(buf))
	if r, _, _ := pQueryFullProcessImageNameW.Call(ph, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

func keyDown(vk uintptr) bool {
	r, _, _ := pGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

// isTerminal 前台是不是终端：Windows Terminal / 老控制台按窗口类名认，其他常见终端按进程名认。
// 终端里 Ctrl+C 没选中时是「中断」信号，所以终端里绝不模拟 Ctrl+C / Ctrl+A。
func isTerminal(fg uintptr) bool {
	if fg == 0 {
		return false
	}
	var buf [128]uint16
	n, _, _ := pGetClassNameW.Call(fg, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	switch syscall.UTF16ToString(buf[:n]) {
	case "CASCADIA_HOSTING_WINDOW_CLASS", "ConsoleWindowClass", "PseudoConsoleWindow":
		return true
	}
	switch strings.ToLower(filepath.Base(processName(windowPID(fg)))) {
	case "windowsterminal.exe", "openconsole.exe", "wezterm-gui.exe", "alacritty.exe", "mintty.exe", "tabby.exe", "warp.exe", "conemu64.exe", "conemu.exe":
		return true
	}
	return false
}

// roundCorners 把窗口裁成圆角矩形（半径 10 DIP）。窗口区域之外的像素系统不画，
// 也就没有投影；窗口大小变了要重新裁。
func roundCorners(h uintptr) {
	var r RECT
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r)))
	w, ht := r.w(), r.h()
	if w <= 0 || ht <= 0 {
		return
	}
	dpi, _, _ := pGetDpiForWindow.Call(h)
	if dpi == 0 {
		dpi = 96
	}
	d := int32(dpi) * 20 / 96 // 圆角直径（物理像素）
	rgn, _, _ := pCreateRoundRectRgn.Call(0, 0, uintptr(w+1), uintptr(ht+1), uintptr(d), uintptr(d))
	if rgn != 0 {
		pSetWindowRgn.Call(h, rgn, 1) // 成功后区域归系统所有
	}
}

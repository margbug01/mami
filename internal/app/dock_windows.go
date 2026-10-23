//go:build windows

package app

import (
	"log"
	"syscall"
	"unsafe"
)

// 小条的拖动：子类化窗口过程，自己处理按下/移动/松开（dockNativeDrag=false）
const dockNativeDrag = false

var dockComctl = syscall.NewLazyDLL("comctl32.dll")
var dockDefSubclass = dockComctl.NewProc("DefSubclassProc")
var dockOwners = map[uintptr]*App{}
var dockSubclass = syscall.NewCallback(func(hwnd uintptr, message uint32, wp, lp, id, data uintptr) uintptr {
	if message == 0x21 {
		return 3
	}
	if a := dockOwners[hwnd]; a != nil {
		if message == 0x82 {
			delete(dockOwners, hwnd)
		} else if a.dockMouse(message) {
			return 0
		}
	}
	result, _, _ := dockDefSubclass.Call(hwnd, uintptr(message), wp, lp)
	return result
})

var dockChildSubclass = syscall.NewCallback(func(hwnd, parent uintptr) uintptr {
	if a := dockOwners[parent]; a != nil {
		a.hookDockWindow(hwnd)
	}
	return 1
})

func (a *App) hookDockWindow(hwnd uintptr) {
	if ok, _, err := dockComctl.NewProc("SetWindowSubclass").Call(hwnd, dockSubclass, 1, 0); ok == 0 {
		log.Println("dock mouse hook:", err)
		return
	}
	dockOwners[hwnd] = a
}

func (a *App) hookDockMouse() {
	a.hookDockWindow(a.dock.hwnd)
	user32.NewProc("EnumChildWindows").Call(a.dock.hwnd, dockChildSubclass, a.dock.hwnd)
}

func (a *App) dockMouse(message uint32) bool {
	d := &a.dock
	switch message {
	case 0x201, 0x203:
		point := cursorPos()
		if !d.shown || !d.dragHit(point) {
			return false
		}
		d.down = point
		d.grab = POINT{X: point.X - d.rect.Left, Y: point.Y - d.rect.Top}
		d.armed, d.dragging = true, false
		user32.NewProc("SetCapture").Call(d.hwnd)
		return true
	case 0x200:
		if !d.armed {
			return false
		}
		point := cursorPos()
		if d.dragging || d.dragMoved(point) {
			d.dragging = true
			origin := d.dragPoint(point)
			pSetWindowPos.Call(d.hwnd, HWND_TOPMOST, uintptr(origin.X), uintptr(origin.Y), 0, 0, SWP_NOSIZE|SWP_NOACTIVATE)
			d.rect = windowRect(d.hwnd)
		}
		return true
	case 0x202:
		if d.armed {
			a.finishDockDrag(true)
			return true
		}
	case 0x215, 0x1F:
		if d.armed {
			a.finishDockDrag(false)
		}
	}
	return false
}

type dockMonitorInfo struct {
	MONITORINFO
	Device [32]uint16
}

func dockMonitor(point POINT) (RECT, RECT, string) {
	monitor, _, _ := pMonitorFromPoint.Call(packPt(point), MONITOR_NEAREST)
	info := dockMonitorInfo{}
	info.CbSize = uint32(unsafe.Sizeof(info))
	pGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
	return info.RcWork, info.RcMonitor, syscall.UTF16ToString(info.Device[:])
}

func dockSavedMonitor(device string) POINT {
	point := cursorPos()
	if device == "" {
		return point
	}
	callback := syscall.NewCallback(func(monitor, dc, rect, data uintptr) uintptr {
		info := dockMonitorInfo{}
		info.CbSize = uint32(unsafe.Sizeof(info))
		pGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&info)))
		if syscall.UTF16ToString(info.Device[:]) == device {
			point = POINT{X: info.RcWork.Left + info.RcWork.w()/2, Y: info.RcWork.Top + info.RcWork.h()/2}
			return 0
		}
		return 1
	})
	user32.NewProc("EnumDisplayMonitors").Call(0, 0, callback, 0)
	return point
}

func dockFullscreen() bool {
	hwnd := foreground()
	if hwnd == 0 {
		return false
	}
	var name [128]uint16
	user32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&name[0])), 128)
	switch syscall.UTF16ToString(name[:]) {
	case "Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd":
		return false
	}
	rect := windowRect(hwnd)
	_, monitor, _ := dockMonitor(POINT{X: rect.Left + rect.w()/2, Y: rect.Top + rect.h()/2})
	return rect.w() > 0 && rect.h() > 0 && rect.Left <= monitor.Left && rect.Top <= monitor.Top && rect.Right >= monitor.Right && rect.Bottom >= monitor.Bottom
}

func dockReleaseCapture(hwnd uintptr) {
	if capture, _, _ := user32.NewProc("GetCapture").Call(); capture == hwnd {
		user32.NewProc("ReleaseCapture").Call()
	}
}

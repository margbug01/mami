//go:build darwin

package app

import (
	"strconv"
	"time"

	"github.com/egoist/mygo"
)

// 小条在 macOS 上用系统的窗口拖动（ui 的 DragWindow → performWindowDragWithEvent），
// 松手后 nativeDockDrag 吸边；不用像 Windows 那样子类化窗口过程
const dockNativeDrag = true

func (a *App) hookDockMouse() {}

func dockReleaseCapture(uintptr) {}

func rectOf(r mygo.Rectangle) RECT {
	return RECT{int32(r.X), int32(r.Y), int32(r.X + r.Width), int32(r.Y + r.Height)}
}

func displayAt(p POINT) mygo.Display {
	for _, d := range mygo.Screen.Displays() {
		b := d.Bounds
		if int(p.X) >= b.X && int(p.X) < b.X+b.Width && int(p.Y) >= b.Y && int(p.Y) < b.Y+b.Height {
			return d
		}
	}
	return mygo.Screen.DisplayNearestPoint(mygo.Point{X: int(p.X), Y: int(p.Y)})
}

// dockMonitor 这一点所在显示器的工作区、整块区域和编号（CGDirectDisplayID，存进配置记住在哪块屏）
func dockMonitor(p POINT) (RECT, RECT, string) {
	d := displayAt(p)
	return rectOf(d.WorkArea), rectOf(d.Bounds), strconv.FormatInt(d.ID, 10)
}

func dockSavedMonitor(device string) POINT {
	for _, d := range mygo.Screen.Displays() {
		if device != "" && strconv.FormatInt(d.ID, 10) == device {
			wa := rectOf(d.WorkArea)
			return POINT{wa.Left + wa.w()/2, wa.Top + wa.h()/2}
		}
	}
	return cursorPos()
}

var (
	fullscreenAt   time.Time // 只在界面线程（tickDock）上读写
	fullscreenLast bool
)

// dockFullscreen 前台程序最前面的窗口盖满了整块屏（全屏看视频、演示）就把小条收起来。
// 查窗口列表比 Windows 贵，tickDock 每 40ms 调一次，结果留半秒
func dockFullscreen() bool {
	if time.Since(fullscreenAt) < 500*time.Millisecond {
		return fullscreenLast
	}
	fullscreenAt, fullscreenLast = time.Now(), false
	w, ok := frontWindowOf(frontmostPID())
	if !ok {
		return false
	}
	r := w.bounds
	_, full, _ := dockMonitor(POINT{r.Left + r.w()/2, r.Top + r.h()/2})
	fullscreenLast = r.w() > 0 && r.h() > 0 && r.Left <= full.Left && r.Top <= full.Top && r.Right >= full.Right && r.Bottom >= full.Bottom
	return fullscreenLast
}

package app

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 菜单栏面板（macOS）：点菜单栏里的 Mami 头像，在它正下方弹出和贴边小条展开后一样的面板——
// 开始说话、说完直接翻译、翻译成、润色档位、更多设置。Mac 上常驻的小工具都待在菜单栏里，
// 所以小条在 Mac 上默认关掉（设置和右键菜单里还能打开）。点面板外面就收起，再点一下头像也收起；
// 右键头像还是原来那个菜单

const (
	panelW     = 196
	panelMainH = 228 // 小条展开是 248，菜单栏面板不画顶上的把手（8 + 间距 12）
	panelMoreH = 396 // 同理 416 - 20
	panelGap   = 6   // 离菜单栏底边、离屏幕左右边
)

type trayPanel struct {
	win         *mygo.Window
	hwnd        uintptr
	shown       atomic.Bool
	icon        atomic.Pointer[RECT] // 打开时头像的位置：在头像上按下不算「点在外面」，松开时头像自己会把面板收起
	more, langs bool                 // 翻到哪页（界面线程）
	h           int32                // 现在的高度（界面线程）
}

// do 在面板的界面线程上跑（测试里没有窗口，就地跑）
func (p *trayPanel) do(fn func()) {
	if p.win == nil {
		fn()
		return
	}
	p.win.Update(fn)
}

func (a *App) createTrayPanel() {
	p := &a.panel
	p.h = panelMainH
	p.win, p.hwnd = a.floatingWindow("mami-tray-panel", panelW, panelMainH, 1, 1, a.trayPanelView)
	a.panelHwnd.Store(p.hwnd)
}

// panelHeight 面板翻到这一页要多高
func panelHeight(more, langs bool) int32 {
	if more || langs {
		return panelMoreH
	}
	return panelMainH
}

// panelRect 面板放在头像正下方、水平居中，左右不出工作区；拿不到头像的位置就放在工作区右上角
func panelRect(icon, area RECT, w, h int32) RECT {
	left, top := icon.Left+icon.w()/2-w/2, max(icon.Bottom, area.Top)+panelGap
	if icon.w() <= 0 {
		left, top = area.Right-panelGap-w, area.Top+panelGap
	}
	left = max(area.Left+panelGap, min(left, area.Right-panelGap-w))
	return RECT{Left: left, Top: top, Right: left + w, Bottom: top + h}
}

// toggleTrayPanel 点了菜单栏头像（界面线程）：开着就收起，没开就在头像下面打开
func (a *App) toggleTrayPanel() {
	p := &a.panel
	if p.win == nil || a.tray == nil {
		return
	}
	if p.shown.Load() {
		a.hideTrayPanel()
		return
	}
	b := a.tray.Bounds()
	icon := RECT{Left: int32(b.X), Top: int32(b.Y), Right: int32(b.X + b.Width), Bottom: int32(b.Y + b.Height)}
	at := POINT{X: icon.Left + icon.w()/2, Y: icon.Bottom}
	dpi := dpiAt(at)
	p.more, p.langs, p.h = false, false, panelMainH
	r := panelRect(icon, workArea(at), scale(panelW, dpi), scale(p.h, dpi))
	p.icon.Store(&icon)
	placeAt(p.hwnd, r.Left, r.Top, r.w(), r.h())
	p.win.ShowInactive() // 不抢焦点：点「开始说话」时字还是贴进原来那个输入框
	p.shown.Store(true)
	markBusy()
}

// hideTrayPanel 收起面板，下次打开从首页开始（界面线程）
func (a *App) hideTrayPanel() {
	p := &a.panel
	if !p.shown.Swap(false) {
		return
	}
	if p.win != nil {
		p.win.Hide()
	}
	p.more, p.langs = false, false
}

// trayPanelClick 鼠标在 Mami 自己的窗口以外按下（钩子那边的协程）：收起面板。按在头像上不管
func (a *App) trayPanelClick(pt POINT) {
	p := &a.panel
	if !p.shown.Load() {
		return
	}
	if icon := p.icon.Load(); icon != nil && dockContains(*icon, pt) {
		return
	}
	p.do(a.hideTrayPanel)
}

func (a *App) trayPanelView(c *ui.Context) {
	cfg, _ := a.config()
	p := &a.panel
	at := dockPanel{more: &p.more, langs: &p.langs, menubar: true, done: func() { p.do(a.hideTrayPanel) }}
	a.drawDock(c, cfg, true, p.more, toneDark(cfg.VoiceDockTone, a.themed(c).Dark), true, at)
	// 翻到「更多设置」「翻译成」要高一些：点的那一帧才知道，改完高度再画一帧
	if h := panelHeight(p.more, p.langs); h != p.h {
		p.h = h
		if p.shown.Load() && p.win != nil {
			r := windowRect(p.hwnd)
			placeAt(p.hwnd, r.Left, r.Top, r.w(), scale(h, dpiAt(POINT{X: r.Left, Y: r.Top})))
			p.win.Invalidate()
		}
	}
}

// ---------- 录音时的菜单栏头像 ----------

// trayRecording 录音的时候菜单栏头像右下角加个红点（代替小条上会呼吸的话筒），停了换回来。只在 macOS
func (a *App) trayRecording(on bool) {
	if !onMac || a.tray == nil || a.trayRec.Swap(on) == on {
		return
	}
	icon, template := trayIcon()
	if on {
		icon, template = recordingIcon(), false
	}
	if err := a.tray.SetIcon(icon, template); err != nil {
		a.trayRec.Store(!on)
	}
}

var recordingIcon = sync.OnceValue(func() []byte {
	icon, _ := trayIcon()
	return withRedDot(icon)
})

// 红点和白边的半径（占图标边长）：菜单栏里头像约 18 点高，红点直径约 6 点
const redDot, redDotRing = 0.17, 0.22

// withRedDot 在图标右下角画一个带白边的红点（像通知角标），抗锯齿；解不开就原样返回
func withRedDot(src []byte) []byte {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return src
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	size := float64(b.Dx())
	dot, ring := size*redDot, size*redDotRing
	cx, cy := float64(b.Max.X)-ring-0.5, float64(b.Max.Y)-ring-0.5
	white, red := color.RGBA{255, 255, 255, 255}, color.RGBA{255, 59, 48, 255} // systemRed
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if d > ring+0.5 {
				continue
			}
			px := out.RGBAAt(x, y)
			px = blendOver(px, white, min(1, ring+0.5-d))
			px = blendOver(px, red, max(0, min(1, dot+0.5-d)))
			out.SetRGBA(x, y, px)
		}
	}
	var buf bytes.Buffer
	if png.Encode(&buf, out) != nil {
		return src
	}
	return buf.Bytes()
}

// blendOver 不透明的颜色 c 按覆盖率 k 盖在 dst（预乘 alpha）上
func blendOver(dst, c color.RGBA, k float64) color.RGBA {
	mix := func(d, s uint8) uint8 { return uint8(math.Round(float64(d)*(1-k) + float64(s)*k)) }
	return color.RGBA{mix(dst.R, c.R), mix(dst.G, c.G), mix(dst.B, c.B), mix(dst.A, 255)}
}

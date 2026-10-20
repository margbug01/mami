package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// 三岛样式（islands 三岛连桥 / leds 分段电平）：✕ | 内容 | ✓ 三块独立圆角岛，中间留缝。
// 状态切换就是缝的开合：录音→识别，右岛被吸进中间；识别→结果，左岛并进来，右边再吐出复制钮。

var (
	islandAccentDark  = ui.RGBA(140, 155, 255, 1)
	islandAccentLight = ui.RGBA(91, 108, 240, 1)
)

const islandGap = 4

func (sk voiceSkin) island() bool { return sk.style == "islands" || sk.style == "leds" }

// islandGapsFor 每个状态下左右两道缝的宽度，0 = 那块岛并进了中间
func islandGapsFor(state voiceViewState) (l, r float32) {
	switch voiceKind(state) {
	case 1:
		return islandGap, islandGap
	case 2:
		return islandGap, 0
	}
	if state.copy || state.retry || state.undo {
		return 0, islandGap
	}
	return 0, 0
}

// islandGaps 眼下的缝宽：从上次切换时的缝宽往当前状态过渡
func (a *App) islandGaps(now time.Time) (float32, float32) {
	l, r := islandGapsFor(a.voiceUI)
	t := ui.EaseOut(progress(now, a.voiceKindAt, voiceKindDur))
	return a.voiceGapL + (l-a.voiceGapL)*t, a.voiceGapR + (r-a.voiceGapR)*t
}

// drawIslands 先整体垫一层细边，再铺底色，缝合拢时补一段连接（bridge=true 时腰部往里收，像水滴被吸进去）
func drawIslands(p *ui.Painter, r ui.Rect, dark bool, gl, gr float32, bridge bool) {
	bg, edge := mercuryShell(dark)
	bg.A = 255 // 岛是实心的：半透明时背后的字会透出来
	const rad = 10
	x0, y0, w, h := r.X+1, r.Y+1, r.W-2, r.H-2
	s := h // 两边的岛是正方形
	rects := []ui.Rect{
		{X: x0, Y: y0, W: s, H: h},
		{X: x0 + s + gl, Y: y0, W: max(0, w-2*s-gl-gr), H: h},
		{X: x0 + w - s, Y: y0, W: s, H: h},
	}
	cy := y0 + h/2
	necks := [][3]float32{} // a, b, 缝宽
	if gr < 0.5 {           // 已经合上：画成一整块，免得接缝处的圆角和细边留下缺口
		rects[1].W += gr + s
		rects = rects[:2]
	} else if gr < islandGap {
		necks = append(necks, [3]float32{x0 + w - s - gr, x0 + w - s, gr})
	}
	if gl < 0.5 {
		rects[1].X, rects[1].W = x0, rects[1].W+s+gl
		rects = rects[1:]
	} else if gl < islandGap {
		necks = append(necks, [3]float32{x0 + s, x0 + s + gl, gl})
	}
	for pass := 0; pass < 2; pass++ {
		grow, col := float32(1), edge
		if pass == 1 {
			grow, col = 0, bg
		}
		for _, q := range rects {
			p.Fill(ui.Rect{X: q.X - grow, Y: q.Y - grow, W: q.W + 2*grow, H: q.H + 2*grow}, col, rad+grow)
		}
		for _, n := range necks {
			if path := neckPath(n[0], n[1], cy, h/2, n[2], grow, rad, bridge); path != nil {
				p.FillPath(path, col)
			}
		}
	}
}

// neckPath a、b 之间的连接：缝越窄越高，缝为 0 时是一整块，左右各伸进岛里 rad 盖住圆角留下的缺口
func neckPath(a, b, cy, half, gap, grow, rad float32, bridge bool) *ui.Path {
	s := 1 - gap/islandGap
	s = s * s * (3 - 2*s)
	hh := half*s + grow
	pinch := float32(0)
	if bridge {
		pinch = half * (1 - s) * 0.9
	}
	if hh-pinch/2 < 0.5 {
		return nil
	}
	m, xa, xb := (a+b)/2, a-rad, b+rad
	path := new(ui.Path)
	path.MoveTo(xa, cy-hh).QuadTo(m, cy-hh+pinch, xb, cy-hh).LineTo(xb, cy+hh).QuadTo(m, cy+hh-pinch, xa, cy+hh)
	return path.Close()
}

// drawEnvelope 三岛连桥的音量：最近 9 次音量连成一条上下对称的平滑包络，两头收细
func drawEnvelope(p *ui.Painter, r ui.Rect, heights [voiceBars]float32, acc ui.Color) {
	const inset = 6
	cy, maxE := r.Y+r.H/2, r.H/2-1
	var xs, es [voiceBars]float32
	for i, v := range heights {
		u := float32(i) / (voiceBars - 1)
		xs[i] = r.X + inset + (r.W-2*inset)*u
		es[i] = 1 + maxE*v*(0.35+0.65*float32(math.Sin(math.Pi*float64(u))))
	}
	path := new(ui.Path)
	path.MoveTo(xs[0], cy-es[0])
	for i := 1; i < voiceBars; i++ {
		path.QuadTo(xs[i-1], cy-es[i-1], (xs[i-1]+xs[i])/2, cy-(es[i-1]+es[i])/2)
	}
	path.LineTo(xs[voiceBars-1], cy-es[voiceBars-1]).LineTo(xs[voiceBars-1], cy+es[voiceBars-1])
	for i := voiceBars - 2; i >= 0; i-- {
		path.QuadTo(xs[i+1], cy+es[i+1], (xs[i+1]+xs[i])/2, cy+(es[i+1]+es[i])/2)
	}
	path.LineTo(xs[0], cy+es[0])
	acc.A = 205
	p.FillPath(path.Close(), acc)
}

// drawSweep 三岛连桥识别中：录音时那条线变淡，一小段亮光在上面来回走
func drawSweep(p *ui.Painter, r ui.Rect, fg, acc ui.Color) {
	t := float64(time.Now().UnixMilli()%600000) / 1000
	const inset, seg = 8, 12
	track := fg
	track.A = 31
	y := r.Y + r.H/2 - 1
	p.Fill(ui.Rect{X: r.X + inset, Y: y, W: r.W - 2*inset, H: 2}, track, 1)
	x := r.X + inset + (r.W-2*inset-seg)*float32(0.5+0.5*math.Sin(t*4))
	p.Fill(ui.Rect{X: x, Y: y - 0.5, W: seg, H: 3}, acc, 1.5)
	p.After(voiceFrame)
}

// drawLEDs 11 格电平，从中间往两边亮；run 在 -1~1 是识别中：中间那块岛变窄了，只画 7 格，亮 run 附近那一两格来回扫；录音时传 -2
func drawLEDs(p *ui.Painter, r ui.Rect, level float32, run float64, fg, acc ui.Color) {
	m, cy := r.X+r.W/2, r.Y+r.H/2
	off := fg
	off.A = 31
	half := min(5, int((r.W/2-3)/6)) // 格子不能伸出中间那块岛
	if run >= -1 {
		half = min(half, 3)
	}
	for i := -half; i <= half; i++ {
		d := float32(math.Abs(float64(i))) / float32(half)
		h := 16 - 6*d
		cell := ui.Rect{X: m + float32(i)*6 - 1.5, Y: cy - h/2, W: 3, H: h}
		p.Fill(cell, off, 1.5)
		on := min(1, max(0, (level*1.15-d)*4))
		if run >= -1 {
			on = float32(max(0, 1-math.Abs(float64(i)-run*float64(half))/1.5))
		}
		if on > 0.01 {
			c := acc
			c.A = uint8(255 * on)
			p.Fill(cell, c, 1.5)
		}
	}
}

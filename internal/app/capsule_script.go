package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/egoist/mygo/ui"
)

// 自定义胶囊样式：styles/<文件夹>/style.js 里写一个 draw(g, s)。Mami 每帧调一次，脚本通过 g 下画图指令，
// 记成一串 capOp，再用 mygo 的画笔画出来：和内置样式同一支笔，不开浏览器。同一串指令既画在浮窗上，
// 也能缩小了画在设置页的预览里。接口说明给作者和 AI 看：skills/mami-capsule-style/SKILL.md

const capsulePrefix = "custom:"

// capsuleBudget 一帧脚本最多跑这么久，超了就打断、这个样式算出错（退回内置样式），死循环也卡不住界面
const capsuleBudget = 60 * time.Millisecond

// capsuleManifest 样式文件夹里的 manifest.json，全可省
type capsuleManifest struct {
	Name     string `json:"name"`     // 设置里显示的名字，默认文件夹名
	Width    int    `json:"width"`    // 窗口大小（DIP），默认 260×110
	Height   int    `json:"height"`   //
	Position string `json:"position"` // bottom（默认，屏幕底部居中）| top | center
	OffsetX  int    `json:"offsetX"`  // 再挪一点（DIP，正数往右/往下）
	OffsetY  int    `json:"offsetY"`  //
}

func loadCapsuleManifest(dir string) (capsuleManifest, error) {
	m := capsuleManifest{Name: filepath.Base(dir)}
	if _, err := os.Stat(filepath.Join(dir, "style.js")); err != nil {
		return m, err
	}
	if b, err := os.ReadFile(filepath.Join(dir, "manifest.json")); err == nil {
		if err := json.Unmarshal(b, &m); err != nil {
			return m, fmt.Errorf("manifest.json: %w", err)
		}
	}
	if strings.TrimSpace(m.Name) == "" {
		m.Name = filepath.Base(dir)
	}
	if m.Width <= 0 {
		m.Width = 260
	}
	if m.Height <= 0 {
		m.Height = 110
	}
	m.Width, m.Height = min(max(m.Width, 40), 1600), min(max(m.Height, 40), 900)
	switch m.Position {
	case "top", "center":
	default:
		m.Position = "bottom"
	}
	return m, nil
}

// capsuleScene 一帧给脚本的状态 s（字段改了要同步 SKILL.md）
type capsuleScene struct {
	kind             string    // recording | recognizing | text | hidden
	level            float64   // 录音时最新音量 0~1
	levels           []float64 // 最近的音量 0~1，旧的在前
	live, text, hint string
	copy, retry      bool
	undo, dark       bool
	t                float64 // 秒，一直往前走，做动画用
	since            float64 // 进入当前 kind 多少秒
	leave            float64 // kind=hidden 时退场进度 0~1，到 1 窗口就藏起来
	hover            string  // 鼠标停在哪个按钮上（button 的 action），没有就是 ""
}

// capOp 一条画图指令，坐标是样式自己的 DIP（窗口左上角是 0,0）
type capOp struct {
	kind           byte // r 矩形 k 矩形描边 c 圆 o 圆环 p 多边形 q 折线 l 线 h 阴影 t 文字 i 图片
	x, y, w, h, r  float32
	width, blur    float32
	dx, dy, spread float32
	pts            []float32
	color          ui.Color
	grad           *ui.LinearGradient
	text           string
	size           float32
	img            *ui.Bitmap
	imgAlpha       float32
	textW, textH   float32
}

type capButton struct {
	x, y, w, h float32
	action     string
}

// capFrame 一帧的结果
type capFrame struct {
	ops     []capOp
	buttons []capButton
}

// capsuleStyle 一个加载好的样式：一个 JS 运行时、它的 draw，文件改了就整个重载
type capsuleStyle struct {
	folder, dir string
	man         capsuleManifest
	mod         time.Time // style.js / manifest.json 里较新的修改时间
	checked     time.Time
	err         error // 加载或运行出错：这个样式先不用，文件改了再试
	vm          *goja.Runtime
	draw        goja.Callable
	g           goja.Value
	images      map[string]*ui.Bitmap

	cur     *capFrame // 正在画的这一帧，g 的方法往里记
	alpha   float32
	measure func(text string, size float32) (float32, float32)
}

func capsuleModTime(dir string) time.Time {
	var t time.Time
	for _, f := range []string{"style.js", "manifest.json"} {
		if info, err := os.Stat(filepath.Join(dir, f)); err == nil && info.ModTime().After(t) {
			t = info.ModTime()
		}
	}
	return t
}

// loadCapsuleStyle 读 manifest、跑一遍 style.js、拿到 draw。出错时返回的样式带着 err
func loadCapsuleStyle(dir string) *capsuleStyle {
	s := &capsuleStyle{folder: filepath.Base(dir), dir: dir, mod: capsuleModTime(dir), checked: time.Now(), images: map[string]*ui.Bitmap{}}
	s.man, s.err = loadCapsuleManifest(dir)
	if s.err != nil {
		return s
	}
	src, err := os.ReadFile(filepath.Join(dir, "style.js"))
	if err != nil {
		s.err = err
		return s
	}
	s.vm = goja.New()
	s.g = s.vm.ToValue(s.api())
	timer := time.AfterFunc(capsuleBudget*5, func() { s.vm.Interrupt("style.js took too long to load") })
	_, err = s.vm.RunScript("style.js", string(src))
	timer.Stop()
	s.vm.ClearInterrupt()
	if err != nil {
		s.err = err
		return s
	}
	draw, ok := goja.AssertFunction(s.vm.Get("draw"))
	if !ok {
		s.err = errors.New("style.js: no function draw(g, s)")
		return s
	}
	s.draw = draw
	return s
}

// frame 跑一帧 draw。measure 量文字宽高（界面线程里由 Context 提供）
func (s *capsuleStyle) frame(sc capsuleScene, measure func(string, float32) (float32, float32)) (*capFrame, error) {
	if s.err != nil {
		return nil, s.err
	}
	f := &capFrame{}
	s.cur, s.alpha, s.measure = f, 1, measure
	levels := make([]any, len(sc.levels))
	for i, v := range sc.levels {
		levels[i] = v
	}
	st := map[string]any{
		"kind": sc.kind, "level": sc.level, "levels": levels, "live": sc.live, "text": sc.text, "hint": sc.hint,
		"copy": sc.copy, "retry": sc.retry, "undo": sc.undo, "dark": sc.dark,
		"w": s.man.Width, "h": s.man.Height, "t": sc.t, "since": sc.since, "leave": sc.leave, "hover": sc.hover,
	}
	timer := time.AfterFunc(capsuleBudget, func() { s.vm.Interrupt("draw took longer than " + capsuleBudget.String()) })
	_, err := s.draw(goja.Undefined(), s.g, s.vm.ToValue(st))
	timer.Stop()
	s.vm.ClearInterrupt()
	s.cur = nil
	if err != nil {
		s.err = err
		return nil, err
	}
	return f, nil
}

// api 脚本里的 g。都是画到这一帧的指令表里；颜色是 "#rrggbb" / "#rrggbbaa" / "rgba(r,g,b,a)"，
// 能填色的地方也能给 g.gradient(...)
func (s *capsuleStyle) api() map[string]any {
	add := func(op capOp) {
		if s.cur != nil {
			s.cur.ops = append(s.cur.ops, op)
		}
	}
	fill := func(v goja.Value) (ui.Color, *ui.LinearGradient) { return s.paint(v) }
	return map[string]any{
		"rect": func(x, y, w, h float64, f goja.Value, r float64) {
			c, g := fill(f)
			add(capOp{kind: 'r', x: f32(x), y: f32(y), w: f32(w), h: f32(h), r: f32(r), color: c, grad: g})
		},
		"strokeRect": func(x, y, w, h float64, color goja.Value, r, width float64) {
			c, _ := fill(color)
			add(capOp{kind: 'k', x: f32(x), y: f32(y), w: f32(w), h: f32(h), r: f32(r), width: f32(orOne(width)), color: c})
		},
		"circle": func(cx, cy, r float64, f goja.Value) {
			c, g := fill(f)
			add(capOp{kind: 'c', x: f32(cx), y: f32(cy), r: f32(r), color: c, grad: g})
		},
		"ring": func(cx, cy, r, width float64, color goja.Value) {
			c, _ := fill(color)
			add(capOp{kind: 'o', x: f32(cx), y: f32(cy), r: f32(r), width: f32(orOne(width)), color: c})
		},
		"poly": func(pts goja.Value, f goja.Value) {
			c, g := fill(f)
			add(capOp{kind: 'p', pts: floats(pts), color: c, grad: g})
		},
		"polyline": func(pts goja.Value, width float64, color goja.Value) {
			c, _ := fill(color)
			add(capOp{kind: 'q', pts: floats(pts), width: f32(orOne(width)), color: c})
		},
		"line": func(x0, y0, x1, y1, width float64, color goja.Value) {
			c, _ := fill(color)
			add(capOp{kind: 'l', x: f32(x0), y: f32(y0), w: f32(x1), h: f32(y1), width: f32(orOne(width)), color: c})
		},
		"shadow": func(x, y, w, h, r, blur float64, color goja.Value, dx, dy, spread float64) {
			c, _ := fill(color)
			add(capOp{kind: 'h', x: f32(x), y: f32(y), w: f32(w), h: f32(h), r: f32(r), blur: f32(blur), color: c, dx: f32(dx), dy: f32(dy), spread: f32(spread)})
		},
		"measure": func(text string, size float64) float64 {
			w, _ := s.measureText(text, f32(size))
			return float64(w)
		},
		// text 画一行字：y 是这行的竖直中线；align left|center|right 对齐到 x；给了 maxWidth 就截断加 …，返回画出来的宽
		"text": func(text string, x, y, size float64, color goja.Value, align string, maxWidth float64) float64 {
			c, _ := fill(color)
			sz := f32(orDefault(size, 13))
			if maxWidth > 0 {
				text = s.ellipsize(text, sz, f32(maxWidth))
			}
			w, h := s.measureText(text, sz)
			left := f32(x)
			switch align {
			case "center":
				left -= w / 2
			case "right":
				left -= w
			}
			add(capOp{kind: 't', x: left, y: f32(y) - h/2, text: text, size: sz, color: c, textW: w, textH: h})
			return float64(w)
		},
		"image": func(file string, x, y, w, h float64) {
			if b := s.image(file); b != nil {
				add(capOp{kind: 'i', x: f32(x), y: f32(y), w: f32(w), h: f32(h), img: b, imgAlpha: s.alpha})
			}
		},
		"icon": func(name string, cx, cy, size float64, color goja.Value) {
			c, _ := fill(color)
			for _, op := range iconOps(name, f32(cx), f32(cy), f32(orDefault(size, 14)), c) {
				add(op)
			}
		},
		// button 一块能点的地方：点了交给 Mami 执行 action；窗口里别处的点击穿透到下面的程序
		"button": func(x, y, w, h float64, action string) {
			if s.cur != nil && w > 0 && h > 0 {
				s.cur.buttons = append(s.cur.buttons, capButton{f32(x), f32(y), f32(w), f32(h), action})
			}
		},
		"gradient": func(from, to string, angle float64) map[string]any {
			return map[string]any{"from": from, "to": to, "angle": angle}
		},
		"alpha": func(a float64) { s.alpha = f32(math.Max(0, math.Min(1, a))) },
		"rgba":  func(r, g, b, a float64) string { return colorString(rgbaF(r, g, b, a)) },
		"mix": func(a, b string, t float64) string {
			ca, _ := parseCapColor(a)
			cb, _ := parseCapColor(b)
			return colorString(mixColor(ca, cb, f32(t)))
		},
	}
}

func (s *capsuleStyle) measureText(text string, size float32) (float32, float32) {
	if s.measure == nil {
		return float32(len([]rune(text))) * size * 0.6, size * 1.3
	}
	return s.measure(text, size)
}

func (s *capsuleStyle) ellipsize(text string, size, maxW float32) string {
	if w, _ := s.measureText(text, size); w <= maxW {
		return text
	}
	r := []rune(text)
	for len(r) > 0 {
		r = r[:len(r)-1]
		if w, _ := s.measureText(string(r)+"…", size); w <= maxW {
			break
		}
	}
	return string(r) + "…"
}

// image 样式文件夹里的 png，按文件名缓存；只认文件夹里的文件
func (s *capsuleStyle) image(file string) *ui.Bitmap {
	if b, ok := s.images[file]; ok {
		return b
	}
	var b *ui.Bitmap
	if filepath.Base(file) == file && strings.HasSuffix(strings.ToLower(file), ".png") {
		if f, err := os.Open(filepath.Join(s.dir, file)); err == nil {
			if img, err := png.Decode(f); err == nil {
				b = ui.NewBitmap(img)
			}
			f.Close()
		}
	}
	s.images[file] = b
	return b
}

// paint 解析填色：颜色字符串或 g.gradient(...)，乘上当前的 g.alpha
func (s *capsuleStyle) paint(v goja.Value) (ui.Color, *ui.LinearGradient) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ui.Color{}, nil
	}
	if o, ok := v.Export().(map[string]any); ok {
		from, _ := parseCapColor(fmt.Sprint(o["from"]))
		to, _ := parseCapColor(fmt.Sprint(o["to"]))
		angle, _ := o["angle"].(float64)
		if i, ok := o["angle"].(int64); ok {
			angle = float64(i)
		}
		return ui.Color{}, &ui.LinearGradient{From: fade(from, s.alpha), To: fade(to, s.alpha), Angle: float32(angle), Oklab: true}
	}
	c, _ := parseCapColor(v.String())
	return fade(c, s.alpha), nil
}

func fade(c ui.Color, a float32) ui.Color {
	c.A = uint8(float32(c.A)*a + 0.5)
	return c
}

// parseCapColor "#rgb" "#rgba" "#rrggbb" "#rrggbbaa" "rgb(r,g,b)" "rgba(r,g,b,a)"；认不出是透明
func parseCapColor(s string) (c ui.Color, ok bool) {
	s = strings.TrimSpace(s)
	if inner, found := strings.CutPrefix(s, "rgba("); found {
		s = "rgb(" + inner
	}
	if inner, found := strings.CutPrefix(s, "rgb("); found {
		parts := strings.Split(strings.TrimSuffix(inner, ")"), ",")
		if len(parts) < 3 {
			return ui.Color{}, false
		}
		v := [4]float64{0, 0, 0, 1}
		for i := 0; i < len(parts) && i < 4; i++ {
			n, err := strconv.ParseFloat(strings.TrimSpace(parts[i]), 64)
			if err != nil {
				return ui.Color{}, false
			}
			v[i] = n
		}
		return rgbaF(v[0], v[1], v[2], v[3]), true
	}
	if !strings.HasPrefix(s, "#") {
		return ui.Color{}, false
	}
	defer func() {
		if recover() != nil {
			c, ok = ui.Color{}, false
		}
	}()
	return ui.Hex(s), true
}

func rgbaF(r, g, b, a float64) ui.Color {
	u8 := func(v float64) uint8 { return uint8(math.Max(0, math.Min(255, math.Round(v)))) }
	return ui.RGBA(u8(r), u8(g), u8(b), float32(math.Max(0, math.Min(1, a))))
}

func colorString(c ui.Color) string { return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A) }

func mixColor(a, b ui.Color, t float32) ui.Color {
	t = max(0, min(1, t))
	l := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t + 0.5) }
	return ui.Color{R: l(a.R, b.R), G: l(a.G, b.G), B: l(a.B, b.B), A: l(a.A, b.A)}
}

func f32(v float64) float32 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0
	}
	return float32(v)
}

func orOne(v float64) float64 { return orDefault(v, 1) }

func orDefault(v, d float64) float64 {
	if v == 0 || math.IsNaN(v) {
		return d
	}
	return v
}

// floats JS 数组 [x0,y0,x1,y1,...] → []float32
func floats(v goja.Value) []float32 {
	arr, _ := v.Export().([]any)
	out := make([]float32, 0, len(arr))
	for _, e := range arr {
		switch n := e.(type) {
		case float64:
			out = append(out, f32(n))
		case int64:
			out = append(out, float32(n))
		}
	}
	return out
}

// iconOps 几个常用图标：x check copy retry undo，画在以 (cx,cy) 为中心、边长 size 的方块里
func iconOps(name string, cx, cy, size float32, c ui.Color) []capOp {
	h, w := size/2, max(1.4, size/9)
	line := func(x0, y0, x1, y1 float32) capOp {
		return capOp{kind: 'l', x: cx + x0*h, y: cy + y0*h, w: cx + x1*h, h: cy + y1*h, width: w, color: c}
	}
	arc := func(mirror float32) []capOp {
		var pts []float32
		for i := 0; i <= 16; i++ {
			a := -0.25*math.Pi + float64(i)/16*1.5*math.Pi
			pts = append(pts, cx+mirror*float32(math.Cos(a))*h*0.75, cy-float32(math.Sin(a))*h*0.75)
		}
		ex, ey := pts[0], pts[1]
		return []capOp{
			{kind: 'q', pts: pts, width: w, color: c},
			{kind: 'q', pts: []float32{ex - mirror*h*0.05, ey - h*0.42, ex, ey, ex - mirror*h*0.42, ey + h*0.05}, width: w, color: c},
		}
	}
	switch name {
	case "x":
		return []capOp{line(-0.6, -0.6, 0.6, 0.6), line(-0.6, 0.6, 0.6, -0.6)}
	case "check":
		return []capOp{{kind: 'q', pts: []float32{cx - h*0.7, cy, cx - h*0.2, cy + h*0.5, cx + h*0.7, cy - h*0.5}, width: w, color: c}}
	case "copy":
		return []capOp{
			{kind: 'k', x: cx - h*0.35, y: cy - h*0.35, w: h * 1.05, h: h * 1.05, r: h * 0.2, width: w, color: c},
			{kind: 'q', pts: []float32{cx - h*0.7, cy + h*0.4, cx - h*0.7, cy - h*0.7, cx + h*0.4, cy - h*0.7}, width: w, color: c},
		}
	case "retry":
		return arc(1)
	case "undo":
		return arc(-1)
	}
	return nil
}

// paintCapsule 把一帧指令画到 (ox, oy)，缩放 k（设置页预览用）
func paintCapsule(p *ui.Painter, f *capFrame, ox, oy, k float32) {
	if f == nil {
		return
	}
	X := func(v float32) float32 { return ox + v*k }
	Y := func(v float32) float32 { return oy + v*k }
	rect := func(op capOp) ui.Rect { return ui.Rect{X: X(op.x), Y: Y(op.y), W: op.w * k, H: op.h * k} }
	path := func(pts []float32, closed bool) *ui.Path {
		pa := new(ui.Path)
		for i := 0; i+1 < len(pts); i += 2 {
			if i == 0 {
				pa.MoveTo(X(pts[0]), Y(pts[1]))
			} else {
				pa.LineTo(X(pts[i]), Y(pts[i+1]))
			}
		}
		if closed {
			pa.Close()
		}
		return pa
	}
	for _, op := range f.ops {
		switch op.kind {
		case 'r':
			if op.grad != nil {
				p.FillGradient(rect(op), *op.grad, op.r*k)
			} else {
				p.Fill(rect(op), op.color, op.r*k)
			}
		case 'k':
			p.Stroke(rect(op), op.color, op.r*k, op.width*k)
		case 'c':
			pa := new(ui.Path).Circle(X(op.x), Y(op.y), op.r*k)
			if op.grad != nil {
				p.FillPathGradient(pa, *op.grad)
			} else {
				p.FillPath(pa, op.color)
			}
		case 'o':
			p.StrokePath(new(ui.Path).Circle(X(op.x), Y(op.y), op.r*k), op.width*k, op.color)
		case 'p':
			if len(op.pts) >= 6 {
				if op.grad != nil {
					p.FillPathGradient(path(op.pts, true), *op.grad)
				} else {
					p.FillPath(path(op.pts, true), op.color)
				}
			}
		case 'q':
			if len(op.pts) >= 4 {
				p.StrokePath(path(op.pts, false), op.width*k, op.color)
			}
		case 'l': // 不用 p.Line：它只画横竖线
			p.StrokePath(new(ui.Path).MoveTo(X(op.x), Y(op.y)).LineTo(X(op.w), Y(op.h)), op.width*k, op.color)
		case 'h':
			p.Shadow(rect(op), op.r*k, op.dx*k, op.dy*k, op.blur*k, op.spread*k, op.color)
		case 't':
			p.Text(X(op.x), Y(op.y), op.text, op.size*k, op.color)
		case 'i':
			o := ui.ImageOptions{}
			if op.imgAlpha < 1 {
				o.Tint = ui.RGBA(255, 255, 255, op.imgAlpha)
			}
			p.ImageRegion(op.img, ui.Rect{}, rect(op), o)
		}
	}
}

// fakeCapsuleScene 设置页预览用的假状态：音量是假的，会动
func fakeCapsuleScene(kind string, now time.Time, dark bool) capsuleScene {
	t := float64(now.UnixMilli()%600000) / 1000
	sc := capsuleScene{kind: kind, dark: dark, t: t, since: math.Mod(t, 30) + 1}
	if kind == "recording" {
		for i := 0; i < 48; i++ {
			x := t - float64(47-i)*0.05
			sc.levels = append(sc.levels, math.Min(1, math.Max(0, 0.45+0.35*math.Sin(x*3.1)+0.15*math.Sin(x*7.3))))
		}
		sc.level = sc.levels[len(sc.levels)-1]
	}
	if kind == "text" {
		sc.text, sc.copy = T("明天三点开会"), true
	}
	return sc
}

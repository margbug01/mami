//go:build windows

package app

import (
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 自定义胶囊样式的浮窗（脚本和画法在 capsule_script.go）：voice_style 写成 "custom:<文件夹名>" 时，
// 胶囊换成一块透明的原生浮层，每帧跑一次 style.js 的 draw。脚本用 g.button 声明的地方才接鼠标，
// 其余点击穿透到下面的程序。样式找不到或脚本出错就照旧用内置胶囊。

type capsuleWin struct {
	win     *mygo.Window
	styles  map[string]*capsuleStyle // 按文件夹缓存，文件改了自动重载（界面线程）
	failed  string                   // 出错的样式，别每次都去试；文件改了再试
	folder  string                   // 正显示的样式
	scene   capsuleScene
	kindAt  time.Time
	shown   bool
	leaveAt time.Time // 不是零值 = 正在退场
	gen     int
	x, y    int32  // 窗口左上角（物理像素）
	dpi     uint32 //
	hits    []capButton
	through bool
	hover   string
	listAt  time.Time // 设置页样式列表的缓存
	list    []capsuleEntry
}

// capsuleEntry styles 文件夹里一个能用的样式
type capsuleEntry struct {
	folder string
	man    capsuleManifest
}

var capsuleEpoch = time.Now()

func (a *App) capsuleStylesDir() string { return filepath.Join(filepath.Dir(a.cfgPath), "styles") }

func (a *App) createCapsuleWindow() {
	var h uintptr
	a.capWin.win, h = a.floatingWindow("mami-capsule", 260, 110, 40, 40, a.capsuleView)
	a.capHwnd.Store(h)
}

// capsule 文件夹对应的样式，文件改了就重载（每半秒最多看一次）；名字不合法返回 nil（界面线程）
func (a *App) capsule(folder string) *capsuleStyle {
	if folder == "" || strings.ContainsAny(folder, `/\:.`) {
		return nil
	}
	w := &a.capWin
	if w.styles == nil {
		w.styles = map[string]*capsuleStyle{}
	}
	s := w.styles[folder]
	if s != nil && time.Since(s.checked) < 500*time.Millisecond {
		return s
	}
	dir := filepath.Join(a.capsuleStylesDir(), folder)
	if s != nil && capsuleModTime(dir).Equal(s.mod) {
		s.checked = time.Now()
		return s
	}
	start := time.Now()
	s = loadCapsuleStyle(dir)
	w.styles[folder] = s
	if w.failed == folder {
		w.failed = "" // 文件改过了，再给一次机会
	}
	if s.err != nil {
		log.Printf("capsule: %s: %v", folder, s.err)
	} else {
		log.Printf("capsule: loaded %s in %dms", folder, time.Since(start).Milliseconds())
	}
	return s
}

// showCapsule 自定义样式时显示浮层并更新状态，返回 false 表示照旧用内置胶囊（界面线程）
func (a *App) showCapsule(state voiceViewState) bool {
	folder, ok := strings.CutPrefix(a.voiceStyle, capsulePrefix)
	if !ok {
		return false
	}
	w := &a.capWin
	s := a.capsule(folder)
	if s == nil || s.err != nil || w.failed == folder {
		return false
	}
	kind := "text"
	switch {
	case state.recording:
		kind = "recording"
	case state.recognizing:
		kind = "recognizing"
	}
	if !w.shown || w.folder != folder || !w.leaveAt.IsZero() {
		if !w.shown || w.folder != folder {
			a.placeCapsule(s.man)
		}
		w.shown, w.folder, w.leaveAt = true, folder, time.Time{}
		w.win.ShowInactive()
		w.gen++
		go a.trackCapsuleMouse(w.gen)
	}
	if kind != w.scene.kind {
		w.kindAt = time.Now()
	}
	var levels []float64
	for _, v := range state.levels {
		levels = append(levels, capLevel(v))
	}
	level := 0.0
	if len(levels) > 0 {
		level = levels[len(levels)-1]
	}
	w.scene = capsuleScene{kind: kind, level: level, levels: levels, live: state.live, text: state.text, hint: state.hint,
		copy: state.copy, retry: state.retry, undo: state.undo}
	w.win.Invalidate()
	return true
}

// capLevel 原始音量（约 0~0.1）→ 0~1，和内置样式的手感一致
func capLevel(v float64) float64 { return min(1, math.Pow(max(0, v)/0.08, 0.6)) }

func (a *App) placeCapsule(m capsuleManifest) {
	w := &a.capWin
	dpi, wa := a.voiceDPI, a.voiceWA
	width, height := scale(int32(m.Width), dpi), scale(int32(m.Height), dpi)
	x := wa.Left + (wa.w()-width)/2
	y := wa.Bottom - height - scale(4, dpi)
	switch m.Position {
	case "top":
		y = wa.Top + scale(4, dpi)
	case "center":
		y = wa.Top + (wa.Bottom-wa.Top-height)/2
	}
	x, y = x+scale(int32(m.OffsetX), dpi), y+scale(int32(m.OffsetY), dpi)
	placeAt(w.win.NativeHandle(), x, y, width, height)
	w.x, w.y, w.dpi = x, y, dpi
}

// hideCapsule 浮层正显示着就开始退场（脚本拿 s.leave 自己画），320ms 后藏窗口，返回 true（界面线程）
func (a *App) hideCapsule() bool {
	w := &a.capWin
	if !w.shown {
		return false
	}
	if !w.leaveAt.IsZero() {
		return true
	}
	w.leaveAt, w.scene.kind, w.kindAt = time.Now(), "hidden", time.Now()
	w.gen++
	gen := w.gen
	w.win.Invalidate()
	time.AfterFunc(320*time.Millisecond, func() {
		w.win.Update(func() {
			if w.gen == gen {
				w.win.Hide()
				w.shown, w.leaveAt = false, time.Time{}
			}
		})
	})
	return true
}

// capsuleView 浮层本体：跑一帧脚本，照指令画，按钮叠成真的按钮
func (a *App) capsuleView(c *ui.Context) {
	w := &a.capWin
	if !w.shown {
		return
	}
	s := a.capsule(w.folder)
	if s == nil {
		return
	}
	now := time.Now()
	sc := w.scene
	sc.dark = toneDark(a.voiceTone, a.themed(c).Dark)
	sc.t, sc.since, sc.hover = now.Sub(capsuleEpoch).Seconds(), now.Sub(w.kindAt).Seconds(), w.hover
	if !w.leaveAt.IsZero() {
		sc.leave = min(1, now.Sub(w.leaveAt).Seconds()/0.3)
	}
	f, err := s.frame(sc, capMeasure(c))
	if err != nil {
		log.Printf("capsule: %s: %v — using the built-in pill", w.folder, err)
		w.failed = w.folder
		w.win.Hide()
		w.shown = false
		return
	}
	w.hits = f.buttons
	hover := ""
	ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { paintCapsule(p, f, r.X, r.Y, 1) }).Children(func() {
		for _, b := range f.buttons {
			action := b.action
			btn := ui.ButtonBase(c).Absolute().Left(b.x).Top(b.y).Size(b.w, b.h).Cursor(ui.CursorPointer).Label(action)
			if btn.Hovered() {
				hover = action
			}
			if btn.Clicked() && w.leaveAt.IsZero() {
				a.capsuleAction(action)
			}
		}
	})
	w.hover = hover
	c.AnimationFrame()
}

func capMeasure(c *ui.Context) func(string, float32) (float32, float32) {
	return func(text string, size float32) (float32, float32) {
		return c.MeasureText(10000, ui.Span{Text: text, Size: size})
	}
}

// capsuleAction 样式里按钮的动作，和内置胶囊的按钮走同一条路
func (a *App) capsuleAction(action string) {
	switch action {
	case "cancel":
		voiceClick(voiceCancel)
	case "stop":
		voiceClick(voiceStop)
	default:
		a.voiceWin.Update(func() {
			switch action {
			case "close":
				a.voiceRetry = nil
				a.voiceLeave()
			case "retry":
				if a.voiceRetry != nil {
					a.voiceRetry()
				}
			case "undo":
				if a.voiceUndo != nil {
					a.voiceUndo()
					a.voiceUndo = nil
				}
			case "copy":
				if !a.voiceUI.copy {
					return
				}
				text, id := a.voiceUI.text, voiceEpoch.Load()
				theWorker.do(func(owner uintptr) { setClipboardText(owner, toCRLF(text), false) })
				time.AfterFunc(700*time.Millisecond, func() { a.voiceDisplay(id, voiceViewState{}, false) })
			}
		})
	}
}

// trackCapsuleMouse 浮层显示期间：鼠标在脚本声明的按钮上才接鼠标，别处让点击穿透下去
func (a *App) trackCapsuleMouse(gen int) {
	w := &a.capWin
	for {
		time.Sleep(30 * time.Millisecond)
		alive := make(chan bool, 1)
		w.win.Update(func() {
			if w.gen != gen || !w.shown {
				alive <- false
				return
			}
			pt := cursorPos()
			px := float32(pt.X-w.x) * 96 / float32(w.dpi)
			py := float32(pt.Y-w.y) * 96 / float32(w.dpi)
			through := true
			for _, b := range w.hits {
				if px >= b.x && px < b.x+b.w && py >= b.y && py < b.y+b.h {
					through = false
					break
				}
			}
			if through != w.through {
				w.through = through
				w.win.SetIgnoreMouseEvents(through)
			}
			alive <- true
		})
		select {
		case ok := <-alive:
			if !ok {
				return
			}
		case <-time.After(time.Second):
			return // 界面线程没回话（在退出）
		}
	}
}

// capsulePreview 设置页里画一个样式的样子（假状态、会动），按比例缩到 maxW×maxH 以内；脚本出错返回错误
func (a *App) capsulePreview(c *ui.Context, folder string, sc capsuleScene, maxW, maxH float32) error {
	s := a.capsule(folder)
	if s == nil {
		return os.ErrNotExist
	}
	k := min(1, maxW/float32(s.man.Width), maxH/float32(s.man.Height))
	box := ui.Box(c).Size(float32(s.man.Width)*k, float32(s.man.Height)*k)
	if s.err != nil {
		box.Draw(func(p *ui.Painter, r ui.Rect) {
			ph := min(r.H, 28)
			p.Fill(ui.Rect{X: r.X, Y: r.Y + (r.H-ph)/2, W: r.W, H: ph}, ui.RGBA(30, 30, 36, 0.35), ph/2)
		})
		return s.err
	}
	f, err := s.frame(sc, capMeasure(c))
	if err != nil {
		return err
	}
	box.Draw(func(p *ui.Painter, r ui.Rect) { paintCapsule(p, f, r.X, r.Y, k) })
	c.After(voiceFrame)
	return nil
}

// capsuleStyles styles 文件夹里有 style.js 的样式，按名字排，2 秒内只读一次目录（界面线程）
func (a *App) capsuleStyles() []capsuleEntry {
	w := &a.capWin
	if time.Since(w.listAt) < 2*time.Second {
		return w.list
	}
	w.listAt, w.list = time.Now(), nil
	entries, _ := os.ReadDir(a.capsuleStylesDir())
	for _, e := range entries {
		if !e.IsDir() || strings.ContainsAny(e.Name(), ".:") {
			continue
		}
		if m, err := loadCapsuleManifest(filepath.Join(a.capsuleStylesDir(), e.Name())); err == nil {
			w.list = append(w.list, capsuleEntry{e.Name(), m})
		}
	}
	sort.Slice(w.list, func(i, j int) bool { return w.list[i].man.Name < w.list[j].man.Name })
	return w.list
}

// capsuleOptions 浮窗样式下拉里的自定义样式
func (a *App) capsuleOptions() []option {
	var opts []option
	for _, e := range a.capsuleStyles() {
		opts = append(opts, option{capsulePrefix + e.folder, e.man.Name})
	}
	return opts
}

func (a *App) openCapsuleStyles() {
	os.MkdirAll(a.capsuleStylesDir(), 0o755)
	mygo.Shell.OpenPath(a.capsuleStylesDir())
}

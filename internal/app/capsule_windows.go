//go:build windows

package app

import (
	"errors"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 自定义胶囊样式的浮窗（脚本和画法在 capsule_script.go）：voice_style 写成 "custom:<文件夹名>" 时，
// 胶囊换成一块透明的原生浮层，每帧跑一次 style.js 的 draw。脚本用 g.button 声明的地方才接鼠标，
// 其余点击穿透到下面的程序。样式找不到或脚本出错就照旧用内置胶囊。

type capsuleWin struct {
	win *mygo.Window
	// styles 按「文件夹/用处」缓存：浮窗一份，设置页每种预览各一份，脚本的全局变量互不串；文件改了自动重载（界面线程）
	styles  map[string]*capsuleStyle
	logged  map[string]time.Time // 每个文件夹哪一版已经写过加载日志：几份一起重载只记一行
	retryID uint64               // 浮窗的样式超时是在哪一次说话（voiceEpoch）：换一次说话就重新加载再试
	folder  string               // 正显示的样式
	scene   capsuleScene
	kindAt  time.Time
	shown   bool
	leaveAt time.Time // 不是零值 = 正在退场
	gen     int
	x, y    int32  // 窗口左上角（物理像素）
	dpi     uint32 //
	hits    []capButton
	through bool // 窗口现在是否点击穿透
	hover   string
	listAt  time.Time // 设置页样式列表的缓存
	list    []capsuleEntry
}

// capsuleEntry styles 文件夹里一个样式（有 style.js 的文件夹；manifest 坏了也列出来，好在设置页标红）
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

// capsule 文件夹对应的样式。slot 是用处："" 是浮窗，设置页的预览用状态名，各跑一份。
// 文件改了就重载（每半秒最多看一次）；名字不合法返回 nil（界面线程）
func (a *App) capsule(folder, slot string) *capsuleStyle {
	if folder == "" || strings.ContainsAny(folder, `/\:.`) {
		return nil
	}
	w := &a.capWin
	if w.styles == nil {
		w.styles, w.logged = map[string]*capsuleStyle{}, map[string]time.Time{}
	}
	key := folder + "/" + slot
	s := w.styles[key]
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
	w.styles[key] = s
	if t, ok := w.logged[folder]; !ok || !t.Equal(s.mod) {
		w.logged[folder] = s.mod
		if s.err != nil {
			log.Printf("capsule: %s: %v", folder, s.err)
		} else {
			log.Printf("capsule: loaded %s in %dms", folder, time.Since(start).Milliseconds())
		}
	}
	return s
}

// showCapsule 自定义样式时显示浮层并更新状态，返回 false 表示照旧用内置胶囊（界面线程）
func (a *App) showCapsule(state voiceViewState) bool {
	w := &a.capWin
	folder, ok := strings.CutPrefix(a.voiceStyle, capsulePrefix)
	var s *capsuleStyle
	if ok {
		s = a.capsule(folder, "")
		if s != nil && s.timedOut && w.retryID != voiceEpoch.Load() {
			// 上次是一帧跑超时，多半是那一帧的数据（长句子）或机器一时忙，不是脚本坏了：新的一次说话重新加载再试
			log.Printf("capsule: %s: trying again after a timeout", folder)
			delete(w.styles, folder+"/")
			s = a.capsule(folder, "")
		}
	}
	if s == nil || s.err != nil {
		a.dropCapsule() // 用内置胶囊：别两颗同时在屏上
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
			w.hits, w.hover = nil, ""
		}
		w.shown, w.folder, w.leaveAt = true, folder, time.Time{}
		w.gen++            // 正在退场的话，作废藏窗口的那个计时器
		w.setThrough(true) // 先整块穿透，capsuleView 每帧看鼠标在不在按钮上再改
		w.win.ShowInactive()
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
	hint := state.hint
	if state.slow && hint == "" {
		hint = T("响应较慢，可取消")
	}
	w.scene = capsuleScene{kind: kind, level: level, levels: levels, live: state.live, text: state.text, hint: hint,
		copy: state.copy, retry: state.retry, undo: state.undo, slow: state.slow}
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
	w.setThrough(true) // 退场时整块穿透：点 ✕ 时鼠标正在按钮上，不然这 320ms 里整个窗口都吞点击
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

// dropCapsule 不等退场直接藏起来：换成内置胶囊的时候（界面线程，别在画浮层的中途调）
func (a *App) dropCapsule() {
	w := &a.capWin
	if !w.shown {
		return
	}
	w.gen++
	w.win.Hide()
	w.shown, w.leaveAt = false, time.Time{}
}

// capsuleView 浮层本体：跑一帧脚本，照指令画，按钮叠成真的按钮
func (a *App) capsuleView(c *ui.Context) {
	w := &a.capWin
	if !w.shown {
		return
	}
	s := a.capsule(w.folder, "")
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
		if s.timedOut {
			w.retryID = voiceEpoch.Load()
		}
		leaving := !w.leaveAt.IsZero()
		w.shown, w.leaveAt = false, time.Time{}
		w.gen++
		gen := w.gen
		w.win.Update(func() { // 别在画这一帧的中途藏窗口
			if w.gen != gen {
				return // 这期间又显示了
			}
			w.win.Hide()
			if !leaving {
				// 接着用内置胶囊显示同一个状态：识别结果、重试钮都还在（failed 的样式 showCapsule 不会再用）
				a.voiceDisplay(voiceEpoch.Load(), a.voiceUI, true)
			}
		})
		return
	}
	w.hits = f.buttons
	hover := ""
	ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { paintCapsule(p, f, r.X, r.Y, 1) }).Children(func() {
		seen := map[string]int{}
		for _, b := range f.buttons {
			action := b.action
			// Key：按钮列表这一帧变了（识别中的 cancel 换成结果的 close），按下和松开也认同一颗；同名的按顺序区分
			key := action + "#" + strconv.Itoa(seen[action])
			seen[action]++
			btn := ui.ButtonBase(c).Key(key).Absolute().Left(b.x).Top(b.y).Size(b.w, b.h).Cursor(ui.CursorPointer).Label(action)
			if btn.Hovered() {
				hover = action
			}
			if btn.Clicked() && w.leaveAt.IsZero() {
				a.capsuleAction(action)
			}
		}
	})
	w.hover = hover
	if w.wantThrough() != w.through {
		w.win.Update(a.syncCapsuleMouse) // 改窗口样式别在画的中途
	}
	c.AnimationFrame()
}

// wantThrough 鼠标在脚本声明的按钮上才接鼠标，别处让点击穿透到下面的程序；退场时整块穿透
func (w *capsuleWin) wantThrough() bool {
	if !w.leaveAt.IsZero() {
		return true
	}
	pt := cursorPos()
	dpi := float32(max(w.dpi, 1))
	px, py := float32(pt.X-w.x)*96/dpi, float32(pt.Y-w.y)*96/dpi
	for _, b := range w.hits {
		if px >= b.x && px < b.x+b.w && py >= b.y && py < b.y+b.h {
			return false
		}
	}
	return true
}

func (a *App) syncCapsuleMouse() {
	if w := &a.capWin; w.shown {
		w.setThrough(w.wantThrough())
	}
}

func (w *capsuleWin) setThrough(v bool) {
	if v != w.through {
		w.through = v
		w.win.SetIgnoreMouseEvents(v)
	}
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

// capsulePreview 设置页里画一个样式的样子（假状态、会动），按比例缩到 maxW×maxH 以内；脚本出错返回错误。
// 每种状态各用一份脚本（slot = sc.kind），和浮窗那份也分开，平滑、粒子这些全局变量互不串
func (a *App) capsulePreview(c *ui.Context, folder string, sc capsuleScene, maxW, maxH float32) error {
	s := a.capsule(folder, sc.kind)
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

// capsuleCheck 预览只画录音、识别、结果三种；带重试 / 撤销 / 提示、识别太慢、刚开始录音、退场这些分支也各跑一帧
// （每次加载跑一遍，不画），出错在设置页就能看到，不用等到真用的时候
func (a *App) capsuleCheck(c *ui.Context, folder string, dark bool) error {
	s := a.capsule(folder, "check")
	if s == nil {
		return os.ErrNotExist
	}
	if s.probed || s.err != nil {
		return s.err
	}
	s.probed = true
	text := T("明天三点开会")
	for _, sc := range []capsuleScene{
		{kind: "text", text: text, retry: true},
		{kind: "text", text: text, undo: true},
		{kind: "text", text: text, hint: T("润色失败，保留原话")},
		{kind: "text", hint: T("先填一下语音识别的 Key")},
		{kind: "recognizing", slow: true, hint: T("响应较慢，可取消")},
		{kind: "recording", live: text},
		{kind: "hidden", leave: 0.5},
	} {
		sc.dark = dark
		if _, err := s.frame(sc, capMeasure(c)); err != nil {
			return err
		}
	}
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
		// manifest.json 坏了也列出来：预览会标红、悬停看原因，不然样式就悄悄不见了
		if m, err := loadCapsuleManifest(filepath.Join(a.capsuleStylesDir(), e.Name())); !errors.Is(err, fs.ErrNotExist) {
			w.list = append(w.list, capsuleEntry{e.Name(), m})
		}
	}
	sort.Slice(w.list, func(i, j int) bool {
		if w.list[i].man.Name != w.list[j].man.Name {
			return w.list[i].man.Name < w.list[j].man.Name
		}
		return w.list[i].folder < w.list[j].folder
	})
	return w.list
}

// capsuleOptions 浮窗样式下拉：builtin 后面接上自定义样式。下拉按名字认选项，所以重名的（模板默认都叫 My Style）
// 后面带上文件夹名；配置里选着、文件夹却没了的也列一项，免得下拉框显示成别的样式
func (a *App) capsuleOptions(builtin []option, current string) []option {
	opts := append([]option(nil), builtin...)
	taken := map[string]bool{}
	for _, o := range builtin {
		taken[o.label] = true
	}
	add := func(folder, name string) {
		label := name
		if taken[label] {
			label = name + " · " + folder
		}
		taken[label] = true
		opts = append(opts, option{capsulePrefix + folder, label})
	}
	found := false
	for _, e := range a.capsuleStyles() {
		add(e.folder, e.man.Name)
		found = found || capsulePrefix+e.folder == current
	}
	if folder, ok := strings.CutPrefix(current, capsulePrefix); ok && !found {
		add(folder, folder)
	}
	return opts
}

func (a *App) openCapsuleStyles() {
	os.MkdirAll(a.capsuleStylesDir(), 0o755)
	mygo.Shell.OpenPath(a.capsuleStylesDir())
}

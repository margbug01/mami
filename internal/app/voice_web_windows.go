//go:build windows

package app

import (
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// 网页胶囊：voice_style 写成 "web:<文件夹名>" 时，胶囊换成 %APPDATA%/Mami/styles/<文件夹名>/index.html
// 这张透明网页。可选的 manifest.json 声明尺寸、位置、开发模式。Mami 往页面注入 window.mami
// （webVoicePreload）：mami.onState 收状态，mami.action 点按钮，带 data-mami-hit 的元素才接鼠标、
// 其余地方点击穿透到下面的窗口。接口说明给作者和 AI 看：skills/mami-capsule-style/SKILL.md。
// 页面找不到就照旧用原生胶囊。

var voiceStateEvent = mygo.NewEvent[webVoiceState]("voice:state")

// webVoiceState 推给页面的胶囊状态（字段改了要同步 SKILL.md）
type webVoiceState struct {
	Kind   string    `json:"kind"`             // recording | recognizing | text | hidden
	Level  float64   `json:"level"`            // 录音时最新音量，已归一到 0~1
	Levels []float64 `json:"levels,omitempty"` // 最近的原始音量（约 0~0.1），旧的在前
	Live   string    `json:"live,omitempty"`   // 边说边出的字（开了实时出字才有）
	Text   string    `json:"text,omitempty"`   // kind=text 时要显示的话
	Hint   string    `json:"hint,omitempty"`   // 附加提示，如「润色失败，保留原话」
	Copy   bool      `json:"copy,omitempty"`   // 该显示复制钮
	Retry  bool      `json:"retry,omitempty"`  // 该显示重试钮
	Undo   bool      `json:"undo,omitempty"`   // 该显示撤销钮
	Tone   string    `json:"tone"`             // 设置里选的底色：dark | light | auto
}

// webStyleManifest 样式文件夹里的 manifest.json，全可省
type webStyleManifest struct {
	Name     string `json:"name"`     // 设置里显示的名字，默认文件夹名
	Width    int    `json:"width"`    // 窗口大小（DIP），默认 260×110
	Height   int    `json:"height"`   //
	Position string `json:"position"` // bottom（默认，屏幕底部居中）| top | center
	OffsetX  int    `json:"offsetX"`  // 再挪一点（DIP，正数往右/往下）
	OffsetY  int    `json:"offsetY"`  //
	Dev      bool   `json:"dev"`      // 开发模式：文件一改就重载、可开 DevTools
}

func loadWebStyle(dir string) (webStyleManifest, error) {
	m := webStyleManifest{Name: filepath.Base(dir)}
	if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
		return m, err
	}
	if b, err := os.ReadFile(filepath.Join(dir, "manifest.json")); err == nil {
		if err := json.Unmarshal(b, &m); err != nil {
			return m, err
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

type webHitRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

type webVoice struct {
	name    string
	man     webStyleManifest
	win     *mygo.Window
	failed  string // 加载不了的样式名，别每帧都去试
	shown   bool
	gen     int
	shownAt time.Time
	painted bool
	x, y    int32  // 窗口左上角（物理像素）
	dpi     uint32 //
	hits    []webHitRect
	hitSet  bool // 页面报过可点区域；没报过就整窗都接鼠标
	through bool // 眼下鼠标穿透

	optAt   time.Time // 设置页样式列表的缓存
	options []option
}

func (a *App) webVoiceStylesDir() string { return filepath.Join(filepath.Dir(a.cfgPath), "styles") }

// webVoiceWindow 当前样式对应的网页窗口；不是网页样式或加载不了时返回 nil（界面线程）
func (a *App) webVoiceWindow() *mygo.Window {
	w := &a.voiceWeb
	name, ok := strings.CutPrefix(a.voiceStyle, "web:")
	if !ok || name == "" || name == w.failed {
		return nil
	}
	if w.win != nil && w.name == name {
		return w.win
	}
	a.dropWebVoice()
	dir := filepath.Join(a.webVoiceStylesDir(), name)
	man, err := loadWebStyle(dir)
	if strings.ContainsAny(name, `/:.`) || strings.ContainsRune(name, filepath.Separator) || err != nil { // 名字只能是一层文件夹名
		log.Printf("voice web: style %q unusable (%v), using the native pill", name, err)
		w.failed = name
		return nil
	}
	devtools := mygo.DevToolsDisabled
	if man.Dev {
		devtools = mygo.DevToolsEnabled
	}
	started := time.Now()
	win := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent: a.anchorWin,
		Width:  man.Width,
		Height: man.Height,
		Hidden: true,
		URL:    (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(filepath.Join(dir, "index.html"))}).String(),
		Page:   mygo.PageOptions{PreloadScript: webVoicePreload, DevTools: devtools},
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting {
			e.PreventDefault()
			win.Hide()
		}
	})
	win.Page().OnDidFinishLoad(func() {
		log.Printf("voice web: %s loaded %dms after create", name, time.Since(started).Milliseconds())
	})
	win.Page().OnDidFailLoad(func(err *mygo.LoadError) { log.Printf("voice web: %s failed to load: %v", name, err) })
	win.Page().OnRenderProcessGone(func(reason string) {
		log.Printf("voice web: renderer gone (%s), reloading", reason)
		win.Page().Reload()
	})
	makeFloating(win.NativeHandle())
	a.voiceWebHwnd.Store(win.NativeHandle())
	w.name, w.man, w.win = name, man, win
	w.hits, w.hitSet, w.through = nil, false, false
	if man.Dev {
		go a.watchWebStyle(win, dir)
	}
	return win
}

// dropWebVoice 关掉眼下的网页窗口（换样式、manifest 改了）（界面线程）
func (a *App) dropWebVoice() {
	w := &a.voiceWeb
	if w.win == nil {
		return
	}
	w.win.Destroy()
	a.voiceWebHwnd.Store(0)
	w.win, w.shown, w.name = nil, false, ""
	w.gen++
}

// warmWebVoice 启动和换样式时先把网页建好，按下热键时就不用等 WebView2 冷启动（任意线程）
func (a *App) warmWebVoice(style string) {
	a.voiceWin.Update(func() {
		a.voiceWeb.failed = ""
		if !strings.HasPrefix(style, "web:") {
			a.dropWebVoice() // 换回原生样式就把 WebView2 那几个进程也放掉
			return
		}
		a.voiceStyle = style
		a.webVoiceWindow()
	})
}

// showWebVoice 网页样式时显示网页胶囊并推送状态，返回 false 表示照旧用原生胶囊（界面线程）
func (a *App) showWebVoice(state voiceViewState) bool {
	win := a.webVoiceWindow()
	if win == nil {
		return false
	}
	w := &a.voiceWeb
	if !w.shown {
		dpi, wa, m := a.voiceDPI, a.voiceWA, w.man
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
		placeAt(win.NativeHandle(), x, y, width, height)
		w.x, w.y, w.dpi = x, y, dpi
		win.ShowInactive()
		w.shown, w.shownAt, w.painted = true, time.Now(), false
		w.gen++
		go a.trackWebVoiceMouse(w.gen)
	}
	kind := "text"
	switch {
	case state.recording:
		kind = "recording"
	case state.recognizing:
		kind = "recognizing"
	}
	level := 0.0
	if n := len(state.levels); n > 0 {
		level = math.Min(1, math.Pow(math.Max(0, state.levels[n-1])/0.08, 0.6))
	}
	voiceStateEvent.Emit(win, webVoiceState{
		Kind: kind, Level: level, Levels: state.levels, Live: state.live, Text: state.text, Hint: state.hint,
		Copy: state.copy, Retry: state.retry, Undo: state.undo, Tone: a.voiceTone,
	})
	return true
}

// hideWebVoice 网页胶囊正显示着就让页面播退场、稍后藏窗口，返回 true（界面线程）
func (a *App) hideWebVoice() bool {
	w := &a.voiceWeb
	if !w.shown || w.win == nil {
		return false
	}
	w.shown = false
	w.gen++
	gen, win := w.gen, w.win
	voiceStateEvent.Emit(win, webVoiceState{Kind: "hidden", Tone: a.voiceTone})
	time.AfterFunc(320*time.Millisecond, func() {
		a.voiceWin.Update(func() {
			if w.gen == gen && w.win == win {
				win.Hide()
			}
		})
	})
	return true
}

// trackWebVoiceMouse 胶囊显示期间：鼠标在页面报的可点区域里才接鼠标，别处让点击穿透下去
func (a *App) trackWebVoiceMouse(gen int) {
	for {
		time.Sleep(30 * time.Millisecond)
		alive := make(chan bool, 1)
		a.voiceWin.Update(func() {
			w := &a.voiceWeb
			if w.gen != gen || !w.shown || w.win == nil {
				alive <- false
				return
			}
			through := false
			if w.hitSet {
				pt := cursorPos()
				px := float64(pt.X-w.x) * 96 / float64(w.dpi)
				py := float64(pt.Y-w.y) * 96 / float64(w.dpi)
				through = !hitAny(w.hits, px, py)
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

func hitAny(rs []webHitRect, x, y float64) bool {
	for _, r := range rs {
		if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
			return true
		}
	}
	return false
}

// watchWebStyle 开发模式：样式文件夹里的文件一改就重载页面，manifest 改了整个窗口重建
func (a *App) watchWebStyle(win *mygo.Window, dir string) {
	newest := func() (all, manifest time.Time) {
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if info, err := d.Info(); err == nil {
				if info.ModTime().After(all) {
					all = info.ModTime()
				}
				if d.Name() == "manifest.json" {
					manifest = info.ModTime()
				}
			}
			return nil
		})
		return
	}
	last, lastMan := newest()
	for {
		time.Sleep(700 * time.Millisecond)
		all, man := newest()
		if all.Equal(last) {
			continue
		}
		manChanged := !man.Equal(lastMan)
		last, lastMan = all, man
		alive := make(chan bool, 1)
		a.voiceWin.Update(func() {
			w := &a.voiceWeb
			if w.win != win {
				alive <- false
				return
			}
			if manChanged {
				log.Printf("voice web: %s manifest changed, recreating", w.name)
				a.dropWebVoice()
				a.webVoiceWindow()
				alive <- false // 新窗口有自己的 watcher
				return
			}
			log.Printf("voice web: %s changed, reloading", w.name)
			win.Page().Reload()
			alive <- true
		})
		select {
		case ok := <-alive:
			if !ok {
				return
			}
		case <-time.After(5 * time.Second):
			return
		}
	}
}

// webStyleOptions 设置页样式下拉里的网页样式，2 秒内读一次目录（界面线程）
func (a *App) webStyleOptions() []option {
	w := &a.voiceWeb
	if time.Since(w.optAt) < 2*time.Second {
		return w.options
	}
	w.optAt, w.options = time.Now(), nil
	entries, _ := os.ReadDir(a.webVoiceStylesDir())
	for _, e := range entries {
		if !e.IsDir() || strings.ContainsAny(e.Name(), ".:") {
			continue
		}
		if m, err := loadWebStyle(filepath.Join(a.webVoiceStylesDir(), e.Name())); err == nil {
			w.options = append(w.options, option{"web:" + e.Name(), m.Name})
		}
	}
	sort.Slice(w.options, func(i, j int) bool { return w.options[i].label < w.options[j].label })
	return w.options
}

func (a *App) openWebStyles() {
	os.MkdirAll(a.webVoiceStylesDir(), 0o755)
	mygo.Shell.OpenPath(a.webVoiceStylesDir())
}

// VoiceWeb 给网页胶囊调的（页面里用注入的 window.mami，别直接调这个）
type VoiceWeb struct{ a *App }

// Click 页面上的按钮：cancel / stop / close / copy / retry / undo，和原生胶囊的按钮走同一条路
func (v *VoiceWeb) Click(ctx context.Context, action string) {
	a := v.a
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

// SetHits 页面上带 data-mami-hit 的元素的位置（CSS 像素）；nil = 页面没标，整窗都接鼠标
func (v *VoiceWeb) SetHits(ctx context.Context, rects []webHitRect) {
	a := v.a
	a.voiceWin.Update(func() {
		w := &a.voiceWeb
		w.hits, w.hitSet = rects, rects != nil
	})
}

// Painted 页面画出第一帧时调一次：记下从显示到出画面用了多久，用来看冷启动
func (v *VoiceWeb) Painted(ctx context.Context) {
	a := v.a
	a.voiceWin.Update(func() {
		if w := &a.voiceWeb; w.shown && !w.painted {
			w.painted = true
			log.Printf("voice web: first frame %dms after show", time.Since(w.shownAt).Milliseconds())
		}
	})
}

// webVoicePreload 每张样式页面自己的脚本之前跑：提供稳定的 window.mami，页面不用管 mygo 的接口
const webVoicePreload = `(function () {
  if (window.mami) return;
  var handlers = [], last = null, painted = false, lastHits = "";
  function host(fn) { if (window.mygo) fn(window.mygo); else setTimeout(function () { host(fn); }, 16); }
  window.mami = {
    version: 1,
    platform: "windows",
    get state() { return last; },
    onState: function (fn) {
      handlers.push(fn);
      if (last) { try { fn(last); } catch (e) { console.error(e); } }
      return function () { handlers = handlers.filter(function (h) { return h !== fn; }); };
    },
    action: function (name) { host(function (m) { m.call("VoiceWeb.Click", String(name)); }); }
  };
  host(function (m) {
    m.on("voice:state", function (s) {
      last = s;
      handlers.slice().forEach(function (h) { try { h(s); } catch (e) { console.error(e); } });
      if (!painted && s.kind !== "hidden") {
        painted = true;
        requestAnimationFrame(function () { m.call("VoiceWeb.Painted"); });
      }
    });
  });
  function scanHits() {
    var els = document.querySelectorAll("[data-mami-hit]"), rects = [];
    for (var i = 0; i < els.length; i++) {
      var b = els[i].getBoundingClientRect(), st = getComputedStyle(els[i]);
      if (b.width > 0 && b.height > 0 && st.visibility !== "hidden" && st.pointerEvents !== "none" && Number(st.opacity) > 0.05)
        rects.push({ x: Math.floor(b.left), y: Math.floor(b.top), w: Math.ceil(b.width), h: Math.ceil(b.height) });
    }
    var key = els.length ? JSON.stringify(rects) : "all";
    if (key !== lastHits) {
      lastHits = key;
      host(function (m) { m.call("VoiceWeb.SetHits", els.length ? rects : null); });
    }
    requestAnimationFrame(scanHits);
  }
  requestAnimationFrame(scanHits);
})();`

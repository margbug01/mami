//go:build windows

package app

import (
	"context"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// 网页胶囊（试验）：voice_style 写成 "web:<名字>" 时，胶囊换成
// %APPDATA%\Mami\styles\<名字>\index.html 这张透明网页。Mami 每次更新胶囊都把状态推给页面
// （voice:state 事件），页面自己画；页面上的按钮调 VoiceWeb.Click。找不到页面就照旧用原生胶囊。
// shortcut: 原型，窗口尺寸写死、设置页里选不到、Mac 没接，revisit when 定插件格式（manifest）
const webVoiceW, webVoiceH = 260, 110

var voiceStateEvent = mygo.NewEvent[webVoiceState]("voice:state")

// webVoiceState 推给页面的胶囊状态
type webVoiceState struct {
	Kind   string    `json:"kind"` // recording | recognizing | text | hidden
	Levels []float64 `json:"levels,omitempty"`
	Live   string    `json:"live,omitempty"`
	Text   string    `json:"text,omitempty"`
	Hint   string    `json:"hint,omitempty"`
	Copy   bool      `json:"copy,omitempty"`
	Retry  bool      `json:"retry,omitempty"`
	Undo   bool      `json:"undo,omitempty"`
	Tone   string    `json:"tone"` // dark | light | auto
}

type webVoice struct {
	name    string
	win     *mygo.Window
	failed  string // 加载不了的样式名，别每帧都去试
	shown   bool
	gen     int
	shownAt time.Time
	painted bool
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
	if w.win != nil {
		w.win.Destroy()
		w.win, w.shown = nil, false
	}
	page := filepath.Join(a.webVoiceStylesDir(), name, "index.html")
	if _, err := os.Stat(page); strings.ContainsAny(name, `/\.:`) || err != nil { // 名字只能是一层文件夹名
		log.Printf("voice web: style %q not found (%s), using the native pill", name, page)
		w.failed = name
		return nil
	}
	started := time.Now()
	win := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent: a.anchorWin,
		Width:  webVoiceW,
		Height: webVoiceH,
		Hidden: true,
		URL:    (&url.URL{Scheme: "file", Path: "/" + filepath.ToSlash(page)}).String(),
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
	w.name, w.win = name, win
	return win
}

// warmWebVoice 启动和换样式时先把网页建好，按下热键时就不用等 WebView2 冷启动（任意线程）
func (a *App) warmWebVoice(style string) {
	if !strings.HasPrefix(style, "web:") {
		return
	}
	a.voiceWin.Update(func() {
		a.voiceStyle = style
		a.voiceWeb.failed = ""
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
		dpi, wa := a.voiceDPI, a.voiceWA
		width, height := scale(webVoiceW, dpi), scale(webVoiceH, dpi)
		placeAt(win.NativeHandle(), wa.Left+(wa.w()-width)/2, wa.Bottom-height-scale(4, dpi), width, height)
		win.ShowInactive()
		w.shown, w.shownAt, w.painted = true, time.Now(), false
		w.gen++
	}
	kind := "text"
	switch {
	case state.recording:
		kind = "recording"
	case state.recognizing:
		kind = "recognizing"
	}
	voiceStateEvent.Emit(win, webVoiceState{
		Kind: kind, Levels: state.levels, Live: state.live, Text: state.text, Hint: state.hint,
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
	time.AfterFunc(260*time.Millisecond, func() {
		a.voiceWin.Update(func() {
			if w.gen == gen && w.win == win {
				win.Hide()
			}
		})
	})
	return true
}

// VoiceWeb 给网页胶囊调的：mygo.call("VoiceWeb.Click", "cancel")
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

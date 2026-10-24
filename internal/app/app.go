//go:build windows

package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 按钮和卡片的尺寸（DIP）
const (
	btnW, btnH   = 38, 38 // 顶层窗口高度系统有下限（约 38 DIP），做成正方形
	popW, toastW = 380, 210
	bodyMaxH     = 320
)

const (
	statusLoading = iota
	statusDone
	statusError
)

// popupState 只在主线程读写（view 里读，别处一律经 popWin.Update 改）
type popupState struct {
	shownAt    time.Time
	heightAt   time.Time
	heightFrom float32
	placedH    int
	above      bool
	status     int
	toast      bool // 快捷键原地替换：只显示一张小的「翻译中…」
	header     string
	text       string
	reqID      int64
	replace    bool
	fg         uintptr
	anchor     POINT
	copied     bool
	terminal   bool   // 来自终端：卡片只给「复制」
	histID     string // 翻译记录里这一条的 id；没开记录时为空
	acted      bool   // 复制/替换/关闭过，关掉卡片时不再记 dismiss
	// selectedAll 是 Mami 自己全选的（快捷键时没选中）：关掉卡片要把选区收回去，
	// 不然整段字一直选着，下一个按键就全没了
	selectedAll bool
	w, h        int // 当前窗口内容尺寸，view 量出新高度时会改
}

type App struct {
	cfgPath, logPath string
	hist             *history
	logPaths         []string // 「记录」页从这些日志里补开记录之前的次数
	pid              uint32

	mu  sync.Mutex
	cfg Config
	tr  *Translator

	dock                                              voiceDock
	dockHwnd                                          atomic.Uintptr
	voiceWin                                          *mygo.Window
	voiceWeb                                          webVoice // 网页胶囊（voice_web_windows.go），只在界面线程用
	voiceUI                                           voiceViewState
	voicePrevious                                     voiceViewState
	voiceWA                                           RECT // 以下三个只在界面线程读写：胶囊所在显示器的工作区
	voiceDPI                                          uint32
	voiceShownW                                       int32
	voiceLeaving                                      bool
	voiceShowAt                                       time.Time // 弹出、退场、宽度变化、内容切换各自的起点
	voiceHideAt                                       time.Time
	voiceResumeAt                                     time.Time
	voiceResumeOpacity, voiceResumeSize, voiceResumeY float32
	voiceKindAt                                       time.Time
	voiceWAt                                          time.Time
	voiceWFrom, voiceWTo                              float32
	voiceStyle                                        string
	voiceTranslate                                    bool
	voiceTarget                                       string
	voiceTone                                         string
	voiceJoined                                       bool
	voiceOrbSize                                      float32
	voiceFlowPalette                                  string
	voiceFrameAt                                      time.Time
	voiceHover                                        bool               // 鼠标在胶囊上：带复制钮的提示先不收
	voiceCopiedAt                                     time.Time          // 点了胶囊上的复制，钮变成 ✓
	voiceRetry                                        func()             // 胶囊上的重试钮：拿上次失败的录音再识别一次（界面线程读写）
	voiceUndo                                         func()             // 胶囊上的撤销钮：撤回刚才的改写
	voiceBarH                                         [voiceBars]float32 // 平滑后的柱高 / 球大小（0~1）
	voiceOrbLv                                        float32
	voiceGapL, voiceGapR                              float32 // 三岛样式：上次切换时两道缝的宽度，从这里过渡到新状态
	voiceHideGen                                      int
	btnWin, popWin, setWin                            *mygo.Window
	anchorWin                                         *mygo.Window
	btnHwnd, popHwnd                                  atomic.Uintptr
	voiceHwnd                                         atomic.Uintptr
	tray                                              *mygo.Tray
	quitting                                          bool

	pop        popupState // 主线程
	popVisible atomic.Bool
	upd        updateState
	popBusy    atomic.Bool // 卡片正在等译文：点别处不关，免得结果被丢掉
	reqSeq     atomic.Int64
	quietID    atomic.Int64 // 静默翻译中的请求号，0 表示没有

	btnShown  atomic.Bool
	busyMoved atomic.Bool // 转圈期间用户点过别处或按过键：焦点/选区可能变了，不能盲贴
	btnBusy   atomic.Bool // 静默翻译中：按钮变成转圈，点别处/打字都不收，翻完才收
	btnHover  atomic.Bool

	// 新手引导（guide.go）：哪一步正摆在设置窗口里，触发键和 Alt+T 先交给引导
	guideVoice, guideTrans atomic.Bool
	guideRec               atomic.Uint64  // 引导正在听的那次录音的代号
	setHwnd                atomic.Uintptr // 设置窗口，引导只接在它前台时按的键
	btnGen                 atomic.Int64
	btnAnchor              atomic.Uint64 // packPt(按钮对应的松开位置)

	// 鼠标手势状态，只在 mouseLoop 那个 goroutine 里用
	downPt     POINT
	downIBeam  bool
	lastUpPt   POINT
	lastUpTime uint32
	dblTime    uint32

	learn   *learner
	samples *sampleLog // 长文口述改前改后的样本，以后学说话习惯用
	learnW  learnWatch
	uiaOK   bool
	set     settingsState // 主线程
	avatar  *ui.Bitmap
}

func (a *App) config() (Config, *Translator) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg, a.tr
}

func (a *App) setConfig(cfg Config) error {
	cfg.normalize()
	tr, err := newTranslator(cfg)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg, a.tr = cfg, tr
	a.mu.Unlock()
	voiceTrigger.Store(0)
	voiceEpoch.Add(1)
	if cfg.VoiceEnabled && !cfg.DisableHotkeys {
		voiceTrigger.Store(voiceKey(cfg.VoiceHotkey))
	}
	if a.voiceWin != nil {
		a.warmWebVoice(cfg.VoiceStyle)
	}
	return nil
}

// ---------- 窗口 ----------

// minW/minH 必须给：不给的话 Windows 按带标题栏窗口的最小尺寸算（约 136×38 DIP），小按钮会被撑宽
func (a *App) floatingWindow(title string, w, h, minW, minH int, view func(c *ui.Context)) (*mygo.Window, uintptr) {
	// 浮出层（Flyout）：无框、不画系统阴影、不抢键盘，窗口本身透明合成，
	// 圆角由界面自己抗锯齿地画。它必须有个 Parent，用一个隐藏的小窗口充当。
	if a.anchorWin == nil {
		a.anchorWin = mygo.NewWindow(mygo.WindowOptions{
			Title: "mami-anchor", Width: 1, Height: 1, Hidden: true, Frameless: true,
			SkipTaskbar: true, Content: ui.View(func(c *ui.Context) {}),
		})
	}
	win := mygo.NewFlyout(mygo.FlyoutOptions{
		Parent:  a.anchorWin,
		Width:   w,
		Height:  h,
		Hidden:  true,
		Content: ui.View(view),
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		if !a.quitting { // Alt+F4 之类只是藏起来
			e.PreventDefault()
			win.Hide()
		}
	})
	hwnd := win.NativeHandle()
	makeFloating(hwnd)
	return win, hwnd
}

func (a *App) createWindows() {
	var h uintptr
	a.btnWin, h = a.floatingWindow("mami-button", btnW, btnH, btnW, btnH, a.buttonView)
	a.btnHwnd.Store(h)
	a.popWin, h = a.floatingWindow("mami-popup", popW, 120, toastW, 40, a.popupView)
	a.popHwnd.Store(h)
	a.voiceWin, h = a.floatingWindow("mami-voice", voiceRecW, voiceWinH, voiceDotsW, voiceWinH, a.voiceView)
	a.voiceHwnd.Store(h)
	a.createDock()
}

func (a *App) isOurs(p POINT) bool {
	h, _, _ := pWindowFromPoint.Call(packPt(p))
	root, _, _ := pGetAncestor.Call(h, GA_ROOT)
	return root != 0 && (root == a.btnHwnd.Load() || root == a.popHwnd.Load() || root == a.voiceHwnd.Load() || root == a.dockHwnd.Load())
}

// ---------- 鼠标手势 ----------

func near(a, b POINT, d int32) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx+dy*dy <= d*d
}

func (a *App) mouseLoop() {
	for ev := range mouseEvents {
		a.onMouse(ev)
	}
}

func (a *App) onMouse(ev mouseEvent) {
	switch ev.kind {
	case mouseDown:
		if ev.ours {
			return
		}
		a.btnGen.Add(1) // 取消还没弹出来的按钮
		if a.btnBusy.Load() {
			a.busyMoved.Store(true)
		}
		a.hideButton()
		if !a.popBusy.Load() {
			a.hidePopup()
		}
		a.downPt, a.downIBeam = ev.pt, ev.ibeam
	case mouseUp:
		if ev.ours {
			return
		}
		drag := !near(a.downPt, ev.pt, 6)
		dbl := ev.time-a.lastUpTime <= a.dblTime && near(a.lastUpPt, ev.pt, 6)
		a.lastUpPt, a.lastUpTime = ev.pt, ev.time
		cfg, _ := a.config()
		if !cfg.ShowButton || !(drag || dbl) {
			return
		}
		// Windows Terminal 等自己画 I 形光标，系统认不出来；终端里靠「选区非空」把关
		if cfg.RequireIBeam && !a.downIBeam && !ev.ibeam && !isTerminal(foreground()) {
			return
		}
		if a.excluded(foreground(), cfg) {
			return
		}
		gen := a.btnGen.Add(1)
		pt := ev.pt
		time.AfterFunc(140*time.Millisecond, func() {
			if a.btnGen.Load() != gen || keyDown(VK_LBUTTON) { // 左键还按着，说明在拖别的东西
				return
			}
			at := pt
			if cfg.OnlyInInputs {
				fi := a.inInput(foreground(), pt)
				if !fi.ok && !(fi.blind && cfg.ShowInBlindApps) {
					if fi.blind {
						log.Printf("no button: 认不出输入框的程序，设置里「认不出输入框的程序也弹」关着")
					}
					return
				}
				if fi.hasSel {
					at = selectionAnchor(fi.sel, pt)
				}
			}
			if a.btnGen.Load() != gen { // 查询期间又点了别处
				return
			}
			if a.btnBusy.Load() { // 上一次还在翻译，按钮被转圈占着
				return
			}
			a.showButton(at, gen)
		})
	case mouseOther:
		if !ev.ours {
			a.hideButton()
			if !a.popBusy.Load() {
				a.hidePopup()
			}
		}
	case mouseWheel:
		if !ev.ours {
			a.hideButton()
		}
	case keyPress: // 打字、删除、方向键都会改掉选区，按钮立刻收起
		if a.btnBusy.Load() {
			a.busyMoved.Store(true)
		}
		a.btnGen.Add(1)
		a.hideButton()
		a.learnTyped(ev.vk)
	}
}

func (a *App) inInput(fg uintptr, pt POINT) focusInfo {
	if fg == 0 {
		return focusInfo{}
	}
	if !a.uiaOK {
		return focusIsEditable(0, fg, pt)
	}
	return focusEditable(fg, pt)
}

// selectionAnchor 把按钮挪到选区末尾（最后一行右下角），而不是鼠标松开的地方。
// 返回的是「假想的鼠标位置」，showButton 会在它右下方 8/12 处放按钮。
// 选区框离谱（不在任何屏幕上）就还用鼠标位置。
func selectionAnchor(sel RECT, mouse POINT) POINT {
	end := POINT{sel.Right, sel.Bottom}
	wa := workArea(end)
	if end.X < wa.Left-50 || end.X > wa.Right+50 || end.Y < wa.Top-50 || end.Y > wa.Bottom+50 {
		return mouse
	}
	dpi := dpiAt(end)
	return POINT{end.X - scale(4, dpi), end.Y - scale(8, dpi)}
}

func (a *App) excluded(h uintptr, cfg Config) bool {
	if h == 0 {
		return false
	}
	pid := windowPID(h)
	if pid == a.pid { // 自己的设置窗口里选字不弹
		return true
	}
	if len(cfg.ExcludeProcesses) == 0 {
		return false
	}
	name := strings.ToLower(filepath.Base(processName(pid)))
	for _, ex := range cfg.ExcludeProcesses {
		if strings.ToLower(strings.TrimSpace(ex)) == name {
			return true
		}
	}
	return false
}

// ---------- 浮动按钮 ----------

func (a *App) showButton(pt POINT, gen int64) {
	dpi := dpiAt(pt)
	s := func(v int32) int32 { return scale(v, dpi) }
	x, y := clampInto(pt.X+s(8), pt.Y+s(12), s(btnW), s(btnH), pt.Y-s(12), workArea(pt))
	a.btnAnchor.Store(uint64(packPt(pt)))
	a.btnHover.Store(false)
	placeAt(a.btnHwnd.Load(), x, y, s(btnW), s(btnH))
	a.btnWin.ShowInactive()
	a.btnShown.Store(true)
	cfg, _ := a.config()
	a.scheduleButtonHide(gen, time.Duration(cfg.ButtonTimeoutMs)*time.Millisecond)
}

func (a *App) scheduleButtonHide(gen int64, d time.Duration) {
	time.AfterFunc(d, func() {
		if a.btnGen.Load() != gen {
			return
		}
		if a.btnHover.Load() {
			a.scheduleButtonHide(gen, 1500*time.Millisecond)
			return
		}
		a.hideButton()
	})
}

func (a *App) hideButton() {
	if a.btnBusy.Load() {
		return // 正在转圈，等 endBusy 收
	}
	if a.btnShown.Swap(false) {
		a.btnWin.Hide()
	}
}

func (a *App) onButtonClick() {
	if a.btnBusy.Load() {
		return // 转圈时再点没用
	}
	a.btnGen.Add(1) // 让自动收起的定时器作废
	up := POINT{int32(uint32(a.btnAnchor.Load())), int32(uint32(a.btnAnchor.Load() >> 32))}
	dpi := dpiAt(up)
	cfg, _ := a.config()
	anchor := POINT{up.X + scale(8, dpi), up.Y + scale(12, dpi)}
	switch cfg.ButtonAction {
	case actionResult, actionReplace:
		// 不弹卡片的两种模式：按钮留在原地转圈，翻完（或出错、超时）再收
		a.busyMoved.Store(false)
		a.btnBusy.Store(true)
		go a.btnWin.Update(func() {})
		a.startTranslate(cfg.ButtonAction == actionReplace, true, anchor)
	default:
		a.btnGen.Add(1)
		a.hideButton()
		a.startTranslate(false, false, anchor)
	}
}

// endBusy 收起转圈的按钮（可在任何线程调用）
func (a *App) endBusy() {
	if a.btnBusy.Swap(false) {
		a.hideButton()
	}
}

// ---------- 翻译流程 ----------

func (a *App) onHotkey(replace bool) {
	if a.guideTrans.Load() && foreground() == a.setHwnd.Load() { // 引导里那句话：不抓选区、不发请求
		a.setWin.Update(func() { a.guideHotkey(replace) })
		return
	}
	theWorker.do(func(owner uintptr) {
		if keyDown(VK_MENU) {
			sendKeys(KEYBDINPUT{WVk: VK_MASK}, KEYBDINPUT{WVk: VK_MASK, DwFlags: KEYEVENTF_KEYUP})
		}
		waitModifiersUp(time.Second) // 不然模拟的 Ctrl+C 会变成 Ctrl+Alt+C
		a.btnGen.Add(1)
		a.hideButton()
		pt := cursorPos()
		dpi := dpiAt(pt)
		a.grabAndTranslate(owner, replace, true, false, POINT{pt.X + scale(12, dpi), pt.Y + scale(16, dpi)})
	})
}

// startTranslate 给划词按钮用。quiet：不弹「翻译中…」，结果出来再说
func (a *App) startTranslate(replace, quiet bool, anchor POINT) {
	theWorker.do(func(owner uintptr) { a.grabAndTranslate(owner, replace, false, quiet, anchor) })
}

// grabAndTranslate 在 worker 线程上执行
func (a *App) grabAndTranslate(owner uintptr, replace, fromHotkey, quiet bool, anchor POINT) {
	a.hidePopup()
	cfg, tr := a.config()
	fg := foreground()
	term := isTerminal(fg)
	wantReplace := replace
	var text string
	var selectedAll, noInput bool
	if term {
		// 终端：没选中时 Ctrl+C 是中断信号，Ctrl+A 各 TUI 意思也不一样，所以一个键都不发，
		// 直接经 UIA 读终端自己的选区；替换也做不到（粘贴不会删掉选中的字），只给卡片
		text = selectedTextUIA()
		replace = false
		log.Printf("grab (terminal, uia): %d chars", len([]rune(text)))
	} else {
		text, selectedAll, noInput = grabSelection(owner, fromHotkey && cfg.SelectAllIfEmpty)
	}
	id := a.reqSeq.Add(1)
	st := popupState{reqID: id, replace: replace, fg: fg, anchor: anchor, toast: replace, terminal: term, selectedAll: selectedAll}
	if cfg.History && !term { // 终端里选的多半是命令输出，不记
		st.histID = historyID(time.Now(), id)
	}

	if term && strings.TrimSpace(text) == "" {
		st.toast, st.status, st.header, st.text = false, statusError, "Mami", T("终端里要先用鼠标选中文字。为了不误发 Ctrl+C 中断正在跑的程序，终端里不会自动全选。")
		a.endBusy()
		a.showPopup(st)
		return
	}
	if noInput {
		st.toast, st.status, st.header, st.text = false, statusError, "Mami", T("没选中文字，光标也不在输入框里。为了不把整页内容都选上，这里不会自动全选。")
		a.endBusy()
		a.showPopup(st)
		return
	}
	if strings.TrimSpace(text) == "" {
		log.Printf("grab selection: nothing copied (fg=%#x hotkey=%v)", fg, fromHotkey)
		st.toast, st.status, st.header, st.text = false, statusError, "Mami", T("没取到选中的文字。这个窗口可能不支持 Ctrl+C。")
		a.endBusy()
		a.showPopup(st)
		return
	}
	if n := utf8.RuneCountInString(text); n > cfg.MaxChars {
		st.toast, st.status, st.header = false, statusError, "Mami"
		st.text = fmt.Sprintf(T("选中了 %d 个字，超过上限 %d。"), n, cfg.MaxChars)
		a.endBusy()
		a.showPopup(st)
		return
	}
	target, header := pickTarget(text, cfg)
	st.header = header
	if term && wantReplace {
		st.header += T(" · 终端里不替换")
	}
	if quiet {
		a.quietID.Store(id)
	} else {
		a.quietID.Store(0)
		a.endBusy() // 快捷键顶掉了静默翻译，别让转圈卡住
		a.showPopup(st)
	}
	log.Printf("translate #%d: %d chars → %s (replace=%v)", id, utf8.RuneCountInString(text), target, replace)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
		defer cancel()
		start := time.Now()
		var first time.Duration
		var last time.Time
		// 请求总是流式的（断了还能留下半截）；只有开了「边翻边显示」、卡片正显示着，才把半截填进卡片
		showPartial := cfg.StreamCard && !quiet && !replace
		res, err := tr.TranslateStream(ctx, text, target, func(p string) {
			if first == 0 {
				first = time.Since(start)
			}
			if !showPartial || time.Since(last) < 150*time.Millisecond {
				return // 收得太密就跳过几帧，最后的结果总会整段填进去
			}
			last = time.Now()
			a.popWin.Update(func() { a.onPartial(id, p) })
		})
		total := time.Since(start)
		if err != nil {
			log.Printf("translate #%d failed after %v: %v", id, total.Round(time.Millisecond), err)
		} else {
			log.Printf("translate #%d ok in %v", id, total.Round(time.Millisecond))
		}
		if st.histID != "" {
			e := historyEntry{ID: st.histID, Time: start.Format(time.RFC3339), App: filepath.Base(processName(windowPID(fg))),
				Mode: "card", FirstMs: first.Milliseconds(), TotalMs: total.Milliseconds(), Translation: res}
			if replace {
				e.Mode = "replace"
			}
			var part *errPartial
			if errors.As(err, &part) {
				e.Translation, e.Incomplete = part.text, true
			}
			if err != nil {
				e.Error = err.Error()
			}
			a.recordTranslation(cfg, tr, e, text, target)
		}
		if quiet {
			a.onQuietResult(id, st, res, err)
			return
		}
		a.popWin.Update(func() { a.onResult(id, res, err) })
	}()
}

// onQuietResult 静默模式（没弹加载卡片）的结果：替换模式能贴就直接贴，否则弹出结果/错误卡片。
// 在翻译的 goroutine 里调用，不在主线程，所以可以用 showPopup。
func (a *App) onQuietResult(id int64, st popupState, res string, err error) {
	if !a.quietID.CompareAndSwap(id, 0) {
		return // 期间又发起了新的翻译
	}
	a.endBusy()
	st.toast = false
	var part *errPartial
	switch {
	case err != nil && !st.replace && errors.As(err, &part):
		st.status, st.text = statusDone, part.text
		st.header += T(" · 连接中断，译文不完整")
	case err != nil:
		st.status, st.text = statusError, err.Error()
	case st.replace && foreground() == st.fg && !a.busyMoved.Load():
		theWorker.paste(res)
		a.recordAction(st.histID, "replace")
		return
	default:
		if st.replace && foreground() != st.fg {
			st.header += T(" · 窗口已切换，没有替换")
		} else if st.replace {
			st.header += T(" · 期间点了别处，没有替换")
		}
		st.status, st.text = statusDone, res
	}
	a.showPopup(st)
}

// onPartial 在主线程上执行：流式收到的半截译文先填进还在「翻译中」的卡片
func (a *App) onPartial(id int64, text string) {
	p := &a.pop
	if !a.popVisible.Load() || p.reqID != id || p.status != statusLoading || p.toast {
		return
	}
	p.text = text
}

// onResult 在主线程上执行
func (a *App) onResult(id int64, res string, err error) {
	p := &a.pop
	if !a.popVisible.Load() || p.reqID != id {
		return // 已经被关掉或者被新请求顶掉了
	}
	a.popBusy.Store(false)
	var part *errPartial
	if err != nil && !p.replace && errors.As(err, &part) {
		p.status, p.text = statusDone, part.text
		p.header += T(" · 连接中断，译文不完整")
		return
	}
	if err != nil {
		p.toast, p.status, p.text = false, statusError, err.Error()
		return
	}
	if p.replace {
		if foreground() == p.fg {
			theWorker.paste(res)
			p.acted = true
			a.recordAction(p.histID, "replace")
			a.hidePopupNow()
			return
		}
		p.header += T(" · 窗口已切换，没有替换")
	}
	p.toast, p.status, p.text = false, statusDone, res
}

// showPopup 可以在任何线程调用
func (a *App) showPopup(st popupState) {
	st.w = popW
	if st.toast {
		st.w = toastW
	}
	st.h = estimateHeight(st)
	a.popWin.Update(func() {
		a.popWin.Hide()
		st.shownAt = time.Now()
		st.heightFrom, st.placedH = float32(st.h), st.h
		area, dpi := workArea(st.anchor), dpiAt(st.anchor)
		below, above := area.Bottom-st.anchor.Y, st.anchor.Y-scale(36, dpi)-area.Top
		st.above = below < scale(bodyMaxH+12+22+8+12+8+32, dpi) && above > below
		a.pop = st
		a.resizePopup(st.w, st.h, st.anchor)
		a.popBusy.Store(st.status == statusLoading)
		a.popVisible.Store(true)
		a.popWin.ShowInactive()
	})
}

// estimateHeight 给第一帧一个差不多的高度，view 量出准确值后会再调
func estimateHeight(st popupState) int {
	h := 12 + 22 + 8 + 20 + 12
	if st.status != statusLoading {
		lines := 1 + utf8.RuneCountInString(st.text)/24 + strings.Count(st.text, "\n")
		h = 12 + 22 + 8 + min(lines*22, bodyMaxH) + 12
	}
	if st.status == statusDone {
		h += 8 + 32
	}
	return h
}

func (a *App) hidePopup() {
	cardKeyMode.Store(0)
	a.popBusy.Store(false)
	if a.popVisible.Swap(false) {
		a.popWin.Update(func() {
			a.noteDismiss("dismiss")
			a.pop.reqID = 0
		})
		a.popWin.Hide()
	}
}

// noteDismiss 在主线程上：译文卡片什么都没做就关掉了，记一笔（close=点了 X，dismiss=点别处/被新翻译顶掉）
func (a *App) noteDismiss(action string) {
	p := &a.pop
	if p.status == statusDone && !p.acted && p.histID != "" {
		p.acted = true
		a.recordAction(p.histID, action)
	}
}

// closeCard / replaceFromCard 在主线程上：卡片的 X / 替换按钮和 Esc / Enter 共用
func (a *App) closeCard() {
	p := &a.pop
	if p.selectedAll && foreground() == p.fg {
		theWorker.do(func(uintptr) { collapseSelection() })
	}
	a.noteDismiss("close")
	a.hidePopupNow()
}

func (a *App) replaceFromCard() {
	p := &a.pop
	if p.status != statusDone || p.terminal {
		return
	}
	text := p.text
	p.acted = true
	a.recordAction(p.histID, "replace")
	a.hidePopupNow()
	theWorker.paste(text)
}

// onCardKey 在键盘钩子起的 goroutine 里调用：卡片开着时按的 Enter / Esc
func (a *App) onCardKey(vk uint32) {
	a.popWin.Update(func() {
		if !a.popVisible.Load() || a.pop.toast {
			return
		}
		switch vk {
		case vkEscape:
			a.closeCard()
		case vkReturn:
			a.replaceFromCard()
		}
	})
}

// hidePopupNow 在主线程（view / Update 回调）里用，不能再套 Update
func (a *App) hidePopupNow() {
	cardKeyMode.Store(0)
	a.popBusy.Store(false)
	if a.popVisible.Swap(false) {
		a.noteDismiss("dismiss")
		a.pop.reqID = 0
		a.popWin.Hide()
	}
}

func (a *App) resizePopup(w, h int, anchor POINT) {
	dpi := dpiAt(anchor)
	width, height := scale(int32(w), dpi), scale(int32(h), dpi)
	area := workArea(anchor)
	x, y := anchor.X, anchor.Y
	if a.pop.above {
		y -= scale(36, dpi) + height
	}
	x = max(area.Left, min(x, area.Right-width))
	y = max(area.Top, min(y, area.Bottom-height))
	placeAt(a.popHwnd.Load(), x, y, width, height)
}

func (p *popupState) animatedHeight(now time.Time) float32 {
	amount := ui.EaseOut(progress(now, p.heightAt, 180*time.Millisecond))
	return p.heightFrom + (float32(p.h)-p.heightFrom)*amount
}

func toCRLF(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.ReplaceAll(s, "\n", "\r\n")
}

// ---------- 快捷键 / 托盘 ----------

func (a *App) registerHotkeys(cfg Config) []string {
	mygo.GlobalShortcut.UnregisterAll()
	var problems []string
	if cfg.DisableHotkeys {
		return nil
	}
	for _, hk := range []struct {
		acc     string
		replace bool
	}{{cfg.Hotkey, true}, {cfg.PopupHotkey, false}} {
		acc := strings.TrimSpace(hk.acc)
		if acc == "" {
			continue
		}
		replace := hk.replace
		if err := mygo.GlobalShortcut.Register(acc, func() { go a.onHotkey(replace) }); err != nil {
			problems = append(problems, acc+T("：")+err.Error())
		}
	}
	if acc := cfg.VoiceHotkey; cfg.VoiceEnabled && strings.Contains(acc, "+") {
		if err := mygo.GlobalShortcut.Register(acc, voiceToggle); err != nil {
			problems = append(problems, T("语音")+" "+acc+T("：")+err.Error())
		}
	}
	return problems
}

func (a *App) createTray() {
	var err error
	a.tray, err = mygo.NewTray(mygo.TrayOptions{Icon: trayPNG, ToolTip: "Mami"})
	if err != nil {
		log.Println("tray:", err)
		return
	}
	a.updateTrayTip()
	a.tray.OnClick(a.openSettings)
	a.tray.OnRightClick(func() {
		cfg, _ := a.config()
		var items []*mygo.MenuItem
		if _, _, latest, _, busy, _ := a.upd.snapshot(); latest != nil {
			items = append(items, &mygo.MenuItem{Label: fmt.Sprintf(T("更新到 %s"), latest.version), Disabled: busy,
				Click: func(*mygo.MenuItem, *mygo.Window) { a.startUpdate() }}, mygo.Separator())
		}
		menu := mygo.NewMenu(append(items, []*mygo.MenuItem{
			{Label: T("设置…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.openSettings() }},
			{Label: T("新手引导…"), Click: func(*mygo.MenuItem, *mygo.Window) { a.openGuide() }},
			{Label: T("划词后显示翻译按钮"), Type: mygo.MenuItemCheckbox, Checked: cfg.ShowButton, Click: func(item *mygo.MenuItem, _ *mygo.Window) {
				a.toggleButton(item.Checked)
			}},
			{Label: T("贴边小条"), Type: mygo.MenuItemCheckbox, Checked: cfg.VoiceDock, Click: func(item *mygo.MenuItem, _ *mygo.Window) {
				a.changeDockConfig(func(cfg *Config) { cfg.VoiceDock = item.Checked })
			}},
			{Label: T("语音直接翻译"), Type: mygo.MenuItemCheckbox, Checked: cfg.VoiceTranslate, Click: func(item *mygo.MenuItem, _ *mygo.Window) {
				a.changeDockConfig(func(cfg *Config) { cfg.VoiceTranslate = item.Checked })
			}},
			{Label: T("打开日志"), Click: func(*mygo.MenuItem, *mygo.Window) { mygo.Shell.OpenPath(a.logPath) }},
			{Label: T("打开翻译记录"), Disabled: !cfg.History, Click: func(*mygo.MenuItem, *mygo.Window) { a.openHistory() }},
			mygo.Separator(),
			{Label: T("退出"), Click: func(*mygo.MenuItem, *mygo.Window) { a.quit() }},
		}...))
		a.tray.SetMenu(menu)
		defer a.tray.SetMenu(nil)
		a.tray.PopUpMenu()
	})
}

func (a *App) updateTrayTip() {
	if a.tray == nil {
		return
	}
	a.tray.SetToolTip("Mami")
}

func (a *App) toggleButton(on bool) {
	cfg, _ := a.config()
	cfg.ShowButton = on
	if err := saveConfig(a.cfgPath, cfg); err != nil {
		log.Println("save config:", err)
	}
	a.setConfig(cfg)
	if !on {
		a.hideButton()
	}
}

func (a *App) quit() {
	dockQuit.Store(true)
	a.quitting = true
	if a.tray != nil {
		a.tray.Destroy()
	}
	mygo.App.Quit()
}

func openLog(path string) *os.File {
	if st, err := os.Stat(path); err == nil && st.Size() > 1<<20 {
		os.Remove(path)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	return f
}

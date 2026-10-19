//go:build windows

package app

import (
	"context"
	"math"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

var (
	voiceTrigger   atomic.Uint32
	voiceEpoch     atomic.Uint64
	voiceActive    atomic.Uint64
	voiceRecording atomic.Uint64 // 正在录音的那次；键盘、胶囊按钮、录音结束都会清掉它
	voiceKeyboard  voiceKeys     // 仅钩子线程使用
	voicePressID   uint64
	voiceEvents    = make(chan voiceEvent, 32)
)

type voiceEvent struct {
	action int
	id     uint64
	fg     uintptr
}

func voiceHookKey(key *kbdLLHook, down bool) bool {
	trigger := voiceTrigger.Load()
	// 已按着别的键再按触发键，也属于组合键（包含 AltGr 的合成左 Ctrl）。
	blocked := false
	if down && key.VkCode == trigger && voiceKeyboard.held == 0 {
		for _, vk := range []uint32{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0x5B, 0x5C} {
			if vk != trigger && keyDown(uintptr(vk)) {
				blocked = true
				break
			}
		}
	}
	if voiceKeyboard.recording && voiceRecording.Load() == 0 {
		voiceKeyboard.recording = false // 录音已在别处结束（点了胶囊、超时、出错），下一下重新开始
	}
	held := voiceKeyboard.held
	action, swallow := voiceKeyboard.step(key.VkCode, down, trigger, voiceActive.Load() != 0 || blocked)
	if !down && key.VkCode == held && held == 0xA5 {
		sendKeys(KEYBDINPUT{WVk: VK_MASK}, KEYBDINPUT{WVk: VK_MASK, DwFlags: KEYEVENTF_KEYUP})
	}
	if action == voiceNone {
		return swallow
	}
	if action == voiceStart {
		voicePressID = voiceEpoch.Add(1)
		voiceActive.Store(voicePressID)
		voiceRecording.Store(voicePressID)
	} else {
		voiceRecording.CompareAndSwap(voicePressID, 0)
	}
	postVoice(action, voicePressID)
	return swallow
}

func postVoice(action int, id uint64) {
	if action == voiceCancel {
		voiceEpoch.Add(1) // 当场作废，不能等队列处理后才取消粘贴。
	}
	select {
	case voiceEvents <- voiceEvent{action: action, id: id, fg: foreground()}:
	default:
		voiceEpoch.Add(1) // 队列满时丢弃整次，而不是遗漏结束一直录音。
	}
}

// voiceClick 胶囊上的 ✕ / ✓：和 Esc / 再按一下触发键走同一条路
func voiceClick(action int) {
	if action == voiceCancel {
		if id := voiceActive.Load(); id != 0 && voiceActive.CompareAndSwap(id, 0) {
			voiceRecording.CompareAndSwap(id, 0)
			postVoice(action, id)
		}
		return
	}
	if id := voiceRecording.Load(); id != 0 && voiceRecording.CompareAndSwap(id, 0) {
		postVoice(action, id)
	}
}

func (a *App) voiceLoop() {
	var cancel context.CancelFunc
	var stop chan struct{}
	var id uint64
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case event := <-voiceEvents:
			switch event.action {
			case voiceStart:
				if cancel != nil {
					cancel()
				}
				id = event.id
				cfg, _ := a.config()
				if voiceEpoch.Load() != id || !cfg.VoiceEnabled || cfg.DisableHotkeys || event.fg == 0 || a.excluded(event.fg, cfg) {
					if voiceEpoch.Load() == id {
						voiceEpoch.Add(1)
					}
					voiceActive.CompareAndSwap(id, 0)
					voiceRecording.CompareAndSwap(id, 0)
					continue
				}
				var ctx context.Context
				ctx, cancel = context.WithCancel(context.Background())
				stop = make(chan struct{})
				go a.runVoice(ctx, stop, id, event.fg, cfg)
			case voiceStop:
				if event.id == id && stop != nil {
					a.voiceBeep(soundStop)
					close(stop)
					stop = nil
				}
			case voiceCancel:
				if cancel != nil {
					a.voiceBeep(soundAbort)
					cancel()
					cancel = nil
				}
			}
		case <-ticker.C:
			if voiceEpoch.Load() != id {
				if cancel != nil {
					cancel()
					cancel = nil
				}
				voiceActive.CompareAndSwap(id, 0)
				voiceRecording.CompareAndSwap(id, 0)
				a.voiceWin.Update(a.voiceLeave)
				id = voiceEpoch.Load()
			}
		}
	}
}

type voiceViewState struct {
	text                   string
	recording, recognizing bool
	levels                 []float64 // 最近的音量，旧的在前
}

func (a *App) voiceBeep(wav []byte) {
	if cfg, _ := a.config(); cfg.VoiceSound {
		playSound(wav)
	}
}

func (a *App) voiceDisplay(id uint64, state voiceViewState, show bool) {
	a.voiceWin.Update(func() {
		if voiceEpoch.Load() != id {
			return
		}
		if !show {
			a.voiceLeave()
			return
		}
		// 窗口只在出现时定宽、之后只会变宽：药丸在窗口里自己伸缩，窗口不跟着每帧改尺寸
		appearing := a.voiceShownW == 0
		if w := scale(max(voiceRecW, a.voiceWidth(state)), a.voiceDPI); appearing || w > a.voiceShownW {
			a.voiceShownW = w
			wa, h := a.voiceWA, scale(voiceWinH, a.voiceDPI)
			placeAt(a.voiceWin.NativeHandle(), wa.Left+(wa.w()-w)/2, wa.Bottom-h-scale(4, a.voiceDPI), w, h)
		}
		now, target := time.Now(), float32(a.voiceWidth(state))
		switch {
		case appearing:
			a.voiceShowAt, a.voiceKindAt = now, time.Time{}
			a.voiceWFrom, a.voiceWTo, a.voiceWAt = target, target, now
		case target != a.voiceWTo:
			a.voiceWFrom, a.voiceWTo, a.voiceWAt = a.voicePillW(now), target, now
		}
		if !appearing && voiceKind(state) != voiceKind(a.voiceUI) {
			a.voiceKindAt = now
		}
		if a.voiceLeaving { // 退到一半又要显示：重新弹出来
			a.voiceLeaving, a.voiceShowAt = false, now
		}
		a.voiceUI = state
		a.voiceWin.ShowInactive()
	})
}

// voiceLeave 先让药丸沉下去淡出，动画走完再真正藏窗口；中途又要显示就作罢（界面线程调用）
func (a *App) voiceLeave() {
	if a.voiceShownW == 0 || a.voiceLeaving {
		return
	}
	a.voiceLeaving, a.voiceHideAt = true, time.Now()
	a.voiceHideGen++
	gen := a.voiceHideGen
	time.AfterFunc(220*time.Millisecond, func() {
		a.voiceWin.Update(func() {
			if a.voiceLeaving && a.voiceHideGen == gen {
				a.voiceWin.Hide()
				a.voiceShownW, a.voiceLeaving = 0, false
			}
		})
	})
}

const (
	voiceH     = 32 // 药丸高
	voiceWinH  = 64 // 果冻球的高度，也给药丸留出上下空间做弹出动画
	voiceOrbW  = 56
	voiceRecW  = 112
	voiceDotsW = 88
)

func voiceKind(state voiceViewState) int {
	switch {
	case state.recording:
		return 1
	case state.recognizing:
		return 2
	}
	return 3
}

// voiceOrb 果冻球样式只管录音和识别；提示文字两种样式都用黑药丸
func (a *App) voiceOrb(state voiceViewState) bool {
	return a.voiceStyle == "orb" && voiceKind(state) != 3
}

func (a *App) voiceWidth(state voiceViewState) int32 {
	if a.voiceOrb(state) {
		return voiceOrbW
	}
	return voiceWidth(state)
}

func voiceWidth(state voiceViewState) int32 {
	switch voiceKind(state) {
	case 1:
		return voiceRecW
	case 2:
		return voiceDotsW
	}
	return min(360, max(96, 28+13*int32(utf8.RuneCountInString(state.text))))
}

// 动画自己按时间算，不用 mygo 的 Animate/AnimatePose：系统关了「动画效果」时它们会直接跳到终点
const (
	voiceInDur   = 280 * time.Millisecond
	voiceOutDur  = 180 * time.Millisecond
	voiceWDur    = 260 * time.Millisecond
	voiceKindDur = 200 * time.Millisecond
)

// progress 返回 start 之后走过 d 的比例（0~1），零值时间当作早已走完
func progress(now, start time.Time, d time.Duration) float32 {
	if start.IsZero() {
		return 1
	}
	return min(1, max(0, float32(now.Sub(start))/float32(d)))
}

// backOut 带一点回弹的缓出：冲过 1 一点再落回来
func backOut(t float32) float32 {
	const c = 1.2
	u := t - 1
	return 1 + (c+1)*u*u*u + c*u*u
}

func (a *App) voicePillW(now time.Time) float32 {
	return a.voiceWFrom + (a.voiceWTo-a.voiceWFrom)*ui.EaseOut(progress(now, a.voiceWAt, voiceWDur))
}

// waveHeight 音量 → 0~1 的柱高：去掉底噪，开方让小声说话也看得见
func waveHeight(level float64) float64 {
	return min(1, math.Sqrt(max(0, level-0.004)/0.1))
}

func (a *App) voiceNotice(id uint64, text string) {
	a.voiceDisplay(id, voiceViewState{text: text}, true)
	time.AfterFunc(2*time.Second, func() {
		if voiceEpoch.Load() == id {
			a.voiceDisplay(id, voiceViewState{}, false)
		}
	})
}

func (a *App) runVoice(ctx context.Context, stop <-chan struct{}, id uint64, fg uintptr, cfg Config) {
	// 所有 UI 更新和最终 worker 任务都核对同一代号，过期结果绝不贴入。
	valid := func() bool { return ctx.Err() == nil && voiceEpoch.Load() == id }
	finish := func() { voiceActive.CompareAndSwap(id, 0) }
	fail := func(err error) {
		finish()
		if valid() {
			a.voiceNotice(id, err.Error())
		}
	}
	pt := cursorPos()
	dpi, wa := dpiAt(pt), workArea(pt)
	a.voiceWin.Update(func() { a.voiceWA, a.voiceDPI, a.voiceStyle = wa, dpi, cfg.VoiceStyle })
	started := time.Now()
	last := time.Time{}
	var levels []float64
	pcm, err := recordVoice(ctx, stop, func() {
		if !valid() {
			return
		}
		started = time.Now()
		a.voiceBeep(soundStart) // 麦克风真开了才响：听到「嘚嘚」就可以说了
		a.voiceDisplay(id, voiceViewState{recording: true}, true)
	}, func(level float64) {
		levels = append(levels, level)
		if len(levels) > voiceBars {
			levels = levels[len(levels)-voiceBars:]
		}
		if valid() && time.Since(last) >= 50*time.Millisecond {
			last = time.Now()
			a.voiceDisplay(id, voiceViewState{recording: true, levels: append([]float64(nil), levels...)}, true)
		}
	})
	voiceRecording.CompareAndSwap(id, 0)
	if !valid() {
		finish()
		return
	}
	if err != nil {
		fail(err)
		return
	}
	if len(pcm) == 0 {
		finish()
		a.voiceNotice(id, T("没听清，请再说一次"))
		return
	}
	a.voiceDisplay(id, voiceViewState{recognizing: true, text: T("识别中…")}, true)
	text, err := transcribe(ctx, cfg, pcm)
	if !valid() {
		finish()
		return
	}
	if err != nil {
		fail(err)
		return
	}
	if strings.TrimSpace(text) == "" {
		finish()
		a.voiceNotice(id, T("没听清，请再说一次"))
		return
	}
	source := text
	text, polishMs := polishVoice(ctx, cfg, source)
	if !valid() {
		finish()
		return
	}
	theWorker.paste(text, func(owner uintptr) bool {
		if !valid() {
			finish()
			return false
		}
		// 修饰键尚未松开时不能发 Ctrl+V；超过时限就改为复制。
		waitModifiersUp(500 * time.Millisecond)
		if !valid() {
			finish()
			return false
		}
		copied := foreground() != fg || keyDown(VK_CONTROL) || keyDown(VK_MENU) || keyDown(VK_SHIFT) || keyDown(VK_LWIN) || keyDown(VK_RWIN)
		if copied {
			if setClipboardText(owner, toCRLF(text), false) {
				a.voiceNotice(id, T("已复制"))
			} else {
				a.voiceNotice(id, T("无法写入剪贴板"))
			}
		} else {
			a.voiceDisplay(id, voiceViewState{}, false)
		}
		finish()
		if cfg.History {
			go a.hist.write(historyEntry{Type: "voice", ID: "voice-" + historyID(started, int64(id)), Time: time.Now().Format(time.RFC3339), Model: cfg.ASRModel, BaseURL: cfg.ASRBaseURL, Source: source, Translation: text, PolishStyle: cfg.PolishStyle, PolishMs: polishMs, TotalMs: time.Since(started).Milliseconds()})
		}
		return !copied
	})
}

const voiceBars = 9

// 胶囊不跟主题：深浅色桌面上都用同一颗黑药丸
var (
	pillBg     = ui.RGBA(20, 20, 20, 0.96)
	pillBorder = ui.RGBA(255, 255, 255, 0.12)
	pillFg     = ui.RGBA(242, 242, 242, 1)
)

func (a *App) voiceView(c *ui.Context) {
	a.themed(c)
	state := a.voiceUI
	now := time.Now()
	dt := min(0.1, max(0, now.Sub(a.voiceFrameAt).Seconds()))
	a.voiceFrameAt = now
	in := progress(now, a.voiceShowAt, voiceInDur)
	opacity, size, dy := ui.EaseOut(in), 0.85+0.15*backOut(in), 6*(1-ui.EaseOut(in))
	moving := in < 1 || progress(now, a.voiceWAt, voiceWDur) < 1 || progress(now, a.voiceKindAt, voiceKindDur) < 1 || voiceKind(state) != 3
	if a.voiceLeaving {
		out := progress(now, a.voiceHideAt, voiceOutDur)
		out *= out // 先慢后快地沉下去
		opacity, size, dy = opacity*(1-out), size*(1-0.12*out), dy+6*out
		moving = true
	}
	kind := ui.EaseOut(progress(now, a.voiceKindAt, voiceKindDur))
	// 柱高和球的大小每帧往目标靠一点（指数平滑），音量 20 次/秒的跳变就被抹顺了
	follow := float32(1 - math.Exp(-dt/0.07))
	latest := 0.0
	if len(state.levels) > 0 {
		latest = state.levels[len(state.levels)-1]
	}
	a.voiceOrbLv += (float32(waveHeight(latest)) - a.voiceOrbLv) * follow
	for i := range a.voiceBarH {
		level := 0.0
		if k := len(state.levels) - voiceBars + i; k >= 0 {
			level = state.levels[k]
		}
		a.voiceBarH[i] += (float32(waveHeight(level)) - a.voiceBarH[i]) * follow
	}
	bars, orbLv := a.voiceBarH, a.voiceOrbLv
	if moving {
		c.AnimationFrame()
	}
	ui.Box(c).Fill().Center().Children(func() {
		if a.voiceOrb(state) {
			recognizing := state.recognizing
			orb := ui.ButtonBase(c).Size(voiceOrbW, voiceOrbW).Background(ui.RGBA(0, 0, 0, 0)).Opacity(opacity*(0.4+0.6*kind)).Scale(size, size).Translate(0, dy)
			orb.Cursor(ui.CursorPointer)
			if recognizing {
				orb.Tooltip(T("取消"))
			} else {
				orb.Tooltip(T("点一下完成，Esc 取消"))
			}
			orb.DrawOver(func(p *ui.Painter, r ui.Rect) { drawOrb(p, r, orbLv, now, recognizing) })
			if orb.Clicked() {
				if recognizing {
					go voiceClick(voiceCancel)
				} else {
					go voiceClick(voiceStop)
				}
			}
			return
		}
		pill := ui.Row(c).Height(voiceH).AlignItems(ui.Center).Justify(ui.Center).Clip().
			Background(pillBg).Border(1, pillBorder).Radius(voiceH / 2)
		pill.Width(a.voicePillW(now)).Opacity(opacity).Scale(size, size).Translate(0, dy).Children(func() {
			row := ui.Row(c).Fill().AlignItems(ui.Center).Opacity(kind).Scale(0.92+0.08*kind, 0.92+0.08*kind)
			switch voiceKind(state) {
			case 1:
				row.Padding(0, 5).Gap(4).Children(func() {
					if pillButton(c, false, T("取消")).Clicked() {
						go voiceClick(voiceCancel)
					}
					ui.Box(c).Grow(1).Height(20).Draw(func(p *ui.Painter, r ui.Rect) { drawWave(p, r, bars) })
					if pillButton(c, true, T("完成")).Clicked() {
						go voiceClick(voiceStop)
					}
				})
			case 2:
				row.Padding(0, 5).Gap(8).Justify(ui.Center).Children(func() {
					if pillButton(c, false, T("取消")).Clicked() {
						go voiceClick(voiceCancel)
					}
					ui.Box(c).Size(36, 12).Draw(drawDots)
				})
			default:
				row.Padding(0, 14).Justify(ui.Center).Children(func() {
					ui.Text(c, state.text).FontSize(12).TextColor(pillFg).MaxLines(1).Tooltip(state.text)
				})
			}
		})
	})
}

// pillButton 24px 圆钮：✕ 是半透明白底，✓ 是实心白底黑勾
func pillButton(c *ui.Context, ok bool, tip string) ui.Element {
	bg, hover, fg := ui.RGBA(255, 255, 255, 0.10), ui.RGBA(255, 255, 255, 0.18), pillFg
	if ok {
		bg, hover, fg = pillFg, ui.RGBA(255, 255, 255, 1), pillBg
	}
	b := ui.ButtonBase(c).Size(22, 22).Radius(11).Background(bg).Cursor(ui.CursorPointer).Tooltip(tip)
	if b.Hovered() {
		b.Background(hover)
	}
	return b.DrawOver(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		path := new(ui.Path)
		if ok {
			path.MoveTo(cx-4, cy+0.3).LineTo(cx-1.3, cy+3).LineTo(cx+4, cy-2.8)
		} else {
			path.MoveTo(cx-3.5, cy-3.5).LineTo(cx+3.5, cy+3.5).MoveTo(cx+3.5, cy-3.5).LineTo(cx-3.5, cy+3.5)
		}
		p.StrokePath(path, 1.6, fg)
	})
}

var (
	waveFrom = ui.RGBA(175, 169, 236, 1) // 淡紫
	waveTo   = ui.RGBA(93, 202, 165, 1)  // 青绿
	orbFrom  = ui.RGBA(127, 119, 221, 1) // 紫
	orbTo    = ui.RGBA(239, 159, 39, 1)  // 橙
)

func mix(a, b ui.Color, t float32) ui.Color {
	f := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t) }
	return ui.Color{R: f(a.R, b.R), G: f(a.G, b.G), B: f(a.B, b.B), A: f(a.A, b.A)}
}

// drawWave 上下对称的柱子，从左到右紫变青；最新的音量在最右边往左流，中间高两头矮
func drawWave(p *ui.Painter, r ui.Rect, heights [voiceBars]float32) {
	const bw, gap = 3, 2.5
	half := float32(voiceBars-1) / 2
	x := r.X + (r.W-(voiceBars*bw+(voiceBars-1)*gap))/2
	for i, v := range heights {
		d := float32(i) - half
		if d < 0 {
			d = -d
		}
		h := 3 + (r.H-3)*v*(1-0.5*d/half)
		p.Fill(ui.Rect{X: x, Y: r.Y + (r.H-h)/2, W: bw, H: h}, mix(waveFrom, waveTo, float32(i)/(voiceBars-1)), bw/2)
		x += bw + gap
	}
}

// drawOrb 果冻球：半径跟音量走，边缘叠两个转速不同的波纹；识别中缩一点、颜色转起来、慢慢呼吸
func drawOrb(p *ui.Painter, r ui.Rect, level float32, now time.Time, recognizing bool) {
	t := float64(now.UnixMilli()%600000) / 1000
	lv := float64(level)
	base, wobble, angle := 13+9*lv, 0.04+0.09*lv, 135+25*math.Sin(t*0.8)
	if recognizing {
		base, wobble, angle = 14+1.5*math.Sin(t*5), 0.04, math.Mod(t*240, 360)
	}
	cx, cy := float64(r.X+r.W/2), float64(r.Y+r.H/2)
	path := new(ui.Path)
	for k := 0; k <= 48; k++ {
		th := float64(k) / 48 * 2 * math.Pi
		rad := base * (1 + wobble*math.Sin(3*th+t*4) + wobble*0.5*math.Sin(5*th-t*6))
		x, y := float32(cx+rad*math.Cos(th)), float32(cy+rad*math.Sin(th))
		if k == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	p.FillPathGradient(path.Close(), ui.LinearGradient{From: orbFrom, To: orbTo, Angle: float32(angle), Oklab: true})
	p.AnimationFrame()
}

// drawDots 识别中：三个点依次跳一下
func drawDots(p *ui.Painter, r ui.Rect) {
	p.AnimationFrame()
	now := float64(time.Now().UnixMilli()) / 1000
	for i := 0; i < 3; i++ {
		lift := math.Max(0, math.Sin((now-float64(i)*0.15)*2*math.Pi))
		color := pillFg
		color.A = uint8(80 + 175*lift)
		x := r.X + r.W/2 + float32(i-1)*10 - 2.5
		p.Fill(ui.Rect{X: x, Y: r.Y + r.H/2 - 2.5 - float32(lift)*3, W: 5, H: 5}, color, 2.5)
	}
}

// runVoiceDemo 截图和调样式用：两种样式各放一遍（录音 3 秒假音量 → 识别 1.5 秒 → 提示 1.5 秒 → 退场），然后退出。会响提示音，不开麦克风
func (a *App) runVoiceDemo() {
	pt := cursorPos()
	dpi, wa := dpiAt(pt), workArea(pt)
	id := voiceEpoch.Load()
	for _, style := range []string{"bars", "orb"} {
		a.voiceWin.Update(func() { a.voiceWA, a.voiceDPI, a.voiceStyle = wa, dpi, style })
		playSound(soundStart)
		var levels []float64
		for start := time.Now(); time.Since(start) < 3*time.Second; time.Sleep(50 * time.Millisecond) {
			x := time.Since(start).Seconds()
			level := 0.06 * math.Abs(math.Sin(x*3)+0.6*math.Sin(x*7.3))
			if x > 1.6 && x < 2.2 {
				level = 0.002 // 中间停一下，看安静时的样子
			}
			levels = append(levels, level)
			levels = levels[max(0, len(levels)-voiceBars):]
			a.voiceDisplay(id, voiceViewState{recording: true, levels: append([]float64(nil), levels...)}, true)
		}
		playSound(soundStop)
		a.voiceDisplay(id, voiceViewState{recognizing: true}, true)
		time.Sleep(1500 * time.Millisecond)
		a.voiceDisplay(id, voiceViewState{text: T("没听清，请再说一次")}, true)
		time.Sleep(1500 * time.Millisecond)
		a.voiceDisplay(id, voiceViewState{}, false)
		time.Sleep(600 * time.Millisecond)
	}
	playSound(soundAbort)
	time.Sleep(400 * time.Millisecond)
	a.quit()
}

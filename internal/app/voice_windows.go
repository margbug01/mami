//go:build windows

package app

import (
	"context"
	"log"
	"math"
	"path/filepath"
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
	voiceTriggerAt uint32 // 上一次触发键事件的钩子时间（毫秒）
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
	// 吞掉的单键（F 键、CapsLock）系统记不到按下，heal 不能拿 GetAsyncKeyState 判它；
	// 改成看间隔：按住时连发约 30ms 一次，隔了 500ms 还没松开就是漏了松开
	swallowTrigger := trigger != 0 && !voiceKeyIsModifier(trigger)
	if swallowTrigger && key.VkCode == trigger && down && voiceKeyboard.pressed[trigger] && key.Time-voiceTriggerAt > 500 {
		voiceKeyboard.pressed[trigger] = false
		if voiceKeyboard.held == trigger {
			voiceKeyboard.held, voiceKeyboard.combo = 0, false
		}
	}
	if key.VkCode == trigger {
		voiceTriggerAt = key.Time
	}
	if healed := voiceKeyboard.heal(func(vk uint32) bool { return (swallowTrigger && vk == trigger) || keyDown(uintptr(vk)) }); len(healed) > 0 {
		log.Printf("voice: cleared stuck keys %x", healed)
	}
	held := voiceKeyboard.held
	busy := voiceActive.Load() != 0
	action, swallow := voiceKeyboard.step(key.VkCode, down, trigger, busy || blocked)
	if down && key.VkCode == trigger && held == 0 && action == voiceNone && !voiceKeyboard.recording {
		log.Printf("voice: trigger ignored (busy=%v modifier=%v others=%v)", busy, blocked, voiceKeyboard.othersDown(trigger))
	}
	if swallowTrigger && key.VkCode == trigger {
		swallow = true
	}
	if !down && key.VkCode == held && (held == 0xA5 || held == 0xA4) { // 单按 Alt 松开会激活菜单栏
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

// voiceToggle 组合键触发键（GlobalShortcut 回调）：没在录就开始，在录就结束。
// 单键走钩子；两者不会同时启用（组合键时 voiceTrigger 是 0）。
func voiceToggle() {
	if voiceRecording.Load() != 0 {
		voiceClick(voiceStop)
		return
	}
	if voiceActive.Load() != 0 {
		return // 还在识别上一段
	}
	id := voiceEpoch.Add(1)
	voiceActive.Store(id)
	voiceRecording.Store(id)
	postVoice(voiceStart, id)
}

// voiceCommand 外部触发（Mami.exe -voice toggle|start|stop|cancel，转给已经在跑的实例）：
// 遥控器、脚踏板、宏工具不用模拟键盘也能开关语音。认不出的返回 false
func voiceCommand(cmd string) bool {
	switch cmd {
	case "toggle":
		voiceToggle()
	case "start":
		if voiceRecording.Load() == 0 {
			voiceToggle()
		}
	case "stop":
		voiceClick(voiceStop)
	case "cancel":
		voiceClick(voiceCancel)
	default:
		return false
	}
	return true
}

// voiceCommandArg 从命令行里找 -voice X / --voice=X
func voiceCommandArg(args []string) (string, bool) {
	for i, a := range args {
		name, val, eq := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") || name != "voice" {
			continue
		}
		if !eq && i+1 < len(args) {
			val = args[i+1]
		}
		return strings.ToLower(strings.TrimSpace(val)), true
	}
	return "", false
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
	copy                   bool // text 是识别结果、已经在剪贴板里：胶囊右边带一个复制钮
	retry                  bool // 识别失败、录音还留着：胶囊右边带一个重试钮
	recording, recognizing bool
	levels                 []float64 // 最近的音量，旧的在前
	live                   string    // 录音中边说边出的字（开了实时出字、用流式模型时才有）
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
			wa, h := a.voiceWA, scale(a.voiceWinHeight(), a.voiceDPI)
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
			a.voiceGapL, a.voiceGapR = a.islandGaps(now) // 切到一半又切：从眼下的缝宽接着走
			a.voiceKindAt = now
		}
		if a.voiceLeaving { // 退到一半又要显示：重新弹出来
			a.voiceLeaving, a.voiceShowAt = false, now
		}
		if state.copy && (!a.voiceUI.copy || a.voiceUI.text != state.text) {
			a.voiceCopiedAt = time.Time{}
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
	voiceH     = 36                                 // 药丸高
	voiceBtn   = voiceH - 10                        // 圆钮直径：上下左右都留 5
	voiceWinH  = 64                                 // 果冻球的高度，也给药丸留出上下空间做弹出动画
	voiceOrbW  = 56                                 // 果冻球默认大小；设置里可以调 40~96
	voiceRecW  = 2*voiceBtn + 10 + 47 + 2*12        // 两颗钮、两边各 5、波形 47、波形左右各空 12
	voiceDotsW = 5 + voiceBtn + 61 + 5              // 两边各 5、✕、三个点占的地方
	voiceCopyW = voiceRecW + 48                     // 没地方贴时那句话：只比录音时宽一点，多的截掉、悬停看全文
	voiceLiveN = (voiceRecW - 2*voiceBtn - 18) / 12 // 边说边出字：胶囊保持录音时的宽度，中间只放得下最新这几个字
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

// voiceOrb 果冻球样式的录音和识别；提示文字在果冻样式下是一条果冻（见 drawJelly）
func (a *App) voiceOrb(state voiceViewState) bool {
	return a.voiceStyle == "orb" && voiceKind(state) != 3 && state.live == "" // 出字了就变成一条果冻
}

func (a *App) orbSize() float32 {
	if a.voiceOrbSize <= 0 {
		return voiceOrbW
	}
	return a.voiceOrbSize
}

func (a *App) voiceWidth(state voiceViewState) int32 {
	if a.voiceOrb(state) {
		return int32(a.orbSize())
	}
	return voiceWidth(state)
}

// voiceWinHeight 窗口高度要装得下最大的果冻球，上下再留一点给弹出动画
func (a *App) voiceWinHeight() int32 {
	if a.voiceStyle == "orb" {
		return max(voiceWinH, int32(a.orbSize())+12)
	}
	return voiceWinH
}

// liveTail 只留最后 voiceLiveN 个字，新字从右边推进来
func liveTail(s string) string {
	r := []rune(s)
	if len(r) <= voiceLiveN {
		return s
	}
	return string(r[len(r)-voiceLiveN:])
}

func voiceWidth(state voiceViewState) int32 {
	switch voiceKind(state) {
	case 1:
		return voiceRecW // 边说边出字也不变宽
	case 2:
		return voiceDotsW
	}
	w := 28 + 13*int32(utf8.RuneCountInString(state.text))
	if state.copy || state.retry {
		return min(voiceCopyW, max(96, w+voiceBtn+4)) // 加上复制钮
	}
	return min(360, max(96, w))
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

// voiceRetryNotice 识别失败：录音留着，胶囊显示「识别失败」和重试钮，10 秒后收起，鼠标停在上面时不收
func (a *App) voiceRetryNotice(id uint64, pcm []byte, cfg Config) {
	a.voiceWin.Update(func() { a.voiceRetry = func() { go a.retryVoice(pcm, cfg) } })
	a.voiceDisplay(id, voiceViewState{text: T("识别失败，点右边重试"), retry: true}, true)
	var check func()
	check = func() {
		a.voiceWin.Update(func() {
			if voiceEpoch.Load() != id || !a.voiceUI.retry {
				return
			}
			if a.voiceHover {
				time.AfterFunc(time.Second, check)
				return
			}
			a.voiceLeave()
		})
	}
	time.AfterFunc(10*time.Second, check)
}

// retryVoice 拿失败那次的整段录音再识别一次，成了就照常润色、贴到眼下的前台窗口
func (a *App) retryVoice(pcm []byte, cfg Config) {
	if voiceActive.Load() != 0 {
		return
	}
	id := voiceEpoch.Add(1)
	if !voiceActive.CompareAndSwap(0, id) {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(asrTimeout+polishMaxWait+5*time.Second, cancel) // 贴入在 worker 里异步完成，不能一返回就取消
	started, fg := time.Now(), foreground()
	a.voiceDisplay(id, voiceViewState{recognizing: true}, true)
	text, err := transcribe(ctx, cfg, pcm)
	if voiceEpoch.Load() != id {
		voiceActive.CompareAndSwap(id, 0)
		return
	}
	if err != nil {
		voiceActive.CompareAndSwap(id, 0)
		log.Printf("voice: retry failed: %v", err)
		a.voiceRetryNotice(id, pcm, cfg)
		return
	}
	if text = strings.TrimSpace(text); text == "" {
		voiceActive.CompareAndSwap(id, 0)
		a.voiceNotice(id, T("没听清，请再说一次"))
		return
	}
	log.Printf("voice: retry ok after %dms", time.Since(started).Milliseconds())
	a.deliverVoice(ctx, id, fg, cfg, text, a.learn.active(), started)
}

// voiceCopyNotice 没地方贴：胶囊显示这句话和复制钮，6 秒后收起，鼠标停在上面时不收
func (a *App) voiceCopyNotice(id uint64, text string) {
	a.voiceDisplay(id, voiceViewState{text: text, copy: true}, true)
	var check func()
	check = func() {
		a.voiceWin.Update(func() {
			if voiceEpoch.Load() != id || !a.voiceUI.copy {
				return
			}
			if a.voiceHover {
				time.AfterFunc(time.Second, check)
				return
			}
			a.voiceLeave()
		})
	}
	time.AfterFunc(6*time.Second, check)
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
	a.voiceWin.Update(func() {
		a.voiceWA, a.voiceDPI, a.voiceStyle, a.voiceTone, a.voiceOrbSize = wa, dpi, cfg.VoiceStyle, cfg.VoiceTone, float32(cfg.VoiceOrbSize)
	})
	if cfg.ASRAPIKey == "" { // 还没填 Key：不录，直接带去填
		voiceRecording.CompareAndSwap(id, 0)
		finish()
		a.voiceNotice(id, T("先填一下语音识别的 Key"))
		a.voiceWin.Update(func() {
			a.openSettings()
			a.set.page, a.set.voiceTab = pageVoice, 2
		})
		return
	}
	if cfg.PolishAuto {
		exe := filepath.Base(processName(windowPID(fg)))
		if s := autoPolishStyle(exe, windowTitle(fg), cfg.PolishStyle); s != cfg.PolishStyle {
			log.Printf("voice: polish %s in %s", s, exe)
			cfg.PolishStyle = s
		}
	}
	learned := a.learn.active()
	cfg.ASRHotwords = withLearned(cfg.ASRHotwords, learned)
	// 流式模型：录音一开始就连上，边录边推；其余模型说话的这几秒里先把连接握好，说完直接复用
	var stream *asrStream
	var live atomic.Pointer[string]
	if cfg.ASRAPIKey != "" && !isStepFunASR(cfg.ASRBaseURL) && isStreamASR(cfg.ASRModel) {
		var onText func(string)
		if cfg.VoiceLiveText {
			onText = func(t string) { live.Store(&t) } // 录音回调每 50ms 刷一次胶囊时顺手带上
		}
		stream = startASRStream(ctx, cfg, onText)
		defer stream.stop()
	} else {
		go warmConnection(ctx, cfg.ASRBaseURL)
	}
	if cfg.PolishStyle != "off" && cfg.PolishAPIKey != "" {
		go warmConnection(ctx, cfg.PolishBaseURL)
	}
	started := time.Now()
	last := time.Time{}
	var levels []float64
	// 按停顿分段：说到一半停顿时先把前面的送去识别，停止时只剩最后一段要等
	type asrJob struct {
		text string
		err  error
		done chan struct{}
	}
	var jobs []*asrJob
	submit := func(pcm []byte) {
		job := &asrJob{done: make(chan struct{})}
		jobs = append(jobs, job)
		go func() {
			job.text, job.err = transcribe(ctx, cfg, pcm)
			close(job.done)
		}()
	}
	var seg []byte
	var segPeak float64
	var segmenter voiceSegmenter
	pcm, err := recordVoice(ctx, stop, func() {
		if !valid() {
			return
		}
		started = time.Now()
		a.voiceBeep(soundStart) // 麦克风真开了才响：听到「嘚嘚」就可以说了
		a.voiceDisplay(id, voiceViewState{recording: true}, true)
	}, func(data []byte, level float64) {
		seg = append(seg, data...)
		segPeak = max(segPeak, level)
		if stream != nil {
			stream.feed(data)
		} else if segmenter.feed(level) {
			submit(seg)
			seg, segPeak = nil, 0
		}
		levels = append(levels, level)
		if len(levels) > voiceBars {
			levels = levels[len(levels)-voiceBars:]
		}
		if valid() && time.Since(last) >= 50*time.Millisecond {
			last = time.Now()
			st := voiceViewState{recording: true, levels: append([]float64(nil), levels...)}
			if p := live.Load(); p != nil {
				st.live = *p
			}
			a.voiceDisplay(id, st, true)
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
	if stream != nil && !voiceTooQuiet(seg, segPeak) {
		job := &asrJob{done: make(chan struct{})}
		jobs = append(jobs, job)
		go func() {
			job.text, job.err = stream.finish()
			close(job.done)
		}()
	} else if stream == nil && !voiceTooQuiet(seg, segPeak) {
		submit(seg)
	} else if len(jobs) == 0 {
		finish()
		log.Printf("voice: too quiet (peak %.4f, %dms), not sent", segPeak, len(seg)/32)
		a.voiceNotice(id, T("没听清，请再说一次"))
		return
	}
	stopped := time.Now()
	a.voiceDisplay(id, voiceViewState{recognizing: true, text: T("识别中…")}, true)
	var parts []string
	for _, job := range jobs {
		<-job.done
		if !valid() {
			finish()
			return
		}
		if job.err != nil {
			finish()
			if valid() {
				log.Printf("voice: recognition failed, kept %dms of audio for retry: %v", len(pcm)/32, job.err)
				a.voiceRetryNotice(id, pcm, cfg)
			}
			return
		}
		if part := strings.TrimSpace(job.text); part != "" {
			parts = append(parts, part)
		}
	}
	log.Printf("voice: %d segment(s), waited %dms after stop", len(jobs), time.Since(stopped).Milliseconds())
	text := joinSegments(parts)
	if text == "" {
		finish()
		a.voiceNotice(id, T("没听清，请再说一次"))
		return
	}
	a.deliverVoice(ctx, id, fg, cfg, text, learned, started)
}

// deliverVoice 识别出的原话 → 润色 → 贴进输入框（贴不了就留在胶囊里给复制），记历史、开始盯手改
func (a *App) deliverVoice(ctx context.Context, id uint64, fg uintptr, cfg Config, source string, learned []learnedWord, started time.Time) {
	valid := func() bool { return ctx.Err() == nil && voiceEpoch.Load() == id }
	finish := func() { voiceActive.CompareAndSwap(id, 0) }
	text, polishMs := polishVoice(ctx, cfg, source, learned...)
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
		switched := foreground() != fg
		modsDown := keyDown(VK_CONTROL) || keyDown(VK_MENU) || keyDown(VK_SHIFT) || keyDown(VK_LWIN) || keyDown(VK_RWIN)
		var in inputState
		if !switched && !modsDown && !isTerminal(fg) {
			in = focusedInput()
		}
		mode := voiceOutput(switched, modsDown, isTerminal(fg) || in.input, systemCaret(fg))
		copied := mode != voicePaste
		switch {
		case mode == voicePaste:
			a.voiceDisplay(id, voiceViewState{}, false)
			a.learnStart(fg, source, text, cfg)
		case !setClipboardText(owner, toCRLF(text), false):
			a.voiceNotice(id, T("无法写入剪贴板"))
		case mode == voicePasteKeep:
			// 认不出是不是输入框（微信这类）：照样贴，但文字留在剪贴板里，贴空了还能手动 Ctrl+V
			a.voiceDisplay(id, voiceViewState{}, false)
			sendCtrl('V')
			a.learnStart(fg, source, text, cfg)
		default:
			log.Printf("voice: no input box (switched=%v modifiers=%v), copied", switched, modsDown)
			a.voiceCopyNotice(id, text)
		}
		finish()
		if cfg.History {
			style := cfg.PolishStyle
			if polishMs == 0 { // 没润色（关了、太短、失败回落原文）：记录里不挂档位
				style = ""
			}
			go a.hist.write(historyEntry{Type: "voice", ID: "voice-" + historyID(started, int64(id)), Time: time.Now().Format(time.RFC3339), Model: cfg.ASRModel, BaseURL: cfg.ASRBaseURL, Source: source, Translation: text, PolishStyle: style, PolishMs: polishMs, TotalMs: time.Since(started).Milliseconds()})
		}
		return !copied
	})
}

const voiceBars = 9

// 渐变波柱的底：深色是黑药丸，浅色是浅灰药丸（由「浮窗底色」决定，不跟主题）
var (
	pillBg          = ui.RGBA(20, 20, 20, 0.96)
	pillBorder      = ui.RGBA(255, 255, 255, 0.12)
	pillFg          = ui.RGBA(242, 242, 242, 1)
	pillLightBg     = ui.RGBA(244, 245, 247, 0.97)
	pillLightBorder = ui.RGBA(0, 0, 0, 0.13)
	pillInk         = ui.RGBA(30, 32, 38, 1)
)

// toneDark 浮窗用不用深色底：auto 跟 Mami 主题，其余按用户选的来
func toneDark(tone string, themeDark bool) bool {
	switch tone {
	case "light":
		return false
	case "auto":
		return themeDark
	}
	return true
}

func (a *App) voiceView(c *ui.Context) {
	dark := toneDark(a.voiceTone, a.themed(c).Dark)
	sk := skinFor(a.voiceStyle, dark)
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
			sz := a.orbSize()
			orb := ui.ButtonBase(c).Size(sz, sz).Background(ui.RGBA(0, 0, 0, 0)).Opacity(opacity*(0.4+0.6*kind)).Scale(size, size).Translate(0, dy)
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
		gl, gr := a.islandGaps(now)
		pill := a.voicePill(c, a.voiceStyle, dark, sk, state, bars, orbLv, gl, gr, kind, now, true)
		a.voiceHover = pill.Hovered()
		pill.Width(a.voicePillW(now)).Opacity(opacity).Scale(size, size).Translate(0, dy)
	})
}

// voicePill 画胶囊本体（不含进出场动画）。live=false 是设置页里的预览：按钮只是样子，点了没反应
func (a *App) voicePill(c *ui.Context, style string, dark bool, sk voiceSkin, state voiceViewState, bars [voiceBars]float32, orbLv, gl, gr, kind float32, now time.Time, live bool) ui.Element {
	pill := ui.Row(c).Height(voiceH).AlignItems(ui.Center).Justify(ui.Center).Clip().
		Background(pillBg).Border(1, pillBorder).Radius(voiceH / 2)
	if !dark {
		pill.Background(pillLightBg).Border(1, pillLightBorder)
	}
	switch style {
	case "orb": // 果冻样式的提示：没有黑底，画一条会抖的渐变果冻
		pill.Background(clearC).Border(0, clearC).Draw(func(p *ui.Painter, r ui.Rect) { drawJelly(p, r, now) })
	case "glass":
		halo := voiceKind(state) == 1
		pill.Background(clearC).Border(0, clearC).Draw(func(p *ui.Painter, r ui.Rect) { drawGlass(p, r, dark, orbLv, halo) })
	case "pixel":
		pill.Background(clearC).Border(0, clearC).Radius(0).Draw(drawPixelPill)
	case "mercury":
		bg, edge := mercuryShell(dark)
		pill.Background(bg).Border(1, edge)
	case "islands", "leds":
		bridge := style == "islands"
		pill.Background(clearC).Border(0, clearC).Radius(0).Draw(func(p *ui.Painter, r ui.Rect) { drawIslands(p, r, dark, gl, gr, bridge) })
	}
	gap, dotsGap := float32(4), float32(0)
	if sk.island() {
		gap, dotsGap = 8, 8 // 钮和中间岛之间：让中间的盒子正好落在中间那块岛上
	}
	pill.Children(func() {
		row := ui.Row(c).Fill().AlignItems(ui.Center).Opacity(kind).Scale(0.92+0.08*kind, 0.92+0.08*kind)
		switch voiceKind(state) {
		case 1:
			row.Padding(0, 5).Gap(gap).Children(func() {
				if pillButton(c, sk, false, T("取消")).Clicked() && live {
					go voiceClick(voiceCancel)
				}
				if state.live != "" {
					ui.Row(c).Grow(1).Shrink(1).Height(20).Clip().AlignItems(ui.Center).Justify(ui.Center).Children(func() {
						ui.Text(c, liveTail(state.live)).FontSize(12).TextColor(sk.fg).MaxLines(1)
					})
				} else {
					ui.Box(c).Grow(1).Height(20).Draw(func(p *ui.Painter, r ui.Rect) { sk.wave(p, r, bars) })
				}
				if pillButton(c, sk, true, T("完成")).Clicked() && live {
					go voiceClick(voiceStop)
				}
			})
		case 2:
			row.Padding(0, 5).Gap(dotsGap).Children(func() { // ✕ 贴左，三个点在 ✕ 和胶囊右边缘之间居中
				if pillButton(c, sk, false, T("取消")).Clicked() && live {
					go voiceClick(voiceCancel)
				}
				ui.Box(c).Grow(1).Height(20).Draw(sk.dots)
			})
		case 3:
			if !state.copy && !state.retry {
				row.Padding(0, 14).Justify(ui.Center).Children(func() {
					ui.Text(c, state.text).FontSize(12).TextColor(sk.fg).MaxLines(1).Tooltip(state.text)
				})
				break
			}
			done := live && !a.voiceCopiedAt.IsZero()
			row.Padding(0, 5, 0, 14).Gap(6).Children(func() {
				ui.Text(c, state.text).FontSize(12).TextColor(sk.fg).MaxLines(1).Grow(1).Shrink(1).Tooltip(state.text)
				if state.retry {
					if pillRetryButton(c, sk, T("重试")).Clicked() && live && a.voiceRetry != nil {
						a.voiceRetry()
					}
					return
				}
				tip := T("已复制，到要用的地方按 Ctrl+V")
				if pillCopyButton(c, sk, done, tip).Clicked() && !done && live {
					text, id := state.text, voiceEpoch.Load()
					theWorker.do(func(owner uintptr) { setClipboardText(owner, toCRLF(text), false) })
					a.voiceCopiedAt = now
					time.AfterFunc(700*time.Millisecond, func() { a.voiceDisplay(id, voiceViewState{}, false) })
				}
			})
		}
	})
	return pill
}

// pillRetryButton 胶囊上的重试钮：一个带箭头的圆弧 ↻
func pillRetryButton(c *ui.Context, sk voiceSkin, tip string) ui.Element {
	rad := float32(voiceBtn / 2)
	if sk.style == "pixel" {
		rad = 0
	}
	b := ui.ButtonBase(c).Size(voiceBtn, voiceBtn).Radius(rad).Background(sk.btn).Cursor(ui.CursorPointer).Tooltip(tip).Shrink(0)
	if b.Hovered() {
		b.Background(sk.btnHover)
	}
	return b.DrawOver(func(p *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		path := new(ui.Path)
		const rr = 4.5
		const a0, sweep = -1.0, 5.0 // 从右上方顺时针绕一圈，顶上留个口
		for i := 0; i <= 14; i++ {
			ang := a0 + sweep*float64(i)/14
			x, y := cx+rr*float32(math.Cos(ang)), cy+rr*float32(math.Sin(ang))
			if i == 0 {
				path.MoveTo(x, y)
			} else {
				path.LineTo(x, y)
			}
		}
		e := a0 + sweep // 箭头在弧的尽头，顺着走向
		ex, ey := cx+rr*float32(math.Cos(e)), cy+rr*float32(math.Sin(e))
		tx, ty := -float32(math.Sin(e)), float32(math.Cos(e))
		nx, ny := float32(math.Cos(e)), float32(math.Sin(e))
		hx, hy := ex+1.6*tx, ey+1.6*ty
		path.MoveTo(hx-3*tx+2.4*nx, hy-3*ty+2.4*ny).LineTo(hx, hy).LineTo(hx-3*tx-2.4*nx, hy-3*ty-2.4*ny)
		p.StrokePath(path, 1.4, sk.fg)
	})
}

// pillCopyButton 胶囊上的复制钮：两个叠着的方框，点过之后变成 ✓
func pillCopyButton(c *ui.Context, sk voiceSkin, done bool, tip string) ui.Element {
	rad := float32(voiceBtn / 2)
	if sk.style == "pixel" {
		rad = 0
	}
	b := ui.ButtonBase(c).Size(voiceBtn, voiceBtn).Radius(rad).Background(sk.btn).Cursor(ui.CursorPointer).Tooltip(tip).Shrink(0)
	if b.Hovered() {
		b.Background(sk.btnHover)
	}
	return b.DrawOver(func(p *ui.Painter, r ui.Rect) {
		if sk.style == "pixel" {
			icon := pixIconCopy
			if done {
				icon = pixIconOK
			}
			drawPixelIcon(p, r, icon, pixCyan)
			return
		}
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		path := new(ui.Path)
		if done {
			path.MoveTo(cx-4, cy+0.3).LineTo(cx-1.3, cy+3).LineTo(cx+4, cy-2.8)
		} else {
			path.MoveTo(cx-1.5, cy-1.5).LineTo(cx+4, cy-1.5).LineTo(cx+4, cy+4).LineTo(cx-1.5, cy+4).Close() // 前面那张
			path.MoveTo(cx-4, cy+1.5).LineTo(cx-4, cy-4).LineTo(cx+1.5, cy-4)                                // 后面露出的一角
		}
		p.StrokePath(path, 1.4, sk.fg)
	})
}

// pillButton 24px 圆钮：✕ 是半透明白底，✓ 是实心白底黑勾
func pillButton(c *ui.Context, sk voiceSkin, ok bool, tip string) ui.Element {
	bg, hover, fg := sk.btn, sk.btnHover, sk.fg
	if ok {
		bg, hover, fg = sk.ok, sk.okHover, sk.okFg
	}
	rad := float32(voiceBtn / 2)
	if sk.style == "pixel" {
		rad = 0
	}
	b := ui.ButtonBase(c).Size(voiceBtn, voiceBtn).Radius(rad).Background(bg).Cursor(ui.CursorPointer).Tooltip(tip)
	if b.Hovered() {
		b.Background(hover)
	}
	return b.DrawOver(func(p *ui.Painter, r ui.Rect) {
		if sk.style == "pixel" {
			if ok {
				drawPixelIcon(p, r, pixIconOK, fg)
			} else {
				drawPixelIcon(p, r, pixIconX, pixPink)
			}
			return
		}
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
	waveFrom    = ui.RGBA(175, 169, 236, 1) // 淡紫
	waveTo      = ui.RGBA(93, 202, 165, 1)  // 青绿
	waveToLight = ui.RGBA(29, 158, 117, 1)
	orbFrom     = ui.RGBA(127, 119, 221, 1) // 紫
	orbTo       = ui.RGBA(239, 159, 39, 1)  // 橙
)

func mix(a, b ui.Color, t float32) ui.Color {
	f := func(x, y uint8) uint8 { return uint8(float32(x) + (float32(y)-float32(x))*t) }
	return ui.Color{R: f(a.R, b.R), G: f(a.G, b.G), B: f(a.B, b.B), A: f(a.A, b.A)}
}

// drawWave 上下对称的柱子，从左到右紫变青；最新的音量在最右边往左流，中间高两头矮
func drawWave(p *ui.Painter, r ui.Rect, heights [voiceBars]float32, from, to ui.Color) {
	const bw, gap = 3, 2.5
	half := float32(voiceBars-1) / 2
	x := r.X + (r.W-(voiceBars*bw+(voiceBars-1)*gap))/2
	for i, v := range heights {
		d := float32(i) - half
		if d < 0 {
			d = -d
		}
		h := 3 + (r.H-3)*v*(1-0.5*d/half)
		p.Fill(ui.Rect{X: x, Y: r.Y + (r.H-h)/2, W: bw, H: h}, mix(from, to, float32(i)/(voiceBars-1)), bw/2)
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
	base *= float64(r.W) / voiceOrbW // 半径按框的大小等比缩放
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

// drawJelly 果冻条：沿胶囊轮廓走一圈，法线方向叠两道转速不同的小波纹，渐变跟果冻球同色
func drawJelly(p *ui.Painter, r ui.Rect, now time.Time) {
	t := float64(now.UnixMilli()%600000) / 1000
	const amp, n = 1.3, 96
	h := float64(r.H) - 2*amp
	rad := h / 2
	x0, y0, line := float64(r.X)+amp+rad, float64(r.Y)+amp+rad, max(0, float64(r.W)-2*amp-h)
	per := 2*line + 2*math.Pi*rad
	path := new(ui.Path)
	for k := 0; k <= n; k++ {
		d := per * float64(k) / n
		var x, y, nx, ny float64
		switch {
		case d < line: // 上边，从左往右
			x, y, nx, ny = x0+d, y0-rad, 0, -1
		case d < line+math.Pi*rad: // 右半圆
			a := (d-line)/rad - math.Pi/2
			nx, ny = math.Cos(a), math.Sin(a)
			x, y = x0+line+rad*nx, y0+rad*ny
		case d < 2*line+math.Pi*rad: // 下边，从右往左
			x, y, nx, ny = x0+line-(d-line-math.Pi*rad), y0+rad, 0, 1
		default: // 左半圆
			a := (d-2*line-math.Pi*rad)/rad + math.Pi/2
			nx, ny = math.Cos(a), math.Sin(a)
			x, y = x0+rad*nx, y0+rad*ny
		}
		u := d / per * 2 * math.Pi
		off := amp * (0.6*math.Sin(5*u+t*4) + 0.4*math.Sin(9*u-t*5))
		px, py := float32(x+nx*off), float32(y+ny*off)
		if k == 0 {
			path.MoveTo(px, py)
		} else {
			path.LineTo(px, py)
		}
	}
	p.FillPathGradient(path.Close(), ui.LinearGradient{From: orbFrom, To: orbTo, Angle: float32(110 + 20*math.Sin(t*0.8)), Oklab: true})
	p.AnimationFrame()
}

// drawDotsColor 识别中：三个点依次跳一下
func drawDotsColor(p *ui.Painter, r ui.Rect, fg ui.Color) {
	p.AnimationFrame()
	now := float64(time.Now().UnixMilli()) / 1000
	for i := 0; i < 3; i++ {
		lift := math.Max(0, math.Sin((now-float64(i)*0.15)*2*math.Pi))
		color := fg
		color.A = uint8(float64(fg.A) * (0.3 + 0.7*lift))
		x := r.X + r.W/2 + float32(i-1)*10 - 2.5
		p.Fill(ui.Rect{X: x, Y: r.Y + r.H/2 - 2.5 - float32(lift)*3, W: 5, H: 5}, color, 2.5)
	}
}

// runVoiceDemo 截图和调样式用：两种样式各放一遍（录音 3 秒假音量 → 识别 1.5 秒 → 提示 1.5 秒 → 退场），然后退出。会响提示音，不开麦克风
func (a *App) runVoiceDemo(only string) {
	pt := cursorPos()
	dpi, wa := dpiAt(pt), workArea(pt)
	id := voiceEpoch.Load()
	cfg, _ := a.config()
	styles := []string{"bars", "orb", "glass", "pixel", "mercury", "islands", "leds"}
	if only != "" {
		styles = strings.Split(only, ",")
	}
	for _, style := range styles {
		a.voiceWin.Update(func() {
			a.voiceWA, a.voiceDPI, a.voiceStyle, a.voiceTone, a.voiceOrbSize = wa, dpi, style, cfg.VoiceTone, voiceOrbW
		})
		playSound(soundStart)
		var levels []float64
		say := []rune("嗯，那个，帮我把这个接口改成用 Go 写，然后三点开会，不对，是四点开会。")
		for start := time.Now(); time.Since(start) < 4500*time.Millisecond; time.Sleep(50 * time.Millisecond) {
			x := time.Since(start).Seconds()
			level := 0.06 * math.Abs(math.Sin(x*3)+0.6*math.Sin(x*7.3))
			if x > 1.6 && x < 2.2 {
				level = 0.002 // 中间停一下，看安静时的样子
			}
			levels = append(levels, level)
			levels = levels[max(0, len(levels)-voiceBars):]
			st := voiceViewState{recording: true, levels: append([]float64(nil), levels...)}
			if n := int((x - 0.8) * 10); cfg.VoiceLiveText && n > 0 { // 边说边出字：先空一会儿，再一个个字冒出来
				st.live = string(say[:min(n, len(say))])
			}
			a.voiceDisplay(id, st, true)
		}
		playSound(soundStop)
		a.voiceDisplay(id, voiceViewState{recognizing: true}, true)
		time.Sleep(1500 * time.Millisecond)
		a.voiceDisplay(id, voiceViewState{text: T("没听清，请再说一次")}, true)
		time.Sleep(1500 * time.Millisecond)
		a.voiceDisplay(id, voiceViewState{text: "帮我把这个接口改成用 Go 写，然后四点开会。", copy: true}, true)
		time.Sleep(2500 * time.Millisecond)
		a.voiceDisplay(id, voiceViewState{text: T("识别失败，点右边重试"), retry: true}, true)
		time.Sleep(2000 * time.Millisecond)
		a.voiceDisplay(id, voiceViewState{}, false)
		time.Sleep(600 * time.Millisecond)
	}
	playSound(soundAbort)
	time.Sleep(400 * time.Millisecond)
	a.quit()
}

// voiceSkin 胶囊每种样式的配色；果冻球样式的提示条用黑胶囊这套
type voiceSkin struct {
	style             string
	fg                ui.Color // 文字、✕ 图标
	btn, btnHover     ui.Color
	ok, okHover, okFg ui.Color
}

var (
	pixBg     = ui.RGBA(27, 16, 54, 1)
	pixEdge   = ui.RGBA(61, 42, 122, 1)
	pixEdgeHi = ui.RGBA(92, 66, 170, 1)
	pixCyan   = ui.RGBA(127, 246, 255, 1)
	pixPink   = ui.RGBA(255, 62, 165, 1)
	clearC    = ui.RGBA(0, 0, 0, 0)
)

// skinFor 磨砂玻璃跟着深浅色走；像素和黑胶囊不跟主题
func skinFor(style string, dark bool) voiceSkin {
	switch {
	case style == "pixel":
		return voiceSkin{style, pixCyan, pixEdge, pixEdgeHi, pixCyan, ui.RGBA(190, 252, 255, 1), pixBg}
	case style == "glass" && !dark:
		ink := ui.RGBA(31, 37, 51, 1)
		return voiceSkin{style, ink, ui.RGBA(255, 255, 255, 0.75), ui.RGBA(255, 255, 255, 1), ink, ui.RGBA(58, 66, 86, 1), ui.RGBA(255, 255, 255, 1)}
	case style == "mercury" && !dark:
		ink := ui.RGBA(28, 31, 38, 1)
		return voiceSkin{style, ink, ui.RGBA(0, 0, 0, 0.07), ui.RGBA(0, 0, 0, 0.12), ink, ui.RGBA(58, 63, 74, 1), ui.RGBA(236, 238, 242, 1)}
	case style == "mercury":
		return voiceSkin{style, pillFg, ui.RGBA(255, 255, 255, 0.09), ui.RGBA(255, 255, 255, 0.17), ui.RGBA(232, 235, 240, 1), ui.RGBA(255, 255, 255, 1), ui.RGBA(30, 32, 38, 1)}
	case (style == "islands" || style == "leds") && !dark:
		return voiceSkin{style, ui.RGBA(30, 32, 38, 1), clearC, ui.RGBA(0, 0, 0, 0.07), clearC, ui.RGBA(0, 0, 0, 0.07), islandAccentLight}
	case style == "islands" || style == "leds":
		return voiceSkin{style, ui.RGBA(232, 234, 240, 1), clearC, ui.RGBA(255, 255, 255, 0.09), clearC, ui.RGBA(255, 255, 255, 0.09), islandAccentDark}
	case style == "glass":
		return voiceSkin{style, pillFg, ui.RGBA(255, 255, 255, 0.14), ui.RGBA(255, 255, 255, 0.26), ui.RGBA(255, 255, 255, 1), ui.RGBA(255, 255, 255, 1), ui.RGBA(27, 32, 48, 1)}
	}
	if !dark {
		return voiceSkin{style, pillInk, ui.RGBA(0, 0, 0, 0.07), ui.RGBA(0, 0, 0, 0.12), pillInk, ui.RGBA(58, 63, 74, 1), ui.RGBA(244, 245, 247, 1)}
	}
	return voiceSkin{style, pillFg, ui.RGBA(255, 255, 255, 0.10), ui.RGBA(255, 255, 255, 0.18), pillFg, ui.RGBA(255, 255, 255, 1), pillBg}
}

func (sk voiceSkin) wave(p *ui.Painter, r ui.Rect, h [voiceBars]float32) {
	switch sk.style {
	case "pixel":
		drawPixelWave(p, r, h)
	case "glass":
		drawWaveMono(p, r, h, sk.fg)
	case "mercury":
		drawMercuryRec(p, r, h[voiceBars-1])
	case "islands":
		drawEnvelope(p, r, h, sk.okFg)
	case "leds":
		drawLEDs(p, r, h[voiceBars-1], -2, sk.fg, sk.okFg)
	default:
		if sk.fg == pillInk { // 浅色底上原来的淡紫淡青太浅，换深一档
			drawWave(p, r, h, orbFrom, waveToLight)
			return
		}
		drawWave(p, r, h, waveFrom, waveTo)
	}
}

func (sk voiceSkin) dots(p *ui.Painter, r ui.Rect) {
	switch sk.style {
	case "pixel":
		drawPixelDots(p, r)
		return
	case "mercury":
		drawMercuryDots(p, r)
		return
	case "islands":
		drawSweep(p, r, sk.fg, sk.okFg)
		return
	case "leds":
		t := float64(time.Now().UnixMilli()%600000) / 1000
		drawLEDs(p, r, 0, math.Sin(t*4), sk.fg, sk.okFg)
		p.AnimationFrame()
		return
	}
	drawDotsColor(p, r, sk.fg)
}

// drawGlass 磨砂玻璃：半透明底、一圈亮边、顶上一道高光；录音时中间垫一团随音量变大的紫青光晕。
// 没有背景模糊：mygo 的 Acrylic 只能整窗开，语音窗口比胶囊大，开了就是一整块模糊的长方形
func drawGlass(p *ui.Painter, r ui.Rect, dark bool, level float32, halo bool) {
	rad := r.H / 2
	bg, edge, hi := ui.RGBA(40, 44, 60, 0.82), ui.RGBA(255, 255, 255, 0.16), ui.RGBA(255, 255, 255, 0.14)
	if !dark {
		bg, edge, hi = ui.RGBA(244, 246, 252, 0.86), ui.RGBA(120, 130, 160, 0.22), ui.RGBA(255, 255, 255, 0.55)
	}
	p.Fill(r, bg, rad)
	if halo {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		w, h := 28+30*level, 6+8*level
		p.Shadow(ui.Rect{X: cx - w/2, Y: cy - h/2, W: w, H: h}, h/2, 0, 0, 12, 2, ui.RGBA(150, 140, 255, 0.45+0.5*level))
		p.Shadow(ui.Rect{X: cx - w/4, Y: cy - h/2, W: w / 2, H: h}, h/2, 0, 0, 10, 0, ui.RGBA(93, 202, 165, 0.30+0.45*level))
	}
	// 高光是上半截的渐变，跟胶囊同一个圆角，两头顺着弧线淡掉（以前是一条直线，两端突兀）
	clear := hi
	clear.A = 0
	p.FillGradient(r, ui.LinearGradient{From: hi, To: clear, Angle: 180, Start: 0, End: 0.55}, rad)
	p.Stroke(ui.Rect{X: r.X + 0.5, Y: r.Y + 0.5, W: r.W - 1, H: r.H - 1}, edge, rad-0.5, 1)
}

// drawWaveMono 玻璃样式的波形：单色细柱，越响越不透明
func drawWaveMono(p *ui.Painter, r ui.Rect, heights [voiceBars]float32, c ui.Color) {
	const bw, gap = 3, 2.5
	half := float32(voiceBars-1) / 2
	x := r.X + (r.W-(voiceBars*bw+(voiceBars-1)*gap))/2
	for i, v := range heights {
		d := float32(i) - half
		if d < 0 {
			d = -d
		}
		h := 3 + (r.H-3)*v*(1-0.5*d/half)
		col := c
		col.A = uint8(float32(c.A) * (0.55 + 0.45*v))
		p.Fill(ui.Rect{X: x, Y: r.Y + (r.H-h)/2, W: bw, H: h}, col, bw/2)
		x += bw + gap
	}
}

// stepPath 四个角各缩两级台阶的矩形，像素风的「圆角」
func stepPath(r ui.Rect, s float32) *ui.Path {
	x0, y0, x1, y1 := r.X, r.Y, r.X+r.W, r.Y+r.H
	path := new(ui.Path)
	path.MoveTo(x0, y0+2*s).LineTo(x0+s, y0+2*s).LineTo(x0+s, y0+s).LineTo(x0+2*s, y0+s).LineTo(x0+2*s, y0).
		LineTo(x1-2*s, y0).LineTo(x1-2*s, y0+s).LineTo(x1-s, y0+s).LineTo(x1-s, y0+2*s).LineTo(x1, y0+2*s).
		LineTo(x1, y1-2*s).LineTo(x1-s, y1-2*s).LineTo(x1-s, y1-s).LineTo(x1-2*s, y1-s).LineTo(x1-2*s, y1).
		LineTo(x0+2*s, y1).LineTo(x0+2*s, y1-s).LineTo(x0+s, y1-s).LineTo(x0+s, y1-2*s).LineTo(x0, y1-2*s)
	return path.Close()
}

// drawPixelPill 深紫底、3px 亮紫边、阶梯角
func drawPixelPill(p *ui.Painter, r ui.Rect) {
	p.FillPath(stepPath(r, 3), pixEdge)
	p.FillPath(stepPath(ui.Rect{X: r.X + 3, Y: r.Y + 3, W: r.W - 6, H: r.H - 6}, 3), pixBg)
}

// drawPixelWave 9 列方块往上叠，每列 5 格，颜色从品红渐到青
func drawPixelWave(p *ui.Painter, r ui.Rect, heights [voiceBars]float32) {
	const cw, gap, cell, cgap, n = 3, 2, 3, 1, 5
	x := r.X + (r.W-(voiceBars*cw+(voiceBars-1)*gap))/2
	bottom := r.Y + (r.H+n*cell+(n-1)*cgap)/2
	for i, v := range heights {
		col := mix(pixPink, pixCyan, float32(i)/(voiceBars-1))
		lit := max(1, int(math.Round(float64(v*n))))
		for k := 0; k < n; k++ {
			c := col
			if k >= lit {
				c.A = 30
			}
			p.Fill(ui.Rect{X: x, Y: bottom - float32(k+1)*cell - float32(k)*cgap, W: cw, H: cell}, c, 0)
		}
		x += cw + gap
	}
}

// drawPixelDots 识别中：三个方块一格一格亮起来
func drawPixelDots(p *ui.Painter, r ui.Rect) {
	p.After(60 * time.Millisecond)
	step := int(time.Now().UnixMilli()/250) % 4
	for i := 0; i < 3; i++ {
		c := pixCyan
		if i >= step {
			c.A = 40
		}
		p.Fill(ui.Rect{X: r.X + r.W/2 + float32(i-1)*10 - 2, Y: r.Y + r.H/2 - 2, W: 4, H: 4}, c, 0)
	}
}

// 8×8 像素图标，# 是亮的格子
var (
	pixIconX = [8]string{
		"........",
		".#....#.",
		"..#..#..",
		"...##...",
		"...##...",
		"..#..#..",
		".#....#.",
		"........",
	}
	pixIconOK = [8]string{
		"........",
		"........",
		"......#.",
		".....#..",
		".#..#...",
		"..##....",
		"........",
		"........",
	}
	pixIconCopy = [8]string{
		"........",
		".####...",
		".#..#...",
		".#.####.",
		".###..#.",
		"...#..#.",
		"...####.",
		"........",
	}
)

func drawPixelIcon(p *ui.Painter, r ui.Rect, rows [8]string, c ui.Color) {
	const cell = 3
	x0, y0 := r.X+(r.W-8*cell)/2, r.Y+(r.H-8*cell)/2
	for y, row := range rows {
		for x, ch := range row {
			if ch == '#' {
				p.Fill(ui.Rect{X: x0 + float32(x)*cell, Y: y0 + float32(y)*cell, W: cell, H: cell}, c, 0)
			}
		}
	}
}

// mercuryShell 水银样式的底：深浅色各一套灰，水银珠本身是镜面，两种底上都看得清
func mercuryShell(dark bool) (bg, edge ui.Color) {
	if dark {
		return ui.RGBA(30, 32, 38, 0.96), ui.RGBA(255, 255, 255, 0.14)
	}
	return ui.RGBA(236, 238, 242, 0.97), ui.RGBA(0, 0, 0, 0.13)
}

// blobPath 一圈带波纹的闭合曲线：rx/ry 是两个方向的半径，wobble 是边缘颤动的幅度
func blobPath(cx, cy, rx, ry, wobble, t, seed float64) *ui.Path {
	path := new(ui.Path)
	for k := 0; k <= 48; k++ {
		th := float64(k) / 48 * 2 * math.Pi
		f := 1 + wobble*(0.5*math.Sin(3*th+t*5+seed)+0.3*math.Sin(5*th-t*7+seed)+0.2*math.Sin(2*th+t*3))
		x, y := float32(cx+rx*f*math.Cos(th)), float32(cy+ry*f*math.Sin(th))
		if k == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	return path.Close()
}

var (
	hgLight = ui.RGBA(250, 251, 254, 1)
	hgDark  = ui.RGBA(40, 45, 54, 1)
	hgBack  = ui.RGBA(178, 186, 200, 1)
)

// drawMercuryDrop 一颗镜面水银珠：上亮下暗的主体，底部一道反光，左上一点高光
func drawMercuryDrop(p *ui.Painter, cx, cy, r, stretch, wobble, t, seed float64) {
	body := blobPath(cx, cy, r*stretch, r, wobble, t, seed)
	p.FillPathGradient(body, ui.LinearGradient{From: hgLight, To: hgDark, Angle: 180, Start: 0.1, End: 0.62})
	p.StrokePath(body, 0.6, ui.RGBA(0, 0, 0, 0.22)) // 顶上是近白的，浅色底上靠这圈细边分出轮廓
	clear := hgBack
	clear.A = 0
	p.FillPathGradient(blobPath(cx, cy+r*0.42, r*stretch*0.72, r*0.5, wobble, t, seed), ui.LinearGradient{From: clear, To: hgBack, Angle: 180})
	p.FillPath(blobPath(cx-r*stretch*0.35, cy-r*0.48, r*stretch*0.28, r*0.17, 0, t, 0), ui.RGBA(255, 255, 255, 0.85))
}

// drawMercuryRec 录音中：一颗水银珠，声音越大越大、越扁、边缘颤得越厉害
func drawMercuryRec(p *ui.Painter, r ui.Rect, level float32) {
	t := float64(time.Now().UnixMilli()%600000) / 1000
	lv := float64(level)
	drawMercuryDrop(p, float64(r.X+r.W/2), float64(r.Y+r.H/2), 6.5+2.5*lv, 1.15+0.9*lv, 0.03+0.16*lv, t, 1)
	p.AnimationFrame()
}

// drawMercuryDots 识别中：三颗小水银珠绕着圈转，圈一胀一缩
func drawMercuryDots(p *ui.Painter, r ui.Rect) {
	t := float64(time.Now().UnixMilli()%600000) / 1000
	cx, cy, d := float64(r.X+r.W/2), float64(r.Y+r.H/2), 5.5+1.5*math.Sin(t*3)
	for k := 0; k < 3; k++ {
		a := t*3.4 + float64(k)*2*math.Pi/3
		drawMercuryDrop(p, cx+math.Cos(a)*d, cy+math.Sin(a)*d*0.6, 3.1, 1, 0.05, t, float64(k))
	}
	p.AnimationFrame()
}

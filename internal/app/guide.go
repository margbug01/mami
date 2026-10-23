package app

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"mami/internal/i18n"

	"github.com/egoist/mygo/ui"
)

// 新手引导：第一次启动时占住设置窗口，四屏——你好 → 自己说一句 → 自己划一句 → 好了
// （macOS 在「你好」后面多一屏：先把辅助功能和麦克风打开）。
// 中间两屏是真上手：按真的触发键（或点麦克风），真开麦，底部胶囊跟着声音动，那句话跟着念一个个字亮起来；
// 在真的输入框里选字，冒出「文A」，点开卡片，Enter 替换。结果是写死的：不识别、不润色、不发请求，也不要 Key。
// 看完或跳过就收起窗口，Mami 留在托盘；托盘菜单「新手引导…」能再来一遍。关窗口也算跳过。

const (
	stepHello = iota
	stepAllow // macOS：打开辅助功能和麦克风；Windows 跳过
	stepVoice
	stepTrans
	stepDone
	guidePages
)

// 语音那屏的几个阶段
const (
	sayIdle      = iota
	sayListening // 麦克风开着，字跟着声音亮
	sayPolishing // 口头禅和改口划掉
	sayDone      // 只剩润色好的那句
)

type guideVoiceState struct {
	phase int
	heard float64 // 亮到第几个字
	level float32 // 最近的音量，麦克风钮跟着呼吸
	msg   string  // 没听清、麦克风打不开
}

type guideTransState struct {
	text    string // 输入框里的字
	card    int    // 0 没有卡片，1 翻译中，2 出了译文
	cardAt  time.Time
	done    bool
	doneAt  time.Time // 换上译文的时刻，那段字亮一下
	copied  bool
	pickAll bool // 点了「文A」：下一帧把整句选上，翻的就是整句
	hadSel  bool // 上一帧输入框里选着字
}

var guideEnter = ui.ElementTransition{Duration: 360 * time.Millisecond, Enter: &ui.Motion{Y: 12}}

// guideScript 要念的那句，[方括号] 里是口头禅和改口：念的时候照念，整理时去掉
func guideScript() string {
	return T("[嗯，那个，]帮我把这个接口改成用 Go 写，然后[三点开会，不对，是]四点开会。")
}

type scriptPart struct {
	text   string
	filler bool
}

func parseScript(s string) []scriptPart {
	var parts []scriptPart
	for s != "" {
		open := strings.IndexByte(s, '[')
		if open < 0 {
			return append(parts, scriptPart{s, false})
		}
		if open > 0 {
			parts = append(parts, scriptPart{s[:open], false})
		}
		end := strings.IndexByte(s[open:], ']')
		if end < 0 { // 没配对就当正文，别吞字
			return append(parts, scriptPart{s[open+1:], false})
		}
		parts = append(parts, scriptPart{s[open+1 : open+end], true})
		s = s[open+end+1:]
	}
	return parts
}

// scriptText 念出来的整句（withFiller）或整理后的那句
func scriptText(parts []scriptPart, withFiller bool) string {
	var b strings.Builder
	for _, p := range parts {
		if withFiller || !p.filler {
			b.WriteString(p.text)
		}
	}
	return b.String()
}

// guideTransPair 划词那屏的原文、卡片标题和译文。中文界面中译英；其他界面用自己的语言写原文、翻成中文（默认设置就是这样翻）
func guideTransPair() (src, header, dst string) {
	src = T("能帮我看看这个插件为什么启动后不生效吗？")
	if l := i18n.Current(); l == i18n.ZH || l == i18n.TW {
		return src, T("中文 → English"), "Could you take a look at why this plugin doesn't take effect after startup?"
	}
	name := "English"
	for _, ch := range i18n.Choices {
		if ch.Value != "auto" && i18n.Resolve(ch.Value, "") == i18n.Current() {
			name = ch.Label
		}
	}
	return src, name + " → 中文", "能帮我看看这个插件为什么启动后不生效吗？"
}

func (a *App) openGuide() {
	a.openSettings()
	a.setWin.Update(func() { // 排在 openSettings 的重置后面
		a.set.guide = true
		a.guideGo(stepHello)
	})
}

// guideGo 换一屏：语音那屏没说完就停掉，回来时从头开始
func (a *App) guideGo(step int) {
	if step == stepAllow && !needPermissions {
		step = stepVoice
	}
	a.guideStopListening()
	a.set.guideStep = step
	a.set.voice = guideVoiceState{}
	if !a.set.trans.done {
		a.set.trans = guideTransState{}
	}
	a.set.trans.card = 0
}

// endGuide 看完或跳过：收起窗口。Key 等第一次真按触发键时再带去填（runVoice 里没 Key 的那条路）
func (a *App) endGuide() {
	a.set.guide = false
	a.guideIdle()
	if needPermissions && !accessibilityOK() { // macOS 没给辅助功能，按触发键什么都不会发生：留在「通用」页的权限那里
		a.set.page = pageGen
		return
	}
	if a.setWin != nil { // 测试里没有真窗口
		a.setWin.Close()
	}
}

// guideIdle 引导不在屏幕上了：触发键和 Alt+T 还给平常的流程
func (a *App) guideIdle() {
	a.guideVoice.Store(false)
	a.guideTrans.Store(false)
	a.guideStopListening()
}

func (a *App) guideStopListening() {
	if id := a.guideRec.Load(); id != 0 && voiceActive.Load() == id {
		voiceClick(voiceCancel)
	}
}

// guideScreen 引导和设置窗口铺同一张雾灰颗粒底
func (a *App) guideScreen(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) { windowTexture(t.Dark).draw(p, r, 0) }).Children(func() {
		a.titleBar(c)
		a.guideView(c)
	})
}

func (a *App) guideView(c *ui.Context) {
	s := &a.set
	a.guideVoice.Store(s.guideStep == stepVoice)
	a.guideTrans.Store(s.guideStep == stepTrans)
	ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Padding(12, 32).Children(func() {
		ui.Column(c).Key(s.guideStep).AlignItems(ui.Center).Gap(14).Transition(guideEnter).Children(func() {
			switch s.guideStep {
			case stepHello:
				a.guideHello(c)
			case stepAllow:
				a.guideAllow(c)
			case stepVoice:
				a.guideSay(c)
			case stepTrans:
				a.guideTranslate(c)
			default:
				a.guideDone(c)
			}
		})
	})
	if s.guideStep == stepAllow || s.guideStep == stepVoice || s.guideStep == stepTrans {
		a.guideFooter(c)
	}
}

// guideFooter 左边跳过、中间进度、右边继续；做完这一屏「继续」才变成主按钮，没做也能点
func (a *App) guideFooter(c *ui.Context) {
	t := c.Theme()
	s := &a.set
	finished := s.voice.phase == sayDone
	switch s.guideStep {
	case stepAllow:
		finished = accessibilityOK() && micOK()
	case stepTrans:
		finished = s.trans.done
	}
	ui.Row(c).Padding(12, 24, 16, 24).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Grow(1).AlignItems(ui.Center).Children(func() {
			if smallLink(c, T("跳过引导")).Clicked() {
				a.endGuide()
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			for i := range guidePages {
				if i == stepAllow && !needPermissions {
					continue
				}
				dot, w := t.Text.Alpha(0.16), float32(6)
				if i == s.guideStep {
					dot, w = t.Accent, 16
				}
				ui.Box(c).Size(w, 6).Radius(3).Background(dot)
			}
		})
		ui.Row(c).Grow(1).Justify(ui.End).AlignItems(ui.Center).Children(func() {
			button := textButton
			if finished {
				button = accentButton
			}
			if button(c, T("继续")).Clicked() {
				a.guideGo(s.guideStep + 1)
			}
		})
	})
}

func (a *App) guideAvatar(c *ui.Context) {
	if a.avatar != nil {
		ui.Image(c, a.avatar).Size(76, 76).Radius(38).Clip().Border(2, ui.RGBA(255, 255, 255, 0.9)).
			Shadow(0, 8, 24, 0, ui.RGBA(10, 20, 35, 0.14)).
			Transition(ui.ElementTransition{Duration: 500 * time.Millisecond, Enter: &ui.Motion{Scale: 0.85}})
	}
}

func guideHeading(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(26).FontWeight(600).TextAlign(ui.Center)
}

// guideLine 标题下那一行；换了内容就淡入淡出
func guideLine(c *ui.Context, text string, color ui.Color) {
	ui.Text(c, text).FontSize(14).TextColor(color).TextAlign(ui.Center).MaxWidth(460).ContentTransition(ui.ContentTransition{})
}

// keyLine 一行里夹着键帽：format 里的 %s 换成 acc 的键帽
func keyLine(c *ui.Context, format, acc string) {
	before, after, _ := strings.Cut(format, "%s")
	ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
		muted := c.Theme().TextMuted
		if before != "" {
			ui.Text(c, strings.TrimSpace(before)).FontSize(14).TextColor(muted).SingleLine().Shrink(0)
		}
		keycaps(c, acc)
		if after != "" {
			ui.Text(c, strings.TrimSpace(after)).FontSize(14).TextColor(muted).SingleLine().Shrink(0)
		}
	})
}

func (a *App) guideHello(c *ui.Context) {
	t := c.Theme()
	a.guideAvatar(c)
	ui.Box(c).Height(2)
	ui.Text(c, T("你好，我是 Mami")).FontSize(30).FontWeight(600)
	guideLine(c, T("在你打字的地方，帮你说，也帮你译"), t.TextMuted)
	ui.Box(c).Height(14)
	if accentButton(c, T("开始")).MinWidth(140).Clicked() {
		a.guideGo(stepAllow)
	}
	if smallLink(c, T("跳过")).Clicked() {
		a.endGuide()
	}
}

// guideAllow macOS：先打开两个权限。没打开的那行给按钮，打开了就打勾（refreshPermissions 每秒看一次）
func (a *App) guideAllow(c *ui.Context) {
	guideHeading(c, T("先打开两个权限"))
	guideLine(c, T("macOS 要你点头，Mami 才能听触发键、往输入框里贴字、用麦克风"), c.Theme().TextMuted)
	ui.Box(c).Height(6)
	ui.Column(c).Width(440).Gap(10).Children(func() { a.permissionRows(c) })
}

// voiceKeyUsable 触发键现在按了有用（开着语音、没关快捷键、设了键）
func (a *App) voiceKeyUsable() (string, bool) {
	cfg, _ := a.config()
	k := strings.TrimSpace(cfg.VoiceHotkey)
	return k, cfg.VoiceEnabled && !cfg.DisableHotkeys && k != "" && k != "Off"
}

func (a *App) guideSay(c *ui.Context) {
	t := c.Theme()
	v := &a.set.voice
	key, keyOK := a.voiceKeyUsable()
	guideHeading(c, T("试试用说的"))
	switch {
	case v.msg != "":
		guideLine(c, v.msg, t.Danger)
	case v.phase == sayListening && keyOK:
		keyLine(c, T("念完再按一下 %s"), key)
	case v.phase == sayListening:
		guideLine(c, T("念完再点一下麦克风"), t.TextMuted)
	case v.phase == sayPolishing:
		guideLine(c, T("整理中…"), t.TextMuted)
	case v.phase == sayDone:
		guideLine(c, T("口头禅和改口，都帮你去掉了"), t.TextMuted)
	case keyOK:
		keyLine(c, T("按一下 %s，念出这句话"), key)
	default:
		guideLine(c, T("点一下麦克风，念出这句话"), t.TextMuted)
	}
	ui.Box(c).Height(4)
	a.sayCard(c)
	ui.Box(c).Height(4)
	a.micButton(c)
}

// sayCard 要念的那句：念到哪亮到哪；整理时口头禅划掉，最后只剩整理好的
func (a *App) sayCard(c *ui.Context) {
	t := c.Theme()
	v := &a.set.voice
	spans := scriptSpans(parseScript(guideScript()), v.phase, v.heard, t.Text, t.TextMuted)
	ui.Column(c).Width(460).Padding(22, 28).Radius(18).Background(paneColor(t)).Border(1, paneBorder(t)).
		Shadow(0, 8, 24, 0, ui.RGBA(10, 20, 35, 0.04)).Children(func() {
		ui.RichText(c, spans...).LineHeight(1.6).TextAlign(ui.Center).ContentTransition(ui.ContentTransition{})
	})
}

// scriptSpans 那句话每一段的样子：念过的字用 text 色，没念到的淡；整理时口头禅划掉；整理好只剩干净的一句
func scriptSpans(parts []scriptPart, phase int, heard float64, text, muted ui.Color) []ui.Span {
	const size = 18
	var spans []ui.Span
	switch phase {
	case sayDone:
		return []ui.Span{{Text: scriptText(parts, false), Size: size, Color: text}}
	case sayPolishing:
		for _, p := range parts {
			sp := ui.Span{Text: p.text, Size: size, Color: text}
			if p.filler {
				sp.Color, sp.Strikethrough = muted.Alpha(0.6), true
			}
			spans = append(spans, sp)
		}
		return spans
	}
	left := int(heard)
	for _, p := range parts {
		r := []rune(p.text)
		k := min(max(left, 0), len(r))
		if k > 0 {
			spans = append(spans, ui.Span{Text: string(r[:k]), Size: size, Color: text})
		}
		if k < len(r) {
			spans = append(spans, ui.Span{Text: string(r[k:]), Size: size, Color: muted.Alpha(0.7)})
		}
		left -= len(r)
	}
	return spans
}

// iconCheck lucide 的 check
var iconCheck = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`))

// micButton 圆麦克风钮：没开时轻轻呼吸，开着时跟着音量鼓起来；点它和按触发键一样
func (a *App) micButton(c *ui.Context) {
	t := c.Theme()
	v := &a.set.voice
	const d = 56
	ui.Box(c).Size(96, 96).Children(func() {
		halo := ui.Box(c)
		grow := float32(0)
		switch v.phase {
		case sayIdle:
			grow = 10 * halo.Loop("breathe", 1800*time.Millisecond, ui.Bounce(ui.EaseInOut))
		case sayListening:
			grow = min(v.level*900, 36)
		}
		size := halo.Spring("halo", d+grow)
		halo.Size(size, size).Radius(size/2).Background(t.Accent.Alpha(0.08)).Attach(ui.AnchorCenter, ui.AnchorCenter)

		bg, fg := t.Accent, t.AccentText
		if v.phase == sayListening {
			bg, fg = ui.Hex("#e5484d"), ui.Hex("#ffffff")
		}
		b := ui.ButtonBase(c).Size(d, d).Radius(d/2).Center().Background(bg).Attach(ui.AnchorCenter, ui.AnchorCenter).Label(T("开始说话"))
		if v.phase == sayIdle || v.phase == sayListening {
			b.Cursor(ui.CursorPointer)
		}
		b.Children(func() {
			switch v.phase {
			case sayPolishing:
				ui.Spinner(c).FontSize(22).TextColor(fg)
			case sayDone:
				ui.Icon(c, iconCheck).FontSize(24).TextColor(fg)
			default:
				ui.Icon(c, iconMic).FontSize(24).TextColor(fg)
			}
		})
		if b.Clicked() && (v.phase == sayIdle || v.phase == sayListening) {
			v.msg = ""
			go voiceToggleSource(true)
		}
	})
	if v.phase == sayDone && smallLink(c, T("再试一次")).Clicked() {
		*v = guideVoiceState{}
	}
}

// guideListen 引导那屏的一次「录音」：真开麦，只拿音量。有声的每 100ms 亮一小段字，说上五秒左右就亮完；
// 停下后补齐剩下的字 → 划掉口头禅 → 只剩整理好的那句。太小声就像平常一样说没听清。在 voiceLoop 起的 goroutine 里跑
func (a *App) guideListen(ctx context.Context, stop <-chan struct{}, id uint64) {
	a.guideRec.Store(id)
	defer a.guideRec.CompareAndSwap(id, 0)
	defer voiceActive.CompareAndSwap(id, 0)
	valid := func() bool { return ctx.Err() == nil && voiceEpoch.Load() == id }
	v := &a.set.voice
	set := func(fn func()) { a.setWin.Update(fn) }
	reset := func(msg string) {
		set(func() { *v = guideVoiceState{msg: msg} })
	}

	cfg, _ := a.config()
	pt := cursorPos()
	dpi, wa := dpiAt(pt), workArea(pt)
	a.voiceWin.Update(func() {
		a.voiceWA, a.voiceDPI, a.voiceStyle, a.voiceTone, a.voiceOrbSize = wa, dpi, cfg.VoiceStyle, cfg.VoiceTone, float32(cfg.VoiceOrbSize)
		a.voiceJoined, a.voiceTranslate = cfg.VoiceJoined, false
	})
	set(func() { *v = guideVoiceState{phase: sayListening} })

	total := float64(utf8.RuneCountInString(scriptText(parseScript(guideScript()), true)))
	step := total / 50
	var heard, peak float64
	var levels []float64
	pcm, err := recordVoice(ctx, stop, func() {
		if valid() {
			a.voiceBeep(soundStart)
			a.voiceDisplay(id, voiceViewState{recording: true}, true)
		}
	}, func(_ []byte, level float64) {
		peak = max(peak, level)
		if level >= voiceSilenceRMS {
			heard = min(heard+step, total)
		}
		levels = append(levels, level)
		levels = levels[max(0, len(levels)-voiceBars):]
		if !valid() {
			return
		}
		a.voiceDisplay(id, voiceViewState{recording: true, levels: append([]float64(nil), levels...)}, true)
		h, lv := heard, float32(level)
		set(func() {
			if v.phase == sayListening {
				v.heard, v.level = h, lv
			}
		})
	})
	voiceRecording.CompareAndSwap(id, 0)
	switch {
	case !valid(): // Esc、点了 ✕、换了一屏：胶囊由 voiceLoop 收
		reset("")
		return
	case err != nil:
		a.voiceDisplay(id, voiceViewState{}, false)
		reset(err.Error())
		return
	case voiceTooQuiet(pcm, peak):
		a.voiceNotice(id, T("没听清，请再说一次"))
		reset(T("没听清，请再说一次"))
		return
	}

	a.voiceDisplay(id, voiceViewState{recognizing: true}, true)
	for heard < total { // 没念完的字很快补上，像识别结果回来了
		heard = min(heard+max(total/12, 1), total)
		h := heard
		set(func() { v.heard, v.level = h, 0 })
		time.Sleep(35 * time.Millisecond)
	}
	time.Sleep(250 * time.Millisecond)
	if voiceEpoch.Load() != id {
		reset("")
		return
	}
	set(func() { v.phase = sayPolishing })
	time.Sleep(1100 * time.Millisecond)
	if voiceEpoch.Load() != id {
		reset("")
		return
	}
	a.voiceDisplay(id, voiceViewState{}, false)
	set(func() { v.phase = sayDone })
}

func (a *App) guideTranslate(c *ui.Context) {
	t := c.Theme()
	cfg, _ := a.config()
	tr := &a.set.trans
	src, header, dst := guideTransPair()
	if tr.text == "" && !tr.done {
		tr.text = src
	}
	if tr.card == 1 && time.Since(tr.cardAt) >= 700*time.Millisecond {
		tr.card = 2
	} else if tr.card == 1 {
		c.After(700*time.Millisecond - time.Since(tr.cardAt))
	}

	guideHeading(c, T("选中，就能翻译"))
	switch { // 提示行在输入框上面，选区是输入框建好后才知道的，所以用上一帧的
	case tr.done && strings.TrimSpace(cfg.Hotkey) != "":
		keyLine(c, T("换好了。下次选中后按 %s，直接就换"), cfg.Hotkey)
	case tr.done:
		guideLine(c, T("换好了"), t.TextMuted)
	case tr.card > 0:
		keyLine(c, T("按 %s 替换"), "Enter")
	case tr.hadSel:
		guideLine(c, T("点一下末尾的「文A」"), t.TextMuted)
	default:
		guideLine(c, T("用鼠标选中下面这句话"), t.TextMuted)
	}
	ui.Box(c).Height(8)

	// 输入框按字的宽度收紧，「文A」就贴在字的末尾，跟真的冒在选区后面一样
	const size = 16
	w, _ := c.MeasureText(10000, ui.Span{Text: tr.text, Size: size})
	boxW := min(w+2*t.Space(3)+8, 600) // 窗口最窄时内容区约 608，再长就在框里滚
	holder := ui.Box(c).Width(boxW)
	holder.Children(func() {
		in := ui.TextInput(c, &tr.text).FontSize(size).Focus()
		if tr.pickAll {
			in.SetTextSelection(0, utf8.RuneCountInString(tr.text))
			tr.pickAll = false
		}
		if tr.done && time.Since(tr.doneAt) < 1200*time.Millisecond {
			in.TextRanges(ui.TextRange{Start: 0, End: utf8.RuneCountInString(tr.text), Color: t.Accent})
			c.After(1200*time.Millisecond - time.Since(tr.doneAt))
		}
		if in.Submitted() && tr.card == 2 {
			a.guideReplace(dst)
		}
		if in.Shortcut(0, ui.KeyEscape) && tr.card > 0 {
			tr.card = 0
		}
		start, end := in.TextSelection()
		tr.hadSel = start != end
		if tr.hadSel && tr.card == 0 && !tr.done {
			bg, fg, hover := buttonColors(t)
			b := ui.ButtonBase(c).Size(btnW, btnH).Center().Radius(cornerR).Background(bg).Border(1, t.Border).
				Cursor(ui.CursorPointer).Label(T("翻译")).Tooltip(T("翻译")).
				Attach(ui.AnchorRight, ui.AnchorLeft).Left(10).
				Transition(ui.ElementTransition{Duration: 220 * time.Millisecond, Enter: &ui.Motion{Scale: 0.6}})
			if b.Hovered() {
				b.Background(hover)
			}
			if b.Children(func() {
				ui.Box(c).Size(26, 26).Draw(func(p *ui.Painter, r ui.Rect) { drawDotMatrix(p, r, fg) })
			}).Clicked() {
				tr.card, tr.cardAt, tr.pickAll = 1, time.Now(), true
			}
		}
	})
	if tr.card > 0 {
		a.guideCard(c, header, dst)
	}
}

// guideCard 和真的译文卡片一个样子（popupView），只是长在引导里
func (a *App) guideCard(c *ui.Context, header, dst string) {
	t := c.Theme()
	tr := &a.set.trans
	ui.Column(c).Width(popW).Padding(12, 14).Gap(8).Background(t.Background).Border(1, t.Border).Radius(cornerR).TextColor(t.Text).
		Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.14)).
		Transition(ui.ElementTransition{Duration: 240 * time.Millisecond, Enter: &ui.Motion{Y: -6}}).Children(func() {
		ui.Row(c).Height(22).Gap(6).AlignItems(ui.Center).Children(func() {
			if a.avatar != nil {
				ui.Image(c, a.avatar).Size(18, 18).Radius(9).Clip()
			}
			ui.Text(c, header).FontSize(12).TextColor(t.TextMuted).SingleLine().Grow(1)
			x := ui.ButtonBase(c).Size(22, 22).Radius(6).Cursor(ui.CursorPointer).Label(T("关闭"))
			if x.Hovered() {
				x.Background(t.SurfaceHover)
			}
			if x.Children(func() { ui.Icon(c, iconClose).FontSize(14).TextColor(t.TextMuted) }).Clicked() {
				tr.card = 0
			}
		})
		if tr.card == 1 {
			ui.Row(c).Height(20).Gap(8).Children(func() {
				ui.Spinner(c).FontSize(14)
				ui.Text(c, T("翻译中…")).FontSize(14).TextColor(t.TextMuted)
			})
			return
		}
		ui.Text(c, dst).FontSize(14)
		ui.Row(c).Height(32).Gap(8).Children(func() {
			ui.Spacer(c)
			label := T("复制")
			if tr.copied {
				label = T("已复制")
			}
			if textButton(c, label).Clicked() {
				if theWorker != nil {
					theWorker.do(func(owner uintptr) { setClipboardText(owner, dst, false) })
				}
				tr.copied = true
			}
			if accentButton(c, T("替换")+" ↵").Clicked() {
				a.guideReplace(dst)
			}
		})
	})
}

func (a *App) guideReplace(dst string) {
	tr := &a.set.trans
	tr.text, tr.card, tr.done, tr.doneAt = dst, 0, true, time.Now()
}

// guideHotkey 在设置窗口前台、停在划词那屏时按的 Alt+T / Alt+Shift+T：跟平常一样，只是不发请求（主线程）
func (a *App) guideHotkey(replace bool) {
	tr := &a.set.trans
	if !a.set.guide || a.set.guideStep != stepTrans || tr.done {
		return
	}
	_, _, dst := guideTransPair()
	if replace {
		a.guideReplace(dst)
		return
	}
	tr.card, tr.cardAt, tr.pickAll = 1, time.Now(), true
}

func (a *App) guideDone(c *ui.Context) {
	t := c.Theme()
	cfg, _ := a.config()
	a.guideAvatar(c)
	ui.Box(c).Height(2)
	ui.Text(c, T("好了，就这么简单")).FontSize(30).FontWeight(600)
	if onMac {
		guideLine(c, T("Mami 住在菜单栏里，点图标打开菜单"), t.TextMuted)
	} else {
		guideLine(c, T("Mami 住在托盘里，单击图标打开设置"), t.TextMuted)
	}
	ui.Box(c).Height(6)
	ui.Row(c).Gap(28).AlignItems(ui.Center).Children(func() {
		if key, ok := a.voiceKeyUsable(); ok {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				keycaps(c, key)
				ui.Text(c, T("说话")).FontSize(13).TextColor(t.TextMuted).SingleLine().Shrink(0)
			})
		}
		if strings.TrimSpace(cfg.Hotkey) != "" && !cfg.DisableHotkeys {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				keycaps(c, cfg.Hotkey)
				ui.Text(c, T("翻译")).FontSize(13).TextColor(t.TextMuted).SingleLine().Shrink(0)
			})
		}
	})
	ui.Box(c).Height(14)
	if accentButton(c, T("开始使用")).MinWidth(140).Clicked() {
		a.endGuide()
	}
}

package app

import (
	"bytes"
	_ "embed"
	"image/png"
	"time"

	"github.com/egoist/mygo/ui"
)

//go:embed assets/tray.png
var trayPNG []byte

//go:embed assets/avatar.png
var avatarPNG []byte

// lucide 的 languages / x / copy 图标
var (
	iconChevron = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>`))
	iconClose   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`))
)

// 点阵版「文 A」：文 5×6、A 3×5，都做成瘦长的；A 在文右边空一列、往下错开 3 格（aOffX/aOffY），拼成 9×8
var (
	glyphWen = []string{
		"..#..",
		"#####",
		".#.#.",
		".#.#.",
		"..#..",
		"##.##",
	}
	glyphA = []string{
		".#.",
		"#.#",
		"#.#",
		"###",
		"#.#",
	}
)

// drawDotMatrix 按物理像素画点阵：每格是整数个像素、点之间至少空 1 像素，
// 不然缩放以后点会糊成线。box 是可用区域（DIP），点阵在里面居中。
const aOffX, aOffY = 6, 3

func drawDotMatrix(p *ui.Painter, box ui.Rect, color ui.Color) {
	s := p.Scale()
	if s <= 0 {
		s = 1
	}
	gw, gh := max(len(glyphWen[0]), aOffX+len(glyphA[0])), max(len(glyphWen), aOffY+len(glyphA))
	g := max(gw, gh, 10)                            // 每格大小按 10 格算：图案变窄只是变窄，不会把点放大
	cell := int(min(box.W, box.H) * s / float32(g)) // 每格物理像素
	cell = max(cell, 2)
	gap := max(1, cell/3)
	dot := cell - gap
	ox := float32(int((box.X+box.W/2)*s+0.5) - cell*gw/2)
	oy := float32(int((box.Y+box.H/2)*s+0.5) - cell*gh/2)
	radius := float32(0)
	if dot >= 3 {
		radius = float32(dot) / 2 / s
	}
	put := func(rows []string, cx, cy int) {
		for y, row := range rows {
			for x, ch := range row {
				if ch != '#' {
					continue
				}
				px := ox + float32((cx+x)*cell)
				py := oy + float32((cy+y)*cell)
				p.Fill(ui.Rect{X: px / s, Y: py / s, W: float32(dot) / s, H: float32(dot) / s}, color, radius)
			}
		}
	}
	put(glyphWen, 0, 0)
	put(glyphA, aOffX, aOffY)
}

func loadAvatar() *ui.Bitmap {
	img, err := png.Decode(bytes.NewReader(avatarPNG))
	if err != nil {
		return nil
	}
	return ui.NewBitmap(img)
}

// ---------- 浮动按钮 ----------

// cornerR 按钮和卡片的圆角半径（DIP）
const cornerR = 10

// buttonColors 划词按钮配色：深色主题纯黑底白字，浅色主题白底黑字，不用主题蓝
func buttonColors(t *ui.Theme) (bg, fg, hover ui.Color) {
	if t.Dark {
		return ui.RGBA(0, 0, 0, 1), ui.RGBA(255, 255, 255, 1), ui.RGBA(38, 38, 38, 1)
	}
	return ui.RGBA(255, 255, 255, 1), ui.RGBA(0, 0, 0, 1), ui.RGBA(235, 235, 235, 1)
}

func (a *App) buttonView(c *ui.Context) {
	t := a.themed(c)
	bg, fg, hoverBg := buttonColors(t)
	b := ui.ButtonBase(c).Fill().Center().
		Background(bg).Border(1, t.Border).Radius(cornerR).Cursor(ui.CursorPointer).Label(T("翻译")).Tooltip(T("翻译"))
	if a.btnBusy.Load() {
		b.Cursor(ui.CursorDefault).Label(T("翻译中")).Tooltip(T("翻译中…"))
		b.Children(func() { ui.Spinner(c).FontSize(18).TextColor(fg) })
		return
	}
	hovered := b.Hovered()
	a.btnHover.Store(hovered)
	if hovered {
		b.Background(hoverBg)
	}
	if b.Children(func() {
		accent := fg
		ui.Box(c).Size(26, 26).Draw(func(p *ui.Painter, r ui.Rect) { drawDotMatrix(p, r, accent) })
	}).Clicked() {
		a.onButtonClick()
	}
}

// textButton / accentButton 就是 ui.Button / ui.PrimaryButton，只多给一个按 MiSans 量出来的最小宽度。
// MyGo 给带左右内边距的按钮量宽时没按主题字体算，MiSans 比系统字体宽，西文标签会被截成「Guar…」。
func textButton(c *ui.Context, label string) ui.Element {
	return fitLabel(c, ui.Button(c, label), label)
}

func accentButton(c *ui.Context, label string) ui.Element {
	return fitLabel(c, ui.PrimaryButton(c, label), label)
}

// segmented 就是 ui.Segmented 的样子，只是每段按 MiSans 量出最小宽度：
// 原生的同样没按主题字体量，「All」「Gelernt」「자동 학습」这类会被截成省略号
func segmented(c *ui.Context, selected *int, labels ...string) ui.Element {
	t := c.Theme()
	parts := ui.SegmentedBase(c, selected, len(labels))
	track, chosen := ui.RGBA(20, 30, 45, 0.055), t.Surface
	if t.Dark {
		track, chosen = ui.RGBA(255, 255, 255, 0.06), t.SurfacePressed
	}
	parts.Track.AlignItems(ui.Stretch).Padding(t.Space(0.75)).Gap(t.Space(0.5)).Radius(t.Radius + 3).Background(track).Label(labels[0])
	parts.Track.Children(func() {
		for i, label := range labels {
			w, _ := c.MeasureText(10000, ui.Span{Text: label, Size: t.FontSize})
			s := parts.Segment(i).Padding(t.Space(1), t.Space(3)).Radius(t.Radius + 1).MinWidth(w + 2*t.Space(3) + 8)
			switch {
			case i == *selected:
				s.Background(chosen).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.1))
			case s.Hovered():
				s.Background(t.SurfaceHover)
			}
			color := t.TextMuted
			if i == *selected {
				color = t.Text
			}
			s.Children(func() { ui.Text(c, label).SingleLine().TextColor(color) })
		}
	})
	return parts.Track
}

func fitLabel(c *ui.Context, b ui.Element, label string) ui.Element {
	t := c.Theme()
	w, _ := c.MeasureText(10000, ui.Span{Text: label, Size: t.FontSize})
	return b.MinWidth(w + 2*t.Space(3.5) + 8) // 两边内边距 + 边框；正好等宽时 MyGo 仍会按小数误差截成省略号，多留几像素
}

// ---------- 译文卡片 ----------

func (a *App) popupView(c *ui.Context) {
	t := a.themed(c)
	p := &a.pop
	const pad, headH, gap, footH = 12, 22, 8, 32
	textW := float32(p.w - 2*pad - 10) // 给滚动条留一点

	// streaming：流式翻译已经收到一部分，卡片先显示出来，标题栏转圈表示还没完
	streaming := p.status == statusLoading && p.text != ""
	body := p.text
	if p.status == statusLoading && !streaming {
		body = ""
	}
	bodyH := float32(20)
	if p.status != statusLoading || streaming {
		_, mh := c.MeasureText(textW, ui.Span{Text: body, Size: 14})
		bodyH = min(mh+2, bodyMaxH)
	}
	want := pad + headH + gap + int(bodyH+0.5) + pad
	if p.status == statusDone {
		want += gap + footH
	}
	// 边收边显示时高度只增不减：换行位置一变就缩回去再撑开，卡片会上下跳
	if a.popVisible.Load() && (want > p.h || (want < p.h && !streaming)) {
		p.h = want
		w, anchor := p.w, p.anchor
		go a.resizePopup(w, want, anchor)
	}

	// 告诉键盘钩子现在 Enter / Esc 归卡片管
	mode := int32(0)
	if a.popVisible.Load() && !p.toast {
		mode = 1
		if p.status == statusDone && !p.terminal {
			mode = 2
		}
	}
	cardKeyFg.Store(p.fg)
	cardKeyMode.Store(mode)

	ui.Column(c).Fill().Padding(pad, 14).Gap(gap).Background(t.Background).Border(1, t.Border).Radius(cornerR).Children(func() {
		ui.Row(c).Height(headH).Gap(6).Children(func() {
			if a.avatar != nil {
				ui.Image(c, a.avatar).Size(18, 18).Radius(9).Clip()
			}
			ui.Text(c, p.header).FontSize(12).TextColor(t.TextMuted).SingleLine().Grow(1)
			if streaming {
				ui.Spinner(c).FontSize(12).TextColor(t.TextMuted)
			}
			if !p.toast {
				x := ui.ButtonBase(c).Size(22, 22).Radius(6).Cursor(ui.CursorPointer).Label(T("关闭"))
				if x.Hovered() {
					x.Background(t.SurfaceHover)
				}
				if x.Children(func() { ui.Icon(c, iconClose).FontSize(14).TextColor(t.TextMuted) }).Clicked() {
					a.closeCard()
				}
			}
		})
		switch {
		case p.status == statusLoading && !streaming:
			ui.Row(c).Height(20).Gap(8).Children(func() {
				ui.Spinner(c).FontSize(14)
				ui.Text(c, T("翻译中…")).FontSize(14).TextColor(t.TextMuted)
			})
		default:
			color := t.Text
			if p.status == statusError {
				color = t.Danger
			}
			ui.Scroll(c).Height(bodyH).Children(func() {
				ui.Text(c, body).FontSize(14).TextColor(color).Width(textW)
			})
		}
		if p.status == statusDone {
			ui.Row(c).Height(footH).Gap(8).Children(func() {
				ui.Spacer(c)
				label := T("复制")
				if p.copied {
					label = T("已复制")
				}
				if textButton(c, label).Clicked() {
					text := p.text
					theWorker.do(func(owner uintptr) { setClipboardText(owner, text, false) })
					p.copied, p.acted = true, true
					a.recordAction(p.histID, "copy")
				}
				if !p.terminal && accentButton(c, T("替换")+" ↵").Clicked() {
					a.replaceFromCard()
				}
			})
		}
	})
}

// ---------- 演示模式：画出按钮和一张示例卡片，用来检查渲染 ----------

func (a *App) runDemo() {
	pt := POINT{300, 260}
	a.showButton(pt, a.btnGen.Add(1))
	a.showPopup(popupState{
		status: statusDone,
		header: T("中文 → English"),
		text:   "Could you take a look at why this plugin doesn't take effect after startup? There's nothing in the logs.\n\nSecond paragraph: the meeting is at 3 p.m. tomorrow.",
		anchor: POINT{pt.X, pt.Y + 70},
	})
	time.AfterFunc(8*time.Second, a.quit)
}

// themed 按配置里的外观（跟随系统 / 浅色 / 深色）给这一帧选主题
func (a *App) themed(c *ui.Context) *ui.Theme {
	cfg, _ := a.config()
	applyTheme(c, cfg.Theme)
	t := c.Theme()
	// 窗口根背景透明，圆角外面才露出桌面；按钮和卡片的底色仍用 t 里的原色
	clear := *t
	clear.Background = ui.RGBA(0, 0, 0, 0)
	c.SetTheme(&clear)
	return t
}

func applyTheme(c *ui.Context, mode string) {
	registerFonts()
	var t *ui.Theme
	switch mode {
	case "light":
		t = ui.LightTheme()
	case "dark":
		t = ui.DarkTheme()
	default: // 跟随系统：拿系统给的主题改一份（字号、对比度这些留着），颜色换成自己的
		cur := *c.Theme()
		t = &cur
	}
	t.Font = fontStack
	mamiColors(t)
	c.SetTheme(t)
}

// mamiColors 雾灰：石墨色强调、偏冷的中性灰、低透明度的细边，替掉 mygo 自带的亮蓝和灰线。
// 输入框、下拉框、按钮用 Surface，所以 Surface 是实白；卡片是半透明的白（paneColor），底下透出整窗的底纹。
// 深色的强调色是浅灰、上面写深字；不用纯白：开关的圆钮固定是白的，强调色太亮就看不见钮
func mamiColors(t *ui.Theme) {
	if t.Dark {
		t.Background = ui.Hex("#1d2025")
		t.Surface = ui.Hex("#24282e")
		t.SurfaceHover = ui.Hex("#2b3037")
		t.SurfacePressed = ui.Hex("#323841")
		t.Border = ui.RGBA(255, 255, 255, 0.09)
		t.Text = ui.Hex("#e7eaee")
		t.TextMuted = ui.Hex("#8d949f")
		t.Accent = ui.Hex("#a7b0bc")
		t.AccentHover = ui.Hex("#b6bec8")
		t.AccentPressed = ui.Hex("#98a1ad")
		t.Selection = ui.RGBA(150, 165, 190, 0.32)
		t.Focus = ui.RGBA(200, 210, 225, 0.45)
		t.AccentText = ui.Hex("#15181c")
	} else {
		t.Background = ui.Hex("#ffffff")
		t.Surface = ui.Hex("#ffffff")
		t.SurfaceHover = ui.Hex("#f3f5f8")
		t.SurfacePressed = ui.Hex("#e9edf2")
		t.Border = ui.RGBA(20, 30, 45, 0.11)
		t.Text = ui.Hex("#1c2027")
		t.TextMuted = ui.Hex("#6a717c")
		t.Accent = ui.Hex("#23272e")
		t.AccentHover = ui.Hex("#363b44")
		t.AccentPressed = ui.Hex("#15181d")
		t.Selection = ui.RGBA(70, 100, 140, 0.2)
		t.Focus = ui.RGBA(35, 39, 46, 0.35)
		t.AccentText = ui.Hex("#ffffff")
	}
}

// paneColor / paneBorder 设置分组、引导里那句话的卡片：半透明白加一圈很淡的边
func paneColor(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(255, 255, 255, 0.045)
	}
	return ui.RGBA(255, 255, 255, 0.66)
}

func paneBorder(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(255, 255, 255, 0.07)
	}
	return ui.RGBA(20, 30, 45, 0.06)
}

// sideSelected 侧边栏选中项的底：一块半透明的白
func sideSelected(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(255, 255, 255, 0.08)
	}
	return ui.RGBA(255, 255, 255, 0.8)
}

// sidebarTheme 侧边栏换一份主题：自己不铺底（坐在整窗底纹上），图标用淡灰。
// mygo 的选中项拿焦点时用 Accent 铺底、AccentText 写字，所以 Accent 就是没选中的图标色，
// AccentText 是选中后图标和字的颜色，底色由 sideItem 盖成半透明白
func sidebarTheme(t *ui.Theme) *ui.Theme {
	s := *t
	s.Surface = ui.Transparent
	s.Accent = t.TextMuted
	s.AccentText = t.Text
	s.SurfacePressed = sideSelected(t)
	if t.Dark {
		s.SurfaceHover = ui.RGBA(255, 255, 255, 0.045)
	} else {
		s.SurfaceHover = ui.RGBA(20, 30, 45, 0.045)
	}
	return &s
}

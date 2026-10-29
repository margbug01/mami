package app

import (
	"log"
	"math"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type voiceDock struct {
	win                                             *mygo.Window
	hwnd                                            uintptr
	expanded, more, langs, shown, pressed, dragging bool
	armed                                           bool
	hoverAt, leaveAt                                time.Time
	down                                            POINT
	grab                                            POINT
	position                                        dockPosition
	area                                            RECT
	dpi                                             uint32
	rect                                            RECT
	seen                                            RECT // 上次摆完窗口读回来的位置；和它对不上就是别人挪过
	texture                                         *texture
	motionAt                                        time.Time
	page                                            string
	pageAt                                          time.Time
	motionFrom, motionTo                            RECT
	reveal, revealFrom, revealTo                    float32
}

var dockQuit atomic.Bool

// dockVoiceToggle 面板上的「开始说话 / 结束」；测试里换成假的
var dockVoiceToggle = func() { go voiceToggleSource(true) }

// dockPlace 摆小条窗口；测试里数调了几次
var dockPlace = placeAt

// dockIdleTick 小条关着（Mac 默认）或全屏时隐藏着：没东西要动，隔久一点才看一次要不要出来
const dockIdleTick = 300 * time.Millisecond

func (d *voiceDock) dragHit(point POINT) bool {
	return dockContains(d.rect, point) && (!d.expanded || point.Y < d.rect.Top+scale(32, d.dpi))
}

func (d *voiceDock) dragPoint(point POINT) POINT {
	return POINT{X: point.X - d.grab.X, Y: point.Y - d.grab.Y}
}

func (d *voiceDock) dragMoved(point POINT) bool {
	deltaX, deltaY := int64(point.X-d.down.X), int64(point.Y-d.down.Y)
	threshold := int64(scale(4, d.dpi))
	return deltaX*deltaX+deltaY*deltaY > threshold*threshold
}

func (a *App) finishDockDrag(commit bool) {
	d := &a.dock
	moved := d.dragging
	d.armed, d.dragging = false, false
	dockReleaseCapture(d.hwnd)
	if moved && commit {
		point := cursorPos()
		d.area, _, d.position.Monitor = dockMonitor(point)
		d.dpi = dpiAt(point)
		area := dockRect{W: int(d.area.w()) * 96 / int(d.dpi), H: int(d.area.h()) * 96 / int(d.dpi)}
		monitor := d.position.Monitor
		d.position = dockSnap(area, int(point.X-d.area.Left)*96/int(d.dpi), int(point.Y-d.area.Top)*96/int(d.dpi))
		d.position.Monitor = monitor
		position := d.position
		d.win.Update(func() { a.changeDockConfig(func(cfg *Config) { cfg.VoiceDockPosition = position }) })
	}
	d.motionAt, d.motionTo = time.Time{}, d.rect
	d.hoverAt, d.leaveAt = time.Now(), time.Time{}
	d.win.Invalidate()
}

func (a *App) createDock() {
	cfg, _ := a.config()
	d := &a.dock
	d.position = cfg.VoiceDockPosition
	point := dockSavedMonitor(d.position.Monitor)
	d.area, _, d.position.Monitor = dockMonitor(point)
	d.dpi = dpiAt(point)
	d.win, d.hwnd = a.floatingWindow("mami-dock", dockThick, dockLong, 1, 1, a.dockView)
	a.dockHwnd.Store(d.hwnd)
	a.hookDockMouse()
	go func() {
		interval := 40 * time.Millisecond
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			d.win.Update(func() {
				if !a.quitting {
					a.tickDock()
					next := 40 * time.Millisecond
					if !d.motionAt.IsZero() {
						next = 8 * time.Millisecond
					} else if !d.shown {
						next = dockIdleTick
					}
					if next != interval {
						interval = next
						ticker.Reset(interval)
					}
				}
			})
			if dockQuit.Load() {
				return
			}
		}
	}()
}

func dockContains(rect RECT, point POINT) bool {
	return point.X >= rect.Left && point.X < rect.Right && point.Y >= rect.Top && point.Y < rect.Bottom
}

func (a *App) tickDock() {
	d := &a.dock
	cfg, _ := a.config()
	center := POINT{X: d.area.Left + d.area.w()/2, Y: d.area.Top + d.area.h()/2}
	d.area, _, d.position.Monitor = dockMonitor(center)
	d.dpi = dpiAt(center)
	visible := cfg.VoiceDock && cfg.VoiceEnabled && !dockFullscreen()
	if !visible {
		if d.armed {
			a.finishDockDrag(false)
		}
		if d.shown {
			d.win.Hide()
			d.shown = false
		}
		d.expanded, d.more, d.langs, d.pressed, d.dragging, d.armed = false, false, false, false, false, false
		d.hoverAt, d.leaveAt = time.Time{}, time.Time{}
		d.motionAt = time.Time{}
		d.reveal = 0
		return
	}
	if d.armed || a.nativeDockDrag() {
		return
	}
	// 窗口被别人挪过（换分辨率、改缩放时系统会挪，MyGo 收到 WM_DPICHANGED 也会按系统建议的位置挪）：
	// 记下它真实的位置，下面就会把它放回该在的边上。不然 d.rect 还记着旧位置、算出来「没变」就不摆，
	// 小条停在屏幕中间，悬停、拖动又都按旧位置判断，怎么点都没反应
	if d.seen != (RECT{}) {
		if actual := windowRect(d.hwnd); actual != d.seen {
			d.rect, d.seen = actual, actual
		}
	}
	point, now := cursorPos(), time.Now()
	inside := dockContains(d.rect, point)
	pressed := keyDown(VK_LBUTTON)
	// 展开时在外面点一下（左键或右键）立刻收起，不用等一秒
	if d.expanded && !inside && (pressed && !d.pressed || keyDown(VK_RBUTTON)) {
		d.expanded, d.more, d.langs, d.leaveAt = false, false, false, time.Time{}
	}
	d.pressed = pressed
	if !d.dragging {
		if inside {
			d.leaveAt = time.Time{}
			if d.hoverAt.IsZero() {
				d.hoverAt = now
			}
			if !pressed && now.Sub(d.hoverAt) >= 200*time.Millisecond {
				d.expanded = true
			}
		} else {
			d.hoverAt, d.leaveAt = time.Time{}, time.Time{}
			d.expanded, d.more, d.langs = false, false, false
		}
		height := 248
		if d.more || d.langs {
			height = 416
		}
		area := dockRect{W: int(d.area.w()) * 96 / int(d.dpi), H: int(d.area.h()) * 96 / int(d.dpi)}
		rect := dockPlacement(area, d.position, d.expanded, height)
		native := RECT{Left: d.area.Left + scale(int32(rect.X), d.dpi), Top: d.area.Top + scale(int32(rect.Y), d.dpi)}
		native.Right = native.Left + scale(int32(rect.W), d.dpi)
		native.Bottom = native.Top + scale(int32(rect.H), d.dpi)
		reveal := float32(0)
		if d.expanded {
			reveal = 1
		}
		if !d.shown {
			d.motionAt = time.Time{}
			d.motionTo, d.revealTo, d.reveal = native, reveal, reveal
		} else if native != d.motionTo || reveal != d.revealTo {
			d.motionFrom, d.motionTo = d.rect, native
			d.revealFrom, d.revealTo = d.reveal, reveal
			d.motionAt = now
		}
		if !d.motionAt.IsZero() {
			progress := min(float64(now.Sub(d.motionAt))/(220*float64(time.Millisecond)), 1)
			eased := 1 - math.Pow(1-progress, 3)
			blend := func(from, to int32) int32 {
				return int32(math.Round(float64(from) + float64(to-from)*eased))
			}
			native = RECT{
				Left: blend(d.motionFrom.Left, d.motionTo.Left), Top: blend(d.motionFrom.Top, d.motionTo.Top),
				Right: blend(d.motionFrom.Right, d.motionTo.Right), Bottom: blend(d.motionFrom.Bottom, d.motionTo.Bottom),
			}
			d.reveal = d.revealFrom + (d.revealTo-d.revealFrom)*float32(eased)
			if progress == 1 {
				d.motionAt = time.Time{}
			}
		}
		if native != d.rect {
			dockPlace(d.hwnd, native.Left, native.Top, native.w(), native.h())
			// 读回来的可能和要的差一点（系统限制最小尺寸、换算取整），记读回来的，免得每一下都当成被挪过再摆一遍
			d.rect, d.seen = native, windowRect(d.hwnd)
		}
	}
	if !d.shown {
		d.win.ShowInactive()
		d.shown = true
	}
}

// nativeDockDrag macOS 上小条交给系统拖（DragWindow），这里只认出「正在拖」，松手后吸到最近的边。
// Windows 上拖动由 dockMouse 在子类化的窗口过程里自己做，这里总是 false
func (a *App) nativeDockDrag() bool {
	d := &a.dock
	if !dockNativeDrag {
		return false
	}
	if keyDown(VK_LBUTTON) && d.shown && (d.dragging || windowRect(d.hwnd) != d.rect) {
		d.dragging = true
		d.rect = windowRect(d.hwnd)
		return true
	}
	if d.dragging {
		a.finishDockDrag(true)
		return true
	}
	return false
}

func (a *App) changeDockConfig(change func(*Config)) {
	cfg, _ := a.config()
	before := cfg
	change(&cfg)
	if err := a.setConfig(cfg); err != nil {
		log.Println("dock config:", err)
		return
	}
	if err := saveConfig(a.cfgPath, cfg); err != nil {
		log.Println("save config:", err)
	}
	if a.setWin != nil {
		a.setWin.Update(func() { syncDockDraft(&a.set.draft, before, cfg) })
	}
	a.updateTrayTip()
}

func dockCycle(value string, values []string) string {
	for index, item := range values {
		if item == value {
			return values[(index+1)%len(values)]
		}
	}
	return values[0]
}

func (a *App) dockView(c *ui.Context) {
	cfg, _ := a.config()
	a.dockContent(c, cfg, a.dock.expanded || a.dock.reveal > 0, a.dock.more, toneDark(cfg.VoiceDockTone, a.themed(c).Dark), a.dock.reveal)
}

// dockContent 贴边小条本体；设置页的外观预览用 renderDockContent(…, interactive=false) 画一个不能点的
func (a *App) dockContent(c *ui.Context, cfg Config, expanded, more, dark bool, reveal ...float32) {
	a.renderDockContent(c, cfg, expanded, more, dark, true, reveal...)
}

func (a *App) renderDockContent(c *ui.Context, cfg Config, expanded, more, dark, interactive bool, reveal ...float32) {
	a.drawDock(c, cfg, expanded, more, dark, interactive, dockPanel{more: &a.dock.more, langs: &a.dock.langs}, reveal...)
}

// dockPanel 展开面板画在哪儿：贴边小条（能拖，翻到哪页记在 a.dock）或菜单栏面板（挂在图标下面不拖，翻页记在 a.panel）。
// 两个各记各的：小条的 tickDock 鼠标一离开就把它那份翻回首页
type dockPanel struct {
	more, langs *bool
	menubar     bool
	done        func() // 点了「开始说话」「完整设置…」之后：菜单栏面板借它收起来
}

func (a *App) drawDock(c *ui.Context, cfg Config, expanded, more, dark, interactive bool, at dockPanel, reveal ...float32) {
	done := func() {
		if at.done != nil {
			at.done()
		}
	}
	pointer, grab := ui.CursorDefault, ui.CursorDefault
	if interactive {
		pointer, grab = ui.CursorPointer, ui.CursorGrab
	}
	t := ui.LightTheme()
	if dark {
		t = ui.DarkTheme()
	}
	mamiColors(t)
	card, fg, muted, border, active := paneColor(t), t.Text, t.TextMuted, t.Border, sideSelected(t)
	accent := t.Accent
	base := windowTexture(dark)
	if a.dock.texture == nil || a.dock.texture.pal != base.pal {
		a.dock.texture = &texture{pal: base.pal, grain: base.grain}
	}
	material := a.dock.texture
	radius := float32(10)
	if expanded {
		radius = 16
	}
	if len(reveal) != 0 {
		radius = 10 + 6*reveal[0]
	}
	background := func(p *ui.Painter, r ui.Rect) {
		material.draw(p, r, radius)
	}
	if !expanded {
		opacity := float32(0.4)
		if voiceRecording.Load() != 0 {
			opacity = 0.8 + 0.2*float32(math.Sin(float64(time.Now().UnixMilli())/180))
		}
		horizontal := a.dock.position.Edge == "top" || a.dock.position.Edge == "bottom"
		layout := ui.Column
		if horizontal {
			layout = ui.Row
		}
		mic := fg
		if voiceRecording.Load() != 0 {
			mic = accent
		}
		tab := layout(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(5).Radius(10).Clip().Draw(background).Border(1, border).Cursor(grab)
		if dockNativeDrag && interactive {
			tab.DragWindow()
		}
		tab.Children(func() {
			// 话筒：圆角竖条 + 托 + 支杆；录音时用主题强调色并呼吸
			ui.Box(c).Size(14, 16).Opacity(max(opacity, 0.85)).Draw(func(p *ui.Painter, r ui.Rect) {
				cx := r.X + r.W/2
				p.Stroke(ui.Rect{X: cx - 3, Y: r.Y + 1, W: 6, H: 9}, mic, 3, 1.4)
				cup := new(ui.Path)
				for i := 0; i <= 10; i++ {
					ang := math.Pi * float64(i) / 10
					x, y := cx+5*float32(math.Cos(ang)), r.Y+7+5*float32(math.Sin(ang))
					if i == 0 {
						cup.MoveTo(x, y)
					} else {
						cup.LineTo(x, y)
					}
				}
				p.StrokePath(cup, 1.4, mic)
				p.Line(cx, r.Y+12, cx, r.Y+15, 1.4, mic)
			})
			if cfg.VoiceTranslate {
				ui.Text(c, voiceLanguageCode(cfg.VoiceTarget)).FontSize(9).TextColor(accent)
			}
		})
		return
	}
	panel := ui.Column(c).Fill().Radius(radius).Clip().Draw(background).Border(1, border)
	feedback := func(control ui.Element) {
		control.Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
		pressed := float32(1)
		if control.Pressed() {
			pressed = 0.97
		}
		size := control.Animate("dock-press", pressed, 100*time.Millisecond)
		control.Scale(size, size)
	}
	content := func() {
		if !at.menubar { // 菜单栏面板不能拖，不画把手
			grip := ui.Box(c).FillWidth().Height(8).Shrink(0).Cursor(grab).Draw(func(p *ui.Painter, r ui.Rect) {
				p.Line(r.X+r.W/2-10, r.Y+r.H/2, r.X+r.W/2+10, r.Y+r.H/2, 2, border)
			})
			if dockNativeDrag && interactive {
				grip.DragWindow()
			}
		}
		button := func(label string, action func()) {
			control := ui.ButtonBase(c).Label(label).Height(30).FillWidth().Padding(0, 6).Radius(6).AlignItems(ui.Center).Justify(ui.Start).Cursor(pointer)
			if interactive && control.Hovered() {
				control.Background(active)
			}
			feedback(control)
			control.Children(func() { ui.Text(c, label).FontSize(11).TextColor(fg).SingleLine() })
			if interactive && control.Clicked() {
				action()
			}
		}
		toggle := func(label string, value bool, change func(*Config)) {
			control := ui.ButtonBase(c).Label(label).FillWidth().Height(30).Padding(0, 6).Radius(6).AlignItems(ui.Center).Cursor(pointer)
			if interactive && control.Hovered() {
				control.Background(active)
			}
			feedback(control)
			control.Children(func() {
				ui.Text(c, label).FontSize(11).TextColor(fg).Grow(1).SingleLine()
				color := muted
				if value {
					color = accent
				}
				ui.Box(c).Size(22, 12).Radius(6).Background(color).Padding(2).Transition(ui.ElementTransition{Duration: 140 * time.Millisecond, Colors: true}).Children(func() {
					dot := ui.Box(c).Size(8, 8).Radius(4).Background(ui.RGB(255, 255, 255))
					offset := float32(0)
					if value {
						offset = 10
					}
					dot.Translate(dot.Animate("dock-toggle", offset, 140*time.Millisecond), 0)
				})
			})
			if interactive && control.Clicked() {
				a.changeDockConfig(change)
			}
		}
		if *at.langs {
			// 语言列表：点一下就选上并回到主面板，不用一个个轮过去
			button("‹  "+T("翻译成"), func() { *at.langs = false })
			ui.Scroll(c).FillWidth().Grow(1).Children(func() {
				ui.Column(c).FillWidth().Background(card).Border(1, paneBorder(t)).Radius(12).Padding(4).Children(func() {
					for _, l := range targetLangs {
						value, picked := l.value, l.value == cfg.VoiceTarget
						item := ui.ButtonBase(c).Label(l.label).FillWidth().Height(28).Padding(0, 6).Radius(6).AlignItems(ui.Center).Cursor(pointer)
						if interactive && item.Hovered() || picked {
							item.Background(active)
						}
						feedback(item)
						item.Children(func() {
							ui.Text(c, langLabel(l.value, l.label)).FontSize(11).TextColor(fg).Grow(1).SingleLine()
							if picked {
								ui.Text(c, "✓").FontSize(11).TextColor(accent)
							}
						})
						if interactive && item.Clicked() {
							a.changeDockConfig(func(v *Config) { v.VoiceTarget = value })
							*at.langs = false
						}
					}
				})
			})
			return
		}
		if more {
			button("‹  "+T("更多设置"), func() { *at.more = false })
			ui.Column(c).FillWidth().Background(card).Border(1, paneBorder(t)).Radius(12).Padding(4).Children(func() {
				toggle(T("按程序自动换档"), cfg.PolishAuto, func(v *Config) { v.PolishAuto = !v.PolishAuto })
				toggle(T("实时文字"), cfg.VoiceLiveText, func(v *Config) { v.VoiceLiveText = !v.VoiceLiveText })
				toggle(T("自动学习"), cfg.VoiceLearn, func(v *Config) { v.VoiceLearn = !v.VoiceLearn })
				toggle(T("选中再说就改写"), cfg.VoiceRewrite, func(v *Config) { v.VoiceRewrite = !v.VoiceRewrite })
				toggle(T("提示音"), cfg.VoiceSound, func(v *Config) { v.VoiceSound = !v.VoiceSound })
				toggle(T("润色"), cfg.PolishStyle != "off", func(v *Config) {
					if v.PolishStyle == "off" {
						v.PolishStyle = "standard"
					} else {
						v.PolishStyle = "off"
					}
				})
			})
			// 点一下换下一个，右边显示当前选的名字（和设置页同一套叫法）
			cycle := func(label, value string, change func(*Config)) {
				control := ui.ButtonBase(c).Label(label).FillWidth().Height(30).Padding(0, 6).Radius(6).AlignItems(ui.Center).Cursor(pointer)
				if interactive && control.Hovered() {
					control.Background(active)
				}
				feedback(control)
				control.Children(func() {
					ui.Text(c, label).FontSize(11).TextColor(fg).Grow(1).SingleLine()
					ui.Text(c, value+"  ›").FontSize(11).TextColor(muted).SingleLine()
				})
				if interactive && control.Clicked() {
					a.changeDockConfig(change)
				}
			}
			ui.Column(c).FillWidth().Background(card).Border(1, paneBorder(t)).Radius(12).Padding(4).Children(func() {
				cycle(T("样式"), voiceStyleName(cfg.VoiceStyle), func(v *Config) {
					v.VoiceStyle = dockCycle(v.VoiceStyle, []string{"bars", "orb", "glass", "pixel", "mercury", "islands", "leds"})
				})
				cycle(T("面板底色"), voiceToneName(cfg.VoiceDockTone), func(v *Config) { v.VoiceDockTone = dockCycle(v.VoiceDockTone, []string{"auto", "light", "dark"}) })
			})
			button(T("完整设置…"), func() { a.openSettings(); done() })
			return
		}
		mic := ui.ButtonBase(c).FillWidth().Height(40).Padding(6).Gap(6).Radius(20).Background(card).Border(1, paneBorder(t)).AlignItems(ui.Center).Cursor(pointer)
		if interactive && mic.Hovered() {
			mic.Background(active)
		}
		feedback(mic)
		mic.Children(func() {
			ui.Box(c).Size(28, 28).Radius(14).Background(accent).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Box(c).Size(7, 7).Radius(4).Background(t.AccentText) })
			label := T("开始说话")
			if voiceRecording.Load() != 0 {
				label = T("结束")
			}
			ui.Text(c, label).FontSize(11).TextColor(fg).Grow(1).SingleLine()
			ui.Text(c, keysLabel(cfg.VoiceHotkey)).FontSize(9).TextColor(muted).SingleLine()
		})
		if interactive && mic.Clicked() {
			dockVoiceToggle()
			done()
		}
		ui.Column(c).FillWidth().Background(card).Border(1, paneBorder(t)).Radius(12).Padding(4).Children(func() {
			toggle(T("说完直接翻译"), cfg.VoiceTranslate, func(v *Config) { v.VoiceTranslate = !v.VoiceTranslate })
			ui.Box(c).FillWidth().Height(1).Background(border)
			targetName := cfg.VoiceTarget
			for _, target := range targetLangs {
				if target.value == cfg.VoiceTarget {
					targetName = langLabel(target.value, target.label)
				}
			}
			target := ui.ButtonBase(c).Label(T("翻译成")).FillWidth().Height(30).Padding(0, 6).Gap(6).AlignItems(ui.Center).Cursor(pointer)
			target.Radius(6)
			if interactive && target.Hovered() {
				target.Background(active)
			}
			feedback(target)
			target.Children(func() {
				ui.Text(c, T("翻译成")).FontSize(11).TextColor(fg).SingleLine()
				ui.Text(c, targetName).FontSize(10).TextColor(muted).SingleLine().Grow(1).Shrink(1)
				ui.Text(c, "›").FontSize(12).TextColor(muted)
			})
			if interactive && target.Clicked() {
				*at.langs = true
			}
		})
		ui.Row(c).FillWidth().Height(28).Padding(3).Gap(2).Background(card).Radius(10).Children(func() {
			styles := []string{"light", "standard", "structured", "prompt"}
			labels := []string{T("轻"), T("标准"), T("结构化"), T("提示词")}
			for index, style := range styles {
				segment := ui.ButtonBase(c).Grow(1).Height(22).Radius(6).AlignItems(ui.Center).Justify(ui.Center).Cursor(pointer)
				text := fg
				if cfg.PolishStyle == style {
					segment.Background(accent)
					text = t.AccentText
				} else if interactive && segment.Hovered() {
					segment.Background(active)
				}
				feedback(segment)
				segment.Children(func() { ui.Text(c, labels[index]).FontSize(10).TextColor(text).SingleLine() })
				if interactive && segment.Clicked() {
					a.changeDockConfig(func(v *Config) { v.PolishStyle = style })
				}
			}
		})
		button(T("更多设置")+"  ›", func() { *at.more = true })
	}
	if len(reveal) == 0 {
		panel.Padding(12).Gap(12).Children(content)
		return
	}
	panel.Children(func() {
		height := float32(248)
		if more || *at.langs {
			height = 416
		}
		width := min(float32(196), float32(a.dock.area.w())*96/float32(max(1, a.dock.dpi)))
		height = min(height, float32(a.dock.area.h())*96/float32(max(1, a.dock.dpi)))
		page := "main"
		if *at.langs {
			page = "languages"
		} else if more {
			page = "more"
		}
		direction := float32(8)
		if page == "main" {
			direction = -8
		}
		if a.dock.page != page {
			if a.dock.page != "" && a.dock.expanded {
				a.dock.pageAt = time.Now()
			}
			a.dock.page = page
		}
		progress := min(float64(time.Since(a.dock.pageAt))/(160*float64(time.Millisecond)), 1)
		fade := float32(1 - math.Pow(1-progress, 3))
		if progress < 1 {
			c.AnimationFrame()
		}
		body := ui.Column(c).Key(page).Size(width, height).Shrink(0).Padding(12).Gap(12).
			Opacity(reveal[0]*reveal[0]*fade).Translate(direction*(1-fade), 0)
		body.Disabled(!a.dock.motionAt.IsZero())
		body.Children(content)
	})
}

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
	texture                                         *texture
}

var dockQuit atomic.Bool

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
		ticker := time.NewTicker(40 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			d.win.Update(func() {
				if !a.quitting {
					a.tickDock()
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
		return
	}
	if d.armed || a.nativeDockDrag() {
		return
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
			d.hoverAt = time.Time{}
			if d.leaveAt.IsZero() {
				d.leaveAt = now
			}
			if now.Sub(d.leaveAt) >= 600*time.Millisecond {
				d.expanded = false
				d.more, d.langs = false, false
			}
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
		if native != d.rect {
			placeAt(d.hwnd, native.Left, native.Top, native.w(), native.h())
			d.rect = native
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
	a.dockContent(c, cfg, a.dock.expanded, a.dock.more, toneDark(cfg.VoiceDockTone, a.themed(c).Dark))
}

func (a *App) dockContent(c *ui.Context, cfg Config, expanded, more, dark bool) {
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
	background := func(p *ui.Painter, r ui.Rect) {
		radius := float32(10)
		if expanded {
			radius = 16
		}
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
		tab := layout(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(5).Radius(10).Clip().Draw(background).Border(1, border).Cursor(ui.CursorGrab)
		if dockNativeDrag {
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
	panel := ui.Column(c).Fill().Padding(12).Gap(12).Radius(16).Clip().Draw(background).Border(1, border)
	panel.Children(func() {
		grip := ui.Box(c).FillWidth().Height(8).Shrink(0).Cursor(ui.CursorGrab).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Line(r.X+r.W/2-10, r.Y+r.H/2, r.X+r.W/2+10, r.Y+r.H/2, 2, border)
		})
		if dockNativeDrag {
			grip.DragWindow()
		}
		button := func(label string, action func()) {
			control := ui.ButtonBase(c).Label(label).Height(30).FillWidth().Padding(0, 6).Radius(6).AlignItems(ui.Center).Justify(ui.Start).Cursor(ui.CursorPointer)
			if control.Hovered() {
				control.Background(active)
			}
			control.Children(func() { ui.Text(c, label).FontSize(11).TextColor(fg).SingleLine() })
			if control.Clicked() {
				action()
			}
		}
		toggle := func(label string, value bool, change func(*Config)) {
			control := ui.ButtonBase(c).Label(label).FillWidth().Height(30).Padding(0, 6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			control.Children(func() {
				ui.Text(c, label).FontSize(11).TextColor(fg).Grow(1).SingleLine()
				color := muted
				if value {
					color = accent
				}
				ui.Box(c).Size(22, 12).Radius(6).Background(color).Padding(2).Children(func() {
					dot := ui.Box(c).Size(8, 8).Radius(4).Background(ui.RGB(255, 255, 255))
					if value {
						dot.Margin(0, 0, 0, 10)
					}
				})
			})
			if control.Clicked() {
				a.changeDockConfig(change)
			}
		}
		if a.dock.langs {
			// 语言列表：点一下就选上并回到主面板，不用一个个轮过去
			button("‹  "+T("翻译成"), func() { a.dock.langs = false })
			ui.Scroll(c).FillWidth().Grow(1).Children(func() {
				ui.Column(c).FillWidth().Background(card).Border(1, paneBorder(t)).Radius(12).Padding(4).Children(func() {
					for _, l := range targetLangs {
						value, picked := l.value, l.value == cfg.VoiceTarget
						item := ui.ButtonBase(c).Label(l.label).FillWidth().Height(28).Padding(0, 6).Radius(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
						if item.Hovered() || picked {
							item.Background(active)
						}
						item.Children(func() {
							ui.Text(c, langLabel(l.value, l.label)).FontSize(11).TextColor(fg).Grow(1).SingleLine()
							if picked {
								ui.Text(c, "✓").FontSize(11).TextColor(accent)
							}
						})
						if item.Clicked() {
							a.changeDockConfig(func(v *Config) { v.VoiceTarget = value })
							a.dock.langs = false
						}
					}
				})
			})
			return
		}
		if more {
			button("‹  "+T("更多设置"), func() { a.dock.more = false })
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
				control := ui.ButtonBase(c).Label(label).FillWidth().Height(30).Padding(0, 6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
				control.Children(func() {
					ui.Text(c, label).FontSize(11).TextColor(fg).Grow(1).SingleLine()
					ui.Text(c, value+"  ›").FontSize(11).TextColor(muted).SingleLine()
				})
				if control.Clicked() {
					a.changeDockConfig(change)
				}
			}
			ui.Column(c).FillWidth().Background(card).Border(1, paneBorder(t)).Radius(12).Padding(4).Children(func() {
				cycle(T("样式"), voiceStyleName(cfg.VoiceStyle), func(v *Config) {
					v.VoiceStyle = dockCycle(v.VoiceStyle, []string{"bars", "orb", "glass", "pixel", "mercury", "islands", "leds"})
				})
				cycle(T("面板底色"), voiceToneName(cfg.VoiceDockTone), func(v *Config) { v.VoiceDockTone = dockCycle(v.VoiceDockTone, []string{"auto", "light", "dark"}) })
			})
			button(T("完整设置…"), a.openSettings)
			return
		}
		mic := ui.ButtonBase(c).FillWidth().Height(40).Padding(6).Gap(6).Radius(20).Background(card).Border(1, paneBorder(t)).AlignItems(ui.Center).Cursor(ui.CursorPointer)
		if mic.Hovered() {
			mic.Background(active)
		}
		mic.Children(func() {
			ui.Box(c).Size(28, 28).Radius(14).Background(accent).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Box(c).Size(7, 7).Radius(4).Background(t.AccentText) })
			label := T("开始说话")
			if voiceRecording.Load() != 0 {
				label = T("结束")
			}
			ui.Text(c, label).FontSize(11).TextColor(fg).Grow(1).SingleLine()
			ui.Text(c, cfg.VoiceHotkey).FontSize(9).TextColor(muted).SingleLine()
		})
		if mic.Clicked() {
			go voiceToggleSource(true)
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
			target := ui.ButtonBase(c).Label(T("翻译成")).FillWidth().Height(30).Padding(0, 6).Gap(6).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			target.Children(func() {
				ui.Text(c, T("翻译成")).FontSize(11).TextColor(fg).SingleLine()
				ui.Text(c, targetName).FontSize(10).TextColor(muted).SingleLine().Grow(1).Shrink(1)
				ui.Text(c, "›").FontSize(12).TextColor(muted)
			})
			if target.Clicked() {
				a.dock.langs = true
			}
		})
		ui.Row(c).FillWidth().Height(28).Padding(3).Gap(2).Background(card).Radius(10).Children(func() {
			styles := []string{"light", "standard", "structured", "prompt"}
			labels := []string{T("轻"), T("标准"), T("结构化"), T("提示词")}
			for index, style := range styles {
				segment := ui.ButtonBase(c).Grow(1).Height(22).Radius(6).AlignItems(ui.Center).Justify(ui.Center).Cursor(ui.CursorPointer)
				text := fg
				if cfg.PolishStyle == style {
					segment.Background(accent)
					text = t.AccentText
				} else if segment.Hovered() {
					segment.Background(active)
				}
				segment.Children(func() { ui.Text(c, labels[index]).FontSize(10).TextColor(text).SingleLine() })
				if segment.Clicked() {
					a.changeDockConfig(func(v *Config) { v.PolishStyle = style })
				}
			}
		})
		button(T("更多设置")+"  ›", func() { a.dock.more = true })
	})
}

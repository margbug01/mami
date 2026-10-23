//go:build windows

package app

import (
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

func TestVoiceDockPreview(t *testing.T) {
	path := os.Getenv("MAMI_VOICE_DOCK_SNAP")
	if path == "" {
		t.Skip("MAMI_VOICE_DOCK_SNAP not set")
	}
	a := &App{}
	a.dock.position = dockPosition{Edge: "right", Fraction: 0.5}
	cfg := defaultConfig()
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Gap(20).Background(ui.RGB(85, 87, 93)).Children(func() {
			for _, dark := range []bool{true, false} {
				if dark {
					cfg.VoiceTone = "dark"
				} else {
					cfg.VoiceTone = "light"
				}
				ui.Row(c).Gap(20).AlignItems(ui.Start).Children(func() {
					for _, translate := range []bool{false, true} {
						ui.Box(c).Size(dockThick, dockLong).Shrink(0).Children(func() { cfg.VoiceTranslate = translate; a.dockContent(c, cfg, false, false, dark) })
					}
					cfg.VoiceTranslate = true
					ui.Box(c).Size(196, 248).Children(func() { a.dockContent(c, cfg, true, false, dark) })
					ui.Box(c).Size(196, 416).Children(func() { a.dockContent(c, cfg, true, true, dark) })
				})
			}
		})
	}, 560, 892)
	tester.Frame()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tester.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestDockMatchesAppTheme(t *testing.T) {
	for _, mode := range []string{"light", "dark"} {
		for _, edge := range []string{"left", "right", "top", "bottom"} {
			for _, expanded := range []bool{false, true} {
				cfg := defaultConfig()
				cfg.Theme = mode
				cfg.VoiceTone = "dark"
				if mode == "dark" {
					cfg.VoiceTone = "light"
				}
				a := &App{cfg: cfg}
				a.dock.position.Edge, a.dock.expanded = edge, expanded
				rect := dockPlacement(dockRect{W: 800, H: 600}, a.dock.position, expanded, 248)
				tester := ui.NewTester(a.dockView, rect.W, rect.H)
				tester.Frame()
				img := tester.Image()
				for _, corner := range [][2]int{{0, 0}, {rect.W - 1, 0}, {0, rect.H - 1}, {rect.W - 1, rect.H - 1}} {
					_, _, _, alpha := img.At(corner[0], corner[1]).RGBA()
					if alpha != 0 {
						t.Fatalf("opaque corner: theme=%s edge=%s expanded=%v corner=%v alpha=%d", mode, edge, expanded, corner, alpha)
					}
				}
				want := windowTexture(mode == "dark")
				got := a.dock.texture
				if got == nil || got.pal != want.pal || got.grain != want.grain || got.bmp == nil {
					t.Fatalf("dock material differs from app: theme=%s edge=%s expanded=%v", mode, edge, expanded)
				}
				if got == want {
					t.Fatal("dock must not resize the main window texture cache")
				}
			}
		}
	}
}

func TestDockIndependentTone(t *testing.T) {
	for _, tone := range []string{"auto", "light", "dark", "", "invalid"} {
		for _, theme := range []string{"light", "dark"} {
			cfg := defaultConfig()
			cfg.Theme, cfg.VoiceDockTone = theme, tone
			cfg.normalize()
			wantTone := tone
			if tone == "" || tone == "invalid" {
				wantTone = "auto"
			}
			if cfg.VoiceDockTone != wantTone {
				t.Fatalf("normalized tone %q: %q", tone, cfg.VoiceDockTone)
			}
			a := &App{cfg: cfg}
			a.dock.expanded = true
			tester := ui.NewTester(a.dockView, 196, 248)
			tester.Frame()
			wantDark := wantTone == "dark" || wantTone == "auto" && theme == "dark"
			if a.dock.texture.pal != windowTexture(wantDark).pal {
				t.Fatalf("independent tone=%s theme=%s", tone, theme)
			}
			before, draft := defaultConfig(), defaultConfig()
			draft.Theme = "light"
			syncDockDraft(&draft, before, cfg)
			if draft.VoiceDockTone != cfg.VoiceDockTone || draft.Theme != "light" || draft.VoiceTone != before.VoiceTone {
				t.Fatal("dock tone sync affected unrelated theme settings")
			}
		}
	}
}

func TestDockVoiceRouting(t *testing.T) {
	previousEvents := voiceEvents
	previousEpoch, previousActive, previousRecording := voiceEpoch.Load(), voiceActive.Load(), voiceRecording.Load()
	voiceEvents = make(chan voiceEvent, 4)
	voiceActive.Store(0)
	voiceRecording.Store(0)
	t.Cleanup(func() {
		voiceEvents = previousEvents
		voiceEpoch.Store(previousEpoch)
		voiceActive.Store(previousActive)
		voiceRecording.Store(previousRecording)
	})
	voiceToggleSource(true)
	start := <-voiceEvents
	if start.action != voiceStart || !start.manual || start.id == 0 || voiceRecording.Load() != start.id {
		t.Fatal("dock start did not use manual voice flow")
	}
	voiceToggleSource(true)
	stop := <-voiceEvents
	if stop.action != voiceStop || stop.id != start.id || voiceRecording.Load() != 0 {
		t.Fatal("dock stop did not reuse voiceClick")
	}
}

func TestDockNonActivation(t *testing.T) {
	result, _, _ := syscall.SyscallN(dockSubclass, 0, 0x21, 0, 0, 1, 0)
	if result != 3 {
		t.Fatalf("WM_MOUSEACTIVATE = %d, want MA_NOACTIVATE", result)
	}
}

func TestDockControls(t *testing.T) {
	a := &App{cfg: defaultConfig(), cfgPath: filepath.Join(t.TempDir(), "config.json")}
	a.dock.position.Edge = "right"
	tester := ui.NewTester(func(c *ui.Context) {
		cfg, _ := a.config()
		a.dockContent(c, cfg, true, a.dock.more, true)
	}, 196, 416)
	tester.Frame()
	click := func(label string) {
		t.Helper()
		if err := tester.Click(label); err != nil {
			t.Fatal(err)
		}
		tester.Frame()
	}
	click(T("说完直接翻译"))
	cfg, _ := a.config()
	if !cfg.VoiceTranslate {
		t.Fatal("translation toggle did not apply")
	}
	click(T("翻译成"))
	if !a.dock.langs {
		t.Fatal("language list did not open")
	}
	click("日语 日本語") // 直接点中想要的，一次到位
	cfg, _ = a.config()
	if cfg.VoiceTarget != "Japanese" || a.dock.langs {
		t.Fatalf("pick language: target=%s list open=%v", cfg.VoiceTarget, a.dock.langs)
	}
	click(T("结构化"))
	cfg, _ = a.config()
	if cfg.PolishStyle != "structured" {
		t.Fatal("polish segment did not apply")
	}
	click(T("更多设置") + "  ›")
	if !a.dock.more {
		t.Fatal("more sheet did not open")
	}
	for _, label := range []string{T("按程序自动换档"), T("实时文字"), T("自动学习"), T("选中再说就改写"), T("提示音"), T("润色")} {
		click(label)
	}
	cfg, _ = a.config()
	if cfg.PolishAuto || !cfg.VoiceLiveText || cfg.VoiceLearn || cfg.VoiceRewrite || cfg.VoiceSound || cfg.PolishStyle != "off" {
		t.Fatalf("more toggles not applied: %+v", cfg)
	}
	click(T("样式"))
	click(T("面板底色"))
	cfg, _ = a.config()
	if cfg.VoiceStyle != "orb" || cfg.VoiceDockTone != "light" || cfg.VoiceTone != defaultConfig().VoiceTone {
		t.Fatal("skin/tone did not cycle")
	}
	stored, _, err := loadConfig(a.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if stored.VoiceTarget != cfg.VoiceTarget || stored.VoiceDockTone != "light" || !stored.VoiceTranslate || stored.PolishStyle != "off" {
		t.Fatal("controls were not persisted")
	}
	click("‹  " + T("更多设置"))
	if a.dock.more {
		t.Fatal("back did not close more sheet")
	}
}

func TestDockNativeChildRouting(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, _ := syscall.UTF16PtrFromString("STATIC")
	point := cursorPos()
	parent, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, 0x80000000, uintptr(point.X-50), uintptr(point.Y-20), 196, 248, 0, 0, 0, 0)
	if parent == 0 {
		t.Fatal(err)
	}
	defer user32.NewProc("DestroyWindow").Call(parent)
	child, _, err := user32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, 0x40000000, 0, 0, 196, 248, parent, 0, 0, 0)
	if child == 0 {
		t.Fatal(err)
	}
	a := &App{}
	d := &a.dock
	d.hwnd, d.shown, d.expanded, d.dpi = parent, true, true, 96
	d.rect = windowRect(parent)
	a.hookDockMouse()
	defer func() {
		d.armed = false
		user32.NewProc("ReleaseCapture").Call()
	}()
	if dockOwners[child] != a {
		t.Fatal("content child does not receive dock mouse hook")
	}
	user32.NewProc("SendMessageW").Call(child, 0x201, 1, 0)
	if !d.armed {
		t.Fatal("mouse down on content child did not arm drag")
	}
	if capture, _, _ := user32.NewProc("GetCapture").Call(); capture != parent {
		t.Fatal("drag did not capture mouse to dock")
	}
	before := d.rect
	d.down.X -= 32
	d.grab.X -= 32
	user32.NewProc("SendMessageW").Call(parent, 0x200, 1, 0)
	if !d.dragging || d.rect.Left != before.Left+32 || d.rect.Top != before.Top {
		t.Fatalf("native mouse move did not move dock: before=%+v after=%+v", before, d.rect)
	}
}

func TestDockDragGeometry(t *testing.T) {
	for _, dpi := range []uint32{96, 144, 192} {
		for _, edge := range []string{"left", "right", "top", "bottom"} {
			d := voiceDock{dpi: dpi, expanded: true, position: dockPosition{Edge: edge}, rect: RECT{Left: -1200, Top: -300, Right: -800, Bottom: 300}}
			header := POINT{X: -1100, Y: -300 + scale(18, dpi)}
			button := POINT{X: -1100, Y: -300 + scale(50, dpi)}
			if !d.dragHit(header) || d.dragHit(button) || d.dragHit(POINT{X: -1201, Y: header.Y}) {
				t.Fatalf("drag hit regions: dpi=%d edge=%s", dpi, edge)
			}
			d.expanded = false
			if !d.dragHit(button) {
				t.Fatal("collapsed dock must be draggable everywhere")
			}
			d.down = header
			d.grab = POINT{X: header.X - d.rect.Left, Y: header.Y - d.rect.Top}
			if got := d.dragPoint(header); got != (POINT{X: d.rect.Left, Y: d.rect.Top}) {
				t.Fatalf("initial grab jumps: %+v", got)
			}
			moved := POINT{X: header.X + 137, Y: header.Y - 81}
			if got := d.dragPoint(moved); got != (POINT{X: d.rect.Left + 137, Y: d.rect.Top - 81}) {
				t.Fatalf("pointer offset not preserved: %+v", got)
			}
			if d.dragMoved(POINT{X: header.X + scale(4, dpi), Y: header.Y}) || !d.dragMoved(moved) {
				t.Fatalf("drag threshold: dpi=%d", dpi)
			}
		}
	}
}

func TestDockCycle(t *testing.T) {
	values := []string{"dark", "light", "auto"}
	for _, test := range []struct{ value, want string }{{"dark", "light"}, {"auto", "dark"}, {"invalid", "dark"}} {
		if got := dockCycle(test.value, values); got != test.want {
			t.Fatalf("cycle %s: %s", test.value, got)
		}
	}
	cfg := defaultConfig()
	if !cfg.VoiceDock || cfg.VoiceDockPosition.Edge != "right" {
		t.Fatal("dock default")
	}
}

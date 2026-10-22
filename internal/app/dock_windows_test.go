//go:build windows

package app

import (
	"image/png"
	"os"
	"path/filepath"
	"syscall"
	"testing"

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
					ui.Box(c).Size(196, 228).Children(func() { a.dockContent(c, cfg, true, false, dark) })
					ui.Box(c).Size(196, 396).Children(func() { a.dockContent(c, cfg, true, true, dark) })
				})
			}
		})
	}, 560, 852)
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
	}, 196, 396)
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
	click(T("浮窗底色"))
	cfg, _ = a.config()
	if cfg.VoiceStyle != "orb" || cfg.VoiceTone != "light" {
		t.Fatal("skin/tone did not cycle")
	}
	stored, _, err := loadConfig(a.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if stored.VoiceTarget != cfg.VoiceTarget || stored.VoiceTone != "light" || !stored.VoiceTranslate || stored.PolishStyle != "off" {
		t.Fatal("controls were not persisted")
	}
	click("‹  " + T("更多设置"))
	if a.dock.more {
		t.Fatal("back did not close more sheet")
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

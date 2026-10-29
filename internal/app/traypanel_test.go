package app

import (
	"bytes"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestPanelRect(t *testing.T) {
	area := RECT{Left: 0, Top: 25, Right: 1920, Bottom: 1080}
	for _, c := range []struct {
		name string
		icon RECT
		left int32
		top  int32
	}{
		{"centered under the icon", RECT{900, 0, 924, 24}, 912 - panelW/2, 25 + panelGap},
		{"kept inside on the right", RECT{1890, 0, 1914, 24}, 1920 - panelGap - panelW, 25 + panelGap},
		{"kept inside on the left", RECT{4, 0, 28, 24}, panelGap, 25 + panelGap},
		{"below a taller icon", RECT{900, 0, 924, 37}, 912 - panelW/2, 37 + panelGap},
		{"no icon yet", RECT{}, 1920 - panelGap - panelW, 25 + panelGap},
	} {
		r := panelRect(c.icon, area, panelW, panelMainH)
		if r.Left != c.left || r.Top != c.top || r.w() != panelW || r.h() != panelMainH {
			t.Errorf("%s: %+v, want left %d top %d", c.name, r, c.left, c.top)
		}
	}
}

func newPanelApp(t *testing.T) *App {
	t.Helper()
	a := &App{avatar: loadAvatar(), cfgPath: filepath.Join(t.TempDir(), "config.json")}
	if err := a.setConfig(defaultConfig()); err != nil {
		t.Fatal(err)
	}
	a.panel.h = panelMainH
	return a
}

// 菜单栏面板翻页：页记在面板自己身上（贴边小条那份不动），高度跟着页变，选完语言回到首页
func TestTrayPanelPages(t *testing.T) {
	a := newPanelApp(t)
	tt := ui.NewTester(a.trayPanelView, panelW, panelMoreH)
	tt.Frame()
	for _, s := range []string{"开始说话", "说完直接翻译", "翻译成", "标准"} {
		if _, ok := tt.Find(s); !ok {
			t.Fatalf("%q not in the panel", s)
		}
	}
	if err := tt.Click("更多设置  ›"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.panel.more || a.dock.more || a.panel.h != panelMoreH {
		t.Fatalf("more: panel %v dock %v height %d", a.panel.more, a.dock.more, a.panel.h)
	}
	if _, ok := tt.Find("提示音"); !ok {
		t.Fatal("more page not drawn")
	}
	if err := tt.Click("‹  更多设置"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.panel.more || a.panel.h != panelMainH {
		t.Fatalf("back: more %v height %d", a.panel.more, a.panel.h)
	}
	if err := tt.Click("翻译成"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.panel.langs || a.dock.langs {
		t.Fatalf("languages: panel %v dock %v", a.panel.langs, a.dock.langs)
	}
	japanese := ""
	for _, l := range targetLangs {
		if l.value == "Japanese" {
			japanese = langLabel(l.value, l.label)
		}
	}
	if err := tt.Click(japanese); err != nil {
		t.Fatalf("%q: %v", japanese, err)
	}
	tt.Frame()
	if cfg, _ := a.config(); cfg.VoiceTarget != "Japanese" || a.panel.langs {
		t.Fatalf("pick: target %s list open %v", cfg.VoiceTarget, a.panel.langs)
	}
}

// 在头像上按下不收（松开时头像自己切换），点别处收起，下次从首页开始
func TestTrayPanelHidesOnClickOutside(t *testing.T) {
	a := newPanelApp(t)
	p := &a.panel
	icon := RECT{100, 0, 124, 24}
	p.icon.Store(&icon)
	p.shown.Store(true)
	p.more = true
	a.trayPanelClick(POINT{X: 110, Y: 10})
	if !p.shown.Load() {
		t.Fatal("a press on the icon closed the panel")
	}
	a.trayPanelClick(POINT{X: 600, Y: 400})
	if p.shown.Load() || p.more {
		t.Fatalf("outside click: shown %v more %v", p.shown.Load(), p.more)
	}
}

// 点「开始说话」：开始录，面板收起（字照样贴进原来的输入框）
func TestTrayPanelStartTalkingCloses(t *testing.T) {
	a := newPanelApp(t)
	defer func(f func()) { dockVoiceToggle = f }(dockVoiceToggle)
	toggled := 0
	dockVoiceToggle = func() { toggled++ }
	a.panel.shown.Store(true)
	tt := ui.NewTester(a.trayPanelView, panelW, panelMainH)
	tt.Frame()
	if err := tt.Click("开始说话"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if toggled != 1 || a.panel.shown.Load() {
		t.Fatalf("toggled %d, still shown %v", toggled, a.panel.shown.Load())
	}
}

// 录音的头像：右下角红点带白边，别处和原图一样
func TestRecordingIconHasARedDot(t *testing.T) {
	src, err := png.Decode(bytes.NewReader(avatarPNG))
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(withRedDot(avatarPNG)))
	if err != nil {
		t.Fatal(err)
	}
	b := src.Bounds()
	if got.Bounds() != b {
		t.Fatalf("size %v, want %v", got.Bounds(), b)
	}
	ring := float64(b.Dx()) * redDotRing
	cx, cy := int(float64(b.Max.X)-ring-0.5), int(float64(b.Max.Y)-ring-0.5)
	if r, g, bl, a := got.At(cx, cy).RGBA(); r>>8 < 230 || g>>8 > 80 || bl>>8 > 80 || a>>8 != 255 {
		t.Errorf("dot center = %d %d %d %d, want red", r>>8, g>>8, bl>>8, a>>8)
	}
	edge := cx + int(float64(b.Dx())*redDot) + 2 // 红点外、白边上
	if r, g, bl, _ := got.At(edge, cy).RGBA(); r>>8 < 230 || g>>8 < 230 || bl>>8 < 230 {
		t.Errorf("ring = %d %d %d, want white", r>>8, g>>8, bl>>8)
	}
	for _, p := range [][2]int{{b.Dx() / 2, b.Dy() / 2}, {b.Dx() / 4, b.Dy() / 3}, {5, b.Dy() - 5}} {
		if got.At(p[0], p[1]) != src.At(p[0], p[1]) {
			r1, g1, b1, a1 := got.At(p[0], p[1]).RGBA()
			r0, g0, b0, a0 := src.At(p[0], p[1]).RGBA()
			if r1 != r0 || g1 != g0 || b1 != b0 || a1 != a0 {
				t.Errorf("pixel %v changed", p)
			}
		}
	}
}

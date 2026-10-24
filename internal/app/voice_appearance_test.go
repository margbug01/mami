//go:build windows

package app

import (
	"image/png"
	"os"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"
	"mami/internal/i18n"
)

func TestVoiceAppearance(t *testing.T) {
	i18n.Set(i18n.ZH)
	cfg := defaultConfig()
	cfg.VoiceStyle, cfg.VoiceFlowPalette, cfg.Theme = "flow", "aurora", "light"
	if os.Getenv("MAMI_APPEARANCE_THEME") == "dark" {
		cfg.Theme = "dark"
	}
	cfg.VoiceDock, cfg.VoiceDockTone, cfg.VoiceTone = true, "dark", "dark"
	app := &App{}
	app.set.voiceTab = 1
	tester := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, cfg.Theme)
		ui.Column(c).Fill().Padding(28).Draw(func(p *ui.Painter, r ui.Rect) { windowTexture(c.Theme().Dark).draw(p, r, 0) }).Children(func() { app.pageVoice(c, &cfg) })
	}, 620, 1020)
	if !slices.Contains(tester.Texts(), "展开面板外观") || !slices.Contains(tester.Texts(), "结果条外观") {
		t.Fatal(tester.Texts())
	}
	if err := tester.Click("落日"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if cfg.VoiceFlowPalette != "sunset" {
		t.Fatal("palette selection failed")
	}
	if cfg.VoiceDockTone != "dark" || cfg.VoiceTone != "dark" {
		t.Fatal("palette changed tone")
	}
	cfg.VoiceDock = false
	tester.Frame()
	if slices.Contains(tester.Texts(), "展开面板外观") {
		t.Fatal("disabled dock still has tone controls")
	}
	if !slices.Contains(tester.Texts(), "结果条外观") {
		t.Fatal("dock toggle hid result controls")
	}
	cfg.VoiceDock = true
	cfg.VoiceFlowPalette = "aurora"
	tester.Frame()
	if path := os.Getenv("MAMI_APPEARANCE_SNAP"); path != "" {
		output, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		if err := png.Encode(output, tester.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVoiceToneSelector(t *testing.T) {
	i18n.Set(i18n.ZH)
	tone := "auto"
	tester := ui.NewTester(func(c *ui.Context) { voiceToneSelector(c, &tone) }, 300, 80)
	for _, choice := range []struct{ label, value string }{{"深色", "dark"}, {"浅色", "light"}, {"跟随主题", "auto"}} {
		if err := tester.Click(choice.label); err != nil {
			t.Fatal(err)
		}
		tester.Frame()
		if tone != choice.value {
			t.Fatalf("%s: got %s", choice.label, tone)
		}
	}
}

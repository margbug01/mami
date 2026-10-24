//go:build windows

package app

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
	"mami/internal/i18n"
)

func TestDockAppearanceImmediatePreview(t *testing.T) {
	i18n.Set(i18n.ZH)
	cfg := defaultConfig()
	cfg.VoiceDockTone = "dark"
	tester := ui.NewTester(func(c *ui.Context) {
		applyTheme(c, "light")
		ui.Column(c).Fill().Padding(20).Background(ui.RGBA(243, 245, 248, 1)).Children(func() {
			ui.Text(c, T("展开面板外观")).FontSize(16).Padding(0, 0, 10, 0)
			ui.Box(c).Width(240).Children(func() { voiceToneSelector(c, &cfg.VoiceDockTone) })
			ui.Box(c).Height(12)
			dockAppearancePreview(c, cfg)
		})
	}, 460, 410)
	capture := func() *image.RGBA {
		src := tester.Image()
		out := image.NewRGBA(src.Bounds())
		draw.Draw(out, out.Bounds(), src, src.Bounds().Min, draw.Src)
		return out
	}
	dark := capture()
	if err := tester.Click("浅色"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if cfg.VoiceDockTone != "light" {
		t.Fatal("tone not changed")
	}
	light := capture()
	changed := 0
	for row := 100; row < 330; row++ {
		for col := 130; col < 360; col++ {
			before := dark.RGBAAt(col, row)
			after := light.RGBAAt(col, row)
			if int(after.R) > int(before.R)+40 {
				changed++
			}
		}
	}
	if changed < 15000 {
		t.Fatalf("preview did not visibly switch without saving: %d pixels", changed)
	}
	if err := tester.Click("跟随主题"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if cfg.VoiceDockTone != "auto" {
		t.Fatal("auto tone not selected")
	}
	if err := tester.Click(T("更多设置") + "  ›"); err != nil {
		t.Fatal(err)
	}
	tester.Frame()
	if dockPreviewRenderer.dock.more {
		t.Fatal("disabled preview handled a click")
	}
	if path := os.Getenv("MAMI_DOCK_APPEARANCE_SNAP"); path != "" {
		combined := image.NewRGBA(image.Rect(0, 0, 920, 410))
		draw.Draw(combined, image.Rect(0, 0, 460, 410), light, image.Point{}, draw.Src)
		draw.Draw(combined, image.Rect(460, 0, 920, 410), dark, image.Point{}, draw.Src)
		output, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		if err := png.Encode(output, combined); err != nil {
			t.Fatal(err)
		}
	}
}

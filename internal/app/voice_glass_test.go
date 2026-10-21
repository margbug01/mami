package app

import (
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestGlassRemainsTranslucent(t *testing.T) {
	for _, dark := range []bool{true, false} {
		pixels := make([]uint32, 0, 2)
		for _, backdrop := range []uint8{0, 255} {
			tt := ui.NewTester(func(c *ui.Context) {
				ui.Box(c).Size(160, 36).Draw(func(p *ui.Painter, r ui.Rect) {
					p.Fill(r, ui.RGBA(backdrop, backdrop, backdrop, 1), 0)
					drawGlass(p, r, dark, 0, false)
				})
			}, 160, 36)
			red, _, _, _ := tt.Image().At(80, 24).RGBA()
			pixels = append(pixels, red)
		}
		if pixels[1] <= pixels[0]+1000 {
			t.Fatalf("glass must reveal backdrop, dark=%v pixels=%v", dark, pixels)
		}
	}
	if path := os.Getenv("MAMI_GLASS_SNAP"); path != "" {
		cfg := defaultConfig()
		cfg.Language, cfg.Theme, cfg.VoiceStyle, cfg.VoiceTone = "zh", "light", "glass", "dark"
		a := &App{avatar: loadAvatar()}
		a.set = settingsState{draft: cfg, page: pageVoice, voiceTab: 1}
		tt := ui.NewTester(a.settingsView, settingsW, 800)
		tt.Frame()
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
}

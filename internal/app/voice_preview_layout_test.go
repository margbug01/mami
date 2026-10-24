//go:build windows

package app

import (
	"fmt"
	"image/png"
	"os"
	"testing"

	"github.com/egoist/mygo/ui"
	"mami/internal/i18n"
)

func TestVoicePreviewResponsive(t *testing.T) {
	i18n.Set(i18n.ZH)
	for _, width := range []int{400, 440, 620} {
		for _, style := range []string{"bars", "orb", "flow", "glass", "pixel", "mercury", "islands", "leds"} {
			t.Run(fmt.Sprintf("%s-%d", style, width), func(t *testing.T) {
				cfg := defaultConfig()
				cfg.VoiceStyle, cfg.VoiceTone, cfg.VoiceDockTone, cfg.Theme = style, "dark", "auto", "light"
				cfg.VoiceDock = true
				app := &App{}
				app.set.voiceTab = 1
				tester := ui.NewTester(func(c *ui.Context) {
					applyTheme(c, "light")
					ui.Column(c).Fill().Padding(28).Draw(func(p *ui.Painter, r ui.Rect) { windowTexture(false).draw(p, r, 0) }).Children(func() { app.pageVoice(c, &cfg) })
				}, width, 1140)
				tester.Frame()
				bounds := make([]ui.Rect, 0, 3)
				for _, label := range []string{"录音中", "识别中…", "结果待复制"} {
					rect, found := tester.Find(label)
					if !found {
						t.Fatal("missing preview", label)
					}
					if rect.X < 56 || rect.X+rect.W > float32(width-56)+1 {
						t.Fatalf("%s overflows preview: %+v width=%d", label, rect, width)
					}
					if rect.Y+rect.H > 1140 {
						t.Fatal("preview clipped vertically", rect)
					}
					bounds = append(bounds, rect)
				}
				for first := 0; first < len(bounds); first++ {
					for second := first + 1; second < len(bounds); second++ {
						left, right := bounds[first], bounds[second]
						if left.X < right.X+right.W && right.X < left.X+left.W && left.Y < right.Y+right.H && right.Y < left.Y+left.H {
							t.Fatal("preview states overlap", left, right)
						}
					}
				}
				if style == "mercury" && width == 440 {
					if bounds[2].Y <= bounds[0].Y {
						t.Fatal("result must wrap on narrow window")
					}
					if path := os.Getenv("MAMI_RESPONSIVE_SNAP"); path != "" {
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
			})
		}
	}
}

//go:build windows

package app

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
)

func render(t *testing.T, dark bool, view func(c *ui.Context)) []byte {
	t.Helper()
	tt := ui.NewTester(view, 360, 120)
	tt.SetDark(dark)
	tt.Frame()
	tt.Frame()
	var b bytes.Buffer
	if err := png.Encode(&b, tt.Image()); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// 内置字体真的被登记了：用系统里没有的名字登记，画出来要和「找不到字体」的不一样，
// 同一个名字下 Regular 和 Bold 也要画得不一样
func TestEmbeddedMiSansRegistered(t *testing.T) {
	for _, f := range [][]byte{miSansRegular, miSansBold} {
		if err := ui.RegisterFont(f, "MiSansEmbeddedTest"); err != nil {
			t.Fatalf("register: %v", err)
		}
	}
	draw := func(family string, bold bool) []byte {
		return render(t, true, func(c *ui.Context) {
			ui.Column(c).Fill().Padding(10).Children(func() {
				e := ui.Text(c, "Mami 划词翻译 Translate").FontSize(24).Font(family)
				if bold {
					e.Bold()
				}
			})
		})
	}
	reg, bold, none := draw("MiSansEmbeddedTest", false), draw("MiSansEmbeddedTest", true), draw("NoSuchFontAnywhere", false)
	if bytes.Equal(reg, none) {
		t.Error("内置 MiSans 没生效：和找不到字体时画得一样")
	}
	if bytes.Equal(reg, bold) {
		t.Error("Regular 和 Bold 画得一样，粗体没生效")
	}
}

// 跟随系统时，主题还得跟着系统的深浅走（只换字体，不能把主题钉死）
func TestAutoThemeStillFollowsSystem(t *testing.T) {
	view := func(c *ui.Context) {
		applyTheme(c, "auto")
		ui.Box(c).Fill().Background(c.Theme().Background)
	}
	light, dark := render(t, false, view), render(t, true, view)
	if bytes.Equal(light, dark) {
		t.Error("跟随系统：浅色和深色画出来一样")
	}
	// 同一个 Tester 里中途切深浅
	tt := ui.NewTester(view, 100, 60)
	tt.SetDark(false)
	tt.Frame()
	tt.Frame()
	before := tt.Image().At(50, 30)
	tt.SetDark(true)
	tt.Frame()
	tt.Frame()
	after := tt.Image().At(50, 30)
	if before == after {
		t.Error("跟随系统：中途切深色，画面没变")
	}
}

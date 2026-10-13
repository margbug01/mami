//go:build windows

package main

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func pickLang(t *testing.T, start, choose string) string {
	t.Helper()
	v := start
	tt := ui.NewTester(func(c *ui.Context) { langSelect(c, &v) }, 400, 400)
	cur := tt.Texts()
	if len(cur) == 0 {
		t.Fatalf("nothing rendered")
	}
	if err := tt.Click(cur[0]); err != nil {
		t.Fatalf("open: %v (texts %q)", err, cur)
	}
	if err := tt.Click(choose); err != nil {
		if err2 := tt.ChooseMenuItem(choose); err2 != nil {
			t.Fatalf("choose %q: %v / %v (texts %q)", choose, err, err2, tt.Texts())
		}
	}
	tt.Frame()
	return v
}

func TestLangSelect(t *testing.T) {
	if got := pickLang(t, "English", "日语 日本語"); got != "Japanese" {
		t.Errorf("English → 日语: got %q", got)
	}
	if got := pickLang(t, "simplified chinese", "繁體中文"); got != "Traditional Chinese" {
		t.Errorf("case-insensitive current → 繁體: got %q", got)
	}
	// 列表外的自定义语言保留成第一个选项，也能改回列表里的
	if got := pickLang(t, "Klingon", "法语 Français"); got != "French" {
		t.Errorf("custom → 法语: got %q", got)
	}
}

// 最后一项要滚动才看得到：用键盘走到底再回车，高亮项会自己滚进视野
func TestLangSelectScrollsToLast(t *testing.T) {
	v := "English"
	tt := ui.NewTester(func(c *ui.Context) { langSelect(c, &v) }, 400, 400)
	if err := tt.Click("英语 English"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(targetLangs); i++ {
		tt.Key(0, ui.KeyDown)
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if v != "Polish" {
		t.Errorf("down×N + enter: got %q, texts %q", v, tt.Texts())
	}
}

// MAMI_SNAP=dir 时，把设置页打开下拉框的样子存成 PNG（真实窗口尺寸）
func TestSnapSettingsDropdown(t *testing.T) {
	dir := os.Getenv("MAMI_SNAP")
	if dir == "" {
		t.Skip("MAMI_SNAP not set")
	}
	a := &App{avatar: loadAvatar()}
	a.set = settingsState{draft: defaultConfig()}
	tt := ui.NewTester(a.settingsView, 560, 700)
	tt.SetDark(true)
	for i, open := range []string{"英语 English", "简体中文"} {
		if err := tt.Click(open); err != nil {
			t.Fatalf("open %q: %v", open, err)
		}
		tt.Frame()
		f, _ := os.Create(filepath.Join(dir, fmt.Sprintf("dropdown%d.png", i)))
		png.Encode(f, tt.Image())
		f.Close()
		tt.Key(0, ui.KeyEscape)
		tt.Frame()
	}
}

func TestSnapButton(t *testing.T) {
	dir := os.Getenv("MAMI_SNAP")
	if dir == "" {
		t.Skip("MAMI_SNAP not set")
	}
	a := &App{}
	for _, dark := range []bool{true, false} {
		tt := ui.NewTester(a.buttonView, btnW, btnH)
		tt.SetDark(dark)
		tt.SetScale(6)
		tt.Frame()
		f, _ := os.Create(filepath.Join(dir, fmt.Sprintf("btn-dark-%v.png", dark)))
		png.Encode(f, tt.Image())
		f.Close()
	}
}

func TestActionSelect(t *testing.T) {
	v := ""
	tt := ui.NewTester(func(c *ui.Context) { actionSelect(c, &v) }, 400, 300)
	if err := tt.Click("显示卡片（先显示「翻译中…」）"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("直接替换，不弹卡片"); err != nil {
		if err2 := tt.ChooseMenuItem("直接替换，不弹卡片"); err2 != nil {
			t.Fatalf("%v / %v %q", err, err2, tt.Texts())
		}
	}
	tt.Frame()
	if v != actionReplace {
		t.Errorf("got %q", v)
	}
}

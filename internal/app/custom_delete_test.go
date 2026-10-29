package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

// 「自定义」页：右键样式卡片 → 移到废纸篓（Windows 叫删除）。删掉的是正在用的样式时，
// 设置里的选择和已经保存的配置都换回默认样式；另一个样式不受影响
func TestDeleteCapsuleStyleFromTheCard(t *testing.T) {
	dir := t.TempDir()
	trash := filepath.Join(t.TempDir(), "trash")
	os.MkdirAll(trash, 0o755)
	defer func(f func(string) error) { trashItem = f }(trashItem)
	trashItem = func(p string) error { return os.Rename(p, filepath.Join(trash, filepath.Base(p))) }

	a := &App{avatar: loadAvatar(), cfgPath: filepath.Join(dir, "config.json")}
	styles := a.capsuleStylesDir()
	for _, want := range []string{"my-style", "my-style-2"} {
		if got, err := createCapsuleStyle(styles); err != nil || got != want {
			t.Fatalf("createCapsuleStyle = %q, %v; want %q", got, err, want)
		}
	}
	cfg := defaultConfig()
	cfg.VoiceStyle = capsulePrefix + "my-style"
	if err := a.setConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := saveConfig(a.cfgPath, cfg); err != nil {
		t.Fatal(err)
	}
	a.set = settingsState{draft: cfg, page: pageCustom}

	tt := ui.NewTester(a.settingsView, settingsW, settingsH)
	tt.Frame()
	for _, name := range []string{"My Style", "My Style 2", "鼠标移到样式上，点右上角就能删除"} {
		if _, ok := tt.Find(name); !ok {
			t.Fatalf("%q not on the Custom page", name)
		}
	}
	if err := tt.RightClick("My Style"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(trashLabel()); err != nil {
		t.Fatalf("menu %v: %v", tt.Menu(), err)
	}
	tt.Frame()
	tt.Frame()

	if _, err := os.Stat(filepath.Join(styles, "my-style")); !os.IsNotExist(err) {
		t.Errorf("my-style still in the styles folder (%v)", err)
	}
	if _, err := os.Stat(filepath.Join(trash, "my-style", "manifest.json")); err != nil {
		t.Errorf("my-style not in the trash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(styles, "my-style-2", "manifest.json")); err != nil {
		t.Errorf("the other style went too: %v", err)
	}
	def := defaultConfig().VoiceStyle
	if a.set.draft.VoiceStyle != def {
		t.Errorf("draft style %q, want %q", a.set.draft.VoiceStyle, def)
	}
	if saved, _ := a.config(); saved.VoiceStyle != def {
		t.Errorf("saved style %q, want %q", saved.VoiceStyle, def)
	}
	var onDisk Config
	if b, err := os.ReadFile(a.cfgPath); err != nil || json.Unmarshal(b, &onDisk) != nil || onDisk.VoiceStyle != def {
		t.Errorf("config file style %q (%v), want %q", onDisk.VoiceStyle, err, def)
	}
	if _, ok := tt.Find("My Style 2"); !ok {
		t.Error("My Style 2 gone from the page")
	}
	for _, e := range a.capsuleStyles() {
		if e.folder == "my-style" {
			t.Error("my-style still listed")
		}
	}
}

// 删的不是正在用的样式：选择不动
func TestDeleteAnotherCapsuleStyleKeepsTheChoice(t *testing.T) {
	dir := t.TempDir()
	defer func(f func(string) error) { trashItem = f }(trashItem)
	trashItem = os.RemoveAll
	a := &App{cfgPath: filepath.Join(dir, "config.json")}
	createCapsuleStyle(a.capsuleStylesDir())
	createCapsuleStyle(a.capsuleStylesDir())
	cfg := defaultConfig()
	cfg.VoiceStyle = capsulePrefix + "my-style"
	a.setConfig(cfg)
	d := cfg
	a.deleteCapsuleStyle(&d, "my-style-2", "My Style 2")
	if saved, _ := a.config(); d.VoiceStyle != cfg.VoiceStyle || saved.VoiceStyle != cfg.VoiceStyle {
		t.Errorf("draft %q saved %q, want both %q", d.VoiceStyle, saved.VoiceStyle, cfg.VoiceStyle)
	}
	if a.set.statusErr {
		t.Errorf("status %q", a.set.status)
	}
}

// 新建的第二个样式起：名字带编号
func TestNewCapsuleStylesGetNumberedNames(t *testing.T) {
	dir := t.TempDir()
	for i, want := range []string{"My Style", "My Style 2", "My Style 3"} {
		folder, err := createCapsuleStyle(dir)
		if err != nil {
			t.Fatal(err)
		}
		m, err := loadCapsuleManifest(filepath.Join(dir, folder))
		if err != nil || m.Name != want {
			t.Errorf("#%d %s: name %q (%v), want %q", i+1, folder, m.Name, err, want)
		}
	}
}

// 卡片右上角的删除按钮：不悬停时看不见；悬停哪张只出哪张的；点了先弹「移到废纸篓」（不直接删），
// 鼠标挪到菜单上（离开卡片）再选，照样删得掉——按钮要是只在悬停时才建，这一下就被吞了
func TestDeleteCapsuleStyleFromItsButton(t *testing.T) {
	dir := t.TempDir()
	trash := filepath.Join(t.TempDir(), "trash")
	os.MkdirAll(trash, 0o755)
	defer func(f func(string) error) { trashItem = f }(trashItem)
	trashItem = func(p string) error { return os.Rename(p, filepath.Join(trash, filepath.Base(p))) }

	a := &App{avatar: loadAvatar(), cfgPath: filepath.Join(dir, "config.json")}
	styles := a.capsuleStylesDir()
	for range 2 {
		if _, err := createCapsuleStyle(styles); err != nil {
			t.Fatal(err)
		}
	}
	cfg := defaultConfig()
	if err := a.setConfig(cfg); err != nil {
		t.Fatal(err)
	}
	a.set = settingsState{draft: cfg, page: pageCustom}
	tt := ui.NewTester(a.settingsView, settingsW, settingsH)
	frames := func() {
		for range 5 {
			tt.Frame()
		}
	}
	frames()
	del, del2 := "删除「My Style」", "删除「My Style 2」"
	if _, ok := tt.Find(del); ok {
		t.Fatal("the delete button shows without hovering")
	}
	card, ok := tt.Find("My Style")
	if !ok {
		t.Fatal("no My Style card")
	}
	tt.Move(card.X+card.W/2, card.Y+card.H/2)
	frames()
	if _, ok := tt.Find(del); !ok {
		t.Fatalf("hovering the card shows no %q; texts %q", del, tt.Texts())
	}
	if _, ok := tt.Find(del2); ok {
		t.Error("hovering one card shows the other's delete button too")
	}
	if err := tt.Click(del); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, err := os.Stat(filepath.Join(styles, "my-style", "manifest.json")); err != nil {
		t.Fatalf("clicking the button deleted right away, before asking: %v", err)
	}
	if got := tt.Menu(); len(got) != 1 || got[0] != trashLabel() {
		t.Fatalf("menu %q, want [%q]", got, trashLabel())
	}
	tt.Move(2, 2) // 去点菜单：鼠标离开卡片
	frames()
	if err := tt.ChooseMenuItem(trashLabel()); err != nil {
		t.Fatal(err)
	}
	frames()
	if _, err := os.Stat(filepath.Join(trash, "my-style", "manifest.json")); err != nil {
		t.Errorf("my-style not in the trash: %v", err)
	}
	if _, err := os.Stat(filepath.Join(styles, "my-style-2", "manifest.json")); err != nil {
		t.Errorf("the other style went too: %v", err)
	}
}

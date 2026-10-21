//go:build windows

package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"mami/internal/i18n"
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
	a.set = settingsState{draft: defaultConfig(), page: pageTrans}
	tt := ui.NewTester(a.settingsView, settingsW, settingsH)
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

// MAMI_SNAP=dir 时，把设置页四个分页（深色）存成 PNG
func TestSnapSettingsPages(t *testing.T) {
	dir := os.Getenv("MAMI_SNAP")
	if dir == "" {
		t.Skip("MAMI_SNAP not set")
	}
	for _, page := range []string{pageTrans, pageBtn, pageKeys, pageVoice, pageWords, pagePolish, pageGen, pageHist} {
		a := &App{avatar: loadAvatar()}
		a.set = settingsState{draft: defaultConfig(), page: page, testMsg: "连接正常（1.4s）：你好，世界 → Hello, world"}
		tt := ui.NewTester(a.settingsView, settingsW, settingsH)
		tt.SetDark(true)
		tt.Frame()
		f, _ := os.Create(filepath.Join(dir, "page-"+page+".png"))
		png.Encode(f, tt.Image())
		f.Close()
	}
}

func TestButtonActionsTranslated(t *testing.T) {
	for _, o := range buttonActions {
		if e, ok := i18n.Dict[o.label]; !ok || e[0] == "" || e[1] == "" {
			t.Errorf("%q 没有译文", o.label)
		}
	}
}

// MAMI_SNAP=dir 时，把 9 种界面语言 × 5 个设置页都截下来（兼容性展开）
func TestSnapSettingsI18n(t *testing.T) {
	dir := os.Getenv("MAMI_SNAP")
	if dir == "" {
		t.Skip("MAMI_SNAP not set")
	}
	defer i18n.Set(i18n.ZH)
	// 每种界面语言的每一页都截一张，用来查文字有没有被挤出、截断
	type snap struct {
		lang, theme, page string
		dark              bool
	}
	var cases []snap
	for i, lang := range []string{"zh", "zh-TW", "en", "ja", "ko", "es", "fr", "de", "ru"} {
		theme, dark := "dark", true
		if i%2 == 1 {
			theme, dark = "light", false
		}
		for _, page := range []string{pageTrans, pageBtn, pageKeys, pageVoice, pageWords, pagePolish, pageGen, pageHist} {
			cases = append(cases, snap{lang, theme, page, dark})
		}
	}
	for _, c := range cases {
		a := &App{avatar: loadAvatar()}
		draft := defaultConfig()
		draft.Language, draft.Theme = c.lang, c.theme
		draft.History = true // 把记录相关的几行也画出来
		draft.ExtraPrompt = "Translate for an AI prompt: be precise, keep structure, placeholders like {name} untouched, do not translate code or file paths."
		a.set = settingsState{draft: draft, page: c.page}
		tt := ui.NewTester(a.settingsView, settingsW, settingsH)
		tt.SetDark(c.dark)
		tt.Frame()
		tt.Frame() // 第一帧才注册字体，按钮宽度按第二帧量的才准
		f, _ := os.Create(filepath.Join(dir, "i18n-"+c.lang+"-"+c.theme+"-"+c.page+".png"))
		png.Encode(f, tt.Image())
		f.Close()
	}
}

// 额外要求框：默认随内容长短；拖右下角把手手动调；点一下恢复自动
func TestExtraPromptResize(t *testing.T) {
	const gripX = settingsW - 42 - 17 // 把手离窗口右边 42，再加分组卡片的内边距和边框
	a := &App{avatar: loadAvatar()}
	a.set = settingsState{draft: defaultConfig(), page: pageTrans}
	tt := ui.NewTester(a.settingsView, settingsW, 1000) // 接口区在上面，窗口拉高让额外要求框露出来
	tt.Frame()
	below, ok := tt.Find("显示") // 把手在「显示」组名上方，中间隔着卡片底边
	if !ok {
		t.Fatalf("找不到「显示」（texts %q）", tt.Texts())
	}
	if a.set.extraManual {
		t.Fatal("没动把手就不该进手动模式")
	}
	grab := func(dy float32) (float32, bool) {
		for y := below.Y - 60; y < below.Y; y += 2 {
			tt.Move(gripX, y)
			tt.Frame()
			tt.Press(gripX, y)
			tt.Frame()
			tt.Move(gripX, y+dy/2)
			tt.Frame()
			tt.Move(gripX, y+dy)
			tt.Frame()
			tt.Release(gripX, y+dy)
			tt.Frame()
			if a.set.extraManual {
				return y, true
			}
		}
		return 0, false
	}
	y, ok := grab(40)
	if !ok {
		t.Fatalf("拖不动把手（texts %q）", tt.Texts())
	}
	grown := a.set.extraH
	if grown < 100 {
		t.Errorf("往下拖 40 后高度只有 %v", grown)
	}
	tt.Frame()
	tt.Move(gripX, y+40)
	tt.Frame()
	tt.ClickAt(gripX, y+40)
	tt.Frame()
	if a.set.extraManual {
		t.Errorf("点一下把手没有恢复自动")
	}
}

// MAMI_DOCS=dir 时，生成 README / 主页用的截图（2 倍清晰度）：卡片和设置页的深色、浅色、日文版
func TestSnapDocs(t *testing.T) {
	dir := os.Getenv("MAMI_DOCS")
	if dir == "" {
		t.Skip("MAMI_DOCS not set")
	}
	defer i18n.Set(i18n.ZH)
	save := func(name string, tt *ui.Tester) {
		tt.Frame()
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, tt.Image())
		f.Close()
	}
	prompt := "这是写给 AI 的提示词，译文要指令明确、没有歧义，语气自然直接。\n代码、命令、文件路径、URL、模型名不翻译。"
	for _, d := range []struct {
		suffix string
		dark   bool
		theme  string
	}{{"", true, "dark"}, {"-light", false, "light"}} {
		i18n.Set(i18n.ZH)
		a := &App{avatar: loadAvatar()}
		cfg := defaultConfig()
		cfg.Theme = d.theme
		a.cfg = cfg
		a.pop = popupState{status: statusDone, header: "中文 → English", w: 380, h: 172,
			text: "Could you take a look at why this plugin doesn't take effect after startup? There's nothing in the logs.\n\nSecond paragraph: the meeting is at 3 p.m. tomorrow."}
		tt := ui.NewTester(a.popupView, 380, 172)
		tt.SetDark(d.dark)
		tt.SetScale(2)
		save("card"+d.suffix+".png", tt)

		cfg.ExtraPrompt = prompt
		cfg.BaseURL, cfg.Model = "https://tokenhub.tencentmaas.com/v1", "hy-mt2-pro"
		a2 := &App{avatar: loadAvatar()}
		a2.set = settingsState{draft: cfg, page: pageTrans}
		tt = ui.NewTester(a2.settingsView, settingsW, settingsH)
		tt.SetDark(d.dark)
		tt.SetScale(2)
		save("settings"+d.suffix+".png", tt)

		// 语音：外观页（样式下拉 + 三种状态的胶囊预览）和词库页
		a3 := &App{avatar: loadAvatar(), learn: newLearner(filepath.Join(t.TempDir(), "learned.json"))}
		a3.learn.add([]learnedWord{{Right: "Claude", Manual: true}, {Right: "Mami", Manual: true}, {Right: "DeepSeek", Manual: true}, {Right: "MyGo", Manual: true}, {Right: "Remotion", Manual: true}})
		a3.learn.add([]learnedWord{{Wrong: "张伟", Right: "章炜"}, {Wrong: "张伟", Right: "章炜"}, {Wrong: "扣持", Right: "蔻驰"}, {Wrong: "阶越", Right: "阶跃"}, {Wrong: "阶越", Right: "阶跃"}})
		cfg.VoiceStyle = "bars"
		a3.set = settingsState{draft: cfg, page: pageVoice, voiceTab: 1}
		tt = ui.NewTester(a3.settingsView, settingsW, settingsH)
		tt.SetDark(d.dark)
		tt.SetScale(2)
		tt.Frame()
		save("voice"+d.suffix+".png", tt)
		a3.set = settingsState{draft: cfg, page: pageWords}
		tt = ui.NewTester(a3.settingsView, settingsW, settingsH)
		tt.SetDark(d.dark)
		tt.SetScale(2)
		tt.Frame()
		save("words"+d.suffix+".png", tt)
	}
	// 日文深色，选择按钮页
	cfg := defaultConfig()
	cfg.Language, cfg.Theme = "ja", "dark"
	a := &App{avatar: loadAvatar()}
	a.set = settingsState{draft: cfg, page: pageBtn}
	tt := ui.NewTester(a.settingsView, settingsW, settingsH)
	tt.SetDark(true)
	tt.SetScale(2)
	save("settings-ja.png", tt)
}

func TestPolishTabs(t *testing.T) {
	defer i18n.Set(i18n.ZH)
	a := &App{avatar: loadAvatar()}
	cfg := defaultConfig()
	cfg.Language = "zh"
	a.set = settingsState{draft: cfg, page: pagePolish}
	tt := ui.NewTester(a.settingsView, settingsW, 800)
	tt.Frame()
	if _, ok := tt.Find("按程序自动换档"); !ok {
		t.Fatal("missing polish options")
	}
	snapshot := func(name string) {
		if dir := os.Getenv("MAMI_SNAP"); dir != "" {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			file, err := os.Create(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if err := png.Encode(file, tt.Image()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, ok := tt.Find("服务商"); ok {
		t.Fatal("API settings visible on options tab")
	}
	snapshot("polish-level.png")
	if err := tt.Click("接口"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.set.polishTab != 1 {
		t.Fatal("API tab did not activate")
	}
	if _, ok := tt.Find("服务商"); !ok {
		t.Fatal("missing API settings")
	}
	if _, ok := tt.Find("按程序自动换档"); ok {
		t.Fatal("polish options visible on API tab")
	}
	snapshot("polish-api.png")
	track, ok := tt.Find("档位")
	if !ok {
		t.Fatal("missing options tab")
	}
	tt.ClickAt(track.X+16, track.Y+track.H/2)
	tt.Frame()
	tt.Frame()
	if a.set.polishTab != 0 || a.set.draft.PolishStyle != cfg.PolishStyle {
		t.Fatalf("tab=%d style=%q want=%q", a.set.polishTab, a.set.draft.PolishStyle, cfg.PolishStyle)
	}
}

func TestProviderPickerClick(t *testing.T) {
	a := &App{avatar: loadAvatar()}
	d := defaultConfig()
	d.Language, d.BaseURL = "zh", "https://index-translate.bilibili.com/v1"
	a.set = settingsState{draft: d, page: pageTrans, transTab: 1}
	tt := ui.NewTester(a.settingsView, settingsW, 800)
	tt.Frame()
	tt.Frame()
	r, ok := tt.Find("DeepSeek")
	if !ok {
		t.Fatalf("找不到 DeepSeek（texts %q）", tt.Texts())
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Frame()
	tt.Frame()
	if got := a.set.draft.BaseURL; got != "https://api.deepseek.com/v1" || a.set.draft.Model != "deepseek-flash" {
		t.Fatalf("点了 DeepSeek 还是 %s / %s", got, a.set.draft.Model)
	}
}

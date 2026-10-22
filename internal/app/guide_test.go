//go:build windows

package app

import (
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"mami/internal/i18n"
)

func guideTester(t *testing.T, step int) (*App, *ui.Tester) {
	t.Helper()
	i18n.Set(i18n.ZH)
	cfg := defaultConfig()
	cfg.Language, cfg.VoiceEnabled = "zh", true // 全新安装时 run.go 会打开语音
	a := &App{avatar: loadAvatar(), cfg: cfg}
	a.set = settingsState{draft: cfg, page: pageTrans, guide: true, guideStep: step}
	tt := ui.NewTester(a.settingsView, settingsW, settingsH)
	tt.Frame()
	tt.Frame()
	return a, tt
}

func click(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	if err := tt.Click(label); err != nil {
		t.Fatalf("click %q: %v (texts %q)", label, err, tt.Texts())
	}
	tt.Frame()
	tt.Frame()
}

// 每种界面语言的那句都配得上括号；去掉口头禅后是一句通顺的话，比念的短
func TestGuideScriptEveryLanguage(t *testing.T) {
	defer i18n.Set(i18n.ZH)
	for l := i18n.ZH; l <= i18n.RU; l++ {
		i18n.Set(l)
		parts := parseScript(guideScript())
		fillers := 0
		for _, p := range parts {
			if strings.ContainsAny(p.text, "[]") || p.text == "" {
				t.Errorf("语言 %d：括号没配好 %q", l, guideScript())
			}
			if p.filler {
				fillers++
			}
		}
		said, polished := scriptText(parts, true), scriptText(parts, false)
		if fillers < 2 || len(polished) >= len(said) {
			t.Errorf("语言 %d：口头禅 %d 处，念的 %q，整理后 %q", l, fillers, said, polished)
		}
		if strings.Contains(polished, "  ") || strings.HasPrefix(polished, " ") {
			t.Errorf("语言 %d：整理后有多余空格 %q", l, polished)
		}
	}
	i18n.Set(i18n.ZH)
	if got := scriptText(parseScript(guideScript()), false); got != "帮我把这个接口改成用 Go 写，然后四点开会。" {
		t.Errorf("中文整理后 %q", got)
	}
	if got := parseScript("a[b"); len(got) != 2 || got[1].filler {
		t.Errorf("没配对的括号要当正文：%+v", got)
	}
}

// 你好 → 说 → 划 → 好了，一路点「继续」；最后「开始使用」收起引导，不切页、不提 Key
func TestGuideWalkthrough(t *testing.T) {
	defer i18n.Set(i18n.ZH)
	a, tt := guideTester(t, stepHello)
	if _, ok := tt.Find("保存"); ok {
		t.Fatal("引导里露出了设置的「保存」")
	}
	click(t, tt, "开始")
	for i, title := range []string{"试试用说的", "选中，就能翻译"} {
		if _, ok := tt.Find(title); !ok {
			t.Fatalf("第 %d 步没有「%s」（texts %q）", i+2, title, tt.Texts())
		}
		click(t, tt, "继续")
	}
	if _, ok := tt.Find("好了，就这么简单"); !ok {
		t.Fatalf("最后一屏不对（texts %q）", tt.Texts())
	}
	click(t, tt, "开始使用")
	if a.set.guide || a.set.page != pageTrans || a.set.voiceTab != 0 || a.set.status != "" || a.guideVoice.Load() || a.guideTrans.Load() {
		t.Fatalf("看完后 guide=%v page=%s voiceTab=%d status=%q", a.set.guide, a.set.page, a.set.voiceTab, a.set.status)
	}
}

// 第一屏和中间两屏都能跳过；跳过后触发键和 Alt+T 不再归引导
func TestGuideSkip(t *testing.T) {
	defer i18n.Set(i18n.ZH)
	for step, label := range []string{"跳过", "跳过引导", "跳过引导"} {
		a, tt := guideTester(t, step)
		if step == stepVoice && !a.guideVoice.Load() || step == stepTrans && !a.guideTrans.Load() {
			t.Fatalf("第 %d 屏没接管快捷键", step+1)
		}
		click(t, tt, label)
		if a.set.guide || a.guideVoice.Load() || a.guideTrans.Load() || a.set.page != pageTrans {
			t.Fatalf("第 %d 屏跳过后 guide=%v voice=%v trans=%v page=%s", step+1, a.set.guide, a.guideVoice.Load(), a.guideTrans.Load(), a.set.page)
		}
	}
}

// 语音那屏：念到哪亮到哪，念完只剩整理好的那句，「继续」变成主按钮
func TestGuideSayPhases(t *testing.T) {
	defer i18n.Set(i18n.ZH)
	a, tt := guideTester(t, stepVoice)
	if !tt.HasText("RAlt") {
		t.Errorf("没提示触发键（texts %q）", tt.Texts())
	}
	lit, dim := ui.RGBA(255, 255, 255, 1), ui.RGBA(128, 128, 128, 1)
	parts := parseScript(guideScript())
	sp := scriptSpans(parts, sayListening, 5, lit, dim)
	if len(sp) < 2 || sp[0].Text != "嗯，那个，" || sp[0].Color != lit || sp[1].Color == lit {
		t.Errorf("念了 5 个字应该只亮「嗯，那个，」：%+v", sp)
	}
	struck := 0
	for _, s := range scriptSpans(parts, sayPolishing, 0, lit, dim) {
		if s.Strikethrough {
			struck++
			if !strings.Contains("嗯，那个，三点开会，不对，是", s.Text) {
				t.Errorf("划掉了不该划的 %q", s.Text)
			}
		}
	}
	if struck != 2 {
		t.Errorf("整理时该划掉两处，划了 %d 处", struck)
	}
	a.set.voice = guideVoiceState{phase: sayDone}
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find("帮我把这个接口改成用 Go 写，然后四点开会。"); !ok {
		t.Errorf("念完没剩整理好的那句（texts %q）", tt.Texts())
	}
	click(t, tt, "再试一次")
	if a.set.voice.phase != sayIdle {
		t.Errorf("再试一次后 phase=%d", a.set.voice.phase)
	}
	a.set.voice = guideVoiceState{msg: T("没听清，请再说一次")}
	tt.Frame()
	if _, ok := tt.Find("没听清，请再说一次"); !ok {
		t.Errorf("没听清时没说（texts %q）", tt.Texts())
	}
}

// 划词那屏：全选 →「文A」→ 卡片 → 替换；Esc 关卡片，Enter 也能替换
func TestGuideTranslate(t *testing.T) {
	defer i18n.Set(i18n.ZH)
	a, tt := guideTester(t, stepTrans)
	src, _, dst := guideTransPair()
	if a.set.trans.text != src {
		t.Fatalf("输入框里是 %q", a.set.trans.text)
	}
	if _, ok := tt.Find("翻译"); ok {
		t.Fatal("没选字就冒出了「文A」")
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Frame()
	tt.Frame()
	click(t, tt, "翻译")
	if a.set.trans.card != 1 {
		t.Fatalf("点了「文A」 card=%d", a.set.trans.card)
	}
	a.set.trans.cardAt = time.Now().Add(-time.Second) // 不等「翻译中…」
	tt.Frame()
	tt.Frame()
	if _, ok := tt.Find(dst); !ok {
		t.Fatalf("卡片里没有译文（texts %q）", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.set.trans.card != 0 {
		t.Fatal("Esc 没关卡片")
	}
	a.set.trans.card, a.set.trans.cardAt = 2, time.Now().Add(-time.Second)
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()
	if !a.set.trans.done || a.set.trans.text != dst {
		t.Fatalf("Enter 后 done=%v text=%q", a.set.trans.done, a.set.trans.text)
	}
	if !tt.HasText("Alt") {
		t.Errorf("换好后没教 Alt+T（texts %q）", tt.Texts())
	}
}

// Alt+T 在引导那屏：直接换，不发请求；Alt+Shift+T 弹卡片
func TestGuideHotkey(t *testing.T) {
	defer i18n.Set(i18n.ZH)
	a, _ := guideTester(t, stepTrans)
	a.guideHotkey(false)
	if a.set.trans.card != 1 || a.set.trans.done {
		t.Fatalf("Alt+Shift+T 后 card=%d done=%v", a.set.trans.card, a.set.trans.done)
	}
	a.guideHotkey(true)
	_, _, dst := guideTransPair()
	if !a.set.trans.done || a.set.trans.text != dst || a.set.trans.card != 0 {
		t.Fatalf("Alt+T 后 done=%v card=%d text=%q", a.set.trans.done, a.set.trans.card, a.set.trans.text)
	}
	a, _ = guideTester(t, stepVoice)
	a.guideHotkey(true)
	if a.set.trans.done {
		t.Error("不在划词那屏时 Alt+T 不该归引导")
	}
}

// MAMI_SNAP=dir 时，把每一屏和几个中间状态按中文深色、英文浅色、德文深色各截一张
func TestSnapGuide(t *testing.T) {
	dir := os.Getenv("MAMI_SNAP")
	if dir == "" {
		t.Skip("MAMI_SNAP not set")
	}
	defer i18n.Set(i18n.ZH)
	type shot struct {
		name string
		step int
		set  func(a *App)
	}
	shots := []shot{
		{"1-hello", stepHello, nil},
		{"2-say", stepVoice, nil},
		{"3-listening", stepVoice, func(a *App) { a.set.voice = guideVoiceState{phase: sayListening, heard: 18, level: 0.03} }},
		{"4-polishing", stepVoice, func(a *App) { a.set.voice = guideVoiceState{phase: sayPolishing} }},
		{"5-said", stepVoice, func(a *App) { a.set.voice = guideVoiceState{phase: sayDone} }},
		{"6-trans", stepTrans, nil},
		{"7-card", stepTrans, func(a *App) { a.set.trans.card, a.set.trans.cardAt = 2, time.Now().Add(-time.Second) }},
		{"8-replaced", stepTrans, func(a *App) {
			_, _, dst := guideTransPair()
			a.set.trans = guideTransState{text: dst, done: true}
		}},
		{"9-done", stepDone, nil},
	}
	for _, c := range []struct {
		lang, theme string
		dark        bool
	}{{"zh", "dark", true}, {"en", "light", false}, {"de", "dark", true}} {
		for _, s := range shots {
			cfg := defaultConfig()
			cfg.Language, cfg.Theme, cfg.VoiceEnabled = c.lang, c.theme, true
			a := &App{avatar: loadAvatar(), cfg: cfg}
			a.set = settingsState{draft: cfg, guide: true, guideStep: s.step}
			tt := ui.NewTester(a.settingsView, settingsW, settingsH)
			tt.SetDark(c.dark)
			tt.SetScale(2)
			tt.Frame()
			if s.set != nil {
				s.set(a)
			}
			if s.name == "6-trans" {
				tt.Key(ui.Ctrl, ui.KeyA) // 选着字，「文A」冒出来
			}
			for range 3 {
				tt.Frame()
			}
			time.Sleep(450 * time.Millisecond) // 等进场动画、麦克风外圈走完
			tt.Frame()
			f, err := os.Create(filepath.Join(dir, fmt.Sprintf("guide-%s-%s.png", c.lang, s.name)))
			if err != nil {
				t.Fatal(err)
			}
			png.Encode(f, tt.Image())
			f.Close()
		}
	}
}

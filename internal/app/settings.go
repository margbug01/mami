//go:build windows

package app

import (
	"context"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"time"

	"mami/internal/i18n"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// settingsState 是设置窗口里正在改、还没保存的那份配置（主线程）
type settingsState struct {
	draft         Config
	exclude       string
	asrHotwords   string
	openAtLogin   bool
	hotkeysOn     bool // 「启用快捷键」开关，保存时写回 DisableHotkeys 的反面
	status        string
	statusErr     bool
	testing       bool
	page          string
	extraH        float32 // 额外要求输入框的高度（手动拖过之后才用）
	extraManual   bool
	gripMoved     bool // 这次按住把手时拖动过
	transTab      int  // 「翻译」页：0 译文、1 接口
	voiceTab      int  // 「语音识别」页：0 通用、1 外观、2 接口
	compat        bool
	stats         *historyStats // 「记录」页的统计，打开设置窗口时清空、5 秒内复用
	custom        bool          // 「服务商」手动选了自定义（地址还是某家预设的也不跳回去）
	testMsg       string
	testErr       bool
	polishTesting bool
	histTab       int // 「记录」页：0 翻译、1 语音
	voiceTesting  bool
	voiceTestMsg  string
	voiceTestErr  bool
	recID         string         // 正在录的快捷键框
	recJob        *keyCaptureJob // 交给键盘钩子的那次录制
	recHint       string
	copiedID      string
	copiedAt      time.Time
	polishTestMsg string
	polishTestErr bool
}

func (a *App) openSettings() {
	cfg, _ := a.config()
	reset := func() {
		keyCapturing.Store(nil)
		a.set = settingsState{
			draft:       cfg,
			exclude:     strings.Join(cfg.ExcludeProcesses, ", "),
			asrHotwords: strings.Join(cfg.ASRHotwords, ", "),
			openAtLogin: mygo.App.OpenAtLogin(),
			hotkeysOn:   !cfg.DisableHotkeys,
			page:        pageTrans,
		}
	}
	if a.setWin == nil {
		reset()
		a.setWin = mygo.NewWindow(mygo.WindowOptions{
			Title:           "Mami · Settings",
			Width:           settingsW,
			Height:          settingsH,
			MinWidth:        settingsW - 60,
			MinHeight:       420,
			DisableMaximize: true,
			Frameless:       true,
			Content:         ui.View(a.settingsView),
		})
		a.setWin.SetIcon(iconPNG)
		a.setWin.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting {
				e.PreventDefault()
				a.stopCapture()
				a.setWin.Hide()
				saved, _ := a.config()
				i18n.Set(i18n.Resolve(saved.Language, sysLocale)) // 没保存的语言预览收回
			}
		})
	} else if !a.setWin.IsVisible() {
		a.setWin.Update(reset) // 重新打开时丢掉上次没保存的改动
	}
	a.setWin.Show()
	a.setWin.Focus()
}

func (a *App) settingsView(c *ui.Context) {
	t := c.Theme()
	s := &a.set
	d := &s.draft
	i18n.Set(i18n.Resolve(d.Language, sysLocale)) // 在设置里改了立刻预览，保存才算数
	applyTheme(c, d.Theme)
	t = c.Theme()
	ui.Column(c).Fill().Children(func() {
		a.titleBar(c)
		ui.Divider(c)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(sidebarW).Children(func() {
				ui.Sidebar(c, &s.page, func() {
					ui.Box(c).Height(8)
					ui.SidebarSection(c, T("划词翻译"), nil, func() {
						ui.SidebarItem(c, pageTrans, iconTrans, T("翻译"))
						ui.SidebarItem(c, pageBtn, iconBtn, T("划词按钮"))
						ui.SidebarItem(c, pageKeys, iconKeys, T("快捷键"))
					})
					ui.SidebarSection(c, T("语音输入"), nil, func() {
						ui.SidebarItem(c, pageVoice, iconMic, T("语音识别"))
						ui.SidebarItem(c, pagePolish, iconPolish, T("润色"))
					})
					ui.Box(c).Height(12)
					ui.SidebarItem(c, pageGen, iconGen, T("通用"))
					ui.SidebarItem(c, pageHist, iconHist, T("记录"))
				}).Grow(1)
			})
			ui.Scroll(c).Grow(1).Children(func() {
				// 每页、每个 tab 单独认控件：按位置认的话，上一页同位置的多行框（热词）会被沿用，Password 对它无效，Key 就明文了
				ui.Column(c).Padding(8, 24, 16, 24).Key(fmt.Sprint(s.page, s.transTab, s.voiceTab)).Children(func() {
					switch s.page {
					case pageVoice:
						a.pageVoice(c, d)
					case pagePolish:
						a.pagePolish(c, d)
					case pageKeys:
						a.pageHotkeys(c, d)
					case pageBtn:
						a.pageButton(c, d)
					case pageGen:
						a.pageGeneral(c, d)
					case pageHist:
						a.pageHistory(c, d)
					default:
						a.pageTranslate(c, d)
					}
				})
			})
		})
		ui.Divider(c)
		ui.Row(c).Padding(10, 24).Gap(8).Children(func() {
			color := t.TextMuted
			if s.statusErr {
				color = t.Danger
			}
			ui.Text(c, s.status).FontSize(12).TextColor(color).Grow(1).MaxLines(2)
			if accentButton(c, T("保存")).Clicked() {
				a.saveDraft()
			}
		})
	})
}

const (
	pageTrans  = "trans"
	pageKeys   = "keys"
	pageBtn    = "btn"
	pageVoice  = "voice"
	pagePolish = "polish"
	pageGen    = "gen"
)

// settingRow 一行设置：左边名字+一句说明，右边控件，行与行之间一条细线
func settingRow(c *ui.Context, title, desc string, ctl func()) {
	t := c.Theme()
	ui.Row(c).Gap(16).Padding(12, 0).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(14)
			if desc != "" {
				ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted)
			}
		})
		ui.Box(c).Width(controlW).Children(ctl)
	})
	ui.Divider(c)
}

// controlW 右侧控件的宽度
const controlW = 280

func pageTitle(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(16).Bold().Padding(10, 0, 4, 0)
}

// pageTabs 页标题下的分段切换，页面太长时按用途拆开
func pageTabs(c *ui.Context, selected *int, labels ...string) {
	ui.Row(c).Padding(4, 0, 8, 0).Children(func() {
		ui.Segmented(c, selected, labels...).Label(labels[0])
	})
}

// subTitle 页内分区的小标题，比页标题轻一档
func subTitle(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(13).Bold().TextColor(c.Theme().TextMuted).Padding(20, 0, 2, 0)
}

// translateAPI 「翻译」页的接口区：服务商、地址、Key、模型、测试、代理
func (a *App) translateAPI(c *ui.Context, d *Config) {
	s := &a.set
	t := c.Theme()
	cur := "custom"
	if !s.custom {
		cur = providerOf(d.BaseURL)
	}
	desc := T("选一个会自动填好接口地址和模型")
	if cur == "bilibili" {
		desc = T("免费、不用 Key，但高峰期常超时")
	}
	settingRow(c, T("服务商"), desc, func() {
		v := cur
		optionSelect(c, &v, providerOptions())
		if v != cur {
			s.custom = v == "custom"
			for _, p := range providers {
				if p.id == v && p.base != "" {
					d.BaseURL, d.Model = p.base, p.model
				}
			}
			s.testMsg, s.testErr = "", false
		}
	})
	settingRow(c, T("接口地址"), T("会自动拼上 /chat/completions"), func() {
		ui.TextInput(c, &d.BaseURL).Placeholder("https://api.deepseek.com/v1")
	})
	settingRow(c, "API Key", T("免费接口留空"), func() {
		ui.TextInput(c, &d.APIKey).Password().Placeholder("sk-…")
	})
	settingRow(c, T("模型"), "", func() { ui.TextInput(c, &d.Model).Placeholder("deepseek-flash") })
	settingRow(c, T("代理"), T("Go 不读系统代理，要走 Clash 得填这里"), func() {
		ui.TextInput(c, &d.Proxy).Placeholder("http://127.0.0.1:7890")
	})
	ui.Row(c).Gap(12).Padding(12, 0).Children(func() {
		color := t.TextMuted
		if s.testErr {
			color = t.Danger
		}
		ui.Text(c, s.testMsg).FontSize(12).TextColor(color).Grow(1).MaxLines(2)
		if textButton(c, T("测试连接")).Disabled(s.testing).Clicked() {
			a.testDraft()
		}
	})
}

// providers 是「服务商」下拉框里的预设：选中后填好接口地址和模型。
// 按接口地址认出当前用的是哪一家，认不出就是自定义。
var providers = []struct{ id, base, model string }{
	{"bilibili", "https://index-translate.bilibili.com/v1", "Index-Translate-35B-A3B"},
	{"tokenhub", "https://tokenhub.tencentmaas.com/v1", "hy-mt2-pro"},
	{"openrouter", "https://openrouter.ai/api/v1", "tencent/hy-mt2-30b-a3b"},
	{"deepseek", "https://api.deepseek.com/v1", "deepseek-flash"},
	{"custom", "", ""},
}

func providerOptions() []option {
	return []option{
		{"bilibili", T("B 站 Index-Translate（免费）")},
		{"tokenhub", T("腾讯 TokenHub · Hy-MT2")},
		{"openrouter", "OpenRouter · Hy-MT2"},
		{"deepseek", "DeepSeek"},
		{"custom", T("自定义")},
	}
}

func providerOf(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	for _, p := range providers {
		if p.base != "" && strings.EqualFold(b, p.base) {
			return p.id
		}
	}
	return "custom"
}

func (a *App) pageVoice(c *ui.Context, d *Config) {
	pageTitle(c, T("语音识别"))
	pageTabs(c, &a.set.voiceTab, T("通用"), T("外观"), T("接口"))
	switch a.set.voiceTab {
	case 1:
		a.voiceLook(c, d)
		return
	case 2:
		a.apiASR(c, d)
		return
	}
	switchRow(c, T("启用语音输入"), T("按一下开始说话，再按一下贴入；Esc 取消"), &d.VoiceEnabled)
	ui.Divider(c)
	settingRow(c, T("触发键"), T("点一下再按键，Backspace 关闭"), func() { a.hotkeyBox(c, "voice", &d.VoiceHotkey, true) })
	switchRow(c, T("提示音"), T("开始、结束、取消时轻响一声"), &d.VoiceSound)
	ui.Divider(c)
	settingRow(c, T("热词"), T("逗号或换行分隔"), func() { ui.TextArea(c, &a.set.asrHotwords).Height(64) })
}

// voiceLook 「语音识别」页的外观 tab：录音时的样式
func (a *App) voiceLook(c *ui.Context, d *Config) {
	settingRow(c, T("样式"), T("录音时屏幕底部的样子"), func() {
		optionSelect(c, &d.VoiceStyle, []option{{"bars", T("渐变波柱")}, {"orb", T("果冻球")}, {"glass", T("磨砂玻璃")}, {"pixel", T("复古像素")}, {"mercury", T("水银")}, {"islands", T("三岛连桥")}, {"leds", T("分段电平")}})
	})
	if d.VoiceStyle != "pixel" && d.VoiceStyle != "orb" { // 像素和果冻球的配色是固定的
		settingRow(c, T("浮窗底色"), T("跟界面主题分开选"), func() {
			optionSelect(c, &d.VoiceTone, []option{{"dark", T("深色")}, {"light", T("浅色")}, {"auto", T("跟随主题")}})
		})
	}
	a.voicePreview(c, d)
	if d.VoiceStyle == "orb" {
		settingRow(c, T("果冻球大小"), T("右边是实际大小的预览"), func() {
			ui.Row(c).Width(controlW).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Slider(c, &d.VoiceOrbSize, 40, 96).Grow(1).Label(T("果冻球大小"))
				size := float32(math.Round(d.VoiceOrbSize))
				ui.Box(c).Size(96, 96).Draw(func(p *ui.Painter, r ui.Rect) {
					now := time.Now()
					level := float32(0.35 + 0.25*math.Sin(float64(now.UnixMilli()%600000)/1000*2.2))
					drawOrb(p, ui.Rect{X: r.X + (r.W-size)/2, Y: r.Y + (r.H-size)/2, W: size, H: size}, level, now, false)
				})
			})
		})
	}
}

// voicePreview 外观 tab 的预览：在一块假桌面上把三种状态并排画出来，选了就能看到，不用保存。
// 用的是跟真浮窗同一个 voicePill，音量是假的
func (a *App) voicePreview(c *ui.Context, d *Config) {
	dark := c.Theme().Dark
	desk, label := ui.RGBA(221, 226, 234, 1), ui.RGBA(30, 32, 38, 0.55)
	if dark {
		desk, label = ui.RGBA(22, 24, 29, 1), ui.RGBA(232, 234, 240, 0.55)
	}
	now := time.Now()
	t := float64(now.UnixMilli()%600000) / 1000
	var bars [voiceBars]float32
	for i := range bars {
		v := 0.45 + 0.35*math.Sin(t*3.1-float64(i)*0.8) + 0.15*math.Sin(t*7.3+float64(i))
		bars[i] = float32(min(1, max(0, v)))
	}
	themeDark := dark
	dark = toneDark(d.VoiceTone, themeDark) // 假桌面跟主题，浮窗按「浮窗底色」
	style, sk := d.VoiceStyle, skinFor(d.VoiceStyle, dark)
	states := []voiceViewState{{recording: true}, {recognizing: true}, {text: T("明天三点开会"), copy: true}}
	captions := []string{T("录音中"), T("识别中…"), T("没地方贴时")}
	h := float32(voiceOrbW) // 图形区固定高度：切到果冻球时预览框不跳
	if style == "orb" {
		h = max(h, float32(math.Round(d.VoiceOrbSize)))
	}
	c.AnimationFrame()
	ui.Row(c).Padding(16, 12).Gap(20).Justify(ui.Center).AlignItems(ui.End).Background(desk).Radius(8).Children(func() {
		for i, st := range states {
			ui.Column(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Height(h).Center().Children(func() { a.previewShape(c, d, style, dark, sk, st, bars, now) })
				ui.Text(c, captions[i]).FontSize(11).TextColor(label)
			})
		}
	})
}

// previewShape 预览里的一个状态：果冻球样式录音/识别时是球，其余是胶囊
func (a *App) previewShape(c *ui.Context, d *Config, style string, dark bool, sk voiceSkin, st voiceViewState, bars [voiceBars]float32, now time.Time) {
	if style == "orb" && voiceKind(st) != 3 {
		sz, rec := float32(math.Round(d.VoiceOrbSize)), st.recognizing
		ui.Box(c).Size(sz, sz).Draw(func(p *ui.Painter, r ui.Rect) { drawOrb(p, r, bars[voiceBars-1], now, rec) })
	} else {
		gl, gr := islandGapsFor(st)
		a.voicePill(c, style, dark, sk, st, bars, bars[voiceBars-1], gl, gr, 1, now, false).Width(float32(min(voiceWidth(st), voiceRecW))) // 预览框放不下全宽的提示，句子长了就截断
	}
}

func (a *App) pagePolish(c *ui.Context, d *Config) {
	pageTitle(c, T("润色"))
	settingRow(c, T("档位"), "", func() {
		optionSelect(c, &d.PolishStyle, []option{{"off", T("关闭")}, {"light", T("轻润色")}, {"standard", T("标准")}, {"structured", T("结构化")}})
	})
	subTitle(c, T("接口"))
	a.apiPolish(c, d)
}

func (a *App) testPolishDraft() {
	cfg := a.set.draft
	cfg.ASRHotwords = splitList(a.set.asrHotwords)
	a.set.polishTesting, a.set.polishTestMsg, a.set.polishTestErr = true, T("润色中…"), false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), polishDeadline(polishSample))
		defer cancel()
		started := time.Now()
		result, err := polish(ctx, cfg, polishSample)
		elapsed := time.Since(started).Milliseconds()
		if err == nil && result == "" {
			err = fmt.Errorf("%s", T("润色结果为空"))
		}
		a.setWin.Update(func() {
			a.set.polishTesting = false
			a.set.polishTestErr = err != nil
			if err != nil {
				result = err.Error()
			}
			a.set.polishTestMsg = fmt.Sprintf("%s · %d ms", result, elapsed)
		})
	}()
}

func (a *App) testVoiceDraft() {
	cfg := a.set.draft
	cfg.ASRHotwords = splitList(a.set.asrHotwords)
	a.set.voiceTesting, a.set.voiceTestMsg, a.set.voiceTestErr = true, T("识别中…"), false
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, err := transcribe(ctx, cfg, make([]byte, 16000))
		a.setWin.Update(func() {
			a.set.voiceTesting = false
			if err != nil {
				a.set.voiceTestMsg, a.set.voiceTestErr = err.Error(), true
			} else {
				a.set.voiceTestMsg, a.set.voiceTestErr = T("语音接口正常"), false
			}
		})
	}()
}

func (a *App) pageTranslate(c *ui.Context, d *Config) {
	pageTitle(c, T("翻译"))
	pageTabs(c, &a.set.transTab, T("译文"), T("接口"))
	if a.set.transTab == 1 {
		a.translateAPI(c, d)
		return
	}
	settingRow(c, T("中文翻成"), T("选中的文字里有汉字时"), func() { langSelect(c, &d.TargetForChinese) })
	settingRow(c, T("其他翻成"), T("不含汉字的文字（英文、日文等）"), func() { langSelect(c, &d.TargetForOther) })
	ui.Column(c).Gap(6).Padding(12, 0).Children(func() {
		ui.Text(c, T("额外要求")).FontSize(14)
		ui.Text(c, T("追加给模型的话，比如风格、术语处理")).FontSize(12).TextColor(c.Theme().TextMuted)
		a.extraPromptBox(c, d)
	})
	ui.Divider(c)
	switchRow(c, T("边翻边显示"), T("模型一边写，卡片一边出字；关掉就等整段译文出来再显示"), &d.StreamCard)
}

func (a *App) pageHotkeys(c *ui.Context, d *Config) {
	pageTitle(c, T("快捷键"))
	switchRow(c, T("启用快捷键"), T("关掉后不注册任何全局快捷键，不会和别的程序抢键；划词按钮照常能用"), &a.set.hotkeysOn)
	ui.Divider(c)
	if !a.set.hotkeysOn {
		return
	}
	settingRow(c, T("原地替换"), T("点一下再按组合键，Backspace 清空"), func() { a.hotkeyBox(c, "hotkey", &d.Hotkey, false) })
	settingRow(c, T("只弹卡片"), T("点一下再按组合键，Backspace 清空"), func() { a.hotkeyBox(c, "popup", &d.PopupHotkey, false) })
	switchRow(c, T("快捷键没选中就全选"), T("按 Alt+T 时没选中文字，先全选整个输入框再翻译"), &d.SelectAllIfEmpty)
}

func (a *App) pageButton(c *ui.Context, d *Config) {
	s := &a.set
	pageTitle(c, T("划词按钮"))
	switchRow(c, T("划词后显示翻译按钮"), T("选中文字后旁边弹出一个小图标，点它出译文"), &d.ShowButton)
	ui.Divider(c)
	switchRow(c, T("只在输入框里弹"), T("网页正文、文档里划词不弹"), &d.OnlyInInputs)
	ui.Divider(c)
	settingRow(c, T("点翻译按钮后"), T("直接替换出错时仍会弹卡片说明原因"), func() { actionSelect(c, &d.ButtonAction) })
	settingRow(c, T("不弹按钮的程序"), T("进程名，逗号分隔"), func() {
		ui.TextInput(c, &s.exclude).Placeholder("game.exe, WindowsTerminal.exe")
	})
	ui.Row(c).Padding(8, 0).Children(func() {
		if textButton(c, s.compatLabel()).Clicked() {
			s.compat = !s.compat
		}
	})
	if s.compat {
		switchRow(c, T("认不出输入框的程序也弹"), T("微信这类程序开着能用，但聊天记录里划词也会弹"), &d.ShowInBlindApps)
		ui.Divider(c)
		switchRow(c, T("只认文本光标"), T("按下鼠标时光标是 I 形才算划词，拖窗口、拖文件不会误弹"), &d.RequireIBeam)
	}
}

func (s *settingsState) compatLabel() string {
	if s.compat {
		return T("▾ 兼容性")
	}
	return T("▸ 兼容性")
}

// targetLangs 是下拉框里的语言：value 原样写进提示词发给模型，label 给人看
var targetLangs = []struct{ value, label string }{
	{"English", "英语 English"},
	{"Simplified Chinese", "简体中文"},
	{"Traditional Chinese", "繁體中文"},
	{"Japanese", "日语 日本語"},
	{"Korean", "韩语 한국어"},
	{"French", "法语 Français"},
	{"German", "德语 Deutsch"},
	{"Spanish", "西班牙语 Español"},
	{"Portuguese", "葡萄牙语 Português"},
	{"Italian", "意大利语 Italiano"},
	{"Russian", "俄语 Русский"},
	{"Arabic", "阿拉伯语 العربية"},
	{"Vietnamese", "越南语 Tiếng Việt"},
	{"Thai", "泰语 ไทย"},
	{"Indonesian", "印尼语 Bahasa Indonesia"},
	{"Malay", "马来语 Bahasa Melayu"},
	{"Hindi", "印地语 हिन्दी"},
	{"Turkish", "土耳其语 Türkçe"},
	{"Dutch", "荷兰语 Nederlands"},
	{"Polish", "波兰语 Polski"},
}

// langSelect 下拉选目标语言。内置的 ui.Select 列表不限高，20 个选项会顶出窗口，
// 所以用 SelectBase 自己画：列表放进限高的滚动区。
// 配置里要是填过列表外的语言，原样保留成第一个选项。
func langSelect(c *ui.Context, value *string) {
	t := c.Theme()
	opts := targetLangs
	label := *value
	known := false
	for _, l := range targetLangs {
		if strings.EqualFold(l.value, strings.TrimSpace(*value)) {
			*value, label, known = l.value, langLabel(l.value, l.label), true
		}
	}
	if !known && strings.TrimSpace(*value) != "" {
		opts = append([]struct{ value, label string }{{*value, *value}}, targetLangs...)
	}
	sel := ui.SelectBase(c, value)
	tr := sel.Trigger.Gap(8).Padding(8, 12).Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Cursor(ui.CursorPointer)
	if tr.Hovered() {
		tr.Background(t.SurfaceHover)
	}
	tr.Children(func() {
		ui.Text(c, label).SingleLine().Grow(1)
		ui.Icon(c, iconChevron).FontSize(14).TextColor(t.TextMuted)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0).Padding(4).Radius(t.Radius+2).Background(t.Background).Border(1, t.Border)
		ui.Scroll(c).Height(langListH).Children(func() {
			ui.Column(c).Gap(1).Children(func() {
				for _, l := range opts {
					item := sel.Item(l.value).Padding(6, 10).Radius(t.Radius)
					switch {
					case item.Highlighted():
						item.Background(t.Accent).TextColor(t.AccentText)
					case l.value == *value:
						item.Background(t.Surface)
					}
					text := langLabel(l.value, l.label)
					item.Children(func() { ui.Text(c, text).SingleLine() })
				}
			})
		})
	})
}

// langListH 下拉列表的可见高度（约 7 行），其余滚动
const langListH = 240

var buttonActions = []struct{ value, label string }{
	{actionCard, "显示卡片（先显示「翻译中…」）"},
	{actionResult, "不显示「翻译中」，出结果再弹"},
	{actionReplace, "直接替换，不弹卡片"},
}

func actionSelect(c *ui.Context, value *string) {
	opts := make([]option, len(buttonActions))
	for i, o := range buttonActions {
		opts[i] = option{o.value, T(o.label)}
	}
	optionSelect(c, value, opts)
}

type option struct{ value, label string }

// optionSelect 下拉选一项：value 是存进配置的值，label 是给人看的
func optionSelect(c *ui.Context, value *string, opts []option) {
	labels := make([]string, len(opts))
	current := opts[0].label
	for i, o := range opts {
		labels[i] = o.label
		if o.value == *value {
			current = o.label
		}
	}
	if ui.Select(c, &current, labels).Changed() {
		for _, o := range opts {
			if o.label == current {
				*value = o.value
			}
		}
	}
}

// switchRow 左边是名字和说明，右边是开关，三者在同一行，不会错位
func switchRow(c *ui.Context, title, desc string, on *bool) {
	ui.Row(c).Gap(16).Padding(12, 0).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(14)
			ui.Text(c, desc).FontSize(12).TextColor(c.Theme().TextMuted)
		})
		ui.Switch(c, on).Label(title)
	})
}

func section(c *ui.Context, title string, fn func()) {
	ui.Column(c).Gap(10).Children(func() {
		ui.Text(c, title).FontSize(13).Bold().TextColor(c.Theme().TextMuted)
		fn()
	})
}

func (a *App) testDraft() {
	s := &a.set
	cfg := s.draft
	cfg.normalize()
	s.testing, s.testMsg, s.testErr = true, T("正在请求 ")+cfg.Model+"…", false
	go func() {
		start := time.Now()
		var res string
		tr, err := newTranslator(cfg)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSec)*time.Second)
			res, err = tr.Translate(ctx, "你好，世界", cfg.TargetForChinese)
			cancel()
		}
		a.setWin.Update(func() {
			s.testing = false
			if err != nil {
				s.testMsg, s.testErr = T("连接失败：")+err.Error(), true
				return
			}
			s.testMsg, s.testErr = fmt.Sprintf(T("连接正常（%s）：你好，世界 → %s"), time.Since(start).Round(time.Millisecond), res), false
		})
	}()
}

func (a *App) saveDraft() {
	s := &a.set
	cfg := s.draft
	cfg.BaseURL, cfg.Model = strings.TrimSpace(cfg.BaseURL), strings.TrimSpace(cfg.Model)
	cfg.APIKey = strings.TrimSpace(cfg.APIKey)
	cfg.ExcludeProcesses = splitList(s.exclude)
	cfg.ASRHotwords = splitList(s.asrHotwords)
	cfg.DisableHotkeys = !s.hotkeysOn
	cfg.normalize()
	if err := a.setConfig(cfg); err != nil {
		s.status, s.statusErr = err.Error(), true
		return
	}
	if err := saveConfig(a.cfgPath, cfg); err != nil {
		s.status, s.statusErr = T("保存失败：")+err.Error(), true
		return
	}
	s.draft = cfg
	if mygo.App.OpenAtLogin() != s.openAtLogin {
		if err := mygo.App.SetOpenAtLogin(s.openAtLogin); err != nil {
			log.Println("open at login:", err)
		}
	}
	a.updateTrayTip()
	go a.hist.prune(cfg.HistoryDays)
	if !cfg.ShowButton {
		a.hideButton()
	}
	if probs := a.registerHotkeys(cfg); len(probs) > 0 {
		s.status, s.statusErr = T("已保存，但快捷键有问题：")+strings.Join(probs, T("；")), true
		return
	}
	s.status, s.statusErr = T("已保存"), false
}

// titleBar 自绘标题栏：整条可拖动，右上角最小化和关闭
func (a *App) titleBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(36).AlignItems(ui.Center).AlignSelf(ui.Stretch).Children(func() {
		ui.Row(c).Grow(1).Gap(8).Padding(0, 0, 0, 12).AlignItems(ui.Center).Height(36).Children(func() {
			if a.avatar != nil {
				ui.Image(c, a.avatar).Size(18, 18).Radius(9).Clip()
			}
			ui.Text(c, "Mami").FontSize(13).Bold()
		}).DragWindow()
		for _, b := range []struct {
			glyph  string
			danger bool
			act    func()
		}{{"–", false, a.setWin.Minimize}, {"✕", true, a.setWin.Close}} {
			btn := ui.Row(c).Width(46).Height(36).Justify(ui.Center).AlignItems(ui.Center).Cursor(ui.CursorPointer)
			if btn.Hovered() {
				if b.danger {
					btn.Background(t.Danger)
				} else {
					btn.Background(t.SurfaceHover)
				}
			}
			glyph := b.glyph
			btn.Children(func() { ui.Text(c, glyph).FontSize(13).TextColor(t.Text) })
			if btn.Clicked() {
				b.act()
			}
		}
	})
}

// 侧边栏图标（Lucide 描边风格，和 iconChevron 一致）
var iconMic = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 2a3 3 0 0 0-3 3v7a3 3 0 0 0 6 0V5a3 3 0 0 0-3-3Z"/><path d="M19 10v2a7 7 0 0 1-14 0v-2"/><path d="M12 19v3"/></svg>`))
var iconPolish = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9.937 15.5A2 2 0 0 0 8.5 14.063l-6.135-1.582a.5.5 0 0 1 0-.962L8.5 9.936A2 2 0 0 0 9.937 8.5l1.582-6.135a.5.5 0 0 1 .963 0L14.063 8.5A2 2 0 0 0 15.5 9.937l6.135 1.581a.5.5 0 0 1 0 .964L15.5 14.063a2 2 0 0 0-1.437 1.437l-1.582 6.135a.5.5 0 0 1-.963 0z"/><path d="M20 3v4"/><path d="M22 5h-4"/></svg>`))
var iconTrans = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m5 8 6 6"/><path d="m4 14 6-6 2-3"/><path d="M2 5h12"/><path d="M7 2h1"/><path d="m22 22-5-10-5 10"/><path d="M14 18h6"/></svg>`))
var iconKeys = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect width="20" height="16" x="2" y="4" rx="2"/><path d="M6 8h.01"/><path d="M10 8h.01"/><path d="M14 8h.01"/><path d="M18 8h.01"/><path d="M8 12h.01"/><path d="M12 12h.01"/><path d="M16 12h.01"/><path d="M7 16h10"/></svg>`))
var iconBtn = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 3h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5"/><path d="M19 3h-2a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h2"/><path d="M9 12h6"/></svg>`))

// jaLangNames 是日语界面下语言的叫法
var jaLangNames = map[string]string{
	"English": "英語", "Simplified Chinese": "中国語（簡体字）", "Traditional Chinese": "中国語（繁体字）",
	"Japanese": "日本語", "Korean": "韓国語", "French": "フランス語", "German": "ドイツ語",
	"Spanish": "スペイン語", "Portuguese": "ポルトガル語", "Italian": "イタリア語", "Russian": "ロシア語",
	"Arabic": "アラビア語", "Vietnamese": "ベトナム語", "Thai": "タイ語", "Indonesian": "インドネシア語",
	"Malay": "マレー語", "Hindi": "ヒンディー語", "Turkish": "トルコ語", "Dutch": "オランダ語", "Polish": "ポーランド語",
}

// langLabel 下拉框里一个语言怎么写：中文界面「英语 English」，英文界面「English」，日语界面「英語 English」。
// zhLabel 是 targetLangs 里的中文写法（「中文名 本地名」，或只有本地名）。
// 列表外的自定义语言原样显示。
func langLabel(value, zhLabel string) string {
	ja, known := jaLangNames[value]
	if !known {
		return zhLabel
	}
	native := zhLabel
	if _, after, ok := strings.Cut(zhLabel, " "); ok {
		native = after
	}
	switch i18n.Current() {
	case i18n.ZH:
		return zhLabel
	case i18n.JA:
		if native == ja {
			return ja
		}
		return ja + " " + native
	}
	// 英语和没有专门名字的其他界面语言：英文名 + 本地名
	if native == value {
		return value
	}
	return value + " " + native
}

func (a *App) pageGeneral(c *ui.Context, d *Config) {
	pageTitle(c, T("通用"))
	langs := make([]option, len(i18n.Choices))
	for i, l := range i18n.Choices {
		langs[i] = option{l.Value, l.Label}
		if l.Value == "auto" {
			langs[i].label = T("跟随系统")
		}
	}
	settingRow(c, T("界面语言"), T("设置窗口、卡片和托盘菜单用哪种语言"), func() { optionSelect(c, &d.Language, langs) })
	settingRow(c, T("外观"), T("浅色、深色，或跟随 Windows"), func() {
		optionSelect(c, &d.Theme, []option{{"auto", T("跟随系统")}, {"light", T("浅色")}, {"dark", T("深色")}})
	})
	switchRow(c, T("开机自启"), T("登录 Windows 后在托盘里自动运行"), &a.set.openAtLogin)
	ui.Divider(c)
	a.updateRow(c)
	switchRow(c, T("保存翻译记录"), T("原文、发给模型的提示词和译文按天存成文件，之后可以交给 AI 分析哪里没翻好"), &d.History)
	ui.Divider(c)
	if !d.History {
		return
	}
	settingRow(c, T("保留时间"), T("更早的记录会自动删除"), func() {
		v := strconv.Itoa(d.HistoryDays)
		optionSelect(c, &v, []option{{"7", T("7 天")}, {"30", T("30 天")}, {"90", T("90 天")}, {"0", T("永久")}})
		d.HistoryDays, _ = strconv.Atoi(v)
	})
	ui.Row(c).Padding(12, 0).Children(func() {
		ui.Text(c, T("记录只存在这台电脑上，终端里的翻译不记。")).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1)
		if textButton(c, T("打开记录文件夹")).Clicked() {
			a.openHistory()
		}
	})
}

// updateRow 「更新」一行：说明里是当前版本/检查结果/下载进度，右边按钮查更新或装新版
func (a *App) updateRow(c *ui.Context) {
	checking, checked, latest, errMsg, busy, progress := a.upd.snapshot()
	cur := mygo.App.Version()
	if cur == "" {
		cur = T("开发版")
	}
	desc := fmt.Sprintf(T("当前版本 %s"), cur)
	switch {
	case busy:
		desc = fmt.Sprintf(T("正在下载 %s… %d%%"), latest.version, int(progress*100))
	case errMsg != "":
		desc = errMsg
	case checking:
		desc = T("正在检查…")
	case latest != nil:
		desc = fmt.Sprintf(T("有新版本 %s（当前 %s），更新后会自动重启"), latest.version, cur)
	case checked:
		desc = fmt.Sprintf(T("已是最新版（%s）"), cur)
	}
	// 不用 settingRow：它给控件固定 280 宽，按钮会被拉成长条；这里按钮按文字宽度、靠右
	ui.Row(c).Gap(16).Padding(12, 0).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, T("更新")).FontSize(14)
			ui.Text(c, desc).FontSize(12).TextColor(c.Theme().TextMuted)
		})
		if latest != nil {
			if accentButton(c, fmt.Sprintf(T("更新到 %s"), latest.version)).Disabled(busy).Clicked() {
				a.startUpdate()
			}
			return
		}
		if textButton(c, T("检查更新")).Disabled(checking).Clicked() {
			go a.checkUpdate(true)
		}
	})
	ui.Divider(c)
}

// extraPromptBox 额外要求输入框：默认按内容自动长短，右下角的把手可以拖着调高度，点一下把手恢复自动
func (a *App) extraPromptBox(c *ui.Context, d *Config) {
	s := &a.set
	t := c.Theme()
	const minH, maxH = 80, 280
	h := s.extraH
	if !s.extraManual {
		w, _ := c.Size()
		_, th := c.MeasureText(max(w-controlPad, 120), ui.Span{Text: d.ExtraPrompt + "\n", Size: 14})
		h = min(max(th+24, minH), maxH)
	}
	ui.TextArea(c, &d.ExtraPrompt).Height(h).Placeholder(T("比如：技术文档风格，术语保留英文"))
	ui.Row(c).Height(14).Justify(ui.End).Children(func() {
		grip := ui.Box(c).Size(36, 14).Cursor(ui.CursorResizeNS).Tooltip(T("拖动调整高度，点一下恢复自动"))
		_, dy, pressed := grip.Dragged()
		clicked := grip.Clicked()
		if pressed {
			if dy != 0 {
				s.extraManual, s.gripMoved = true, true
				s.extraH = min(max(h+dy, minH), maxH)
			}
		} else {
			// 松手那一帧把手还在指针下面，会算成一次点击：拖过就不算
			if clicked && !s.gripMoved {
				s.extraManual = false
			}
			s.gripMoved = false
		}
		muted := t.TextMuted
		grip.Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + 8, Y: r.Y + 4, W: 20, H: 2}, muted, 1)
			p.Fill(ui.Rect{X: r.X + 8, Y: r.Y + 8, W: 20, H: 2}, muted, 1)
		})
	})
}

// sidebarW 侧边栏宽度；controlPad 是额外要求框的文字区离窗口左右的距离之和（侧边栏 + 页面左右留白 48 + 输入框内边距）
const sidebarW = 212

// settingsW, settingsH 设置窗口的默认尺寸；侧边栏放得下「Auswahl-Schaltfläche」这类长词
const settingsW, settingsH = 732, 480

const controlPad = sidebarW + 48 + 28

var iconGen = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 4h-7"/><path d="M10 4H3"/><path d="M21 12h-9"/><path d="M8 12H3"/><path d="M21 20h-5"/><path d="M12 20H3"/><path d="M14 2v4"/><path d="M8 10v4"/><path d="M16 18v4"/></svg>`))

// apiASR 「语音识别」页的接口区
func (a *App) apiASR(c *ui.Context, d *Config) {
	cur := "custom"
	for _, p := range asrProviders {
		if strings.EqualFold(d.ASRBaseURL, p.base) {
			cur = p.id
		}
	}
	settingRow(c, T("服务商"), T("选一个会自动填好接口地址和模型"), func() {
		v := cur
		opts := []option{{"stepfun", T("阶跃星辰 StepFun")}, {"qwen", T("阿里云百炼 · 千问 ASR")}}
		if cur == "custom" { // 自定义就是直接改下面的地址，选项只用来显示
			opts = append(opts, option{"custom", T("自定义")})
		}
		optionSelect(c, &v, opts)
		for _, p := range asrProviders {
			if v != cur && p.id == v {
				d.ASRBaseURL, d.ASRModel, d.ASRAPIKey = p.base, p.model, "" // Key 各家不通用
			}
		}
	})
	settingRow(c, T("接口地址"), T("语音始终直连，不使用代理"), func() { ui.TextInput(c, &d.ASRBaseURL) })
	settingRow(c, T("模型"), "", func() { ui.TextInput(c, &d.ASRModel) })
	settingRow(c, "API Key", T("换服务商要重新填"), func() { ui.TextInput(c, &d.ASRAPIKey).Password().Placeholder("sk-…") })
	ui.Row(c).Padding(12, 0).Gap(12).Children(func() {
		color := c.Theme().TextMuted
		if a.set.voiceTestErr {
			color = c.Theme().Danger
		}
		ui.Text(c, a.set.voiceTestMsg).FontSize(12).TextColor(color).Grow(1).MaxLines(2)
		if textButton(c, T("测试语音接口")).Disabled(a.set.voiceTesting).Clicked() {
			a.testVoiceDraft()
		}
	})
}

// asrProviders 语音识别的预设；不是阶跃的地址都按 OpenAI 兼容 chat 接口发音频（见 transcribeChat）
var asrProviders = []struct{ id, base, model string }{
	{"stepfun", "https://api.stepfun.com/v1", "stepaudio-3-asr-max"},
	{"qwen", "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen3-asr-flash"},
}

// apiPolish 「润色」页的接口区
func (a *App) apiPolish(c *ui.Context, d *Config) {
	settingRow(c, T("接口地址"), T("润色始终直连，不使用代理"), func() { ui.TextInput(c, &d.PolishBaseURL) })
	settingRow(c, T("模型"), "", func() { ui.TextInput(c, &d.PolishModel) })
	settingRow(c, "API Key", T("OpenAI 兼容接口都能用"), func() { ui.TextInput(c, &d.PolishAPIKey).Password().Placeholder("sk-…") })
	ui.Row(c).Padding(12, 0).Gap(12).Children(func() {
		color := c.Theme().TextMuted
		if a.set.polishTestErr {
			color = c.Theme().Danger
		}
		ui.Text(c, a.set.polishTestMsg).FontSize(12).TextColor(color).Grow(1).MaxLines(2)
		if textButton(c, T("测试润色")).Disabled(a.set.polishTesting).Clicked() {
			a.testPolishDraft()
		}
	})
}

// hotkeyBox 快捷键框：点一下开始录，直接按下新的键就记上；Esc 取消，Backspace 清空。
// 录制靠键盘钩子（captureKey），所以能分清左右 Alt，录的时候现有快捷键也不会触发。
func (a *App) hotkeyBox(c *ui.Context, id string, value *string, voice bool) {
	s := &a.set
	t := c.Theme()
	rec := s.recID == id
	box := ui.Row(c).Height(38).Gap(6).Padding(0, 12).AlignItems(ui.Center).Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Cursor(ui.CursorPointer)
	switch {
	case rec:
		box.Border(1, t.Accent)
	case box.Hovered():
		box.Background(t.SurfaceHover)
	}
	box.Children(func() {
		if rec {
			hint := s.recHint
			if hint == "" {
				hint = T("按下新的快捷键…")
				if voice {
					hint = T("按一个键或组合键…")
				}
			}
			ui.Text(c, hint).FontSize(13).TextColor(t.Accent).SingleLine().Grow(1)
			return
		}
		v := strings.TrimSpace(*value)
		if v == "" || v == "Off" {
			ui.Text(c, T("未设置")).TextColor(t.TextMuted).SingleLine().Grow(1)
			return
		}
		for _, part := range strings.Split(v, "+") {
			w, _ := c.MeasureText(200, ui.Span{Text: part, Size: 13})
			ui.Box(c).Shrink(0).Padding(2, 8).Radius(4).Background(t.Background).Border(1, t.Border).Children(func() {
				ui.Text(c, part).FontSize(13).SingleLine().MinWidth(w + 2) // 量出来的宽度偏紧，RAlt 会被省略成 R…
			})
		}
	})
	if box.Clicked() {
		if rec {
			a.stopCapture()
		} else {
			a.startCapture(id, value, voice)
		}
	}
}

func (a *App) startCapture(id string, value *string, voice bool) {
	s := &a.set
	job := &keyCaptureJob{keyCapture: keyCapture{voice: voice}, hwnd: a.setWin.NativeHandle()}
	job.report = func(state int, acc, hint string) {
		a.setWin.Update(func() {
			if s.recJob != job { // 已经取消或换了别的框；乱序到达的旧消息也在这里丢掉
				return
			}
			switch state {
			case captureDone:
				*value = acc
				a.stopCapture()
			case captureCancel:
				a.stopCapture()
			default:
				s.recHint = hint
			}
		})
	}
	s.recID, s.recJob, s.recHint = id, job, ""
	keyCapturing.Store(job)
}

func (a *App) stopCapture() {
	s := &a.set
	if s.recJob != nil {
		keyCapturing.CompareAndSwap(s.recJob, nil)
	}
	s.recID, s.recJob, s.recHint = "", nil, ""
}

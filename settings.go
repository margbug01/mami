//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// settingsState 是设置窗口里正在改、还没保存的那份配置（主线程）
type settingsState struct {
	draft       Config
	exclude     string
	openAtLogin bool
	status      string
	statusErr   bool
	testing     bool
	page        string
	extraH      float32 // 额外要求输入框的高度（手动拖过之后才用）
	extraManual bool
	gripMoved   bool // 这次按住把手时拖动过
	advanced    bool
	compat      bool
	testMsg     string
	testErr     bool
}

func (a *App) openSettings() {
	cfg, _ := a.config()
	reset := func() {
		a.set = settingsState{
			draft:       cfg,
			exclude:     strings.Join(cfg.ExcludeProcesses, ", "),
			openAtLogin: mygo.App.OpenAtLogin(),
		}
	}
	if a.setWin == nil {
		reset()
		a.setWin = mygo.NewWindow(mygo.WindowOptions{
			Title:           "Mami · Settings",
			Width:           700,
			Height:          480,
			MinWidth:        640,
			MinHeight:       420,
			DisableMaximize: true,
			Frameless:       true,
			Content:         ui.View(a.settingsView),
		})
		a.setWin.SetIcon(iconPNG)
		a.setWin.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting {
				e.PreventDefault()
				a.setWin.Hide()
				saved, _ := a.config()
				setLang(resolveLang(saved.Language, sysLocale)) // 没保存的语言预览收回
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
	setLang(resolveLang(d.Language, sysLocale)) // 在设置里改了立刻预览，保存才算数
	applyTheme(c, d.Theme)
	t = c.Theme()
	ui.Column(c).Fill().Children(func() {
		a.titleBar(c)
		ui.Divider(c)
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(sidebarW).Children(func() {
				ui.Sidebar(c, &s.page, func() {
					ui.Box(c).Height(8)
					ui.SidebarItem(c, pageConn, iconConn, T("接口"))
					ui.SidebarItem(c, pageTrans, iconTrans, T("翻译"))
					ui.SidebarItem(c, pageKeys, iconKeys, T("快捷键"))
					ui.SidebarItem(c, pageBtn, iconBtn, T("划词按钮"))
					ui.SidebarItem(c, pageGen, iconGen, T("通用"))
				}).Grow(1)
			})
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).Padding(8, 24, 16, 24).Children(func() {
					switch s.page {
					case pageTrans:
						a.pageTranslate(c, d)
					case pageKeys:
						a.pageHotkeys(c, d)
					case pageBtn:
						a.pageButton(c, d)
					case pageGen:
						a.pageGeneral(c, d)
					default:
						a.pageConnection(c, d)
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
			if ui.PrimaryButton(c, T("保存")).Clicked() {
				a.saveDraft()
			}
		})
	})
}

const (
	pageConn  = "conn"
	pageTrans = "trans"
	pageKeys  = "keys"
	pageBtn   = "btn"
	pageGen   = "gen"
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

func (a *App) pageConnection(c *ui.Context, d *Config) {
	s := &a.set
	t := c.Theme()
	pageTitle(c, T("接口（OpenAI 兼容）"))
	settingRow(c, T("接口地址"), T("会自动拼上 /chat/completions"), func() {
		ui.TextInput(c, &d.BaseURL).Placeholder("https://api.deepseek.com/v1")
	})
	settingRow(c, "API Key", T("免费接口留空"), func() {
		ui.TextInput(c, &d.APIKey).Password().Placeholder("sk-…")
	})
	settingRow(c, T("模型"), "", func() { ui.TextInput(c, &d.Model).Placeholder("deepseek-flash") })
	ui.Row(c).Gap(12).Padding(12, 0).Children(func() {
		color := t.TextMuted
		if s.testErr {
			color = t.Danger
		}
		ui.Text(c, s.testMsg).FontSize(12).TextColor(color).Grow(1).MaxLines(2)
		if ui.Button(c, T("测试连接")).Disabled(s.testing).Clicked() {
			a.testDraft()
		}
	})
	ui.Row(c).Padding(8, 0).Children(func() {
		if ui.Button(c, s.advancedLabel()).Clicked() {
			s.advanced = !s.advanced
		}
	})
	if s.advanced {
		settingRow(c, T("代理"), T("Go 不读系统代理，要走 Clash 得填这里"), func() {
			ui.TextInput(c, &d.Proxy).Placeholder("http://127.0.0.1:7890")
		})
	}
}

func (a *App) pageTranslate(c *ui.Context, d *Config) {
	pageTitle(c, T("翻译"))
	settingRow(c, T("中文翻成"), T("选中的文字里有汉字时"), func() { langSelect(c, &d.TargetForChinese) })
	settingRow(c, T("其他翻成"), T("不含汉字的文字（英文、日文等）"), func() { langSelect(c, &d.TargetForOther) })
	ui.Column(c).Gap(6).Padding(12, 0).Children(func() {
		ui.Text(c, T("额外要求")).FontSize(14)
		ui.Text(c, T("追加给模型的话，比如风格、术语处理")).FontSize(12).TextColor(c.Theme().TextMuted)
		a.extraPromptBox(c, d)
	})
}

func (a *App) pageHotkeys(c *ui.Context, d *Config) {
	pageTitle(c, T("快捷键"))
	settingRow(c, T("原地替换"), T("写法如 Ctrl+Shift+Y、Alt+F2"), func() { ui.TextInput(c, &d.Hotkey).Placeholder("Alt+T") })
	settingRow(c, T("只弹卡片"), T("留空表示不用"), func() { ui.TextInput(c, &d.PopupHotkey).Placeholder("Alt+Shift+T") })
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
		if ui.Button(c, s.compatLabel()).Clicked() {
			s.compat = !s.compat
		}
	})
	if s.compat {
		switchRow(c, T("认不出输入框的程序也弹"), T("微信这类程序开着能用，但聊天记录里划词也会弹"), &d.ShowInBlindApps)
		ui.Divider(c)
		switchRow(c, T("只认文本光标"), T("按下鼠标时光标是 I 形才算划词，拖窗口、拖文件不会误弹"), &d.RequireIBeam)
	}
}

func (s *settingsState) advancedLabel() string {
	if s.advanced {
		return T("▾ 高级")
	}
	return T("▸ 高级")
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
		panel.Margin(4, 0, 0, 0).Padding(4).Radius(t.Radius+2).Background(t.Background).Border(1, t.Border).
			Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, 0.35))
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
var iconConn = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22v-5"/><path d="M9 8V2"/><path d="M15 8V2"/><path d="M18 8v5a4 4 0 0 1-4 4h-4a4 4 0 0 1-4-4V8Z"/></svg>`))
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
	switch curLang.Load() {
	case langZH:
		return zhLabel
	case langJA:
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
	langs := make([]option, len(languageChoices))
	for i, l := range languageChoices {
		langs[i] = option{l.value, l.label}
		if l.value == "auto" {
			langs[i].label = T("跟随系统")
		}
	}
	settingRow(c, T("界面语言"), T("设置窗口、卡片和托盘菜单用哪种语言"), func() { optionSelect(c, &d.Language, langs) })
	settingRow(c, T("外观"), T("浅色、深色，或跟随 Windows"), func() {
		optionSelect(c, &d.Theme, []option{{"auto", T("跟随系统")}, {"light", T("浅色")}, {"dark", T("深色")}})
	})
	switchRow(c, T("开机自启"), T("登录 Windows 后在托盘里自动运行"), &a.set.openAtLogin)
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
const sidebarW = 180

const controlPad = sidebarW + 48 + 28

var iconGen = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 4h-7"/><path d="M10 4H3"/><path d="M21 12h-9"/><path d="M8 12H3"/><path d="M21 20h-5"/><path d="M12 20H3"/><path d="M14 2v4"/><path d="M8 10v4"/><path d="M16 18v4"/></svg>`))

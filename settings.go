//go:build windows

package main

import (
	"context"
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
			Title:           "划词翻译 · 设置",
			Width:           560,
			Height:          700,
			MinWidth:        480,
			MinHeight:       420,
			DisableMaximize: true,
			Content:         ui.View(a.settingsView),
		})
		a.setWin.SetIcon(iconPNG)
		a.setWin.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting {
				e.PreventDefault()
				a.setWin.Hide()
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
	ui.Column(c).Fill().Children(func() {
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(20, 24).Gap(18).Children(func() {
				ui.Row(c).Gap(12).Children(func() {
					if a.avatar != nil {
						ui.Image(c, a.avatar).Size(40, 40).Radius(20).Clip()
					}
					ui.Column(c).Gap(2).Children(func() {
						ui.Text(c, "划词翻译").FontSize(18).Bold()
						ui.Text(c, "选中文字点「翻译」，或按快捷键原地替换。").FontSize(12).TextColor(t.TextMuted)
					})
				})

				section(c, "接口（OpenAI 兼容）", func() {
					ui.Form(c, func() {
						ui.Field(c, "接口地址", func() {
							ui.TextInput(c, &d.BaseURL).Placeholder("https://api.deepseek.com/v1")
						}).Description("会自动拼上 /chat/completions")
						ui.Field(c, "API Key", func() {
							ui.TextInput(c, &d.APIKey).Password().Placeholder("sk-…（B 站免费接口留空）")
						})
						ui.Field(c, "模型", func() { ui.TextInput(c, &d.Model).Placeholder("deepseek-flash") })
						ui.Field(c, "代理", func() {
							ui.TextInput(c, &d.Proxy).Placeholder("留空直连，例如 http://127.0.0.1:7890")
						}).Description("Go 不读系统代理，要走 Clash 得填这里")
					})
				})

				section(c, "翻译", func() {
					ui.Form(c, func() {
						ui.Field(c, "中文翻成", func() { langSelect(c, &d.TargetForChinese) }).
							Description("选中的文字里有汉字时，翻成这个语言")
						ui.Field(c, "其他翻成", func() { langSelect(c, &d.TargetForOther) }).
							Description("不含汉字的文字（英文、日文等）翻成这个语言")
						ui.Field(c, "额外要求", func() {
							ui.TextArea(c, &d.ExtraPrompt).Height(64).Placeholder("比如：技术文档风格，术语保留英文")
						})
					})
				})

				section(c, "快捷键", func() {
					ui.Form(c, func() {
						ui.Field(c, "原地替换", func() { ui.TextInput(c, &d.Hotkey).Placeholder("Alt+T") })
						ui.Field(c, "只弹卡片", func() { ui.TextInput(c, &d.PopupHotkey).Placeholder("Alt+Shift+T") }).
							Description("写法如 Ctrl+Shift+Y、Alt+F2，留空表示不用")
					})
				})

				section(c, "划词按钮", func() {
					ui.Column(c).Gap(14).Children(func() {
						switchRow(c, "划词后显示翻译按钮", "选中文字后旁边弹出一个小图标，点它出译文。关掉就只能用快捷键", &d.ShowButton)
						switchRow(c, "只在输入框里弹", "网页正文、文档里划词不弹；关掉就是哪里划词都弹", &d.OnlyInInputs)
						switchRow(c, "认不出输入框的程序也弹", "微信这类程序不告诉系统哪里是输入框，开着能用但聊天记录里划词也会弹；关掉就只能用快捷键", &d.ShowInBlindApps)
						switchRow(c, "只认文本光标", "按下鼠标时光标是 I 形才算划词，拖窗口、拖文件不会误弹", &d.RequireIBeam)
						switchRow(c, "快捷键没选中就全选", "按 Alt+T 时没选中文字，先全选整个输入框再翻译", &d.SelectAllIfEmpty)
					})
					ui.Form(c, func() {
						ui.Field(c, "点翻译按钮后", func() { actionSelect(c, &d.ButtonAction) }).
							Description("不想看到「翻译中…」就选后两个；直接替换出错时仍会弹卡片说明原因")
						ui.Field(c, "不弹按钮的程序", func() {
							ui.TextInput(c, &s.exclude).Placeholder("game.exe, WindowsTerminal.exe")
						}).Description("填进程名，逗号分隔")
					})
				})

				section(c, "其他", func() {
					switchRow(c, "开机自启", "登录 Windows 后在托盘里自动运行", &s.openAtLogin)
				})
			})
		})
		ui.Divider(c)
		ui.Row(c).Padding(12, 24).Gap(8).Children(func() {
			color := t.TextMuted
			if s.statusErr {
				color = t.Danger
			}
			ui.Text(c, s.status).FontSize(12).TextColor(color).Grow(1).MaxLines(2)
			if ui.Button(c, "测试连接").Disabled(s.testing).Clicked() {
				a.testDraft()
			}
			if ui.PrimaryButton(c, "保存").Clicked() {
				a.saveDraft()
			}
		})
	})
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
			*value, label, known = l.value, l.label, true
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
					text := l.label
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
	labels := make([]string, len(buttonActions))
	current := buttonActions[0].label
	for i, o := range buttonActions {
		labels[i] = o.label
		if o.value == *value {
			current = o.label
		}
	}
	if ui.Select(c, &current, labels).Changed() {
		for _, o := range buttonActions {
			if o.label == current {
				*value = o.value
			}
		}
	}
}

// switchRow 左边是名字和说明，右边是开关，三者在同一行，不会错位
func switchRow(c *ui.Context, title, desc string, on *bool) {
	ui.Row(c).Gap(16).Children(func() {
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
	s.testing, s.status, s.statusErr = true, "正在请求 "+cfg.Model+"…", false
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
				s.status, s.statusErr = "连接失败："+err.Error(), true
				return
			}
			s.status, s.statusErr = "连接正常（"+time.Since(start).Round(time.Millisecond).String()+"）：你好，世界 → "+res, false
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
		s.status, s.statusErr = "保存失败："+err.Error(), true
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
		s.status, s.statusErr = "已保存，但快捷键有问题："+strings.Join(probs, "；"), true
		return
	}
	s.status, s.statusErr = "已保存", false
}

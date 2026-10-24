//go:build windows

package app

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"mami/internal/i18n"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// settingsState 是设置窗口里正在改、还没保存的那份配置（主线程）
type settingsState struct {
	draft                 Config
	exclude               string
	openAtLogin           bool
	hotkeysOn             bool // 「启用快捷键」开关，保存时写回 DisableHotkeys 的反面
	status                string
	statusErr             bool
	testing               bool
	page                  string
	extraH                float32 // 额外要求输入框的高度（手动拖过之后才用）
	extraManual           bool
	gripMoved             bool // 这次按住把手时拖动过
	polishTab             int
	transTab              int    // 「翻译」页：0 译文、1 接口
	voiceTab              int    // 「语音识别」页：0 通用、1 外观、2 接口
	wordAdd               bool   // 词库：「+ 添加」点开成了输入框
	editWord              string // 词库：正在原地改的那个词
	editText              string
	editSeen              bool     // 输入框拿到过焦点，之后失焦就算提交
	editAliases           []string // 词库编辑面板里还留着的听法，点 ✕ 去掉
	aliasNew              string   // 编辑面板里正在手加的听法
	aliasAdding           bool
	wordsMsg              string
	wordsErr              bool
	stats                 *historyStats // 「记录」页的统计，打开设置窗口时清空、5 秒内复用
	customTrans           bool          // 接口区手动选了自定义（地址还是某家预设的也不跳回去）
	customASR             bool
	customPolish          bool
	testMsg               string
	testErr               bool
	polishTesting         bool
	histTab               int // 「记录」页：0 语音、1 翻译
	voiceTesting          bool
	voiceTestMsg          string
	voiceTestErr          bool
	recID                 string         // 正在录的快捷键框
	recJob                *keyCaptureJob // 交给键盘钩子的那次录制
	recHint               string
	historyDeleteRevision uint64
	copiedID              string
	copiedAt              time.Time
	polishTestMsg         string
	polishTestErr         bool
	guide                 bool // 新手引导占着内容区（guide.go）
	guideStep             int
	voice                 guideVoiceState
	trans                 guideTransState
}

func (a *App) openSettings() {
	cfg, _ := a.config()
	reset := func() {
		keyCapturing.Store(nil)
		a.set = settingsState{
			draft:       cfg,
			exclude:     strings.Join(cfg.ExcludeProcesses, ", "),
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
			StateKey:        "settings", // 记住上次的大小和位置（mygo 存在 window-state.json，退出时写盘），下次打开还是那么大
			Content:         ui.View(a.settingsView),
		})
		a.setWin.SetIcon(iconPNG)
		a.setHwnd.Store(a.setWin.NativeHandle())
		a.setWin.OnClose(func(e *mygo.CloseEvent) {
			if !a.quitting {
				e.PreventDefault()
				a.stopCapture()
				a.guideIdle() // 引导里关窗口也算跳过：别留着麦克风和快捷键接管
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
	if s.guide {
		a.guideScreen(c)
		return
	}
	// 整窗一张雾灰颗粒底；侧边栏和内容区都不铺底，只有分组卡片是半透明的白
	ui.Row(c).Fill().AlignItems(ui.Stretch).Draw(func(p *ui.Painter, r ui.Rect) { windowTexture(t.Dark).draw(p, r, 0) }).Children(func() {
		base := t
		c.SetTheme(sidebarTheme(base))
		line := paneBorder(t)
		ui.Column(c).Width(sidebarW).Draw(func(p *ui.Painter, r ui.Rect) {
			p.Fill(ui.Rect{X: r.X + r.W - 1, Y: r.Y, W: 1, H: r.H}, line, 0)
		}).Children(func() {
			a.brandMark(c)
			ui.Sidebar(c, &s.page, func() {
				ui.SidebarSection(c, T("划词翻译"), nil, func() {
					a.sideItem(c, pageTrans, iconTrans, T("翻译"))
					a.sideItem(c, pageBtn, iconBtn, T("划词按钮"))
					a.sideItem(c, pageKeys, iconKeys, T("快捷键"))
				})
				ui.SidebarSection(c, T("语音输入"), nil, func() {
					a.sideItem(c, pageVoice, iconMic, T("语音识别"))
					a.sideItem(c, pagePolish, iconPolish, T("润色"))
					a.sideItem(c, pageWords, iconWords, T("词库"))
				})
				ui.Box(c).Height(12)
				a.sideItem(c, pageHist, iconHist, T("记录"))
				a.sideItem(c, pageGen, iconGen, T("通用"))
			}).Grow(1)
		})
		c.SetTheme(base)
		ui.Column(c).Grow(1).Children(func() {
			a.titleBar(c)
			ui.Scroll(c).Grow(1).Children(func() {
				// 每页、每个 tab 单独认控件：按位置认的话，上一页同位置的多行框（热词）会被沿用，Password 对它无效，Key 就明文了
				ui.Column(c).Padding(0, 28, 16, 28).Key(fmt.Sprint(s.page, s.transTab, s.voiceTab, s.polishTab, s.histTab)).Transition(ui.ElementTransition{
					Duration: 160 * time.Millisecond, Colors: true, Enter: &ui.Motion{Y: 6},
				}).Children(func() {
					switch s.page {
					case pageVoice:
						a.pageVoice(c, d)
					case pageWords:
						a.pageWords(c, d)
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
			ui.Row(c).Padding(10, 28, 14, 28).Gap(8).AlignItems(ui.Center).Children(func() {
				color := t.TextMuted
				if s.statusErr {
					color = t.Danger
				}
				pending, _, failed := a.hist.deletionState()
				if len(pending) > 0 {
					c.After(100 * time.Millisecond)
					ui.Text(c, fmt.Sprintf(T("已删除 %d 条记录"), len(pending))).FontSize(12).TextColor(t.TextMuted)
					if smallLink(c, T("撤销")).Clicked() {
						a.hist.undoDeletes()
					}
					ui.Spacer(c)
				} else if failed {
					ui.Text(c, T("删除失败，请重试")).FontSize(12).TextColor(t.Danger).Grow(1)
				} else {
					ui.Text(c, s.status).FontSize(12).TextColor(color).Grow(1).MaxLines(2)
				}
				if accentButton(c, T("保存")).MinWidth(88).Clicked() {
					a.saveDraft()
				}
			})
		})
	})
}

// brandMark 侧边栏顶上：头像 + Mami + 版本号，安安静静的一行。整块能拖窗口
func (a *App) brandMark(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Padding(18, 18, 8, 18).Gap(10).AlignItems(ui.Center).Children(func() {
		if a.avatar != nil {
			ui.Image(c, a.avatar).Size(28, 28).Radius(14).Clip().Border(1, t.Border).
				Shadow(0, 2, 6, 0, ui.RGBA(10, 20, 35, 0.08)).Shrink(0)
		}
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, "Mami").FontSize(15).FontWeight(600).TextColor(t.Text).SingleLine()
			if v := mygo.App.Version(); v != "" {
				ui.Text(c, strings.TrimPrefix(v, "v")).FontSize(11).TextColor(t.TextMuted.Alpha(0.75)).SingleLine()
			}
		})
	}).DragWindow()
}

// sideItem 侧边栏一项。选中的那项底盖成半透明的白再带一点影子：mygo 拿焦点时会拿 Accent 铺满，太重
func (a *App) sideItem(c *ui.Context, id string, icon *ui.SVG, label string) {
	e := ui.SidebarItem(c, id, icon, label).Transition(ui.ElementTransition{Duration: 120 * time.Millisecond, Colors: true})
	if a.set.page == id {
		e.Background(c.Theme().SurfacePressed).Shadow(0, 1, 3, 0, ui.RGBA(10, 20, 35, 0.07)).FontWeight(600)
	}
}

const (
	pageTrans  = "trans"
	pageKeys   = "keys"
	pageBtn    = "btn"
	pageVoice  = "voice"
	pagePolish = "polish"
	pageWords  = "words"
	pageGen    = "gen"
)

// settingRow 一行设置：左边名字+一句说明，右边控件。放在 settingGroup 卡片里，行之间不画线
func settingRow(c *ui.Context, title, desc string, ctl func()) {
	settingRowW(c, title, desc, controlW, ctl)
}

func settingRowW(c *ui.Context, title, desc string, w float32, ctl func()) {
	t := c.Theme()
	ui.Row(c).Gap(16).Padding(12, 0).Children(func() {
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(14)
			if desc != "" {
				ui.Text(c, desc).FontSize(12).TextColor(t.TextMuted)
			}
		})
		ui.Box(c).Width(w).Children(ctl)
	})
}

// settingGroup 一组相关设置：上面一个小灰字组名，下面一块圆角卡片；页里只有一组时组名留空
func settingGroup(c *ui.Context, title string, fn func()) {
	t := c.Theme()
	if title == "" {
		ui.Box(c).Height(8)
	} else {
		ui.Text(c, title).FontSize(12).TextColor(t.TextMuted).Padding(18, 4, 8, 4)
	}
	ui.Column(c).Padding(4, 16).Radius(12).Background(paneColor(t)).Border(1, paneBorder(t)).
		Shadow(0, 1, 2, 0, ui.RGBA(10, 20, 35, 0.03)).Children(fn)
}

// selectW 只放几个字的下拉框用这个宽度
const selectW = 180

// controlW 右侧控件的宽度（卡片两边各占 16，再宽左边说明就挤成好几行）
const controlW = 240

func pageTitle(c *ui.Context, title string) {
	ui.Text(c, title).FontSize(21).FontWeight(600).Padding(0, 0, 8, 0)
}

// pageTabs 页标题下的分段切换，页面太长时按用途拆开
func pageTabs(c *ui.Context, selected *int, labels ...string) {
	ui.Row(c).Padding(4, 0, 8, 0).Children(func() {
		segmented(c, selected, labels...)
	})
}

// apiPreset 接口区的一家服务商：点一下填好地址和模型；noKey 的不用填 Key，note 是选中后下面那行小字
type apiPreset struct {
	id, label, base, model, keyURL, note string
	noKey                                bool
	logo                                 *ui.SVG // 名字前面的厂商 logo，自定义没有
}

// custom 是最后一格：地址自己填
func customPreset() apiPreset { return apiPreset{id: "custom", label: T("自定义")} }

var translatePresets = func() []apiPreset {
	return []apiPreset{
		{id: "bilibili", label: T("B 站"), logo: logoBilibili, base: "https://index-translate.bilibili.com/v1", model: "Index-Translate-35B-A3B", note: T("免费、不用 Key，但高峰期常超时"), noKey: true},
		{id: "tokenhub", label: "TokenHub", logo: logoTencent, base: "https://tokenhub.tencentmaas.com/v1", model: "hy-mt2-pro"},
		{id: "openrouter", label: "OpenRouter", logo: logoOpenRouter, base: "https://openrouter.ai/api/v1", model: "tencent/hy-mt2-30b-a3b", keyURL: "https://openrouter.ai/keys"},
		{id: "deepseek", label: "DeepSeek", logo: logoDeepSeek, base: "https://api.deepseek.com/v1", model: "deepseek-flash", keyURL: "https://platform.deepseek.com/api_keys"},
		customPreset(),
	}
}

// asrPresets 不是阶跃的地址都按 OpenAI 兼容 chat 接口发音频（见 transcribeChat）
var asrPresets = func() []apiPreset {
	return []apiPreset{
		{id: "stepfun", label: T("阶跃星辰"), logo: logoStepFun, base: "https://api.stepfun.com/v1", model: "stepaudio-3-asr-max", keyURL: "https://platform.stepfun.com/interface-key"},
		{id: "qwen", label: T("阿里云百炼"), logo: logoBailian, base: "https://dashscope.aliyuncs.com/compatible-mode/v1", model: "qwen-audio-3.1-asr-flash-streaming", keyURL: "https://bailian.console.aliyun.com/"},
		customPreset(),
	}
}

var polishPresets = func() []apiPreset {
	return []apiPreset{
		{id: "deepseek", label: "DeepSeek", logo: logoDeepSeek, base: "https://api.deepseek.com/v1", model: "deepseek-flash", keyURL: "https://platform.deepseek.com/api_keys"},
		{id: "qwen", label: T("阿里云百炼"), logo: logoBailian, base: "https://dashscope.aliyuncs.com/compatible-mode/v1", model: "qwen-flash", keyURL: "https://bailian.console.aliyun.com/"},
		customPreset(),
	}
}

// presetOf 按接口地址认出是哪一家，认不出或者手动选了自定义就是 custom
func presetOf(ps []apiPreset, base string, custom bool) apiPreset {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	for _, p := range ps {
		if !custom && p.base != "" && strings.EqualFold(b, p.base) {
			return p
		}
	}
	return customPreset()
}

// apiFields 一块接口卡片的上半截：服务商分段条 → 地址 → Key（带「获取 Key」）→ 模型。
// 换一家时地址、模型跟着换；地址变了 Key 换成那家以前用过的（没有就空），切走前的 Key 记进 saved
func (a *App) apiFields(c *ui.Context, ps []apiPreset, custom *bool, base, model, key *string, saved *map[string]string, urlDesc string, switched func()) {
	t := c.Theme()
	cur := presetOf(ps, *base, *custom)
	if id := providerPicker(c, ps, cur.id); id != "" && id != cur.id {
		*custom = id == "custom"
		for _, p := range ps {
			if p.id == id && p.base != "" {
				if keyFor(*base) != keyFor(p.base) {
					*saved = rememberKey(*saved, *base, *key)
					*key = (*saved)[keyFor(p.base)]
				}
				*base, *model = p.base, p.model
			}
		}
		cur = presetOf(ps, *base, *custom)
		switched()
	}
	if cur.note != "" {
		ui.Text(c, cur.note).FontSize(12).TextColor(t.TextMuted).Padding(8, 2, 0, 2)
	}
	// 预设也显示地址，填好了能直接看到、能改；改成认不出的地址就自动算自定义
	settingRow(c, T("接口地址"), urlDesc, func() { ui.TextInput(c, base).Placeholder("https://…/v1") })
	if !cur.noKey {
		ui.Row(c).Gap(16).Padding(12, 0).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "API Key").FontSize(14)
				if cur.keyURL != "" && smallLink(c, T("获取 Key")+" ↗").AlignSelf(ui.Start).Clicked() {
					go mygo.Shell.OpenExternal(cur.keyURL)
				}
			})
			ui.Box(c).Width(controlW).Children(func() { ui.TextInput(c, key).Password().Placeholder("sk-…") })
		})
	}
	settingRow(c, T("模型"), "", func() { ui.TextInput(c, model).Placeholder(cur.model) })
}

// rememberKey 返回记上 base→key 的新 map（不改原来那份：草稿和正在用的配置共用同一个 map）
func rememberKey(m map[string]string, base, key string) map[string]string {
	key = strings.TrimSpace(key)
	if key == "" || keyFor(base) == "" {
		return m
	}
	out := make(map[string]string, len(m)+1)
	for b, k := range m {
		out[b] = k
	}
	out[keyFor(base)] = key
	return out
}

// logoW 分段里 logo 占的宽：14 的图 + 6 的间距
const logoW = 20

// providerPicker 服务商：跟页顶 tab 同款的分段条，铺满卡片宽、每段等分；
// 某些语言名字太长放不下时退回下拉框。返回这一帧点中的那家
func providerPicker(c *ui.Context, ps []apiPreset, cur string) (picked string) {
	t := c.Theme()
	idx := len(ps) - 1
	labels := make([]string, len(ps))
	need := float32(0)
	for i, p := range ps {
		labels[i] = p.label
		if p.id == cur {
			idx = i
		}
		w, _ := c.MeasureText(10000, ui.Span{Text: p.label, Size: 13})
		if p.logo != nil {
			w += logoW
		}
		need += w + 2*t.Space(1.5) + 4 + t.Space(0.5)
	}
	winW, _ := c.Size()
	if need > winW-sidebarW-48-34 {
		v := cur
		settingRow(c, T("服务商"), "", func() {
			opts := make([]option, len(ps))
			for i, p := range ps {
				opts[i] = option{p.id, p.label}
			}
			optionSelect(c, &v, opts)
		})
		if v != cur {
			return v
		}
		return ""
	}
	ui.Text(c, T("服务商")).FontSize(14).Padding(14, 0, 8, 0)
	// 不能拿局部变量当 selected：mygo 在这一帧画完后才写进去，写进局部变量就丢了，所以按每段的 Clicked 认
	sel := idx
	parts := ui.SegmentedBase(c, &sel, len(ps))
	parts.Track.AlignItems(ui.Stretch).Padding(t.Space(0.5)).Gap(t.Space(0.5)).Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Label(T("服务商"))
	chosen := t.Background
	if t.Dark {
		chosen = t.SurfacePressed
	}
	parts.Track.Children(func() {
		for i, label := range labels {
			w, _ := c.MeasureText(10000, ui.Span{Text: label, Size: 13})
			if ps[i].logo != nil {
				w += logoW
			}
			s := parts.Segment(i).Grow(1).Padding(t.Space(1.5), t.Space(1.5)).Justify(ui.Center).Radius(max(t.Radius-t.Space(0.5), 0)).MinWidth(w + 2*t.Space(1.5) + 4)
			color := t.TextMuted // 没选中的字淡一档，选中那段才是正文色，条子看着轻
			switch {
			case i == idx:
				s.Background(chosen).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12))
				color = t.Text
			case s.Hovered():
				s.Background(t.SurfaceHover)
				color = t.Text
			}
			s.Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					if lg := ps[i].logo; lg != nil {
						ui.Image(c, lg).Size(14, 14).Shrink(0)
					}
					lw, _ := c.MeasureText(10000, ui.Span{Text: label, Size: 13})
					ui.Text(c, label).FontSize(13).TextColor(color).SingleLine().Shrink(0).MinWidth(lw + 2) // 同 fitLabel，不然被截成省略号
				})
			})
			if s.Clicked() && i != idx {
				picked = ps[i].id
			}
		}
	})
	return picked
}

// testRow 接口卡片最下面：左边测试结果，右边测试按钮
func testRow(c *ui.Context, msg string, isErr, busy bool, label string, run func()) {
	ui.Row(c).Gap(12).Padding(12, 0).AlignItems(ui.Center).Children(func() {
		color := c.Theme().TextMuted
		if isErr {
			color = c.Theme().Danger
		}
		ui.Text(c, msg).FontSize(12).TextColor(color).Grow(1).MaxLines(2)
		if textButton(c, label).Disabled(busy).Clicked() {
			run()
		}
	})
}

// translateAPI 「翻译」页的接口 tab
func (a *App) translateAPI(c *ui.Context, d *Config) {
	s := &a.set
	settingGroup(c, "", func() {
		a.apiFields(c, translatePresets(), &s.customTrans, &d.BaseURL, &d.Model, &d.APIKey, &d.SavedKeys, T("会自动拼上 /chat/completions"), func() { s.testMsg, s.testErr = "", false })
		testRow(c, s.testMsg, s.testErr, s.testing, T("测试连接"), a.testDraft)
	})
	settingGroup(c, T("网络"), func() {
		settingRow(c, T("代理"), T("Go 不读系统代理，要走 Clash 得填这里"), func() {
			ui.TextInput(c, &d.Proxy).Placeholder("http://127.0.0.1:7890")
		})
	})
}

// apiASR 「语音识别」页的接口 tab
func (a *App) apiASR(c *ui.Context, d *Config) {
	s := &a.set
	a.apiFields(c, asrPresets(), &s.customASR, &d.ASRBaseURL, &d.ASRModel, &d.ASRAPIKey, &d.SavedKeys, T("语音始终直连，不使用代理"), func() { s.voiceTestMsg, s.voiceTestErr = "", false })
	testRow(c, s.voiceTestMsg, s.voiceTestErr, s.voiceTesting, T("测试语音接口"), a.testVoiceDraft)
}

// apiPolish 「润色」页的接口区
func (a *App) apiPolish(c *ui.Context, d *Config) {
	s := &a.set
	a.apiFields(c, polishPresets(), &s.customPolish, &d.PolishBaseURL, &d.PolishModel, &d.PolishAPIKey, &d.SavedKeys, T("润色始终直连，不使用代理"), func() { s.polishTestMsg, s.polishTestErr = "", false })
	testRow(c, s.polishTestMsg, s.polishTestErr, s.polishTesting, T("测试润色"), a.testPolishDraft)
}

func (a *App) pageVoice(c *ui.Context, d *Config) {
	pageTitle(c, T("语音识别"))
	pageTabs(c, &a.set.voiceTab, T("通用"), T("外观"), T("接口"))
	switch a.set.voiceTab {
	case 1:
		settingGroup(c, "", func() { a.voiceLook(c, d) })
		return
	case 2:
		settingGroup(c, "", func() { a.apiASR(c, d) })
		return
	}
	settingGroup(c, "", func() {
		switchRow(c, T("启用语音输入"), T("按一下开始说话，再按一下贴入；Esc 取消"), &d.VoiceEnabled)
		settingRowW(c, T("触发键"), T("点一下再按键，Backspace 关闭"), selectW, func() { a.hotkeyBox(c, "voice", &d.VoiceHotkey, true) })
		switchRow(c, T("提示音"), T("开始、结束、取消时轻响一声"), &d.VoiceSound)
		switchRow(c, T("边说边出字"), T("说话时胶囊里实时显示识别的字，说完照常润色后贴入；需要千问 3.1 这类流式模型"), &d.VoiceLiveText)
	})
}

// pageWords 「词库」页：识别和润色要认得的词。自动纠正学到的对子也只按正确写法显示成一个词，
// 错写藏在悬停提示里（润色还要用「错写→正确」这张表，数据不丢）。改动直接落盘，不走「保存」。
func (a *App) pageWords(c *ui.Context, d *Config) {
	t := c.Theme()
	s := &a.set
	pageTitle(c, T("词库"))
	settingGroup(c, "", func() {
		switchRow(c, T("自动学习"), T("贴入后你手改过的词会记住，下次识别和润色都用对的写法"), &d.VoiceLearn)
		if a.samples != nil {
			n := a.samples.count()
			settingRowW(c, T("长文改写"), fmt.Sprintf(T("已存 %d 段：较长的口述和你改完的样子，只存在本机，以后用来学你的说话习惯"), n), 64, func() {
				if n > 0 && smallLink(c, T("清空")).AlignSelf(ui.End).Clicked() {
					go a.samples.clear()
				}
			})
		}
	})
	groups := groupWords(a.learn.all())
	ui.Row(c).Gap(12).AlignItems(ui.End).Padding(16, 0, 10, 0).Children(func() {
		ui.Column(c).Grow(1).Shrink(1).Gap(3).Children(func() {
			if len(groups) > 0 {
				ui.Text(c, fmt.Sprintf(T("共 %d 个"), len(groups))).FontSize(14).Bold()
			}
			ui.Text(c, T("让识别认得这些写法，比如人名、产品名")).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Row(c).Gap(12).Children(func() {
			if smallLink(c, T("导入…")).Clicked() {
				go a.importWords()
			}
			if smallLink(c, T("导出…")).Clicked() {
				go a.exportWords()
			}
		})
	})
	if s.wordsMsg != "" {
		color := t.TextMuted
		if s.wordsErr {
			color = t.Danger
		}
		ui.Text(c, s.wordsMsg).FontSize(12).TextColor(color).Padding(0, 0, 8, 0).MaxLines(2)
	}
	// 固定三列的格子：内容区 472 宽，四列放大字号后「Claude Opus」这种会被截断
	const cols = 3
	n := len(groups) + 1 // 最后一格是「+ 添加」
	for row := 0; row < n; row += cols {
		ui.Row(c).Gap(8).Padding(0, 0, 8, 0).Children(func() {
			for i := row; i < row+cols; i++ {
				switch {
				case i < len(groups):
					a.wordCell(c, groups[i])
				case i == len(groups):
					a.addCell(c)
				default:
					ui.Box(c).Grow(1).Basis(0) // 补空位，格子宽度才一致
				}
			}
		})
		for i := row; i < min(row+cols, len(groups)); i++ {
			if groups[i].Right == s.editWord {
				a.wordEditor(c, groups[i])
			}
		}
	}
	if len(groups) == 0 {
		ui.Text(c, T("你改过两次的词会自动加进来，也可以自己添加")).FontSize(12).TextColor(t.TextMuted).Padding(12, 0, 0, 0)
	}
}

// wordGroup 词库里显示的一个词：同一个正确写法的几条（纯热词 + 各种错写）合成一个
type wordGroup struct {
	Right  string
	Words  []learnedWord
	Wrongs []string
	Blocks []string
	Count  int
	Manual bool
}

// active 跟 learner.active 同一个规则：手动加的，或同一写法累计见过 learnPromote 次
func (g wordGroup) active() bool { return g.Manual || g.Count >= learnPromote }

// groupWords 按正确写法合并，最近加的（文件里靠后的）在前
func groupWords(words []learnedWord) []wordGroup {
	var out []wordGroup
	at := map[string]int{}
	for i := len(words) - 1; i >= 0; i-- {
		w := words[i]
		j, ok := at[w.Right]
		if !ok {
			j = len(out)
			at[w.Right] = j
			out = append(out, wordGroup{Right: w.Right})
		}
		g := &out[j]
		g.Words = append(g.Words, w)
		g.Count += w.Count
		g.Manual = g.Manual || w.Manual
		if w.Wrong != "" {
			g.Wrongs = append(g.Wrongs, w.Wrong)
		}
		g.Blocks = append(g.Blocks, w.Block...)
	}
	return out
}

func (g wordGroup) tip() string {
	var parts []string
	if len(g.Wrongs) > 0 {
		parts = append(parts, fmt.Sprintf(T("曾听成 %s"), strings.Join(g.Wrongs, "、")))
	}
	if len(g.Blocks) > 0 {
		parts = append(parts, fmt.Sprintf(T("旁边有 %s 时不换"), strings.Join(g.Blocks, "、")))
	}
	return strings.Join(append(parts, g.status()), " · ")
}

// status 这个词是怎么来的、生效没有
func (g wordGroup) status() string {
	switch {
	case !g.active():
		return T("再遇到一次就生效")
	case g.Manual:
		return T("手动添加")
	default:
		return fmt.Sprintf(T("学到 %d 次"), g.Count)
	}
}

// smallLink 次要操作用的小号文字链接（导入导出一年点一次，不配做成按钮）
func smallLink(c *ui.Context, label string) ui.Element {
	t := c.Theme()
	w, _ := c.MeasureText(10000, ui.Span{Text: label, Size: 12})
	l := ui.Text(c, label).FontSize(12).Cursor(ui.CursorPointer).SingleLine().Shrink(0).MinWidth(w + 8) // 同 fitLabel：不按 MiSans 量会被截成省略号
	if l.Hovered() {
		return l.TextColor(t.Accent)
	}
	return l.TextColor(t.TextMuted)
}

// wordCell 词库的一格：点一下原地改写法，悬停露出 ✕；自动学到的前面一个小点，还没生效的虚线淡字
func (a *App) wordCell(c *ui.Context, g wordGroup) {
	t := c.Theme()
	s := &a.set
	editing := s.editWord == g.Right
	cell := ui.Row(c).Key(g.Right).Grow(1).Basis(0).Shrink(1).Height(40).Gap(8).AlignItems(ui.Center).Padding(0, 10, 0, 12).Radius(t.Radius).Border(1, t.Border).Cursor(ui.CursorText).Tooltip(g.tip())
	hover := cell.Hovered()
	bg, ink := t.Surface, t.Text
	if !g.active() {
		cell.BorderStyle(ui.BorderDashed)
		bg, ink = ui.RGBA(0, 0, 0, 0), t.TextMuted
	}
	if hover {
		bg = t.SurfaceHover
	}
	if editing {
		cell.Border(1, t.Accent)
	}
	removed := false
	cell.Background(bg).Children(func() {
		if !g.Manual {
			ui.Box(c).Size(6, 6).Radius(3).Background(t.Accent)
		}
		ui.Text(c, g.Right).FontSize(15).TextColor(ink).SingleLine().Grow(1).Shrink(1)
		x := ui.Text(c, "✕").FontSize(11).TextColor(t.TextMuted).Cursor(ui.CursorPointer).Tooltip(T("删除"))
		if !hover {
			x.TextColor(ui.RGBA(0, 0, 0, 0)) // 占着位置，悬停时文字不跳
		}
		if hover && x.Clicked() {
			removed = true
			for _, w := range g.Words {
				a.learn.remove(w.Wrong, w.Right)
			}
		}
	})
	if cell.Clicked() && !removed {
		if editing { // 再点一下收起，不保存
			s.editWord = ""
			return
		}
		s.editWord, s.editText, s.editSeen, s.wordAdd = g.Right, g.Right, false, false
		s.editAliases = slices.Clone(g.Wrongs)
	}
}

// wordEditor 点开一格后在它那一行下面展开：改写法，看一眼系统猜的/学到的听法，错的点 ✕。不用自己写规则
// （「旁边有这些词时不换」只留在导入导出里给想手调的人，界面上不出现；换错了改回来系统自己会忘掉那条）
func (a *App) wordEditor(c *ui.Context, g wordGroup) {
	t := c.Theme()
	s := &a.set
	save, cancel := false, false
	ui.Column(c).Key("edit:"+g.Right).Gap(12).Padding(14, 16).Margin(0, 0, 8, 0).Radius(t.Radius).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, T("写法")).FontSize(12).TextColor(t.TextMuted)
			if ui.TextInput(c, &s.editText).Height(34).AutoFocus().Submitted() {
				save = true
			}
		})
		ui.Column(c).Gap(6).Children(func() {
			ui.Text(c, T("也可能听成")).FontSize(12).TextColor(t.TextMuted)
			if len(s.editAliases) == 0 && !s.aliasAdding {
				ui.Text(c, T("还没有。加词时会自动猜几种，你改过的也会记下来")).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Row(c).Wrap().Gap(6).Children(func() {
				for i, w := range s.editAliases {
					ui.Row(c).Key(w).Gap(6).Height(28).Padding(0, 8, 0, 10).AlignItems(ui.Center).Radius(t.Radius).Background(t.Background).Border(1, t.Border).Children(func() {
						ui.Text(c, w).FontSize(13).SingleLine()
						x := ui.Text(c, "✕").FontSize(11).TextColor(t.TextMuted).Cursor(ui.CursorPointer)
						if x.Hovered() {
							x.TextColor(t.Danger)
						}
						if x.Clicked() {
							s.editAliases = slices.Delete(slices.Clone(s.editAliases), i, i+1)
						}
					})
				}
				if s.aliasAdding {
					in := ui.TextInput(c, &s.aliasNew).Height(28).Width(120).AutoFocus()
					if in.Submitted() {
						s.addAliasDraft(g.Right)
					}
					return
				}
				add := ui.Row(c).Height(28).Padding(0, 10).AlignItems(ui.Center).Radius(t.Radius).Border(1, t.Border).Cursor(ui.CursorPointer).Children(func() {
					ui.Text(c, "+ "+T("添加")).FontSize(13).TextColor(t.TextMuted).SingleLine()
				})
				if add.Clicked() {
					s.aliasAdding, s.aliasNew = true, ""
				}
			})
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, g.status()).FontSize(12).TextColor(t.TextMuted).Grow(1).Shrink(1).MaxLines(2) // 听法上面已经列着了，这里只说来历
			if textButton(c, T("取消")).Clicked() {
				cancel = true
			}
			if accentButton(c, T("完成")).Clicked() {
				save = true
			}
		})
	})
	if save {
		s.addAliasDraft(g.Right)
		if to := strings.TrimSpace(s.editText); to != "" {
			a.learn.edit(g.Right, to, s.editAliases, g.Blocks)
		}
		cancel = true
	}
	if cancel {
		s.editWord, s.editText, s.editAliases = "", "", nil
		s.aliasAdding, s.aliasNew = false, ""
	}
}

// addAliasDraft 把编辑面板里手打的听法（逗号可分多个）加进草稿；跟写法本身或已有的重了就不加
func (s *settingsState) addAliasDraft(right string) {
	for _, w := range splitList(s.aliasNew) {
		if foldKey(w) != foldKey(right) && !slices.ContainsFunc(s.editAliases, func(x string) bool { return foldKey(x) == foldKey(w) }) {
			s.editAliases = append(slices.Clone(s.editAliases), w)
		}
	}
	s.aliasAdding, s.aliasNew = false, ""
}

// guessAliases 手动加的词在后台让润色模型猜几种听错的写法，挂进「也可能听成」，用户什么都不用填。
// 一次最多猜 5 个词，没配润色 Key 就不猜
func (a *App) guessAliases(words []learnedWord) {
	cfg, _ := a.config()
	if cfg.PolishAPIKey == "" {
		return
	}
	n := 0
	for _, w := range words {
		if w.Wrong != "" || n == 5 {
			continue
		}
		n++
		ctx, cancel := context.WithTimeout(context.Background(), polishMaxWait)
		out, err := chatOnce(ctx, cfg, aliasPrompt, w.Right)
		cancel()
		if err != nil {
			reason, _, _ := strings.Cut(err.Error(), ":")
			log.Printf("aliases: %s", reason)
			continue
		}
		if al := parseAliases(out, w.Right); len(al) > 0 {
			a.learn.addAliases(w.Right, al)
			log.Printf("aliases: %d for one word", len(al))
			a.setWin.Update(func() {})
		}
	}
}

// addCell 最后一格「+ 添加」，点开原地变输入框
func (a *App) addCell(c *ui.Context) {
	t := c.Theme()
	s := &a.set
	if s.wordAdd {
		a.wordInput(c, "+", T("回车添加，逗号隔开多个"), func(text string) {
			words := parseWordList(text)
			a.learn.add(words)
			go a.guessAliases(words)
		})
		return
	}
	cell := ui.Row(c).Grow(1).Basis(0).Shrink(1).Height(40).Justify(ui.Center).AlignItems(ui.Center).Radius(t.Radius).Border(1, t.Border).BorderStyle(ui.BorderDashed).Cursor(ui.CursorPointer)
	if cell.Hovered() {
		cell.Background(t.SurfaceHover)
	}
	cell.Children(func() {
		ui.Text(c, "+ "+T("添加")).FontSize(14).TextColor(t.TextMuted).SingleLine()
	})
	if cell.Clicked() {
		s.wordAdd, s.editWord, s.editText, s.editSeen = true, "", "", false
	}
}

// wordInput 格子里的输入框：回车或点别处就提交，空着算取消
func (a *App) wordInput(c *ui.Context, key, placeholder string, done func(string)) {
	s := &a.set
	in := ui.TextInput(c, &s.editText).Placeholder(placeholder).Grow(1).Basis(0).Shrink(1).Height(40).Radius(c.Theme().Radius).AutoFocus()
	focused := in.Focused()
	if in.Submitted() || (s.editSeen && !focused) {
		if text := strings.TrimSpace(s.editText); text != "" {
			done(text)
		}
		s.editWord, s.editText, s.editSeen, s.wordAdd = "", "", false, false
		return
	}
	s.editSeen = s.editSeen || focused
}

func (a *App) wordsStatus(msg string, isErr bool) {
	a.setWin.Update(func() { a.set.wordsMsg, a.set.wordsErr = msg, isErr })
}

var wordFileFilters = []mygo.FileFilter{{Name: "Text", Extensions: []string{"txt", "tsv", "csv"}}}

// importWords 在界面线程之外跑：文件对话框、读文件，结果回到设置窗口；导入的直接生效
func (a *App) importWords() {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.setWin, Title: T("导入词库"), Filters: wordFileFilters})
	if err != nil || len(paths) == 0 {
		return
	}
	b, err := os.ReadFile(paths[0])
	if err != nil {
		a.wordsStatus(err.Error(), true)
		return
	}
	n := a.learn.add(parseWordList(string(b)))
	a.wordsStatus(fmt.Sprintf(T("已导入 %d 条"), n), false)
}

func (a *App) exportWords() {
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: a.setWin, Title: T("导出词库"), DefaultPath: "mami-words.txt", Filters: wordFileFilters})
	if err != nil || path == "" {
		return
	}
	if err := os.WriteFile(path, []byte(formatWordList(a.learn.all())), 0o600); err != nil {
		a.wordsStatus(err.Error(), true)
		return
	}
	a.wordsStatus(T("已导出"), false)
}

// voiceLook 「语音识别」页的外观 tab：录音时的样式
func (a *App) voiceLook(c *ui.Context, d *Config) {
	switchRow(c, T("贴边小条"), T("悬停展开，拖动可贴到屏幕任一边缘"), &d.VoiceDock)
	settingRow(c, T("面板底色"), T("跟界面主题分开选"), func() {
		optionSelect(c, &d.VoiceDockTone, []option{{"auto", T("跟随主题")}, {"light", T("浅色")}, {"dark", T("深色")}})
	})
	settingRow(c, T("样式"), T("录音时屏幕底部的样子"), func() {
		optionSelect(c, &d.VoiceStyle, []option{{"bars", T("渐变波柱")}, {"orb", T("果冻球")}, {"flow", T("流光")}, {"glass", T("磨砂玻璃")}, {"pixel", T("复古像素")}, {"mercury", T("水银")}, {"islands", T("三岛连桥")}, {"leds", T("分段电平")}})
	})
	if d.VoiceStyle != "pixel" && d.VoiceStyle != "orb" { // 像素和果冻球的配色是固定的
		settingRow(c, T("浮窗底色"), T("跟界面主题分开选"), func() {
			optionSelect(c, &d.VoiceTone, []option{{"dark", T("深色")}, {"light", T("浅色")}, {"auto", T("跟随主题")}})
		})
	}
	if d.VoiceStyle == "islands" || d.VoiceStyle == "leds" {
		switchRow(c, T("连体"), "", &d.VoiceJoined)
	}
	if d.VoiceStyle == "flow" {
		settingRow(c, T("流光配色"), "", func() {
			options := make([]option, 0, len(flowPalettes))
			for _, palette := range flowPalettes {
				options = append(options, option{palette.id, T(palette.name)})
			}
			optionSelect(c, &d.VoiceFlowPalette, options)
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
	c.After(voiceFrame)
	ui.Row(c).Padding(16, 12).Margin(4, 0, 12, 0).Gap(20).Justify(ui.Center).AlignItems(ui.End).Background(desk).Radius(c.Theme().Radius).Children(func() {
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
	if style == "flow" && voiceKind(st) != 3 {
		ui.Box(c).Size(flowSize, flowSize).Draw(func(p *ui.Painter, r ui.Rect) {
			drawFlowOrb(p, r, bars[voiceBars-1], now, st.recognizing, d.VoiceFlowPalette)
		})
	} else if style == "orb" && voiceKind(st) != 3 {
		sz, rec := float32(math.Round(d.VoiceOrbSize)), st.recognizing
		ui.Box(c).Size(sz, sz).Draw(func(p *ui.Painter, r ui.Rect) { drawOrb(p, r, bars[voiceBars-1], now, rec) })
	} else {
		gl, gr := islandGapsFor(st)
		a.voicePill(c, style, dark, sk, st, bars, bars[voiceBars-1], gl, gr, 1, now, false, d.VoiceJoined, d.VoiceFlowPalette).Width(float32(min(voiceWidth(st), voiceRecW))) // 预览框放不下全宽的提示，句子长了就截断
	}
}

func (a *App) pagePolish(c *ui.Context, d *Config) {
	pageTitle(c, T("润色"))
	pageTabs(c, &a.set.polishTab, T("档位"), T("接口"))
	if a.set.polishTab == 1 {
		settingGroup(c, "", func() { a.apiPolish(c, d) })
		return
	}
	settingGroup(c, "", func() {
		settingRowW(c, T("档位"), "", selectW, func() {
			optionSelect(c, &d.PolishStyle, []option{{"off", T("关闭")}, {"light", T("轻润色")}, {"standard", T("标准")}, {"structured", T("结构化")}, {"prompt", T("提示词")}})
		})
		switchRow(c, T("选中再说就改写"), T("先选中一段字再按触发键，说「翻成英文」「写正式点」「缩短一半」就照着改；说的不是要求就换成你说的话"), &d.VoiceRewrite)
		switchRow(c, T("说完直接翻译"), T("开着时说中文，贴进去的是润色好的译文；不分程序，全局生效"), &d.VoiceTranslate)
		if d.VoiceTranslate {
			settingRow(c, T("翻译成"), "", func() { langSelect(c, &d.VoiceTarget) })
		}
		switchRow(c, T("按程序自动换档"), T("在 Cursor、ChatGPT 这类 AI 工具里用「提示词」档，在微信、QQ 里用「轻润色」，其他地方用上面选的"), &d.PolishAuto)
	})
}

func (a *App) testPolishDraft() {
	cfg := a.set.draft
	cfg.ASRHotwords = withLearned(cfg.ASRHotwords, a.learn.active())
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
	cfg.ASRHotwords = withLearned(cfg.ASRHotwords, a.learn.active())
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
	settingGroup(c, T("语言"), func() {
		settingRow(c, T("中文翻成"), T("选中的文字里有汉字时"), func() { langSelect(c, &d.TargetForChinese) })
		settingRow(c, T("其他翻成"), T("不含汉字的文字（英文、日文等）"), func() { langSelect(c, &d.TargetForOther) })
	})
	settingGroup(c, T("额外要求"), func() {
		ui.Column(c).Gap(8).Padding(12, 0).Children(func() {
			ui.Text(c, T("追加给模型的话，比如风格、术语处理")).FontSize(12).TextColor(c.Theme().TextMuted)
			a.extraPromptBox(c, d)
		})
	})
	settingGroup(c, T("显示"), func() {
		switchRow(c, T("边翻边显示"), T("模型一边写，卡片一边出字；关掉就等整段译文出来再显示"), &d.StreamCard)
	})
}

func (a *App) pageHotkeys(c *ui.Context, d *Config) {
	pageTitle(c, T("快捷键"))
	settingGroup(c, "", func() {
		switchRow(c, T("启用快捷键"), T("关掉后不注册任何全局快捷键，不会和别的程序抢键；划词按钮照常能用"), &a.set.hotkeysOn)
		if !a.set.hotkeysOn {
			return
		}
		settingRowW(c, T("原地替换"), T("点一下再按组合键，Backspace 清空"), selectW, func() { a.hotkeyBox(c, "hotkey", &d.Hotkey, false) })
		settingRowW(c, T("只弹卡片"), T("点一下再按组合键，Backspace 清空"), selectW, func() { a.hotkeyBox(c, "popup", &d.PopupHotkey, false) })
		switchRow(c, T("快捷键没选中就全选"), T("按 Alt+T 时没选中文字，先全选整个输入框再翻译"), &d.SelectAllIfEmpty)
	})
}

func (a *App) pageButton(c *ui.Context, d *Config) {
	s := &a.set
	pageTitle(c, T("划词按钮"))
	settingGroup(c, "", func() {
		switchRow(c, T("划词后显示翻译按钮"), T("选中文字后旁边弹出一个小图标，点它出译文"), &d.ShowButton)
		switchRow(c, T("只在输入框里弹"), T("网页正文、文档里划词不弹"), &d.OnlyInInputs)
		settingRow(c, T("点翻译按钮后"), T("直接替换出错时仍会弹卡片说明原因"), func() { actionSelect(c, &d.ButtonAction) })
		settingRow(c, T("不弹按钮的程序"), T("进程名，逗号分隔"), func() {
			ui.TextInput(c, &s.exclude).Placeholder("game.exe, WindowsTerminal.exe")
		})
	})
	settingGroup(c, T("兼容性"), func() {
		switchRow(c, T("认不出输入框的程序也弹"), T("微信这类程序开着能用，但聊天记录里划词也会弹"), &d.ShowInBlindApps)
		switchRow(c, T("只认文本光标"), T("按下鼠标时光标是 I 形才算划词，拖窗口、拖文件不会误弹"), &d.RequireIBeam)
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
		ui.Text(c, title).FontSize(12).TextColor(c.Theme().TextMuted).Padding(6, 4, 0, 4) // 跟 settingGroup 的组名一个样子
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

// titleBar 自绘标题栏：没有字，整条可拖动，右上角最小化和关闭。名字和头像在侧边栏的品牌卡上
func (a *App) titleBar(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(36).AlignItems(ui.Center).AlignSelf(ui.Stretch).Shrink(0).Children(func() {
		ui.Row(c).Grow(1).Height(36).DragWindow()
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
var iconWords = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 7v14"/><path d="M3 18a1 1 0 0 1-1-1V4a1 1 0 0 1 1-1h5a4 4 0 0 1 4 4 4 4 0 0 1 4-4h5a1 1 0 0 1 1 1v13a1 1 0 0 1-1 1h-6a3 3 0 0 0-3 3 3 3 0 0 0-3-3z"/></svg>`))
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
	settingGroup(c, T("界面"), func() {
		settingRowW(c, T("界面语言"), T("设置窗口、卡片和托盘菜单用哪种语言"), selectW, func() { optionSelect(c, &d.Language, langs) })
		settingRowW(c, T("外观"), T("浅色、深色，或跟随 Windows"), selectW, func() {
			optionSelect(c, &d.Theme, []option{{"auto", T("跟随系统")}, {"light", T("浅色")}, {"dark", T("深色")}})
		})
	})
	settingGroup(c, T("翻译记录"), func() {
		switchRow(c, T("保存翻译记录"), T("原文、发给模型的提示词和译文按天存成文件，之后可以交给 AI 分析哪里没翻好"), &d.History)
		if !d.History {
			return
		}
		settingRowW(c, T("保留时间"), T("更早的记录会自动删除"), selectW, func() {
			v := strconv.Itoa(d.HistoryDays)
			optionSelect(c, &v, []option{{"7", T("7 天")}, {"30", T("30 天")}, {"90", T("90 天")}, {"0", T("永久")}})
			d.HistoryDays, _ = strconv.Atoi(v)
		})
		ui.Row(c).Gap(12).Padding(4, 0, 14, 0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, T("记录只存在这台电脑上，终端里的翻译不记。")).FontSize(12).TextColor(c.Theme().TextMuted).Grow(1).Shrink(1)
			if smallLink(c, T("打开记录文件夹")).Clicked() {
				a.openHistory()
			}
		})
	})
	settingGroup(c, T("启动和更新"), func() {
		switchRow(c, T("开机自启"), T("登录 Windows 后在托盘里自动运行"), &a.set.openAtLogin)
		a.updateRow(c)
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

// sidebarW 侧边栏宽度；controlPad 是额外要求框的文字区离窗口左右的距离之和（侧边栏 + 页面左右留白 56 + 输入框内边距）
const sidebarW = 212

// settingsW, settingsH 设置窗口的默认尺寸；侧边栏放得下「Auswahl-Schaltfläche」这类长词
const settingsW, settingsH = 732, 480

const controlPad = sidebarW + 56 + 32 + 28 // 多出来的 32 是分组卡片左右内边距

var iconGen = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 4h-7"/><path d="M10 4H3"/><path d="M21 12h-9"/><path d="M8 12H3"/><path d="M21 20h-5"/><path d="M12 20H3"/><path d="M14 2v4"/><path d="M8 10v4"/><path d="M16 18v4"/></svg>`))

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
		keycaps(c, *value)
	})
	if box.Clicked() {
		if rec {
			a.stopCapture()
		} else {
			a.startCapture(id, value, voice)
		}
	}
}

// keycaps 把「Alt+Shift+T」画成一个个键帽；空着或 Off 显示「未设置」
func keycaps(c *ui.Context, acc string) {
	t := c.Theme()
	v := strings.TrimSpace(acc)
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

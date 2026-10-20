//go:build windows

package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// 「记录」页的语音标签：几个数字，下面按天分组列出最近说过的话

const voiceListMax = 100

func (a *App) voiceHistory(c *ui.Context, d *Config, st *historyStats) {
	t := c.Theme()
	if !d.History {
		ui.Text(c, T("还没打开「保存翻译记录」（在「通用」页），语音记录也不会保存。")).
			FontSize(12).TextColor(t.TextMuted).Padding(0, 0, 8, 0)
	}
	now := time.Now()
	sum := summarizeVoice(st.voice, now)
	if sum.total == 0 {
		ui.Column(c).Gap(6).Padding(48, 0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, T("还没有语音记录")).FontSize(14).Bold()
			ui.Text(c, T("按一下语音键说句话，记录会出现在这里。")).FontSize(12).TextColor(t.TextMuted)
		})
		return
	}
	ui.Row(c).Gap(8).Padding(4, 0, 12, 0).Children(func() {
		statCard(c, T("次数"), fmt.Sprint(sum.total))
		statCard(c, T("今天"), fmt.Sprint(sum.today))
		statCard(c, T("字数"), fmt.Sprint(sum.chars))
		statCard(c, T("比打字约省下"), shortDuration(sum.savedMs))
	})
	if sum.nPolish > 0 {
		statRow(c, T("润色耗时"), fmt.Sprintf(T("平均 %.1f 秒"), float64(sum.polishMs)/float64(sum.nPolish)/1000))
	}
	days, hours := voiceActivity(st.voice, now.Location())
	section(c, T("最近半年"), func() { heatmap(c, days) })
	ui.Box(c).Height(16)
	section(c, T("一天里什么时候用"), func() { hourChart(c, hours) })
	ui.Box(c).Height(12)

	list := st.voice[:min(len(st.voice), voiceListMax)]
	day := ""
	for _, e := range list {
		when, err := time.Parse(time.RFC3339, e.Time)
		if err != nil {
			continue
		}
		when = when.In(now.Location())
		if k := when.Format("2006-01-02"); k != day {
			day = k
			ui.Text(c, dayLabel(when, now)).FontSize(12).Bold().TextColor(t.TextMuted).Padding(10, 2, 6, 2)
		}
		a.voiceCard(c, e, when)
	}
	if len(st.voice) > len(list) {
		ui.Text(c, fmt.Sprintf(T("只显示最近 %d 条，完整的在记录文件夹里。"), voiceListMax)).
			FontSize(12).TextColor(t.TextMuted).Padding(10, 2, 0, 2)
	}
	if d.History {
		ui.Row(c).Padding(12, 0).Children(func() {
			ui.Spacer(c)
			if textButton(c, T("打开记录文件夹")).Clicked() {
				a.openHistory()
			}
		})
	}
}

// voiceCard 一条记录：时间和档位一行，贴出去的文字，润色改过时下面淡淡带一行原话；悬停才露出「复制」
func (a *App) voiceCard(c *ui.Context, e historyEntry, when time.Time) {
	t := c.Theme()
	final := strings.TrimSpace(voiceFinal(e))
	source := strings.TrimSpace(e.Source)
	card := ui.Column(c).Gap(6).Padding(10, 12).Margin(0, 0, 6, 0).Radius(t.Radius).Background(t.Surface).Border(1, t.Surface)
	hovered := card.Hovered()
	if hovered {
		card.Border(1, t.Border)
	}
	card.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Text(c, when.Format("15:04")).FontSize(12).TextColor(t.TextMuted)
			if label := polishLabel(e.PolishStyle); label != "" {
				ui.Text(c, label).FontSize(11).TextColor(t.Accent).Padding(1, 7).Radius(8).Background(t.Accent.Alpha(0.12))
			}
			ui.Spacer(c)
			copied := a.set.copiedID == e.ID && time.Since(a.set.copiedAt) < 1500*time.Millisecond
			label := T("复制")
			if copied {
				label = T("已复制")
				c.AnimationFrame() // 1.5 秒后自己变回「复制」
			}
			opacity := float32(0)
			if hovered || copied {
				opacity = 1
			}
			if textButton(c, label).Height(22).Padding(0, 8).FontSize(12).Opacity(opacity).Clicked() {
				mygo.Clipboard.WriteText(final)
				a.set.copiedID, a.set.copiedAt = e.ID, time.Now()
			}
		})
		ui.Text(c, final).FontSize(14).Selectable()
		if source != "" && source != final {
			ui.Row(c).Gap(8).Children(func() {
				ui.Box(c).Width(2).AlignSelf(ui.Stretch).Radius(1).Background(t.Border)
				ui.Text(c, source).FontSize(12).TextColor(t.TextMuted).MaxLines(2).Grow(1).Tooltip(source)
			})
		}
	})
}

func polishLabel(style string) string {
	switch style {
	case "light":
		return T("轻润色")
	case "standard":
		return T("标准")
	case "structured":
		return T("结构化")
	case "prompt":
		return T("提示词")
	}
	return ""
}

func dayLabel(day, now time.Time) string {
	y, m, d := day.Date()
	switch ny, nm, nd := now.Date(); {
	case y == ny && m == nm && d == nd:
		return T("今天")
	case day.Format("2006-01-02") == now.AddDate(0, 0, -1).Format("2006-01-02"):
		return T("昨天")
	case y == ny:
		return day.Format(T("1月2日")) // 译文是 Go 的日期格式串
	}
	return day.Format(T("2006年1月2日"))
}

// shortDuration 12 秒 / 3 分钟 / 1.5 小时
func shortDuration(ms int64) string {
	switch s := float64(ms) / 1000; {
	case s < 60:
		return fmt.Sprintf(T("%d 秒"), int(s))
	case s < 3600:
		return fmt.Sprintf(T("%d 分钟"), int(s/60))
	default:
		return fmt.Sprintf(T("%.1f 小时"), s/3600)
	}
}

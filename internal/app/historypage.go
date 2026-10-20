//go:build windows

package app

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// 「记录」页：最近 26 周的热力图、24 小时分布和几项统计

const (
	pageHist   = "hist"
	heatWeeks  = 26
	heatCell   = 13
	heatGap    = 3
	hourBarW   = 14
	hourBarMax = 64
)

var iconHist = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 3v16a2 2 0 0 0 2 2h16"/><path d="M18 17V9"/><path d="M13 17V5"/><path d="M8 17v-3"/></svg>`))

// historyStatsNow 打开这一页时现读文件，5 秒内不重复读
func (a *App) historyStatsNow() *historyStats {
	s := &a.set
	if s.stats == nil || time.Since(s.stats.loaded) > 5*time.Second {
		dir := ""
		if a.hist != nil {
			dir = a.hist.dir
		}
		s.stats = loadHistoryStats(dir, a.logPaths)
	}
	return s.stats
}

func (a *App) pageHistory(c *ui.Context, d *Config) {
	t := c.Theme()
	st := a.historyStatsNow()
	pageTitle(c, T("记录"))
	ui.Row(c).Padding(4, 0, 12, 0).Children(func() {
		segmented(c, &a.set.histTab, T("语音"), T("翻译"))
	})
	if a.set.histTab == 0 {
		a.voiceHistory(c, d, st)
		return
	}
	if !d.History {
		ui.Text(c, T("还没打开「保存翻译记录」（在「通用」页）。下面的次数和耗时是从日志里补出来的，没有原文和用法。")).
			FontSize(12).TextColor(t.TextMuted).Padding(0, 0, 8, 0)
	}

	// 四个数字
	today := st.days[time.Now().Format("2006-01-02")]
	avg := 0.0
	if st.activeDays() > 0 {
		avg = float64(st.total) / float64(st.activeDays())
	}
	ui.Row(c).Gap(8).Padding(4, 0, 12, 0).Children(func() {
		statCard(c, T("总次数"), fmt.Sprint(st.total))
		statCard(c, T("今天"), fmt.Sprint(today))
		statCard(c, T("用过的天数"), fmt.Sprint(st.activeDays()))
		statCard(c, T("日均"), fmt.Sprintf("%.1f", avg))
	})

	section(c, T("一天里什么时候用"), func() { hourChart(c, st.hours) })
	ui.Box(c).Height(8)

	if st.total > 0 {
		settingGroup(c, T("明细"), func() { historyDetails(c, st) })
	}
	a.historyFooter(c, d, st)
}

// historyDetails 「记录」翻译 tab 底下的几行明细
func historyDetails(c *ui.Context, st *historyStats) {
	if top := st.topHours(3); len(top) > 0 {
		parts := make([]string, len(top))
		for i, h := range top {
			parts[i] = fmt.Sprintf(T("%s 点（%d 次）"), h.key, h.n)
		}
		statRow(c, T("最常用的时段"), strings.Join(parts, " · "))
	}
	if top := st.topApps(3); len(top) > 0 {
		parts := make([]string, len(top))
		for i, p := range top {
			parts[i] = fmt.Sprintf("%s %d", strings.TrimSuffix(p.key, ".exe"), p.n)
		}
		statRow(c, T("常用程序"), strings.Join(parts, " · "))
	}
	if st.used+st.unused > 0 {
		statRow(c, T("译文用上了吗"), fmt.Sprintf(T("用了 %d 次（复制或替换）· 没用就关掉 %d 次"), st.used, st.unused))
	}
	if st.nMs > 0 {
		v := fmt.Sprintf(T("平均 %.1f 秒"), float64(st.sumMs)/float64(st.nMs)/1000)
		if st.nFirst > 0 {
			v += fmt.Sprintf(T("，首字 %.1f 秒"), float64(st.sumFirst)/float64(st.nFirst)/1000)
		}
		statRow(c, T("耗时"), v)
	}
	if st.errors > 0 {
		statRow(c, T("失败"), fmt.Sprintf(T("%d 次"), st.errors))
	}
}

// historyFooter 卡片下面一行：开记录前的日志说明 + 记录文件夹入口
func (a *App) historyFooter(c *ui.Context, d *Config, st *historyStats) {
	t := c.Theme()
	ui.Row(c).Gap(12).Padding(10, 4, 0, 4).AlignItems(ui.Center).Children(func() {
		note := ""
		if st.fromLog > 0 {
			note = fmt.Sprintf(T("其中 %d 次来自开记录之前的日志，只有时间和耗时。"), st.fromLog)
		}
		ui.Text(c, note).FontSize(12).TextColor(t.TextMuted).Grow(1).Shrink(1)
		if d.History && smallLink(c, T("打开记录文件夹")).Clicked() {
			a.openHistory()
		}
	})
}

func statCard(c *ui.Context, label, value string) {
	t := c.Theme()
	ui.Column(c).Grow(1).Gap(2).Padding(10, 12).Radius(t.Radius).Background(t.Surface).Children(func() {
		ui.Text(c, value).FontSize(20).Bold().SingleLine()
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).SingleLine()
	})
}

func statRow(c *ui.Context, label, value string) {
	t := c.Theme()
	ui.Row(c).Gap(16).Padding(10, 0).Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(t.TextMuted).Width(120)
		ui.Text(c, value).FontSize(13).Grow(1)
	})
}

// heatColor 0 次是底色，其余按占最多那天的比例分 4 档
func heatColor(t *ui.Theme, n, most int) ui.Color {
	if n == 0 || most == 0 {
		return t.Surface
	}
	switch r := float32(n) / float32(most); {
	case r <= 0.25:
		return t.Accent.Alpha(0.3)
	case r <= 0.5:
		return t.Accent.Alpha(0.5)
	case r <= 0.75:
		return t.Accent.Alpha(0.75)
	}
	return t.Accent
}

// heatmap 一列一周（周一在上），最右一列是本周；鼠标停在格子上显示日期和次数
func heatmap(c *ui.Context, days map[string]int) {
	t := c.Theme()
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	wd := (int(today.Weekday()) + 6) % 7 // 周一 = 0
	start := today.AddDate(0, 0, -wd-7*(heatWeeks-1))
	most := 0
	for _, n := range days {
		most = max(most, n)
	}
	ui.Row(c).Gap(heatGap).Children(func() {
		for w := 0; w < heatWeeks; w++ {
			ui.Column(c).Gap(heatGap).Children(func() {
				for d := 0; d < 7; d++ {
					day := start.AddDate(0, 0, w*7+d)
					if day.After(today) {
						ui.Box(c).Size(heatCell, heatCell)
						continue
					}
					key := day.Format("2006-01-02")
					n := days[key]
					ui.Box(c).Size(heatCell, heatCell).Radius(3).Background(heatColor(t, n, most)).
						Tooltip(fmt.Sprintf(T("%s：%d 次"), key, n))
				}
			})
		}
	})
	ui.Row(c).Gap(4).Padding(6, 0, 0, 0).AlignItems(ui.Center).Children(func() {
		ui.Text(c, fmt.Sprintf(T("最多的一天 %d 次"), most)).FontSize(12).TextColor(t.TextMuted).Grow(1)
		ui.Text(c, T("少")).FontSize(12).TextColor(t.TextMuted)
		for _, n := range []int{0, 1, 2, 3, 4} {
			ui.Box(c).Size(10, 10).Radius(2).Background(heatColor(t, n, 4))
		}
		ui.Text(c, T("多")).FontSize(12).TextColor(t.TextMuted)
	})
}

// hourChart 24 根柱子，0 点到 23 点
func hourChart(c *ui.Context, hours [24]int) {
	t := c.Theme()
	most := 0
	for _, n := range hours {
		most = max(most, n)
	}
	ui.Row(c).Height(hourBarMax).Gap(3).AlignItems(ui.End).Children(func() {
		for h, n := range hours {
			bh := float32(2)
			color := t.Surface
			if n > 0 && most > 0 {
				bh = max(4, float32(hourBarMax)*float32(n)/float32(most))
				color = t.Accent.Alpha(0.35 + 0.65*float32(n)/float32(most))
			}
			ui.Box(c).Width(hourBarW).Height(bh).Radius(2).Background(color).
				Tooltip(fmt.Sprintf(T("%s:00–%s:59 · %d 次"), itoa2(h), itoa2(h), n))
		}
	})
	ui.Row(c).Gap(3).Padding(4, 0, 0, 0).Children(func() {
		for h := 0; h < 24; h++ {
			label := ""
			if h%6 == 0 {
				label = fmt.Sprint(h)
			}
			ui.Text(c, label).FontSize(11).TextColor(t.TextMuted).Width(hourBarW).SingleLine()
		}
	})
}

package app

import (
	"image"

	"github.com/egoist/mygo/ui"
)

var flowSwatches [len(flowPalettes)]*ui.Bitmap
var dockPreviewRenderer App

func dockAppearancePreview(c *ui.Context, cfg Config) {
	theme := c.Theme()
	dark := toneDark(cfg.VoiceDockTone, theme.Dark)
	desk := ui.RGBA(221, 226, 234, 1)
	if theme.Dark {
		desk = ui.RGBA(22, 24, 29, 1)
	}
	ui.Row(c).Padding(14).Margin(0, 0, 12, 0).Gap(24).Center().Background(desk).Radius(theme.Radius).Children(func() {
		ui.Column(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(28, 76).Children(func() { dockPreviewRenderer.renderDockContent(c, cfg, false, false, dark, false) })
			ui.Text(c, T("收起时")).FontSize(11).TextColor(theme.TextMuted)
		})
		ui.Column(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(196, 248).Children(func() { dockPreviewRenderer.renderDockContent(c, cfg, true, false, dark, false) })
			ui.Text(c, T("悬停展开")).FontSize(11).TextColor(theme.TextMuted)
		})
	})
}

func voiceToneSelector(c *ui.Context, value *string) {
	theme := c.Theme()
	track, chosen := ui.RGBA(20, 30, 45, 0.055), theme.Surface
	if theme.Dark {
		track, chosen = ui.RGBA(255, 255, 255, 0.06), theme.SurfacePressed
	}
	ui.RadioGroup(c, func() {
		for _, choice := range []option{{"auto", T("跟随主题")}, {"light", T("浅色")}, {"dark", T("深色")}} {
			button := ui.RadioBase(c, value, choice.value).Center().Grow(1).Padding(7, 8).Radius(theme.Radius + 1).Label(choice.label)
			color := theme.TextMuted
			if *value == choice.value {
				button.Background(chosen).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.1))
				color = theme.Text
			} else if button.Hovered() {
				button.Background(theme.SurfaceHover)
			}
			button.Children(func() { ui.Text(c, choice.label).FontSize(12).SingleLine().TextColor(color) })
		}
	}).Row().Gap(2).Padding(3).Radius(theme.Radius + 3).Background(track)
}

func flowPaletteChoices(c *ui.Context, value *string) {
	theme := c.Theme()
	ui.Row(c).Gap(8).Margin(0, 0, 4, 0).Children(func() {
		for index, palette := range flowPalettes {
			if flowSwatches[index] == nil {
				frame := image.NewRGBA(image.Rect(0, 0, 48, 48))
				renderFlow(frame, flowTexture(palette.id), 4)
				flowSwatches[index] = ui.NewBitmap(frame)
			}
			selected := flowPaletteIndex(*value) == index
			button := ui.ButtonBase(c).Grow(1).Padding(9, 4).Radius(10).Label(T(palette.name)).Cursor(ui.CursorPointer)
			if selected {
				background, edge := flowShell(palette.id, theme.Dark)
				button.Background(background).Border(1, edge)
			} else {
				button.Background(clearC).Border(1, clearC)
				if button.Hovered() {
					button.Background(theme.SurfaceHover)
				}
			}
			button.Children(func() {
				ui.Column(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Size(28, 28).Draw(func(p *ui.Painter, r ui.Rect) { p.ImageRegion(flowSwatches[index], ui.Rect{}, r, ui.ImageOptions{}) })
					color := theme.TextMuted
					if selected {
						color = theme.Text
					}
					ui.Text(c, T(palette.name)).FontSize(12).TextColor(color).SingleLine()
				})
			})
			if button.Clicked() {
				*value = palette.id
			}
		}
	})
}

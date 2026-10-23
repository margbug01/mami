package app

import "github.com/egoist/mygo/ui"

type flowPalette struct {
	id, name string
	base     [3]float64
	spots    [4][3]float64
	accent   ui.Color
}

var flowPalettes = [...]flowPalette{
	{"ice", "冰海", [3]float64{5, 173, 245}, [4][3]float64{{143, 255, 244}, {61, 229, 255}, {4, 46, 139}, {87, 255, 255}}, ui.RGBA(29, 133, 165, 1)},
	{"sunset", "落日", [3]float64{250, 98, 49}, [4][3]float64{{255, 219, 141}, {255, 140, 161}, {148, 18, 62}, {255, 173, 96}}, ui.RGBA(187, 76, 49, 1)},
	{"aurora", "极光", [3]float64{63, 104, 255}, [4][3]float64{{135, 255, 221}, {148, 159, 255}, {55, 22, 155}, {57, 226, 236}}, ui.RGBA(99, 89, 201, 1)},
	{"berry", "莓果", [3]float64{232, 62, 145}, [4][3]float64{{255, 184, 222}, {249, 114, 193}, {92, 20, 112}, {234, 139, 255}}, ui.RGBA(173, 60, 130, 1)},
	{"jade", "翡翠", [3]float64{9, 183, 132}, [4][3]float64{{167, 255, 209}, {75, 235, 181}, {3, 74, 83}, {113, 249, 175}}, ui.RGBA(20, 132, 99, 1)},
}

func flowPaletteIndex(id string) int {
	for index, palette := range flowPalettes {
		if palette.id == id {
			return index
		}
	}
	return 0
}

func flowTint(accent ui.Color, neutral uint8, amount float64, alpha float32) ui.Color {
	blend := func(channel uint8) uint8 { return uint8(float64(neutral)*(1-amount) + float64(channel)*amount) }
	return ui.RGBA(blend(accent.R), blend(accent.G), blend(accent.B), alpha)
}

func flowShell(id string, dark bool) (ui.Color, ui.Color) {
	accent := flowPalettes[flowPaletteIndex(id)].accent
	if dark {
		return flowTint(accent, 12, 0.25, 0.97), flowTint(accent, 255, 0.7, 0.4)
	}
	return flowTint(accent, 255, 0.09, 0.97), flowTint(accent, 255, 0.6, 0.45)
}

func flowSkin(id string, dark bool) voiceSkin {
	accent := flowPalettes[flowPaletteIndex(id)].accent
	if dark {
		return voiceSkin{"flow", flowTint(accent, 255, 0.13, 1), flowTint(accent, 255, 0.5, 0.12), flowTint(accent, 255, 0.5, 0.22), flowTint(accent, 255, 0.5, 1), flowTint(accent, 255, 0.3, 1), flowTint(accent, 12, 0.25, 1)}
	}
	return voiceSkin{"flow", flowTint(accent, 18, 0.4, 1), flowTint(accent, 255, 1, 0.1), flowTint(accent, 255, 1, 0.19), accent, flowTint(accent, 255, 0.85, 1), flowTint(accent, 255, 0.05, 1)}
}

//go:build windows

package app

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestFlowPalettes(t *testing.T) {
	seen := map[string]bool{}
	var previous []byte
	for _, palette := range flowPalettes {
		if seen[palette.id] {
			t.Fatal("duplicate palette")
		}
		seen[palette.id] = true
		cfg := defaultConfig()
		cfg.VoiceFlowPalette = palette.id
		cfg.normalize()
		encoded, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var restored Config
		if err := json.Unmarshal(encoded, &restored); err != nil {
			t.Fatal(err)
		}
		if restored.VoiceFlowPalette != palette.id {
			t.Fatal("palette not persisted")
		}
		texture := flowTexture(palette.id)
		frame := image.NewRGBA(image.Rect(0, 0, 64, 64))
		later := image.NewRGBA(frame.Bounds())
		renderFlow(frame, texture, 0)
		renderFlow(later, texture, 3)
		if bytes.Equal(frame.Pix, later.Pix) {
			t.Fatal("static palette", palette.id)
		}
		if bytes.Equal(previous, frame.Pix) {
			t.Fatal("identical palette", palette.id)
		}
		previous = append(previous[:0], frame.Pix...)
		for _, dark := range []bool{false, true} {
			skin := flowSkin(palette.id, dark)
			background, _ := flowShell(palette.id, dark)
			if skin.fg == background || skin.btn == skin.btnHover {
				t.Fatal("invalid palette skin")
			}
		}
	}
	for _, id := range []string{"", "invalid"} {
		cfg := defaultConfig()
		cfg.VoiceFlowPalette = id
		cfg.normalize()
		if cfg.VoiceFlowPalette != "ice" {
			t.Fatal("fallback missing")
		}
	}
}

func TestFlowPaletteScreenshot(t *testing.T) {
	path := os.Getenv("MAMI_FLOW_SNAP")
	if path == "" {
		t.Skip("set MAMI_FLOW_SNAP to render palette review")
	}
	now := flowStarted.Add(4 * time.Second)
	app := &App{}
	tester := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(26).Gap(20).Background(ui.RGBA(243, 245, 248, 1)).Children(func() {
			ui.Text(c, "MAMI / 流光配色").FontSize(24).TextColor(ui.RGBA(38, 49, 61, 1))
			ui.Text(c, "原生组件预览 · 上：48px 录音球及浅色结果条 · 下：深色结果条").FontSize(13).TextColor(ui.RGBA(108, 120, 133, 1))
			ui.Row(c).Gap(14).Children(func() {
				for _, palette := range flowPalettes {
					ui.Column(c).Width(200).Gap(16).AlignItems(ui.Center).Children(func() {
						ui.Text(c, palette.name).FontSize(16).TextColor(ui.RGBA(52, 65, 78, 1))
						ui.Column(c).Width(200).Height(170).Center().Gap(22).Background(ui.RGBA(231, 236, 242, 1)).Radius(16).Children(func() {
							ui.Box(c).Size(48, 48).Draw(func(p *ui.Painter, r ui.Rect) { drawFlowOrb(p, r, 0.5, now, false, palette.id) })
							app.voicePill(c, "flow", false, flowSkin(palette.id, false), voiceViewState{text: "明天三点开会", copy: true}, [voiceBars]float32{}, 0, 0, 0, 1, now, false, false, palette.id).Width(184)
						})
						ui.Column(c).Width(200).Height(132).Center().Gap(18).Background(ui.RGBA(29, 34, 42, 1)).Radius(16).Children(func() {
							ui.Box(c).Size(48, 48).Draw(func(p *ui.Painter, r ui.Rect) { drawFlowOrb(p, r, 0.5, now, true, palette.id) })
							app.voicePill(c, "flow", true, flowSkin(palette.id, true), voiceViewState{text: "明天三点开会", copy: true}, [voiceBars]float32{}, 0, 0, 0, 1, now, false, false, palette.id).Width(184)
						})
					})
				}
			})
		})
	}, 1110, 490)
	tester.Frame()
	output, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	if err := png.Encode(output, tester.Image()); err != nil {
		t.Fatal(err)
	}
}

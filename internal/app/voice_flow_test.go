//go:build windows

package app

import (
	"bytes"
	"image"
	"testing"
)

func TestFlowStyle(t *testing.T) {
	cfg := defaultConfig()
	cfg.VoiceStyle = "flow"
	cfg.normalize()
	if cfg.VoiceStyle != "flow" {
		t.Fatal("flow rejected")
	}
	app := &App{voiceStyle: "flow", voiceOrbSize: 96}
	for _, state := range []voiceViewState{{recording: true}, {recognizing: true}, {recording: true, live: "文字"}} {
		if !app.voiceOrb(state) || app.voiceWidth(state) != flowSize {
			t.Fatal("flow must remain a standalone ball")
		}
	}
	if app.voiceOrb(voiceViewState{text: "完成", copy: true}) {
		t.Fatal("result must retain copy UI")
	}
	if app.voiceWinHeight() < flowSize {
		t.Fatal("clipped ball")
	}
	app.voiceStyle = "orb"
	if app.voiceOrb(voiceViewState{recording: true, live: "文字"}) {
		t.Fatal("existing orb behavior changed")
	}
	if app.orbSize() != 96 {
		t.Fatal("existing orb size changed")
	}
}

func TestFlowPixels(t *testing.T) {
	texture := flowTexture("ice")
	first := image.NewRGBA(image.Rect(0, 0, flowPixels, flowPixels))
	second := image.NewRGBA(first.Bounds())
	renderFlow(first, texture, 0)
	renderFlow(second, texture, 3)
	if bytes.Equal(first.Pix, second.Pix) {
		t.Fatal("not flowing")
	}
	difference, count := 0, 0
	for offset := 0; offset < len(first.Pix); offset += 4 {
		alpha := first.Pix[offset+3]
		if alpha != second.Pix[offset+3] {
			t.Fatal("silhouette moves")
		}
		for channel := 0; channel < 3; channel++ {
			if first.Pix[offset+channel] > alpha {
				t.Fatal("not premultiplied")
			}
			if alpha == 255 {
				difference += intAbs(int(first.Pix[offset+channel]) - int(second.Pix[offset+channel]))
				count++
			}
		}
	}
	if float64(difference)/float64(count) < 2 {
		t.Fatal("flow too subtle")
	}
	if first.RGBAAt(0, 0).A != 0 || first.RGBAAt(flowPixels/2, flowPixels/2).A != 255 {
		t.Fatal("invalid circular mask")
	}
}

func intAbs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func BenchmarkFlowFrame(b *testing.B) {
	texture := flowTexture("ice")
	target := image.NewRGBA(image.Rect(0, 0, flowPixels, flowPixels))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		renderFlow(target, texture, float64(index)/30)
	}
}

package app

import (
	"image"
	"testing"
)

// 关设置：预览和检查用的脚本、没在用的流光配色放掉；浮窗那份脚本和当前配色留着
func TestSettingsClosedDropsPreviewOnly(t *testing.T) {
	saved := flowViews
	t.Cleanup(func() { flowViews = saved })
	for i := range flowViews {
		flowViews[i][0].texture = image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	a := &App{}
	a.capWin.styles = map[string]*capsuleStyle{"aurora/": {}, "aurora/recording": {}, "aurora/check": {}, "orb/text": {}}
	a.capWin.list = []capsuleEntry{{folder: "aurora"}}
	cfg := defaultConfig()
	cfg.VoiceFlowPalette = flowPalettes[1].id
	a.settingsClosed(cfg)
	if len(a.capWin.styles) != 1 || a.capWin.styles["aurora/"] == nil {
		t.Fatalf("styles left: %v", a.capWin.styles)
	}
	if a.capWin.list != nil {
		t.Fatal("style list should be re-read next time")
	}
	for i := range flowViews {
		if kept := flowViews[i][0].texture != nil; kept != (i == 1) {
			t.Fatalf("palette %d kept=%v", i, kept)
		}
	}
}

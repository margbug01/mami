package app

import "testing"

// 「开始」之后：macOS 停在权限那屏，Windows 跳过它直接到「试试用说的」
func TestGuideAllowStep(t *testing.T) {
	a := &App{}
	a.guideGo(stepAllow)
	want := stepVoice
	if needPermissions {
		want = stepAllow
	}
	if a.set.guideStep != want {
		t.Fatalf("guideGo(stepAllow) -> step %d, want %d", a.set.guideStep, want)
	}
	a.guideGo(stepVoice)
	if a.set.guideStep != stepVoice {
		t.Fatalf("guideGo(stepVoice) -> step %d", a.set.guideStep)
	}
}

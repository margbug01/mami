//go:build windows

package app

import "testing"

func TestNearCaret(t *testing.T) {
	caret := RECT{Left: 500, Top: 1000, Right: 502, Bottom: 1020}
	for _, c := range []struct {
		pt   POINT
		want bool
	}{
		{POINT{X: 300, Y: 1010}, true},  // 同一行双击
		{POINT{X: 900, Y: 1045}, true},  // 下一行
		{POINT{X: 300, Y: 900}, false},  // 上面聊天记录
		{POINT{X: 300, Y: 1100}, false}, // 远在下面
	} {
		if got := nearCaret(caret, c.pt); got != c.want {
			t.Errorf("%v: got %v", c.pt, got)
		}
	}
}

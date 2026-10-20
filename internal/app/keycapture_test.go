package app

import "testing"

func TestKeyCaptureCombo(t *testing.T) {
	type ev struct {
		vk   uint32
		down bool
	}
	cases := []struct {
		name  string
		evs   []ev
		state int
		acc   string
	}{
		{"alt+t", []ev{{0xA4, true}, {'T', true}}, captureDone, "Alt+T"},
		{"right alt counts as alt", []ev{{0xA5, true}, {0xA0, true}, {'Y', true}}, captureDone, "Alt+Shift+Y"},
		{"ctrl+shift+f2", []ev{{0xA2, true}, {0xA1, true}, {0x71, true}}, captureDone, "Ctrl+Shift+F2"},
		{"bare f8 ok", []ev{{0x77, true}}, captureDone, "F8"},
		{"bare letter refused", []ev{{'T', true}, {'T', false}}, captureGoing, ""},
		{"released modifier no longer counts", []ev{{0xA4, true}, {0xA4, false}, {'T', true}}, captureGoing, ""},
		{"esc cancels", []ev{{0x1B, true}}, captureCancel, ""},
		{"backspace clears", []ev{{0x08, true}}, captureDone, ""},
		{"ctrl+backspace is a hotkey", []ev{{0xA2, true}, {0x08, true}}, captureDone, "Ctrl+Backspace"},
		{"unknown key refused", []ev{{0xA2, true}, {0xBA, true}}, captureGoing, ""},
	}
	for _, c := range cases {
		var k keyCapture
		state, acc := captureGoing, ""
		for _, e := range c.evs {
			if state, acc, _ = k.step(e.vk, e.down); state != captureGoing {
				break
			}
		}
		if state != c.state || acc != c.acc {
			t.Errorf("%s: got %d %q, want %d %q", c.name, state, acc, c.state, c.acc)
		}
	}
}

func TestKeyCaptureVoice(t *testing.T) {
	type ev struct {
		vk   uint32
		down bool
	}
	cases := []struct {
		name  string
		evs   []ev
		state int
		acc   string
	}{
		{"right ctrl tap", []ev{{0xA3, true}, {0xA3, false}}, captureDone, "RCtrl"},
		{"left alt tap", []ev{{0xA4, true}, {0xA4, false}}, captureDone, "LAlt"},
		{"modifier down alone not done", []ev{{0xA3, true}}, captureGoing, ""},
		{"altgr", []ev{{0xA2, true}, {0xA5, true}, {0xA5, false}}, captureDone, "RAlt"},
		{"combo", []ev{{0xA5, true}, {'T', true}}, captureDone, "Alt+T"},
		{"ctrl+space", []ev{{0xA2, true}, {0x20, true}}, captureDone, "Ctrl+Space"},
		{"f9", []ev{{0x78, true}}, captureDone, "F9"},
		{"capslock", []ev{{0x14, true}}, captureDone, "CapsLock"},
		{"bare letter refused", []ev{{'T', true}, {'T', false}}, captureGoing, ""},
		{"two modifiers released", []ev{{0xA2, true}, {0xA0, true}, {0xA0, false}, {0xA2, false}}, captureGoing, ""},
		{"backspace off", []ev{{0x08, true}}, captureDone, "Off"},
		{"esc cancels", []ev{{0x1B, true}}, captureCancel, ""},
	}
	for _, c := range cases {
		k := keyCapture{voice: true}
		state, acc := captureGoing, ""
		for _, e := range c.evs {
			if state, acc, _ = k.step(e.vk, e.down); state != captureGoing {
				break
			}
		}
		if state != c.state || acc != c.acc {
			t.Errorf("%s: got %d %q, want %d %q", c.name, state, acc, c.state, c.acc)
		}
	}
}

package app

import "testing"

func TestVoiceKeys(t *testing.T) {
	const trigger = 0xA3
	type key struct {
		vk      uint32
		down    bool
		busy    bool
		action  int
		swallow bool
	}
	tap := func(busy bool, action int) []key {
		return []key{{trigger, true, busy, voiceNone, false}, {trigger, false, busy, action, false}}
	}
	start := []key{{trigger, true, false, voiceStart, false}, {trigger, false, true, voiceNone, false}}
	join := func(parts ...[]key) (out []key) {
		for _, p := range parts {
			out = append(out, p...)
		}
		return
	}
	for _, tc := range []struct {
		name string
		keys []key
	}{
		{"toggle", join(start, []key{{'A', true, true, voiceNone, false}, {'A', false, true, voiceNone, false}}, tap(true, voiceStop))},
		{"repeat", []key{{trigger, true, false, voiceStart, false}, {trigger, true, true, voiceNone, false}, {trigger, false, true, voiceNone, false}, {trigger, true, true, voiceNone, false}, {trigger, false, true, voiceStop, false}}},
		{"combo-first", []key{{trigger, true, false, voiceStart, false}, {'C', true, true, voiceCancel, false}, {'C', false, false, voiceNone, false}, {trigger, false, false, voiceNone, false}, {trigger, true, false, voiceStart, false}}},
		{"combo-stop-ignored", join(start, []key{{trigger, true, true, voiceNone, false}, {'C', true, true, voiceNone, false}, {'C', false, true, voiceNone, false}, {trigger, false, true, voiceNone, false}}, tap(true, voiceStop))},
		{"prior-key", []key{{'C', true, false, voiceNone, false}, {trigger, true, false, voiceNone, false}, {'C', false, false, voiceNone, false}, {trigger, false, false, voiceNone, false}}},
		{"escape-recording", join(start, []key{{0x1B, true, true, voiceCancel, true}, {0x1B, true, false, voiceNone, true}, {0x1B, false, false, voiceNone, true}}, []key{{trigger, true, false, voiceStart, false}})},
		{"escape-first-press", []key{{trigger, true, false, voiceStart, false}, {0x1B, true, true, voiceCancel, true}, {0x1B, false, false, voiceNone, true}, {trigger, false, false, voiceNone, false}}},
		{"escape-asr", []key{{0x1B, true, true, voiceCancel, true}, {0x1B, false, false, voiceNone, true}}},
		{"idle-escape", []key{{0x1B, true, false, voiceNone, false}, {0x1B, false, false, voiceNone, false}}},
		{"busy-asr", tap(true, voiceNone)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var state voiceKeys
			for index, k := range tc.keys {
				action, swallow := state.step(k.vk, k.down, trigger, k.busy)
				if action != k.action || swallow != k.swallow {
					t.Fatalf("step %d: %d,%v want %d,%v", index, action, swallow, k.action, k.swallow)
				}
			}
		})
	}
	var state voiceKeys
	if action, _ := state.step(trigger, true, 0, false); action != voiceNone {
		t.Fatal("Off started")
	}
	if voiceKey("RAlt") != 0xA5 || voiceKey("RCtrl") != 0xA3 || voiceKey("RShift") != 0xA1 || voiceKey("Off") != 0 {
		t.Fatal("key mapping")
	}
}

func TestVoiceKeysHeal(t *testing.T) {
	const trigger = 0xA3
	var s voiceKeys
	// Win+L 锁屏：按下收到了，松开在安全桌面上收不到
	s.step(0x5B, true, trigger, false)
	s.step('L', true, trigger, false)
	if action, _ := s.step(trigger, true, trigger, false); action != voiceNone {
		t.Fatal("stuck keys should block, as before the fix")
	}
	s.step(trigger, false, trigger, false)
	if healed := s.heal(0, func(vk uint32) bool { return false }); len(healed) != 2 {
		t.Fatalf("healed %v", healed)
	}
	if action, _ := s.step(trigger, true, trigger, false); action != voiceStart {
		t.Fatal("trigger should start after healing")
	}
	// 触发键自己的松开丢了：held 清掉，下一下重新算
	s.heal(0, func(vk uint32) bool { return false })
	if s.held != 0 || s.pressed[trigger] {
		t.Fatal("held trigger not cleared")
	}
	// 真按着的键不动
	s = voiceKeys{}
	s.step('A', true, trigger, false)
	if healed := s.heal(0, func(vk uint32) bool { return vk == 'A' }); len(healed) != 0 || !s.pressed['A'] {
		t.Fatal("real key cleared")
	}
}

func TestVoiceOutput(t *testing.T) {
	for _, c := range []struct {
		switched, mods, input, caret bool
		want                         int
	}{
		{false, false, true, false, voicePaste},
		{false, false, true, true, voicePaste},
		{false, false, false, true, voicePasteKeep},
		{false, false, false, false, voiceCopyCard},
		{true, false, true, true, voiceCopyCard},
		{false, true, true, true, voiceCopyCard},
	} {
		if got := voiceOutput(c.switched, c.mods, c.input, c.caret); got != c.want {
			t.Errorf("%+v: got %d", c, got)
		}
	}
}

func TestVoiceSkins(t *testing.T) {
	for _, icon := range [][8]string{pixIconX, pixIconOK, pixIconCopy} {
		for _, row := range icon {
			if len(row) != 8 {
				t.Fatalf("pixel icon row %q", row)
			}
		}
	}
	if !toneDark("dark", false) || toneDark("light", true) || !toneDark("auto", true) || toneDark("auto", false) {
		t.Error("toneDark")
	}
	for in, want := range map[string]string{"": "dark", "x": "dark", "light": "light", "auto": "auto"} {
		cfg := defaultConfig()
		cfg.VoiceTone = in
		cfg.normalize()
		if cfg.VoiceTone != want {
			t.Errorf("tone %q → %q", in, cfg.VoiceTone)
		}
	}
	gaps := func(s voiceViewState) [2]float32 { l, r := islandGapsFor(s); return [2]float32{l, r} }
	if gaps(voiceViewState{recording: true}) != [2]float32{4, 4} || gaps(voiceViewState{recognizing: true}) != [2]float32{4, 0} ||
		gaps(voiceViewState{text: "x", copy: true}) != [2]float32{0, 4} || gaps(voiceViewState{text: "x"}) != [2]float32{0, 0} {
		t.Error("island gaps")
	}
	for _, style := range []string{"bars", "orb", "glass", "pixel", "mercury", "islands", "leds", "joined", "nope", ""} {
		cfg := defaultConfig()
		cfg.VoiceStyle = style
		cfg.normalize()
		want := style
		if style == "joined" {
			want = "leds"
			if !cfg.VoiceJoined {
				t.Error("joined style must migrate to connected layout")
			}
		}
		if style == "nope" || style == "" {
			want = "bars"
		}
		if cfg.VoiceStyle != want {
			t.Errorf("%q → %q", style, cfg.VoiceStyle)
		}
	}
}

// TestVoiceKeysAsyncState 钩子里问到的按键状态，Windows 是这次事件之前的，macOS 是之后的（松开事件送到时 HID 已经记成松开）。
// 两种都得是：单按一下开始录、再单按一下结束，打字时没有键被当成卡住
func TestVoiceKeysAsyncState(t *testing.T) {
	const trigger = 0xA5 // 右 Alt / 右 ⌥
	type ev struct {
		vk   uint32
		down bool
	}
	for _, after := range []bool{false, true} {
		var s voiceKeys
		physical := map[uint32]bool{}
		var actions []int
		var healed []uint32
		feed := func(events ...ev) {
			for _, e := range events {
				before := physical[e.vk]
				physical[e.vk] = e.down
				releasing := uint32(0)
				if !e.down {
					releasing = e.vk
				}
				healed = append(healed, s.heal(releasing, func(vk uint32) bool {
					if vk == e.vk && !after {
						return before
					}
					return physical[vk]
				})...)
				if a, _ := s.step(e.vk, e.down, trigger, s.recording); a != voiceNone {
					actions = append(actions, a)
				}
			}
		}
		feed(ev{'H', true}, ev{'I', true}, ev{'H', false}, ev{'I', false}) // 打字，两个键交叠
		feed(ev{trigger, true}, ev{trigger, false})                        // 单按：开始
		feed(ev{trigger, true}, ev{trigger, false})                        // 再单按：结束
		if len(healed) != 0 {
			t.Errorf("after=%v: healed %x while typing and tapping", after, healed)
		}
		if len(actions) != 2 || actions[0] != voiceStart || actions[1] != voiceStop {
			t.Errorf("after=%v: actions %v, want [start stop]", after, actions)
		}
	}
}

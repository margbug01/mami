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

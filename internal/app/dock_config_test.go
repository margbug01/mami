package app

import (
	"encoding/json"
	"math"
	"testing"
)

func TestDockDraftSyncPreservesEdits(t *testing.T) {
	before := defaultConfig()
	draft := before
	draft.Model = "unsaved model"
	draft.VoiceSound = !before.VoiceSound
	after := before
	after.VoiceTranslate = true
	after.VoiceDockPosition = dockPosition{Edge: "left", Fraction: 0.7, Monitor: "monitor2"}
	syncDockDraft(&draft, before, after)
	if !draft.VoiceTranslate || draft.VoiceDockPosition != after.VoiceDockPosition {
		t.Fatal("dock change was not synchronized")
	}
	if draft.Model != "unsaved model" || draft.VoiceSound == before.VoiceSound {
		t.Fatal("unrelated draft edits were lost")
	}
}

func TestDockConfigCompatibility(t *testing.T) {
	for _, test := range []struct {
		data    string
		enabled bool
	}{{`{}`, true}, {`{"voice_dock":false}`, false}} {
		cfg := defaultConfig()
		if err := json.Unmarshal([]byte(test.data), &cfg); err != nil {
			t.Fatal(err)
		}
		cfg.normalize()
		if cfg.VoiceDock != test.enabled {
			t.Fatalf("dock compatibility: %s", test.data)
		}
	}
	cfg := defaultConfig()
	cfg.VoiceDockPosition = dockPosition{Edge: "unknown", Fraction: math.Inf(1)}
	cfg.normalize()
	if cfg.VoiceDockPosition.Edge != "right" || cfg.VoiceDockPosition.Fraction != 0.5 {
		t.Fatal("invalid dock position not normalized")
	}
	cfg.VoiceDockPosition = dockPosition{Edge: "bottom", Fraction: 0.8, Monitor: "device-2"}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Config
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.VoiceDockPosition != cfg.VoiceDockPosition {
		t.Fatal("position did not round trip")
	}
}

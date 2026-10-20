package app

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestStreamASRRouting(t *testing.T) {
	if !isStreamASR("qwen-audio-3.1-asr-flash-streaming") || isStreamASR("qwen3-asr-flash") {
		t.Fatal("isStreamASR")
	}
	for in, want := range map[string]string{
		"https://dashscope.aliyuncs.com/compatible-mode/v1": "wss://dashscope.aliyuncs.com/api-ws/v1/inference",
		"https://maas.qianwenaiapi.com/compatible-mode/v1/": "wss://maas.qianwenaiapi.com/api-ws/v1/inference",
		"http://127.0.0.1:8080/v1":                          "ws://127.0.0.1:8080/api-ws/v1/inference",
	} {
		if got, err := streamURL(in); err != nil || got != want {
			t.Errorf("streamURL(%q) = %q, %v", in, got, err)
		}
	}
	if got := joinSentences(map[int]string{2: "第二句。", 0: "第一句，", 1: ""}); got != "第一句，第二句。" {
		t.Errorf("joinSentences = %q", got)
	}
}

func TestSavedKeysRoundTrip(t *testing.T) {
	m := rememberKey(nil, "https://api.stepfun.com/v1/", "sk-step")
	if m[keyFor("https://API.stepfun.com/v1")] != "sk-step" || rememberKey(m, "https://x/v1", " ")["https://x/v1"] != "" {
		t.Fatalf("rememberKey %v", m)
	}
	p := t.TempDir() + "/config.json"
	cfg := defaultConfig()
	cfg.SavedKeys = m
	cfg.PolishBaseURL, cfg.PolishAPIKey = "https://api.deepseek.com/v1", "sk-ds"
	if err := saveConfig(p, cfg); err != nil {
		t.Fatal(err)
	}
	back, _, err := loadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if back.SavedKeys["https://api.stepfun.com/v1"] != "sk-step" || back.SavedKeys["https://api.deepseek.com/v1"] != "sk-ds" {
		t.Fatalf("SavedKeys after reload: %d entries", len(back.SavedKeys))
	}
	if len(m) != 1 {
		t.Fatal("saveConfig mutated caller's map")
	}
}

func TestLiveText(t *testing.T) {
	done := map[int]string{1: "第一句。"}
	if got := liveText(done, map[int]string{2: "第二"}); got != "第一句。第二" {
		t.Errorf("liveText = %q", got)
	}
	if got := liveText(done, nil); got != "第一句。" {
		t.Errorf("liveText no partial = %q", got)
	}
	long := strings.Repeat("字", voiceLiveN+5)
	if tail := liveTail(long); utf8.RuneCountInString(tail) != voiceLiveN {
		t.Errorf("liveTail = %q", tail)
	}
	if liveTail("短句") != "短句" {
		t.Error("liveTail short")
	}
}

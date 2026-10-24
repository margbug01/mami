package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/coder/websocket"
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

// 服务端卡住时，流式识别要在时限内报错，而不是等满 asrTimeout
func TestASRStreamStallTimesOut(t *testing.T) {
	defer func(s, f time.Duration) { asrStartWait, asrFinishWait = s, f }(asrStartWait, asrFinishWait)
	asrStartWait, asrFinishWait = 300*time.Millisecond, 300*time.Millisecond
	for _, tc := range []struct {
		name, started, want string
	}{
		{"no task-started", "", "no response"},
		{"no task-finished", `{"header":{"event":"task-started"}}`, "no result"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				c, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer c.CloseNow()
				if tc.started != "" {
					c.Write(r.Context(), websocket.MessageText, []byte(tc.started))
				}
				for { // 收着音频，就是不回结果
					if _, _, err := c.Read(r.Context()); err != nil {
						return
					}
				}
			}))
			defer srv.Close()
			cfg := Config{ASRBaseURL: srv.URL + "/compatible-mode/v1", ASRModel: "x-streaming", ASRAPIKey: "k"}
			s := startASRStream(context.Background(), cfg, nil)
			defer s.stop()
			s.feed(make([]byte, 3200))
			t0 := time.Now()
			_, err := s.finish(asrFinishWait)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if d := time.Since(t0); d > 2*time.Second {
				t.Fatalf("took %v", d)
			}
		})
	}
}

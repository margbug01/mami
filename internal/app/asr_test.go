package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestASRRequestAndSSE(t *testing.T) {
	expected := `{"audio":{"data":"AQIDBA==","input":{"transcription":{"language":"zh","model":"stepaudio-3-asr-max","enable_itn":true,"hotwords":["接口"]},"format":{"type":"pcm","codec":"pcm_s16le","rate":16000,"bits":16,"channel":1}}}}`
	var want any
	json.Unmarshal([]byte(expected), &want)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/asr/sse" || r.Method != "POST" || r.Header.Get("Authorization") != "Bearer test-key" || r.Header.Get("Accept") != "text/event-stream" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("incorrect request metadata")
		}
		var got any
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("incorrect request body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(": ping\n\ndata: {\"type\":\"transcript.text.delta\",\"delta\":\"草稿\"}\n\ndata: {\"type\":\"transcript.text.done\",\"text\":\"最终接口\"}\n\n"))
	}))
	defer server.Close()
	cfg := defaultConfig()
	cfg.ASRBaseURL, cfg.ASRAPIKey, cfg.ASRHotwords, cfg.Proxy = server.URL, "test-key", []string{"接口"}, "http://127.0.0.1:1"
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	got, err := transcribe(context.Background(), cfg, []byte{1, 2, 3, 4})
	if err != nil || got != "最终接口" {
		t.Fatalf("got %q, %v", got, err)
	}
}

func TestASRErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		want       string
	}{
		{"event", `data: {"type":"error","message":"bad key"}` + "\n", 200, "bad key"},
		{"http", "denied", 401, "HTTP 401: denied"},
		{"redact", "secret-key", 403, "[redacted]"},
		{"malformed", "data: {\n", 200, "ASR SSE"},
		{"truncated", `data: {"type":"transcript.text.delta","delta":"half"}` + "\n", 200, "未收到最终结果"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); w.Write([]byte(tc.body)) }))
			defer server.Close()
			cfg := defaultConfig()
			cfg.ASRBaseURL, cfg.ASRAPIKey = server.URL, "secret-key"
			_, err := transcribe(context.Background(), cfg, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret-key") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
	for _, body := range []string{"", "data: {\"type\":\"transcript.text.done\",\"text\":\"\"}\n"} {
		if got, err := readASRStream(strings.NewReader(body)); got != "" || err != nil {
			t.Fatalf("silence: %q %v", got, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := defaultConfig()
	cfg.ASRAPIKey = "test"
	if _, err := transcribe(ctx, cfg, nil); err == nil {
		t.Fatal("canceled request succeeded")
	}
}

func TestVoiceConfigNormalize(t *testing.T) {
	cfg := defaultConfig()
	if cfg.VoiceEnabled || cfg.VoiceHotkey != "RAlt" {
		t.Fatal("upgrade must be opt-in")
	}
	cfg.VoiceHotkey = "invalid"
	cfg.ASRBaseURL = " https://example.test/v1/// "
	cfg.ASRModel = " "
	cfg.ASRHotwords = nil
	cfg.normalize()
	if cfg.VoiceHotkey != "RAlt" || cfg.ASRBaseURL != "https://example.test/v1" || cfg.ASRModel != "stepaudio-3-asr-max" || cfg.ASRHotwords == nil {
		t.Fatal("normalize")
	}
	for _, key := range []string{"RAlt", "RCtrl", "RShift", "LAlt", "LCtrl", "F9", "CapsLock", "Ctrl+Space", "Off"} {
		cfg.VoiceHotkey = key
		cfg.normalize()
		if cfg.VoiceHotkey != key {
			t.Fatal(key)
		}
	}
	if got := splitList("Go, 接口\n语音，StepFun"); !reflect.DeepEqual(got, []string{"Go", "接口", "语音", "StepFun"}) {
		t.Fatal(got)
	}
}

package app

import (
	"context"
	"encoding/base64"
	"encoding/binary"
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
	got, err := transcribeStepFun(context.Background(), cfg, []byte{1, 2, 3, 4})
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
			_, err := transcribeStepFun(context.Background(), cfg, nil)
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

func TestTranscribeChat(t *testing.T) {
	var got map[string]any
	var auth, path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, path = r.Header.Get("Authorization"), r.URL.Path
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":" 四点开会。 "}}]}`))
	}))
	defer server.Close()
	cfg := defaultConfig()
	cfg.ASRBaseURL, cfg.ASRModel, cfg.ASRAPIKey, cfg.ASRHotwords = server.URL+"/compatible-mode/v1", "qwen3-asr-flash", "sk-test", []string{"Mami", "StepFun"}
	text, err := transcribe(context.Background(), cfg, []byte{1, 2, 3, 4})
	if err != nil || text != "四点开会。" {
		t.Fatalf("got %q %v", text, err)
	}
	if path != "/compatible-mode/v1/chat/completions" || auth != "Bearer sk-test" || got["model"] != "qwen3-asr-flash" {
		t.Fatalf("request: %s %s %v", path, auth, got["model"])
	}
	msgs := got["messages"].([]any)
	if sys := msgs[0].(map[string]any); sys["role"] != "system" || sys["content"].([]any)[0].(map[string]any)["text"] != "Mami，StepFun" {
		t.Fatalf("hotwords: %v", sys)
	}
	audio := msgs[1].(map[string]any)["content"].([]any)[0].(map[string]any)["input_audio"].(map[string]any)["data"].(string)
	b64, ok := strings.CutPrefix(audio, "data:audio/wav;base64,")
	wav, _ := base64.StdEncoding.DecodeString(b64)
	if !ok || len(wav) != 48 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || binary.LittleEndian.Uint32(wav[24:]) != 16000 || binary.LittleEndian.Uint32(wav[40:]) != 4 {
		t.Fatalf("wav header: %v", wav)
	}
	if got["asr_options"].(map[string]any)["enable_itn"] != true {
		t.Fatal("asr_options")
	}
}

func TestIsStepFunASR(t *testing.T) {
	for base, want := range map[string]bool{
		"https://api.stepfun.com/v1": true, "https://stepfun.com/v1": true,
		"https://dashscope.aliyuncs.com/compatible-mode/v1": false, "https://evilstepfun.com/v1": false, "https://stepfun.com.evil.io/v1": false,
	} {
		if isStepFunASR(base) != want {
			t.Errorf("%s: want %v", base, want)
		}
	}
}

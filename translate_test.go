package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEndpoint(t *testing.T) {
	cases := map[string]string{
		"https://api.openai.com/v1":                  "https://api.openai.com/v1/chat/completions",
		"https://x.com/v1/":                          "https://x.com/v1/chat/completions",
		" https://x.com/v1/chat/completions ":        "https://x.com/v1/chat/completions",
		"http://localhost:8317/v1/chat/completions/": "http://localhost:8317/v1/chat/completions",
	}
	for in, want := range cases {
		if got := endpoint(in); got != want {
			t.Errorf("endpoint(%q)=%q want %q", in, got, want)
		}
	}
}

func TestPickTarget(t *testing.T) {
	cfg := defaultConfig()
	for text, want := range map[string]string{
		"帮我看一下这个插件":       "English",
		"帮我 fix this bug": "English",
		"Hello world":     "Simplified Chinese",
		"日本語のテキスト":        "Simplified Chinese",
		"한국어 텍스트":         "Simplified Chinese",
	} {
		if got, _ := pickTarget(text, cfg); got != want {
			t.Errorf("pickTarget(%q)=%q want %q", text, got, want)
		}
	}
}

func TestCleanOutput(t *testing.T) {
	for in, want := range map[string]string{
		"<think>hmm</think>\nHello": "Hello",
		"```\nHello\nWorld\n```":    "Hello\nWorld",
		"```text\nHello\n```":       "Hello",
		"  Hello  ":                 "Hello",
		"Use `fmt.Println` here":    "Use `fmt.Println` here",
	} {
		if got := cleanOutput(in); got != want {
			t.Errorf("cleanOutput(%q)=%q want %q", in, got, want)
		}
	}
}

func TestTranslateOK(t *testing.T) {
	var gotAuth, gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth, gotPath = r.Header.Get("Authorization"), r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"<think>x</think>Hello, world"}}]}`))
	}))
	defer srv.Close()

	cfg := defaultConfig()
	cfg.BaseURL, cfg.APIKey, cfg.Model = srv.URL+"/v1", "sk-test", "m1"
	tr, _ := newTranslator(cfg)
	got, err := tr.Translate(context.Background(), "你好，世界", "English")
	if err != nil || got != "Hello, world" {
		t.Fatalf("got %q %v", got, err)
	}
	if gotAuth != "Bearer sk-test" || gotPath != "/v1/chat/completions" || gotBody["model"] != "m1" {
		t.Fatalf("auth=%q path=%q body=%v", gotAuth, gotPath, gotBody)
	}
	msgs := gotBody["messages"].([]any)
	sys := msgs[0].(map[string]any)["content"].(string)
	user := msgs[1].(map[string]any)["content"].(string)
	if !strings.Contains(sys, "into English") || user != "你好，世界" {
		t.Fatalf("sys=%q user=%q", sys, user)
	}
	if _, ok := gotBody["temperature"]; ok {
		t.Fatal("temperature should be omitted by default")
	}
}

func TestTranslateArrayContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":[{"type":"text","text":"你好"}]}}]}`))
	}))
	defer srv.Close()
	cfg := defaultConfig()
	cfg.BaseURL, cfg.APIKey = srv.URL, "k"
	tr, _ := newTranslator(cfg)
	if got, err := tr.Translate(context.Background(), "hi", "Chinese"); err != nil || got != "你好" {
		t.Fatalf("got %q %v", got, err)
	}
}

func TestTranslateErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer srv.Close()
	cfg := defaultConfig()
	cfg.BaseURL, cfg.APIKey = srv.URL, "bad"
	tr, _ := newTranslator(cfg)
	_, err := tr.Translate(context.Background(), "hi", "Chinese")
	if err == nil || !strings.Contains(err.Error(), "401") || !strings.Contains(err.Error(), "invalid api key") {
		t.Fatalf("err=%v", err)
	}

	cfg.APIKey, cfg.Model = "", "deepseek-flash"
	tr, _ = newTranslator(cfg)
	if _, err := tr.Translate(context.Background(), "hi", "Chinese"); err == nil || !strings.Contains(err.Error(), "API Key") {
		t.Fatalf("missing key err=%v", err)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "config.json")
	cfg, created, err := loadConfig(p)
	if err != nil || !created || cfg.Model != "Index-Translate-35B-A3B" || cfg.BaseURL != "https://index-translate.bilibili.com/v1" {
		t.Fatalf("default: %+v created=%v err=%v", cfg, created, err)
	}
	cfg.APIKey, cfg.ExcludeProcesses = "k1", splitList("a.exe， b.exe;\n c.exe ,")
	if err := saveConfig(p, cfg); err != nil {
		t.Fatal(err)
	}
	got, created, err := loadConfig(p)
	if err != nil || created || got.APIKey != "k1" || len(got.ExcludeProcesses) != 3 || got.ExcludeProcesses[1] != "b.exe" {
		t.Fatalf("reload: %+v created=%v err=%v", got, created, err)
	}
	os.WriteFile(p, []byte("\xef\xbb\xbf{\"model\":\"m2\",\"max_chars\":0}"), 0o600)
	got, _, err = loadConfig(p)
	if err != nil || got.Model != "m2" || got.MaxChars != 6000 || got.BaseURL == "" {
		t.Fatalf("bom/partial: %+v %v", got, err)
	}
}

// MAMI_LIVE_KEY=sk-... go test -run Live -v  走真实的 DeepSeek
func TestLiveDeepSeek(t *testing.T) {
	key := os.Getenv("MAMI_LIVE_KEY")
	if key == "" {
		t.Skip("MAMI_LIVE_KEY not set")
	}
	cfg := defaultConfig()
	cfg.APIKey = key
	tr, _ := newTranslator(cfg)
	for _, c := range []struct{ text, target string }{
		{"帮我看一下这个插件为什么启动后没有生效，日志里什么都没有。", "English"},
		{"Could you check why the plugin does not take effect after startup?", "Simplified Chinese"},
	} {
		got, err := tr.Translate(context.Background(), c.text, c.target)
		if err != nil || got == "" {
			t.Fatalf("%q: %q %v", c.text, got, err)
		}
		t.Logf("%s → %s", c.text, got)
	}
}

// B 站 Index-Translate：没 key 也能发、不带 Authorization，用它自己的单条 user 提示词并关掉思考
func TestIndexTranslateRequest(t *testing.T) {
	var got map[string]any
	var auth, ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth, ua = r.Header.Get("Authorization"), r.Header.Get("User-Agent")
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"choices":[{"message":{"content":"<think></think>Hello"}}]}`))
	}))
	defer srv.Close()
	cfg := defaultConfig()
	cfg.BaseURL, cfg.APIKey = srv.URL, ""
	tr, _ := newTranslator(cfg)
	res, err := tr.Translate(context.Background(), " 你好 ", "English")
	if err != nil || res != "Hello" {
		t.Fatalf("res=%q err=%v", res, err)
	}
	if auth != "" || ua == "" || strings.HasPrefix(ua, "Go-http-client") {
		t.Errorf("auth=%q ua=%q", auth, ua)
	}
	msgs := got["messages"].([]any)
	if len(msgs) != 1 {
		t.Fatalf("messages %v", msgs)
	}
	m := msgs[0].(map[string]any)
	want := "请将以下文本翻译为English，直接输出翻译结果，不要进行任何解释。\n\n你好"
	if m["role"] != "user" || m["content"] != want {
		t.Errorf("prompt %q", m["content"])
	}
	if k, _ := got["chat_template_kwargs"].(map[string]any); k == nil || k["enable_thinking"] != false {
		t.Errorf("chat_template_kwargs %v", got["chat_template_kwargs"])
	}
	if got["temperature"] != float64(0) {
		t.Errorf("temperature %v", got["temperature"])
	}
}

func TestIndexPromptExtra(t *testing.T) {
	p := indexPrompt("碳纤维", "English", "术语保留英文\n【硬性要求】碳纤维→carbon fiber")
	want := "请将以下文本翻译成English，并且严格遵循所有约束要求。\n\n【源文】\n碳纤维\n\n【约束要求】\n1. 【注意】术语保留英文\n2. 【硬性要求】碳纤维→carbon fiber\n\n只输出译文，不要有任何额外说明。"
	if p != want {
		t.Errorf("got %q", p)
	}
}

// 微信这类问不出选区的程序：鼠标要和输入光标在同一行附近才算
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

func TestHyPrompt(t *testing.T) {
	if p := hyPrompt(" 你好 ", "English", ""); p != "将以下文本翻译为English，注意只需要输出翻译后的结果，不要额外解释：\n\n你好" {
		t.Errorf("zh: %q", p)
	}
	if p := hyPrompt("Hallo", "English", ""); p != "Translate the following segment into English, without additional explanation.\n\nHallo" {
		t.Errorf("xx: %q", p)
	}
	if p := hyPrompt("hi", "Simplified Chinese", "a\n\nb"); p != "将以下文本翻译为Simplified Chinese，注意只需要输出翻译后的结果，不要额外解释。翻译要求：\n1. a\n2. b\n\n待翻译文本：\n\nhi" {
		t.Errorf("extra: %q", p)
	}
}

func TestButtonActionNormalize(t *testing.T) {
	for in, want := range map[string]string{"": actionCard, "bogus": actionCard, actionResult: actionResult, actionReplace: actionReplace} {
		c := defaultConfig()
		c.ButtonAction = in
		c.normalize()
		if c.ButtonAction != want {
			t.Errorf("%q → %q", in, c.ButtonAction)
		}
	}
}

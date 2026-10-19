package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestPolishRequest(t *testing.T) {
	for _, host := range []string{"api.deepseek.com", "deepseek.com", "API.DEEPSEEK.COM", "other.test", "notdeepseek.com", "deepseek.com.other.test"} {
		for _, style := range []string{"light", "standard", "structured"} {
			t.Run(host+"/"+style, func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-key" || !strings.HasPrefix(r.RequestURI, "/") {
						t.Error("request headers/path")
					}
					var body struct {
						Model       string              `json:"model"`
						Temperature float64             `json:"temperature"`
						Stream      bool                `json:"stream"`
						Messages    []map[string]string `json:"messages"`
						Thinking    map[string]string   `json:"thinking"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
						return
					}
					wantThinking := strings.EqualFold(host, "api.deepseek.com") || host == "deepseek.com"
					if (body.Thinking != nil) != wantThinking || (wantThinking && body.Thinking["type"] != "disabled") {
						t.Error("thinking provider guard")
					}
					wantPrompt := polishBasePrompt + polishStyles[style] + "\n以下是用户的专有名词，出现时按这个写法原样保留：Go、Mami、DeepSeek"
					if body.Model != "deepseek-flash" || body.Temperature != 0.3 || body.Stream || len(body.Messages) != 2 {
						t.Error("request body")
						return
					}
					if body.Messages[0]["role"] != "system" || body.Messages[0]["content"] != wantPrompt || body.Messages[1]["role"] != "user" || body.Messages[1]["content"] != polishSample {
						t.Error("messages")
					}
					io.WriteString(w, `{"choices":[{"message":{"content":"  帮我把这个接口改成用 Go 写，四点开会。  "}}]}`)
				}))
				defer server.Close()
				original := http.DefaultTransport
				transport := original.(*http.Transport).Clone()
				address := server.Listener.Addr().String()
				transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, address)
				}
				proxyURL, _ := url.Parse("http://127.0.0.1:1")
				transport.Proxy = http.ProxyURL(proxyURL)
				http.DefaultTransport = transport
				defer func() { http.DefaultTransport = original; transport.CloseIdleConnections() }()
				cfg := defaultConfig()
				cfg.PolishBaseURL, cfg.PolishAPIKey, cfg.PolishStyle = "http://"+host+"/v1///", "test-key", style
				cfg.Proxy = "http://127.0.0.1:1"
				cfg.ASRHotwords = []string{"Go", "Mami", "DeepSeek"}
				got, err := polish(context.Background(), cfg, polishSample)
				if err != nil || got != "帮我把这个接口改成用 Go 写，四点开会。" {
					t.Fatalf("%q %v", got, err)
				}
			})
		}
	}
}

func TestPolishWithoutHotwordsAndRedirect(t *testing.T) {
	received := make(chan struct{}, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received <- struct{}{} }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]string `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if len(body.Messages) != 2 || body.Messages[0]["content"] != polishBasePrompt+polishStyles["standard"] {
			t.Error("unexpected hotword prompt")
		}
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	cfg := defaultConfig()
	cfg.PolishBaseURL, cfg.PolishAPIKey = server.URL, "test-key"
	_, err := polish(context.Background(), cfg, polishSample)
	if err == nil || !strings.Contains(err.Error(), "HTTP 307") || len(received) != 0 {
		t.Fatal("redirect must not forward key")
	}
}

func TestCleanPolishOutput(t *testing.T) {
	for _, input := range []string{"  原文。 \n", "```text\n原文。\n```", "\"原文。\"", "“原文。”", "「原文。」", "'原文。'", "```\n\"原文。\"\n```", "\"```text\n原文。\n```\"", "<think>hidden</think>原文。"} {
		if got := cleanPolishOutput(input); got != "原文。" {
			t.Errorf("%q => %q", input, got)
		}
	}
	if cleanPolishOutput("\"\"") != "" || cleanPolishOutput("\"问句？") != "\"问句？" {
		t.Fatal("empty/unmatched quotes")
	}
}

func TestPolishVoiceFallback(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		delay      time.Duration
		want       string
	}{
		{"success", `{"choices":[{"message":{"content":"最终结果"}}]}`, 200, 0, "最终结果"},
		{"http", "test-key private-source", 503, 0, "private-source"},
		{"empty", `{"choices":[{"message":{"content":"  "}}]}`, 200, 0, "private-source"},
		{"no choices", `{"choices":[]}`, 200, 0, "private-source"},
		{"invalid", `invalid test-key private-source`, 200, 0, "private-source"},
		{"timeout", "", 200, 3 * time.Second, "private-source"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.delay > 0 {
					select {
					case <-time.After(tc.delay):
					case <-r.Context().Done():
						return
					}
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer server.Close()
			cfg := defaultConfig()
			cfg.PolishBaseURL, cfg.PolishAPIKey = server.URL, "test-key"
			var logs bytes.Buffer
			previous := log.Writer()
			log.SetOutput(&logs)
			defer log.SetOutput(previous)
			got, elapsed := polishVoice(context.Background(), cfg, "private-source")
			if got != tc.want {
				t.Fatalf("got %q", got)
			}
			if tc.delay > 0 && (elapsed < 2400 || elapsed > 2900) {
				t.Fatalf("timeout %d ms", elapsed)
			}
			if strings.Contains(logs.String(), "test-key") || strings.Contains(logs.String(), "private-source") {
				t.Fatal("sensitive log")
			}
			if tc.name != "success" && logs.Len() == 0 {
				t.Fatal("missing fallback log")
			}
			if tc.status != 200 {
				_, err := polish(context.Background(), cfg, "private-source")
				if err == nil || !strings.Contains(err.Error(), "HTTP 503") || !strings.Contains(err.Error(), "private-source") || strings.Contains(err.Error(), "test-key") {
					t.Fatal("HTTP body/redaction")
				}
			}
		})
	}
}

func TestPolishCancelAndSkip(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	cfg := defaultConfig()
	cfg.PolishBaseURL, cfg.PolishAPIKey = server.URL, "test-key"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan string, 1)
	go func() { result, _ := polishVoice(ctx, cfg, "source"); done <- result }()
	<-entered
	cancel()
	select {
	case got := <-done:
		if got != "source" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("parent cancellation ignored")
	}
	cfg.PolishStyle = "off"
	if got, elapsed := polishVoice(context.Background(), cfg, "source"); got != "source" || elapsed != 0 {
		t.Fatal("off should skip")
	}
	cfg.PolishStyle, cfg.PolishAPIKey = "standard", ""
	if got, elapsed := polishVoice(context.Background(), cfg, "source"); got != "source" || elapsed != 0 {
		t.Fatal("no key should skip")
	}
}

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestVoiceTranslatePreview(t *testing.T) {
	path := os.Getenv("MAMI_VOICE_TRANSLATE_SNAP")
	if path == "" {
		t.Skip("MAMI_VOICE_TRANSLATE_SNAP not set")
	}
	a := &App{voiceTranslate: true, voiceTarget: "English"}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(20).Gap(14).Background(ui.RGBA(70, 73, 80, 1)).Children(func() {
			for _, dark := range []bool{true, false} {
				for _, style := range []string{"bars", "glass", "mercury", "pixel", "islands", "leds"} {
					ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
						ui.Text(c, fmt.Sprintf("%s dark=%v", style, dark)).Width(150).FontSize(12).TextColor(pillFg)
						for _, state := range []voiceViewState{{recording: true}, {recognizing: true}} {
							var bars [voiceBars]float32
							for i := range bars {
								bars[i] = float32(i+1) / voiceBars
							}
							a.voicePill(c, style, dark, skinFor(style, dark), state, bars, 0.4, islandGap, islandGap, 1, time.Now(), false, false, "ice").Width(float32(voiceRecW))
						}
					})
				}
			}
		})
	}, 520, 660)
	tt.Frame()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestVoiceTranslateCorrectionsBeforeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]string `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if got := body.Messages[1]["content"]; got != "正词" {
			t.Errorf("input = %q", got)
		}
		fmt.Fprint(w, `{"choices":[{"message":{"content":"错词"}}]}`)
	}))
	defer server.Close()
	cfg := defaultConfig()
	cfg.VoiceTranslate, cfg.PolishAPIKey, cfg.PolishBaseURL = true, "test", server.URL
	got, elapsed := polishVoice(context.Background(), cfg, "错词", learnedWord{Wrong: "错词", Right: "正词"})
	if got != "错词" || elapsed < 1 {
		t.Fatalf("output must not be corrected or guarded: %q, %d", got, elapsed)
	}
}

func TestVoiceTranslateConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.VoiceTranslate || cfg.VoiceTarget != "English" {
		t.Fatal("defaults", cfg.VoiceTarget)
	}
	for _, target := range []string{"", "  ", " Japanese "} {
		cfg.VoiceTarget = target
		cfg.normalize()
		want := strings.TrimSpace(target)
		if want == "" {
			want = "English"
		}
		if cfg.VoiceTarget != want {
			t.Fatalf("target = %q, want %q", cfg.VoiceTarget, want)
		}
	}
}

func TestVoiceTranslatePrompt(t *testing.T) {
	cfg := defaultConfig()
	if voicePolishPrompt(cfg) != polishBasePrompt+polishStyles[cfg.PolishStyle] {
		t.Fatal("ordinary prompt changed")
	}
	cfg.VoiceTranslate, cfg.VoiceTarget, cfg.PolishStyle = true, "Japanese", "off"
	prompt := voicePolishPrompt(cfg)
	if strings.Contains(prompt, "- 不翻译，原话什么语言就输出什么语言；") || !strings.Contains(prompt, "Japanese") || !strings.Contains(prompt, polishStyles["light"]) {
		t.Fatal(prompt)
	}
}

func TestVoiceLanguageCode(t *testing.T) {
	for target, want := range map[string]string{"English": "EN", "Japanese": "JA", "Korean": "KO", "Simplified Chinese": "ZH", "Traditional Chinese": "ZH", "Dutch": "NL", "Malay": "MS", " esperanto ": "ES", "X": "X", "": ""} {
		if got := voiceLanguageCode(target); got != want {
			t.Errorf("%q = %q, want %q", target, got, want)
		}
	}
	for _, lang := range targetLangs {
		if len(voiceLanguageCode(lang.value)) != 2 {
			t.Error(lang.value)
		}
	}
}

func TestVoiceTranslateRequests(t *testing.T) {
	for _, mode := range []string{"off", "short", "missing-key", "failed", "both-failed"} {
		t.Run(mode, func(t *testing.T) {
			polishCalls, fallbackCalls := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Messages []map[string]string `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.URL.Path == "/polish/chat/completions" {
					polishCalls++
					if !strings.Contains(body.Messages[0]["content"], "English") {
						t.Error("missing target")
					}
					if mode == "failed" || mode == "both-failed" {
						w.WriteHeader(503)
						return
					}
				} else {
					fallbackCalls++
					if mode == "both-failed" {
						w.WriteHeader(503)
						return
					}
				}
				fmt.Fprint(w, `{"choices":[{"message":{"content":"Hello."}}]}`)
			}))
			defer server.Close()
			cfg := defaultConfig()
			cfg.VoiceTranslate = true
			cfg.PolishAPIKey, cfg.APIKey = "test", "test"
			cfg.PolishBaseURL, cfg.BaseURL, cfg.Model = server.URL+"/polish", server.URL+"/translate", "test"
			if mode == "off" {
				cfg.PolishStyle = "off"
			}
			if mode == "missing-key" {
				cfg.PolishAPIKey = ""
			}
			got, elapsed := polishVoice(context.Background(), cfg, "你好")
			if mode == "both-failed" {
				if got != "你好" || elapsed != 0 {
					t.Fatalf("fallback = %q, %d", got, elapsed)
				}
			} else if got != "Hello." || elapsed < 1 {
				t.Fatalf("translation = %q, %d", got, elapsed)
			}
			wantPolish, wantFallback := 1, 0
			if mode == "missing-key" {
				wantPolish = 0
			}
			if mode == "missing-key" || mode == "failed" || mode == "both-failed" {
				wantFallback = 1
			}
			if polishCalls != wantPolish || fallbackCalls != wantFallback {
				t.Fatalf("calls = %d/%d, want %d/%d", polishCalls, fallbackCalls, wantPolish, wantFallback)
			}
		})
	}
}

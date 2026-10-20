//go:build windows

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPolishConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"voice_enabled":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := loadConfig(path)
	if err != nil || cfg.PolishStyle != "standard" || cfg.PolishBaseURL != "https://api.deepseek.com/v1" || cfg.PolishModel != "deepseek-flash" {
		t.Fatal("upgrade defaults", err)
	}
	cfg.PolishStyle, cfg.PolishBaseURL, cfg.PolishModel, cfg.PolishAPIKey = "invalid", " https://example.test/v1/// ", " ", " test-secret "
	cfg.normalize()
	if cfg.PolishStyle != "standard" || cfg.PolishBaseURL != "https://example.test/v1" || cfg.PolishModel != "deepseek-flash" || cfg.PolishAPIKey != "test-secret" {
		t.Fatal("normalize")
	}
	for _, style := range []string{"off", "light", "standard", "structured"} {
		cfg.PolishStyle = style
		cfg.normalize()
		if cfg.PolishStyle != style {
			t.Fatal("valid style changed")
		}
	}
	cfg.PolishBaseURL = " / "
	cfg.normalize()
	if cfg.PolishBaseURL != "https://api.deepseek.com/v1" {
		t.Fatal("empty URL default")
	}
	if err := saveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk Config
	if err := json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if disk.PolishAPIKey != "" || disk.PolishAPIKeyDPAPI == "" || bytes.Contains(raw, []byte(cfg.PolishAPIKey)) {
		t.Fatal("key not encrypted")
	}
	loaded, _, err := loadConfig(path)
	if err != nil || loaded.PolishAPIKey != cfg.PolishAPIKey || loaded.PolishAPIKeyDPAPI != "" {
		t.Fatal("DPAPI roundtrip", err)
	}
	if err := os.WriteFile(path, []byte(`{"polish_api_key":"legacy-secret"}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, _, err = loadConfig(path)
	if err != nil || loaded.PolishAPIKey != "legacy-secret" {
		t.Fatal("plaintext load", err)
	}
	raw, err = os.ReadFile(path)
	if err != nil || bytes.Contains(raw, []byte("legacy-secret")) || !bytes.Contains(raw, []byte("polish_api_key_dpapi")) {
		t.Fatal("plaintext migration", err)
	}
	if err := os.WriteFile(path, []byte(`{"polish_api_key_dpapi":"invalid"}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, _, err = loadConfig(path)
	if err != nil || loaded.PolishAPIKey != "" || loaded.PolishAPIKeyDPAPI != "" {
		t.Fatal("invalid ciphertext", err)
	}
}

func TestPolishLive(t *testing.T) {
	if os.Getenv("MAMI_POLISH_LIVE") != "1" {
		t.Skip("set MAMI_POLISH_LIVE=1 to call polishing API")
	}
	cfg, _, err := loadConfig(filepath.Join(os.Getenv("APPDATA"), "Mami", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PolishAPIKey == "" {
		t.Fatal("missing polishing key")
	}
	for _, style := range []string{"light", "standard", "structured"} {
		t.Run(style, func(t *testing.T) {
			cfg.PolishStyle = style
			ctx, cancel := context.WithTimeout(context.Background(), polishDeadline(polishSample))
			defer cancel()
			started := time.Now()
			result, err := polish(ctx, cfg, polishSample)
			elapsed := time.Since(started)
			if err != nil {
				t.Fatalf("polishing failed after %s (%T)", elapsed, err)
			}
			if result == "" || (!strings.Contains(result, "四点") && !strings.Contains(result, "4 点")) || strings.Contains(result, "三点") {
				t.Fatal("polishing result failed correction check")
			}
			t.Logf("%s: %s (%d ms)", style, strings.ReplaceAll(result, cfg.PolishAPIKey, "[redacted]"), elapsed.Milliseconds())
		})
	}
}

func TestPolishCancelClick(t *testing.T) {
	previousEpoch := voiceEpoch.Load()
	voiceActive.Store(123)
	voiceRecording.Store(0)
	voiceClick(voiceCancel)
	if voiceEpoch.Load() != previousEpoch+1 || voiceActive.Load() != 0 {
		t.Fatal("recognizing cancellation did not invalidate generation")
	}
	select {
	case event := <-voiceEvents:
		if event.action != voiceCancel || event.id != 123 {
			t.Fatal("cancel event")
		}
	default:
		t.Fatal("missing cancel event")
	}
}

func TestPolishHistory(t *testing.T) {
	dir := t.TempDir()
	h := &history{dir: dir}
	h.write(historyEntry{Type: "voice", ID: "voice-test", Time: time.Now().Format(time.RFC3339), Source: polishSample, Translation: "四点开会。", PolishStyle: "standard", PolishMs: 800})
	entries := readHistory(t, dir)
	if len(entries) != 1 || entries[0].Source != polishSample || entries[0].Translation != "四点开会。" || entries[0].PolishStyle != "standard" || entries[0].PolishMs != 800 {
		t.Fatal("voice history roundtrip")
	}
	stats := loadHistoryStats(dir, nil)
	if stats.total != 0 || stats.sumMs != 0 || len(stats.days) != 0 || stats.nFirst != 0 || stats.used != 0 || stats.unused != 0 {
		t.Fatal("voice affected translation statistics")
	}
}

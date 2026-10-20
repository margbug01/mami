//go:build windows

package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
	"unsafe"
)

func stepFunCredential(t *testing.T) string {
	t.Helper()
	advapi := syscall.NewLazyDLL("advapi32.dll")
	var credential *struct {
		Flags, Type             uint32
		Target, Comment         *uint16
		Written                 uint64
		BlobSize                uint32
		Blob                    *byte
		Persist, Count          uint32
		Attributes, Alias, User uintptr
	}
	result, _, err := advapi.NewProc("CredReadW").Call(uintptr(unsafe.Pointer(u16("Alma/StepFun"))), 1, 0, uintptr(unsafe.Pointer(&credential)))
	if result == 0 {
		t.Fatalf("CredReadW Alma/StepFun: %v", err)
	}
	defer advapi.NewProc("CredFree").Call(uintptr(unsafe.Pointer(credential)))
	if credential.BlobSize == 0 || credential.BlobSize > 1<<20 {
		t.Fatal("empty or invalid credential blob")
	}
	blob := unsafe.Slice(credential.Blob, credential.BlobSize)
	var key string
	if bytes.Contains(blob, []byte{0}) {
		if len(blob)%2 != 0 {
			t.Fatal("invalid UTF-16 credential")
		}
		words := make([]uint16, len(blob)/2)
		for index := range words {
			words[index] = binary.LittleEndian.Uint16(blob[index*2:])
		}
		key = strings.TrimRight(string(utf16.Decode(words)), "\x00")
	} else {
		key = string(blob)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		t.Fatal("empty credential")
	}
	return key
}

func TestVoiceConfigDPAPI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := defaultConfig()
	cfg.ASRAPIKey = "unit-test-asr-secret"
	cfg.APIKey = "unit-test-translation-secret"
	cfg.VoiceEnabled = true
	cfg.VoiceHotkey = "RCtrl"
	if err := saveConfig(path, cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var disk Config
	if err = json.Unmarshal(raw, &disk); err != nil {
		t.Fatal(err)
	}
	if disk.ASRAPIKey != "" || disk.ASRAPIKeyDPAPI == "" || bytes.Contains(raw, []byte(cfg.ASRAPIKey)) {
		t.Fatal("ASR key not encrypted")
	}
	loaded, created, err := loadConfig(path)
	if err != nil || created || loaded.ASRAPIKey != cfg.ASRAPIKey || loaded.APIKey != cfg.APIKey || loaded.ASRAPIKeyDPAPI != "" || loaded.VoiceHotkey != "RCtrl" || !loaded.VoiceEnabled {
		t.Fatal("DPAPI roundtrip failed")
	}
	legacy := []byte(`{"asr_api_key":"unit-test-asr-secret"}`)
	if err = os.WriteFile(path, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err = loadConfig(path); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	if bytes.Contains(raw, []byte("unit-test-asr-secret")) {
		t.Fatal("plaintext migration failed")
	}
}

func TestASRLive(t *testing.T) {
	if os.Getenv("MAMI_ASR_LIVE") != "1" {
		t.Skip("set MAMI_ASR_LIVE=1 to call StepFun")
	}
	key := stepFunCredential(t)
	wav, err := os.ReadFile(`C:\Users\marg0\.config\alma\scratch\mami-asr\test.wav`)
	if err != nil {
		t.Fatal(err)
	}
	if len(wav) <= 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || binary.LittleEndian.Uint32(wav[24:28]) != 16000 || binary.LittleEndian.Uint16(wav[22:24]) != 1 || binary.LittleEndian.Uint16(wav[34:36]) != 16 {
		t.Fatal("expected 44-byte-header, 16k mono 16bit WAV")
	}
	cfg := defaultConfig()
	cfg.ASRAPIKey = key
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	text, err := transcribe(ctx, cfg, wav[44:])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "接口") {
		t.Fatal("ASR result did not contain expected word 接口")
	}
	t.Logf("StepFun recognized expected word (%d characters)", len([]rune(text)))
}

// 冷连接 vs 预热后的真实耗时；MAMI_ASR_LIVE=1 时跑
func TestWarmConnectionLive(t *testing.T) {
	if os.Getenv("MAMI_ASR_LIVE") != "1" {
		t.Skip("set MAMI_ASR_LIVE=1 to call StepFun")
	}
	cfg := defaultConfig()
	cfg.ASRAPIKey = stepFunCredential(t)
	wav, err := os.ReadFile(`C:\Users\marg0\.config\alma\scratch\mami-asr\test.wav`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, warm := range []bool{false, true, false, true} {
		directTransport.CloseIdleConnections()
		if warm {
			warmConnection(ctx, cfg.ASRBaseURL)
		}
		started := time.Now()
		if _, err := transcribe(ctx, cfg, wav[44:]); err != nil {
			t.Fatal(err)
		}
		t.Logf("warm=%v: %d ms", warm, time.Since(started).Milliseconds())
	}
}

func TestVoicePCM(t *testing.T) {
	if unsafe.Sizeof(waveHeader{}) != 48 || unsafe.Offsetof(waveFormat{}.Extra) != 16 {
		t.Fatal("unexpected Win64 waveIn ABI layout")
	}
	if pcmRMS(nil) != 0 || pcmRMS([]byte{0, 0, 0, 0}) != 0 || pcmRMS([]byte{0xff, 0x7f}) < 0.99 {
		t.Fatal("RMS calculation")
	}
}

func TestWaveInLive(t *testing.T) {
	if os.Getenv("MAMI_WAVEIN_LIVE") != "1" {
		t.Skip("set MAMI_WAVEIN_LIVE=1 to record microphone")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stop := make(chan struct{})
	pcm, err := recordVoice(ctx, stop, func() { time.AfterFunc(time.Second, func() { close(stop) }) }, nil)
	if err != nil {
		t.Fatalf("microphone unavailable or recording failed: %v", err)
	}
	if len(pcm) < 28000 || len(pcm) > 36000 {
		t.Fatalf("expected about 32000 bytes, got %d", len(pcm))
	}
	t.Logf("recorded %d PCM bytes", len(pcm))
}

func TestVoicePill(t *testing.T) {
	if waveHeight(0) != 0 || waveHeight(1) != 1 || !(waveHeight(0.02) < waveHeight(0.06)) {
		t.Fatal("waveHeight")
	}
	if voiceWidth(voiceViewState{recording: true}) != voiceRecW || voiceWidth(voiceViewState{recognizing: true}) != voiceDotsW || voiceWidth(voiceViewState{text: "已复制"}) != 96 || voiceWidth(voiceViewState{text: strings.Repeat("长", 80)}) != 360 {
		t.Fatal("voiceWidth")
	}
	if backOut(0) != 0 || backOut(1) != 1 || backOut(0.8) <= 1 {
		t.Fatal("backOut should overshoot then land on 1")
	}
	start := time.Unix(100, 0)
	if progress(start, time.Time{}, time.Second) != 1 || progress(start.Add(500*time.Millisecond), start, time.Second) != 0.5 || progress(start.Add(-time.Second), start, time.Second) != 0 {
		t.Fatal("progress")
	}
	voiceRecording.Store(7)
	voiceClick(voiceStop)
	voiceClick(voiceStop)
	if ev := <-voiceEvents; ev.action != voiceStop || ev.id != 7 || len(voiceEvents) != 0 || voiceRecording.Load() != 0 {
		t.Fatal("click should post one stop and clear recording")
	}
	voiceKeyboard = voiceKeys{recording: true}
	voiceHookKey(&kbdLLHook{VkCode: 'A'}, true)
	if voiceKeyboard.recording {
		t.Fatal("keyboard should resync after click")
	}
	voiceKeyboard = voiceKeys{}
}

func TestVoiceChime(t *testing.T) {
	for _, wav := range [][]byte{soundStart, soundStop, soundAbort} {
		size := binary.LittleEndian.Uint32(wav[40:])
		if string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " || int(size) != len(wav)-44 || binary.LittleEndian.Uint32(wav[24:]) != 22050 {
			t.Fatal("bad wav header")
		}
		peak := 0
		for i := 44; i+1 < len(wav); i += 2 {
			peak = max(peak, abs(int(int16(binary.LittleEndian.Uint16(wav[i:])))))
		}
		if peak < 2000 || peak >= 32767 {
			t.Fatalf("peak %d: silent or clipped", peak)
		}
	}
	if d := float64(len(soundStart)-44) / 2 / 22050; d < 0.12 || d > 0.25 {
		t.Fatalf("start chime %.2fs", d)
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func TestVoiceHistorySummary(t *testing.T) {
	now := time.Date(2026, 10, 20, 9, 0, 0, 0, time.Local)
	list := []historyEntry{
		{ID: "a", Time: now.Add(-time.Hour).Format(time.RFC3339), Source: "嗯那个十个字的原话啊", Translation: "十个字的润色结果呀。", TotalMs: 3000, PolishMs: 600},
		{ID: "b", Time: now.AddDate(0, 0, -1).Format(time.RFC3339), Source: "五个字原话", TotalMs: 60000},
	}
	v := summarizeVoice(list, now)
	// 第一条 10 字：打字 12 秒减 3 秒 = 9 秒；第二条说得比打字还慢，不倒扣
	if v.total != 2 || v.today != 1 || v.chars != 15 || v.savedMs != 9000 || v.nPolish != 1 || v.polishMs != 600 {
		t.Fatalf("%+v", v)
	}
	if voiceFinal(list[1]) != "五个字原话" || voiceFinal(list[0]) != "十个字的润色结果呀。" {
		t.Fatal("voiceFinal")
	}
	if dayLabel(now, now) != T("今天") || dayLabel(now.AddDate(0, 0, -1), now) != T("昨天") || dayLabel(now.AddDate(0, -1, 0), now) != now.AddDate(0, -1, 0).Format(T("1月2日")) {
		t.Fatal("dayLabel")
	}
	if shortDuration(12500) != fmt.Sprintf(T("%d 秒"), 12) || shortDuration(185000) != fmt.Sprintf(T("%d 分钟"), 3) {
		t.Fatal("shortDuration")
	}
}

func TestVoiceOrbSize(t *testing.T) {
	for in, want := range map[float64]float64{0: voiceOrbW, -3: voiceOrbW, 20: 40, 200: 96, 63.6: 64} {
		cfg := defaultConfig()
		cfg.VoiceOrbSize = in
		if cfg.normalize(); cfg.VoiceOrbSize != want {
			t.Fatalf("%v -> %v, want %v", in, cfg.VoiceOrbSize, want)
		}
	}
	a := &App{voiceStyle: "orb", voiceOrbSize: 90}
	if a.voiceWidth(voiceViewState{recording: true}) != 90 || a.voiceWinHeight() != 102 || a.voiceWidth(voiceViewState{text: "已复制"}) != 96 {
		t.Fatal("orb layout")
	}
	if (&App{voiceStyle: "bars"}).voiceWinHeight() != voiceWinH {
		t.Fatal("bars height")
	}
}

// MAMI_QWEN_KEY=sk-… [MAMI_QWEN_BASE=…] 时真连千问 ASR（OpenAI 兼容 chat 接口）
func TestQwenASRLive(t *testing.T) {
	key := os.Getenv("MAMI_QWEN_KEY")
	if key == "" {
		t.Skip("set MAMI_QWEN_KEY to call Qwen ASR")
	}
	wav, err := os.ReadFile(`C:\Users\marg0\.config\alma\scratch\mami-asr\test.wav`)
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultConfig()
	cfg.ASRBaseURL, cfg.ASRModel, cfg.ASRAPIKey = "https://dashscope.aliyuncs.com/compatible-mode/v1", "qwen3-asr-flash", key
	if b := os.Getenv("MAMI_QWEN_BASE"); b != "" {
		cfg.ASRBaseURL = b
	}
	if m := os.Getenv("MAMI_QWEN_MODEL"); m != "" {
		cfg.ASRModel = m
	}
	cfg.ASRHotwords = strings.Split(os.Getenv("MAMI_QWEN_HOTWORDS"), ",")
	if cfg.ASRHotwords[0] == "" {
		cfg.ASRHotwords = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	start := time.Now()
	text, err := transcribe(ctx, cfg, wav[44:])
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s · %v · %q", cfg.ASRModel, time.Since(start).Round(time.Millisecond), text)
	if !strings.Contains(text, "接口") {
		t.Fatal("ASR result did not contain expected word 接口")
	}
}

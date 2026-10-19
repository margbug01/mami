//go:build windows

package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
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

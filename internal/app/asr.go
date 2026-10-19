package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func transcribe(ctx context.Context, cfg Config, pcm []byte) (string, error) {
	cfg.normalize()
	if cfg.ASRAPIKey == "" {
		return "", errors.New(T("请先填写语音 API Key"))
	}
	body := map[string]any{"audio": map[string]any{
		"data": base64.StdEncoding.EncodeToString(pcm),
		"input": map[string]any{
			"transcription": map[string]any{"language": "zh", "model": cfg.ASRModel, "enable_itn": true, "hotwords": cfg.ASRHotwords},
			"format":        map[string]any{"type": "pcm", "codec": "pcm_s16le", "rate": 16000, "bits": 16, "channel": 1},
		},
	}}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ASRBaseURL+"/audio/asr/sse", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+cfg.ASRAPIKey)
	// 语音单独直连，不继承翻译代理或环境代理；不跟随重定向泄露密钥。
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 90 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.ReplaceAll(string(data), cfg.ASRAPIKey, "[redacted]"))
	}
	text, err := readASRStream(resp.Body)
	if err != nil {
		return "", errors.New(strings.ReplaceAll(err.Error(), cfg.ASRAPIKey, "[redacted]"))
	}
	return text, nil
}

func readASRStream(r io.Reader) (string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 4096), 4<<20)
	var delta strings.Builder
	var final string
	done := false
	for sc.Scan() {
		payload, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "data:")
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "[DONE]" {
			break
		}
		var event struct {
			Type    string `json:"type"`
			Delta   string `json:"delta"`
			Text    string `json:"text"`
			Message string `json:"message"`
		}
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			return "", fmt.Errorf("ASR SSE: %w", err)
		}
		switch event.Type {
		case "transcript.text.delta":
			delta.WriteString(event.Delta)
		case "transcript.text.done":
			final, done = event.Text, true
		case "error":
			return "", fmt.Errorf("ASR: %s", event.Message)
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if done {
		return strings.TrimSpace(final), nil
	}
	if delta.Len() == 0 {
		return "", nil
	}
	// 没收到最终事件时不把中断的半句话当成成功。
	return "", errors.New(T("语音连接中断，未收到最终结果"))
}

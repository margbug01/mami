package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

func transcribe(ctx context.Context, cfg Config, pcm []byte) (string, error) {
	cfg.normalize()
	if cfg.ASRAPIKey == "" {
		return "", errors.New(T("请先填写语音 API Key"))
	}
	if isStepFunASR(cfg.ASRBaseURL) {
		return transcribeStepFun(ctx, cfg, pcm)
	}
	return transcribeChat(ctx, cfg, pcm)
}

// transcribeStepFun 阶跃自家的流式接口 /audio/asr/sse
func transcribeStepFun(ctx context.Context, cfg Config, pcm []byte) (string, error) {
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
	resp, err := directClient(asrTimeout).Do(req)
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

const asrTimeout = 90 * time.Second

// directTransport 语音识别和润色共用：不继承翻译代理或环境代理（走代理 ASR 从 0.9s 变 3.5s），
// 连接一直复用——每句话省一次 TCP+TLS 握手，国内到服务商约 0.2~0.5s。
var directTransport = func() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return t
}()

// directClient 不跟随重定向，免得密钥跟着去别的域
func directClient(timeout time.Duration) *http.Client {
	return &http.Client{Transport: directTransport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

// warmConnection 录音一开始就把到服务商的连接建好，说完发请求时直接复用。不带密钥，响应内容不看。
func warmConnection(ctx context.Context, base string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, base+"/models", nil)
	if err != nil {
		return
	}
	resp, err := directClient(5 * time.Second).Do(req)
	if err != nil {
		return
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	resp.Body.Close()
}

// voiceSegmenter 录音时按停顿切段：一段攒够 3 秒、里面至少 1 秒有声音、结尾又静了 0.7 秒，
// 就把这段先送去识别，停止时只剩最后一段要等。吵的环境里切不出来，就退回整段识别。
// shortcut: 阈值写死，revisit when 真人用下来切得太碎或者切不动
type voiceSegmenter struct {
	total, speech, silence int // 当前段的块数、有声块数、结尾连续安静块数（每块 100ms）
}

const voiceSilenceRMS = 0.01

func (s *voiceSegmenter) feed(level float64) (cut bool) {
	s.total++
	if level < voiceSilenceRMS {
		s.silence++
	} else {
		s.speech++
		s.silence = 0
	}
	if s.total >= 30 && s.speech >= 10 && s.silence >= 7 {
		*s = voiceSegmenter{}
		return true
	}
	return false
}

// joinSegments 把各段识别结果接起来：两边都是西文（ASCII）才补空格，中文直接相连
func joinSegments(parts []string) string {
	var b strings.Builder
	for _, part := range parts {
		if b.Len() > 0 {
			prev, _ := utf8.DecodeLastRuneInString(b.String())
			next, _ := utf8.DecodeRuneInString(part)
			if prev < utf8.RuneSelf && next < utf8.RuneSelf {
				b.WriteByte(' ')
			}
		}
		b.WriteString(part)
	}
	return b.String()
}

// voiceTooQuiet 误触两下或者一句没说：不到 300ms，或者整段最响的 100ms 都没过底噪，
// 就不发网络、不等那一秒。peak 是录音时各段 RMS 的最大值（waveHeight 把 0.004 当底噪，正常说话 ≥0.02）。
func voiceTooQuiet(pcm []byte, peak float64) bool {
	return len(pcm) < 16000*2*3/10 || peak < 0.006
}

// isStepFunASR 阶跃用自家的 /audio/asr/sse；其余地址按 OpenAI 兼容的 chat 接口发音频（千问 qwen3-asr-flash 这类）
func isStepFunASR(base string) bool {
	u, err := url.Parse(base)
	return err == nil && (u.Hostname() == "stepfun.com" || strings.HasSuffix(u.Hostname(), ".stepfun.com"))
}

// transcribeChat 把录音包成 WAV，作为 input_audio 发到 /chat/completions。
// 热词放进 system 消息当上下文；content 必须写成数组，千问对纯字符串报 400。
func transcribeChat(ctx context.Context, cfg Config, pcm []byte) (string, error) {
	var messages []any
	if len(cfg.ASRHotwords) > 0 {
		messages = append(messages, map[string]any{"role": "system", "content": []any{map[string]any{"type": "text", "text": strings.Join(cfg.ASRHotwords, "，")}}})
	}
	messages = append(messages, map[string]any{"role": "user", "content": []any{map[string]any{
		"type":        "input_audio",
		"input_audio": map[string]any{"data": "data:audio/wav;base64," + base64.StdEncoding.EncodeToString(pcmToWAV(pcm))},
	}}})
	raw, err := json.Marshal(map[string]any{
		"model":       cfg.ASRModel,
		"messages":    messages,
		"stream":      false,
		"asr_options": map[string]any{"language": "zh", "enable_itn": true},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.ASRBaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.ASRAPIKey)
	resp, err := directClient(asrTimeout).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.ReplaceAll(string(data[:min(len(data), 4096)]), cfg.ASRAPIKey, "[redacted]"))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("ASR: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", nil
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}

// pcmToWAV 给 16k 单声道 16 位 PCM 加上 44 字节的 WAV 头
func pcmToWAV(pcm []byte) []byte {
	h := make([]byte, 44, 44+len(pcm))
	copy(h, "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+len(pcm)))
	copy(h[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(h[16:], 16)
	binary.LittleEndian.PutUint16(h[20:], 1) // PCM
	binary.LittleEndian.PutUint16(h[22:], 1) // 单声道
	binary.LittleEndian.PutUint32(h[24:], 16000)
	binary.LittleEndian.PutUint32(h[28:], 16000*2)
	binary.LittleEndian.PutUint16(h[32:], 2)
	binary.LittleEndian.PutUint16(h[34:], 16)
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(len(pcm)))
	return append(h, pcm...)
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

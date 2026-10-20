package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/coder/websocket"
)

// isStreamASR 千问 3.1 ASR（qwen-audio-3.1-asr-flash-streaming）只开放 DashScope 的 WebSocket，
// 走 /chat/completions 会报 400 url error。
func isStreamASR(model string) bool { return strings.HasSuffix(model, "-streaming") }

// streamURL 把兼容接口地址（https://host/compatible-mode/v1）换成同一主机的 WebSocket 推理地址
func streamURL(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("ASR: bad base URL %q", base)
	}
	u.Scheme = "wss"
	if strings.HasPrefix(base, "http://") {
		u.Scheme = "ws"
	}
	u.Path, u.RawQuery = "/api-ws/v1/inference", ""
	return u.String(), nil
}

// asrStream 边录边发：录音开始就连上，每块音频直接推过去，停止时 finish 只等最后一句。
type asrStream struct {
	audio  chan []byte
	onText func(string) // 边说边出字：每来一次识别结果（含没说完的半句）就给一次全文；nil=不要
	cancel context.CancelFunc
	done   chan struct{}
	text   string
	err    error
}

func startASRStream(ctx context.Context, cfg Config, onText func(string)) *asrStream {
	ctx, cancel := context.WithCancel(ctx)
	// 10 分钟的 100ms 块；连接还没握好时先在这里排队，不丢开头
	s := &asrStream{audio: make(chan []byte, 6000), onText: onText, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		defer cancel()
		s.text, s.err = s.run(ctx, cfg)
		if s.err != nil && cfg.ASRAPIKey != "" {
			s.err = errors.New(strings.ReplaceAll(s.err.Error(), cfg.ASRAPIKey, "[redacted]"))
		}
	}()
	return s
}

func (s *asrStream) feed(data []byte) {
	select {
	case s.audio <- append([]byte(nil), data...):
	default: // 队列满（连不上又录了很久）：丢掉，finish 会报连接错误
	}
}

// finish 告诉服务端说完了，等识别结果
func (s *asrStream) finish() (string, error) {
	close(s.audio)
	t := time.AfterFunc(asrTimeout, s.cancel)
	defer t.Stop()
	<-s.done
	return s.text, s.err
}

func (s *asrStream) stop() { s.cancel() }

type dsEvent struct {
	Header struct {
		Event        string `json:"event"`
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	} `json:"header"`
	Payload struct {
		Output struct {
			Sentence struct {
				ID          int    `json:"sentence_id"`
				Text        string `json:"text"`
				SentenceEnd bool   `json:"sentence_end"`
				Heartbeat   bool   `json:"heartbeat"`
			} `json:"sentence"`
		} `json:"output"`
	} `json:"payload"`
}

func (s *asrStream) run(ctx context.Context, cfg Config) (string, error) {
	addr, err := streamURL(cfg.ASRBaseURL)
	if err != nil {
		return "", err
	}
	conn, resp, err := websocket.Dial(ctx, addr, &websocket.DialOptions{
		HTTPClient: directClient(0),
		HTTPHeader: http.Header{"Authorization": {"bearer " + cfg.ASRAPIKey}},
	})
	if err != nil {
		if resp != nil {
			return "", fmt.Errorf("ASR: HTTP %d", resp.StatusCode)
		}
		return "", err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)
	taskID := fmt.Sprintf("%032x", time.Now().UnixNano())
	params := map[string]any{"sample_rate": 16000, "format": "pcm"}
	if len(cfg.ASRHotwords) > 0 {
		vocab := map[string]int{}
		for _, w := range cfg.ASRHotwords {
			if w = strings.TrimSpace(w); w != "" {
				vocab[w] = 4
			}
		}
		params["vocabulary"] = vocab
	}
	header := func(action string) map[string]any {
		return map[string]any{"action": action, "task_id": taskID, "streaming": "duplex"}
	}
	send := func(v any) error {
		raw, _ := json.Marshal(v)
		return conn.Write(ctx, websocket.MessageText, raw)
	}
	if err := send(map[string]any{"header": header("run-task"), "payload": map[string]any{
		"task_group": "audio", "task": "asr", "function": "recognition",
		"model": cfg.ASRModel, "parameters": params, "input": map[string]any{},
	}}); err != nil {
		return "", err
	}
	sentences, partial := map[int]string{}, map[int]string{}
	started := false
	for {
		_, raw, err := conn.Read(ctx)
		if err != nil {
			return "", err
		}
		var ev dsEvent
		if json.Unmarshal(raw, &ev) != nil {
			continue
		}
		switch ev.Header.Event {
		case "task-started":
			if !started {
				started = true
				go func() {
					for {
						select {
						case chunk, ok := <-s.audio:
							if !ok {
								send(map[string]any{"header": header("finish-task"), "payload": map[string]any{"input": map[string]any{}}})
								return
							}
							if conn.Write(ctx, websocket.MessageBinary, chunk) != nil {
								return
							}
						case <-ctx.Done():
							return
						}
					}
				}()
			}
		case "result-generated":
			st := ev.Payload.Output.Sentence
			if st.Heartbeat {
				continue
			}
			if st.SentenceEnd {
				sentences[st.ID] = strings.TrimSpace(st.Text)
				delete(partial, st.ID)
			} else {
				partial[st.ID] = strings.TrimSpace(st.Text) // 半句会被后来的结果改写，只留最新的
			}
			if s.onText != nil {
				s.onText(liveText(sentences, partial))
			}
		case "task-finished":
			conn.Close(websocket.StatusNormalClosure, "")
			return joinSentences(sentences), nil
		case "task-failed":
			return "", fmt.Errorf("ASR: %s %s", ev.Header.ErrorCode, ev.Header.ErrorMessage)
		}
	}
}

// liveText 已说完的句子加上正在说的半句
func liveText(done, partial map[int]string) string {
	if len(partial) == 0 {
		return joinSentences(done)
	}
	all := make(map[int]string, len(done)+len(partial))
	for id, t := range done {
		all[id] = t
	}
	for id, t := range partial {
		all[id] = t
	}
	return joinSentences(all)
}

func joinSentences(m map[int]string) string {
	ids := make([]int, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if m[id] != "" {
			parts = append(parts, m[id])
		}
	}
	return joinSegments(parts)
}

// transcribeStream 整段音频一次性推完（设置页测试、非实时调用用）
func transcribeStream(ctx context.Context, cfg Config, pcm []byte) (string, error) {
	s := startASRStream(ctx, cfg, nil)
	defer s.stop()
	for i := 0; i < len(pcm); i += 3200 {
		s.feed(pcm[i:min(i+3200, len(pcm))])
	}
	return s.finish()
}

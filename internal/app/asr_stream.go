package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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

// 连接卡住时别干等：握手到 task-started 的时限、停止后等最后结果的时限（正常各约 0.2s / 0.1s）。
// 超时就报错，实时录音那边会换新连接把整段重识别一次（runVoice）。变量是为了测试能调小
var (
	asrStartWait  = 6 * time.Second
	asrFinishWait = 12 * time.Second
)

// asrStream 边录边发：录音开始就连上，每块音频直接推过去，停止时 finish 只等最后一句。
type asrStream struct {
	audio  chan []byte
	onText func(string) // 边说边出字：每来一次识别结果（含没说完的半句）就给一次全文；nil=不要
	cancel context.CancelCauseFunc
	startT *time.Timer // 收到 task-started 就停
	phase  string      // 卡在哪一步，只在 run 所在的协程里读写
	done   chan struct{}
	text   string
	err    error
}

func startASRStream(ctx context.Context, cfg Config, onText func(string)) *asrStream {
	ctx, cancel := context.WithCancelCause(ctx)
	// 10 分钟的 100ms 块；连接还没握好时先在这里排队，不丢开头
	s := &asrStream{audio: make(chan []byte, 6000), onText: onText, cancel: cancel, done: make(chan struct{})}
	s.startT = time.AfterFunc(asrStartWait, func() { cancel(fmt.Errorf("ASR: no response within %v", asrStartWait)) })
	begun := time.Now()
	go func() {
		defer close(s.done)
		defer cancel(nil)
		defer s.startT.Stop()
		s.text, s.err = s.run(ctx, cfg)
		if s.err != nil && ctx.Err() != nil {
			s.err = context.Cause(ctx) // 超时、发送失败：报真正的原因，而不是笼统的 context canceled
		}
		if s.err != nil && cfg.ASRAPIKey != "" && strings.Contains(s.err.Error(), cfg.ASRAPIKey) { // 只在真带着 Key 时重写，别把「取消」也变成一个普通错误
			s.err = errors.New(strings.ReplaceAll(s.err.Error(), cfg.ASRAPIKey, "[redacted]"))
		}
		if s.err != nil && !errors.Is(s.err, context.Canceled) {
			log.Printf("asr stream: %v (stuck at %s, %dms in)", s.err, s.phase, time.Since(begun).Milliseconds())
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

// backlog 还没发出去的音频（上行慢时会积压）每块多给 100ms：发它的时间不算服务端慢
func (s *asrStream) backlog() time.Duration {
	return min(time.Duration(len(s.audio))*100*time.Millisecond, asrTimeout)
}

// finish 告诉服务端说完了，最多等 wait 拿识别结果
func (s *asrStream) finish(wait time.Duration) (string, error) {
	close(s.audio)
	t := time.AfterFunc(wait, func() { s.cancel(fmt.Errorf("ASR: no result %v after stop", wait)) })
	defer t.Stop()
	<-s.done
	return s.text, s.err
}

func (s *asrStream) stop() { s.cancel(nil) }

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
	s.phase = "connect"
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
	s.phase = "task-started"
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
				s.startT.Stop()
				s.phase = "result"
				go func() {
					for {
						select {
						case chunk, ok := <-s.audio:
							if !ok {
								if err := send(map[string]any{"header": header("finish-task"), "payload": map[string]any{"input": map[string]any{}}}); err != nil {
									s.cancel(fmt.Errorf("ASR: send finish: %w", err))
								}
								return
							}
							// 发不出去就别让读那边干等到超时
							if err := conn.Write(ctx, websocket.MessageBinary, chunk); err != nil {
								s.cancel(fmt.Errorf("ASR: send audio: %w", err))
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
	return s.finish(asrTimeout) // 整段一次推完、可能很长，照旧等足
}

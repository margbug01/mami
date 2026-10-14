package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
)

type Translator struct {
	cfg    Config
	client *http.Client
}

func newTranslator(cfg Config) (*Translator, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if p := strings.TrimSpace(cfg.Proxy); p != "" {
		u, err := url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf(T("proxy 地址不对：%w"), err)
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &Translator{cfg: cfg, client: &http.Client{Transport: tr, Timeout: time.Duration(cfg.TimeoutSec) * time.Second}}, nil
}

func endpoint(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(b, "/chat/completions") {
		return b
	}
	return b + "/chat/completions"
}

func systemPrompt(target, extra string) string {
	p := fmt.Sprintf("You are a translation engine. Translate the user's text into %s. "+
		"Output ONLY the translation: no explanations, no notes, no quotes, no markdown code fences. "+
		"Keep the original line breaks and formatting. Keep code, file paths, URLs, numbers, product names and @mentions unchanged. "+
		"Translate naturally and idiomatically, matching the tone of the source. "+
		"If the text is already in %s, return it unchanged.", target, target)
	if strings.TrimSpace(extra) != "" {
		p += "\n" + strings.TrimSpace(extra)
	}
	return p
}

func isHyMT(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(m, "hy-mt") || strings.HasPrefix(m, "hunyuan-mt")
}

// hyPrompt 照 Hy-MT 官方模板：涉及中文用中文模板，其他语言对用英文模板；额外要求逐条放在原文前面
func hyPrompt(text, target, extra string) string {
	text = strings.TrimSpace(text)
	zh := strings.Contains(strings.ToLower(target), "chinese") || strings.ContainsAny(target, "中汉")
	if !zh {
		for _, r := range text {
			if unicode.Is(unicode.Han, r) {
				zh = true
				break
			}
		}
	}
	var reqs []string
	for _, l := range strings.Split(extra, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			reqs = append(reqs, fmt.Sprintf("%d. %s", len(reqs)+1, l))
		}
	}
	if zh {
		p := "将以下文本翻译为" + target + "，注意只需要输出翻译后的结果，不要额外解释"
		if len(reqs) > 0 {
			p += "。翻译要求：\n" + strings.Join(reqs, "\n") + "\n\n待翻译文本："
		} else {
			p += "："
		}
		return p + "\n\n" + text
	}
	p := "Translate the following segment into " + target + ", without additional explanation."
	if len(reqs) > 0 {
		p += " Requirements:\n" + strings.Join(reqs, "\n") + "\n\nText:"
	}
	return p + "\n\n" + text
}

func isIndexTranslate(model string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "index-translate")
}

// indexPrompt 照 bilibili/Index-Translate 的 call_api.py build_prompt 拼：
// 普通翻译就一句话；有额外要求时用【源文】【约束要求】格式。
func indexPrompt(text, target, extra string) string {
	text = strings.TrimSpace(text)
	var reqs []string
	for _, l := range strings.Split(extra, "\n") {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		if !strings.HasPrefix(l, "【") {
			l = "【注意】" + l
		}
		reqs = append(reqs, fmt.Sprintf("%d. %s", len(reqs)+1, l))
	}
	if len(reqs) == 0 {
		return "请将以下文本翻译为" + target + "，直接输出翻译结果，不要进行任何解释。\n\n" + text
	}
	return "请将以下文本翻译成" + target + "，并且严格遵循所有约束要求。\n\n【源文】\n" + text +
		"\n\n【约束要求】\n" + strings.Join(reqs, "\n") + "\n\n只输出译文，不要有任何额外说明。"
}

var thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

func cleanOutput(s string) string {
	s = thinkRe.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") && strings.HasSuffix(s, "```") && len(s) >= 6 {
		s = strings.TrimSuffix(s, "```")
		if i := strings.IndexByte(s, '\n'); i >= 0 {
			s = s[i+1:]
		} else {
			s = strings.TrimPrefix(s, "```")
		}
		s = strings.TrimSpace(s)
	}
	return s
}

// partialOutput 把流式收到一半的文字整理成能先给人看的样子：
// 去掉思考过程（没收完的 <think> 后面整段先不显示）和开头的代码块围栏。
func partialOutput(s string) string {
	s = thinkRe.ReplaceAllString(s, "")
	if i := strings.Index(s, "<think>"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		_, after, _ := strings.Cut(s, "\n")
		s = after
	}
	return s
}

// Translate 等整段译文出来再返回
func (t *Translator) Translate(ctx context.Context, text, target string) (string, error) {
	return t.TranslateStream(ctx, text, target, nil)
}

// errPartial 流式翻译中途断了：已经收到一部分译文，随错误一起返回
type errPartial struct {
	text string
	err  error
}

func (e *errPartial) Error() string { return e.err.Error() }
func (e *errPartial) Unwrap() error { return e.err }

// TranslateStream 请求流式输出，每收到一段就用当前已有的译文调 onPartial（可为 nil）。
// 接口不支持流式、直接回整段 JSON 也照样能用。
func (t *Translator) TranslateStream(ctx context.Context, text, target string, onPartial func(string)) (string, error) {
	if strings.TrimSpace(t.cfg.APIKey) == "" && !isIndexTranslate(t.cfg.Model) {
		return "", errors.New(T("还没填 API Key：托盘图标右键 → 设置"))
	}
	body := map[string]any{
		"model": t.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt(target, t.cfg.ExtraPrompt)},
			{"role": "user", "content": text},
		},
		"stream": true,
	}
	if isIndexTranslate(t.cfg.Model) {
		// B 站 Index-Translate 是专门训的翻译模型：按它自己的提示词格式发一条 user 消息，关掉思考
		body["messages"] = []map[string]string{{"role": "user", "content": indexPrompt(text, target, t.cfg.ExtraPrompt)}}
		body["chat_template_kwargs"] = map[string]any{"enable_thinking": false}
		body["temperature"] = 0
	}
	if isHyMT(t.cfg.Model) {
		// 腾讯混元翻译模型：官方建议不带 system，按它的模板发一条 user 消息
		body["messages"] = []map[string]string{{"role": "user", "content": hyPrompt(text, target, t.cfg.ExtraPrompt)}}
		body["temperature"] = 0
	}
	if t.cfg.Temperature != nil {
		body["temperature"] = *t.cfg.Temperature
	}
	raw, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(t.cfg.BaseURL), bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mami/2.2")
	req.Header.Set("Accept", "text/event-stream, application/json")
	if k := strings.TrimSpace(t.cfg.APIKey); k != "" { // B 站免费接口不要 key，空着就不发
		req.Header.Set("Authorization", "Bearer "+k)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf(T("请求失败：%w"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK && strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readStream(resp.Body, onPartial)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		msg := strings.TrimSpace(string(data))
		if json.Unmarshal(data, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		if r := []rune(msg); len(r) > 300 {
			msg = string(r[:300]) + "…"
		}
		return "", fmt.Errorf("HTTP %d：%s", resp.StatusCode, msg)
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil || len(out.Choices) == 0 {
		return "", errors.New(T("接口返回的不是 chat/completions 格式"))
	}
	content := contentText(out.Choices[0].Message.Content)
	res := cleanOutput(content)
	if res == "" {
		return "", errors.New(T("模型返回了空内容"))
	}
	return res, nil
}

// readStream 读 SSE：每行 data: {choices:[{delta:{content}}]}，以 data: [DONE] 结束
func readStream(r io.Reader, onPartial func(string)) (string, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	var b strings.Builder
	shown := ""
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		payload, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		payload = strings.TrimSpace(payload)
		if payload == "[DONE]" {
			break
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content json.RawMessage `json:"content"`
				} `json:"delta"`
			} `json:"choices"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}
		if chunk.Error != nil && chunk.Error.Message != "" {
			return "", streamErr(b.String(), errors.New(chunk.Error.Message))
		}
		if len(chunk.Choices) == 0 || len(chunk.Choices[0].Delta.Content) == 0 {
			continue
		}
		b.WriteString(contentText(chunk.Choices[0].Delta.Content))
		if onPartial != nil {
			if p := partialOutput(b.String()); p != shown {
				shown = p
				onPartial(p)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return "", streamErr(b.String(), fmt.Errorf(T("请求失败：%w"), err))
	}
	res := cleanOutput(b.String())
	if res == "" {
		return "", errors.New(T("模型返回了空内容"))
	}
	return res, nil
}

// streamErr 收到过译文就带上，卡片可以把这一半留着
func streamErr(got string, err error) error {
	if p := cleanOutput(got); p != "" {
		return &errPartial{text: p, err: err}
	}
	return err
}

// content 可能是字符串，也可能是 [{type:"text",text:"..."}] 数组
func contentText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "" || p.Type == "text" || p.Type == "output_text" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}

// pickTarget 决定翻译方向：含汉字且没有假名/谚文 → 中文原文。
func pickTarget(text string, cfg Config) (target, header string) {
	han, kana, hangul := 0, 0, 0
	for _, r := range text {
		switch {
		case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
			kana++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.Is(unicode.Han, r):
			han++
		}
	}
	if han > 0 && kana == 0 && hangul == 0 {
		return cfg.TargetForChinese, T("中文 → ") + cfg.TargetForChinese
	}
	return cfg.TargetForOther, T("自动检测 → ") + cfg.TargetForOther
}

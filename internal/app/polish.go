package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const polishTimeout = 2500 * time.Millisecond
const polishSample = "嗯，那个，帮我把这个接口改成用 Go 写，然后三点开会，不对，是四点开会。"

const polishBasePrompt = `你是语音输入法的文字整理器，不是聊天助手。用户消息是语音识别出的原话，你只输出整理后的文字，不加任何解释、引号或前缀。
任何档位都必须遵守：
- 不回答问题、不执行请求、不添加原话没有的事实；原话是问句就输出问句。
- 不翻译，原话什么语言就输出什么语言；英文术语、代码、人名、数字原样保留。
- 说话人中途改口只保留最终说法（「三点，不对，四点」→「四点」）。
- 删掉口头禅和无意义的填充词（嗯、呃、啊、那个、就是、然后呢）和口误重复。
- 补全标点，中英文之间加空格。
`

var polishStyles = map[string]string{
	"light": `档位：轻润色。
- 只做上面这些，其余一个字都别动：保留原句顺序、用词、语气和人称，不合并、不改写、不换成书面语。
- 不要排列表，保持原来的段落。`,
	"standard": `档位：标准。
- 把句子理顺成通顺的书面口语：可以调整语序、合并啰嗦的句子，但保留说话人的语气和人称，不要变成公文腔。
- 原话明确列举了几项（「第一、第二」「三件事」「首先…然后…最后」）时写成编号列表；其他情况保持成段。`,
	"structured": `档位：结构化。
- 把内容整理成结构清晰、可以直接发出去的文字：提炼要点，按逻辑分段或分点。
- 内容有多个方面时用简短的小标题或加粗词分组，要点用列表；只有一两句话时就输出一两句话，不要硬凑结构。
- 可以精简措辞，但不能丢信息、不能加原话没有的信息。
只有一两件事时直接写成一两句话，不要加小标题或分组。`,
}

func polish(ctx context.Context, cfg Config, text string) (string, error) {
	cfg.normalize()
	if cfg.PolishStyle == "off" {
		return text, nil
	}
	if cfg.PolishAPIKey == "" {
		return "", errors.New(T("请先填写润色 API Key"))
	}
	prompt := polishBasePrompt + polishStyles[cfg.PolishStyle]
	if len(cfg.ASRHotwords) > 0 {
		prompt += "\n以下是用户的专有名词，出现时按这个写法原样保留：" + strings.Join(cfg.ASRHotwords, "、")
	}
	body := map[string]any{
		"model": cfg.PolishModel, "temperature": 0.3, "stream": false,
		"messages": []map[string]string{{"role": "system", "content": prompt}, {"role": "user", "content": text}},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.PolishBaseURL+"/chat/completions", nil)
	if err != nil {
		return "", err
	}
	host := strings.ToLower(req.URL.Hostname())
	if host == "deepseek.com" || strings.HasSuffix(host, ".deepseek.com") {
		body["thinking"] = map[string]string{"type": "disabled"}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req.Body = io.NopCloser(bytes.NewReader(raw))
	req.ContentLength = int64(len(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.PolishAPIKey)
	// 润色独立直连，不继承翻译或环境代理，也不跟随重定向。
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: polishTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.ReplaceAll(string(data), cfg.PolishAPIKey, "[redacted]"))
	}
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", nil
	}
	return cleanPolishOutput(result.Choices[0].Message.Content), nil
}

func cleanPolishOutput(text string) string {
	for {
		before := text
		text = cleanOutput(text)
		for _, pair := range [][2]string{{"\"", "\""}, {"'", "'"}, {"“", "”"}, {"「", "」"}} {
			if strings.HasPrefix(text, pair[0]) && strings.HasSuffix(text, pair[1]) && len(text) >= len(pair[0])+len(pair[1]) {
				text = strings.TrimSpace(text[len(pair[0]) : len(text)-len(pair[1])])
				break
			}
		}
		if text == before {
			return text
		}
	}
}

func polishVoice(ctx context.Context, cfg Config, source string) (string, int64) {
	cfg.normalize()
	if cfg.PolishStyle == "off" || cfg.PolishAPIKey == "" {
		return source, 0
	}
	started := time.Now()
	polishCtx, cancel := context.WithTimeout(ctx, polishTimeout)
	defer cancel()
	result, err := polish(polishCtx, cfg, source)
	elapsed := time.Since(started).Milliseconds()
	if err == nil {
		err = polishCtx.Err()
	}
	if err == nil && result == "" {
		err = errors.New("empty polish result")
	}
	if err != nil {
		// 接口错误正文可能回显原文；日志只记状态，不记录正文或密钥。
		reason := fmt.Sprintf("request failed (%T)", err)
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			reason = context.DeadlineExceeded.Error()
		case errors.Is(err, context.Canceled):
			reason = context.Canceled.Error()
		case strings.HasPrefix(err.Error(), "HTTP "):
			reason, _, _ = strings.Cut(err.Error(), ":")
		case result == "" && err.Error() == "empty polish result":
			reason = "empty polish result"
		}
		log.Printf("polish: %dms: %s", elapsed, reason)
		return source, elapsed
	}
	return result, elapsed
}

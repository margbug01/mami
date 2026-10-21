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
	"unicode/utf8"
)

// polishDeadline 润色等多久：2s 起步，每个字再加 15ms（模型按长度吐字，一分钟口述 200 多字光生成就要 3s），
// 最多 8s；到点就回落原文。
func polishDeadline(text string) time.Duration {
	return min(polishMaxWait, 2*time.Second+15*time.Millisecond*time.Duration(utf8.RuneCountInString(text)))
}

const polishMaxWait = 8 * time.Second
const polishMinRunes = 5 // 比这短的（「好的」「收到」）没什么可润色，不值得再等一次网络
const polishSample = "嗯，那个，帮我把这个接口改成用 Go 写，然后三点开会，不对，是四点开会。"

const polishBasePrompt = `你是语音输入法的文字整理器，不是聊天助手。用户消息是语音识别出的原话，你只输出整理后的文字，不加任何解释、引号或前缀。
任何档位都必须遵守：
- 不回答问题、不执行请求、不添加原话没有的事实；原话是问句就输出问句。
- 不翻译，原话什么语言就输出什么语言；英文术语、代码、人名、数字默认原样保留，但说话人明确纠正或说明写法时，以该说明为准。
- 纠正只能依据本次原话中的明确证据，不能从本提示词的示例、词库或常识猜一个新名字。改时间只改时间，改一个字只替换那个字，其他字及顺序保持不变。没有说明名字写法时，「小明」就保留「小明」。
- 区分正文与口述纠正说明：中途改口只保留最终说法（「三点，不对，四点」→「四点」）。紧接正文的「X 是某某的 X」「不是这个字，是……」若明确指向前文用字，把说明应用到前文，再删去这段说明。说明中的同音字也可能识别错，应从引用的完整词语取正确字。例如「发给小兰，兰是蓝天的蓝」→「发给小蓝」，只能把兰换成蓝，不能把小换掉或把名字重排；「通知小明，小明是黄晓明的小明」→「通知晓明」，依据是原话确实引用了黄晓明。
- 先识别引用范围，再处理纠正。出现「保留这个例子」「原样引用」「这句话」等引出被讨论的文字时，例子里的改口和说明也属于正文，不得应用或删除；这一条优先于纠正规则。例如「保留这个例子：发给小兰，兰是蓝天的蓝」须保留整段，不替换成小蓝。人物介绍、事实和问题也不是纠正说明（「小明喜欢黄晓明」「小明是王小明的儿子」原样保留）；无法确定写法时保留原文，不猜。
- 删掉口头禅和无意义的填充词（嗯、呃、啊、那个、就是、然后呢）和口误重复。
- 感谢、夸奖、道歉、打招呼这些是有意思的话，不是填充词，原话里有就留着（「OK，谢谢你，你真棒」不能只剩「你真棒」）；开头单独的「OK」「好」这类应答词也可以留。
- 补全标点，中英文之间加空格。
- 先读懂整段想表达什么，再整理。语音识别会听错字、断错句：放在句子里说不通的词（例：「它跳转的是文件夹，任务它跳转是文件管理器」里的「任务」是多余的错字，应为「它跳转的是文件夹，是文件管理器」）、被句号拆开的半句（「不是。浏览器」→「不是浏览器」），按最可能的原意改顺。
- 说了半截、指代不清的，补成完整的句子（「最主要是终端」→「最主要的问题在终端」）。念出来的按键按书面写（control V → Ctrl+V）。
- 只改说不通的地方，说得通的不动；不加原话没有的信息。
`

var polishStyles = map[string]string{
	"prompt": `档位：提示词（用户在给 AI 写指令）。
- 目标是让 AI 准确理解：理顺语序、去掉重复，正文中的所有要求、限制、例子和细节一个都不能丢，也不要替用户补充要求；说话人对正文的纠正说明按上面的规则先应用，不作为正文细节保留。
- 文件名、路径、函数名、变量名、命令、报错信息、英文术语、模型名一律原样保留，不改大小写、不翻译。
- 内容较长且有好几条要求时，分段或用编号列出要求；只有一两句话时就输出一两句话，不要加标题，不要硬套「背景/任务/要求」的模板。
- 保持用户本来直接的口吻，不要自己添加「请」「您好」之类客套话；原话里本来就有的谢谢、夸奖、道歉照样保留。`,
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

// polish 整理语音识别出的原话；learned 是自动学到的纠错词对（见 learn.go）
// 名词表只管写法，不拿来替换读音相近的词（「同学张伟」不能被改成用户词库里的「章炜」）；
// 替换只靠纠错表，那是用户同一处改过两次才生效的
const (
	polishHotwordsLead = "\n以下是用户的专有名词，原文里出现时按这个写法写（用字、大小写照抄）；不要把读音相近的别的词改成它们："
	polishTableLead    = "\n以下是这位用户语音识别常见的错写（左→右，大小写和空格不同也算），出现左边的一般就是右边那个，改过来；上下文明显说的是别的人或东西时不改："
)

func polish(ctx context.Context, cfg Config, text string, learned ...learnedWord) (string, error) {
	cfg.normalize()
	if cfg.PolishStyle == "off" {
		return text, nil
	}
	if cfg.PolishAPIKey == "" {
		return "", errors.New(T("请先填写润色 API Key"))
	}
	prompt := polishBasePrompt + polishStyles[cfg.PolishStyle]
	if len(cfg.ASRHotwords) > 0 {
		prompt += polishHotwordsLead + strings.Join(cfg.ASRHotwords, "、")
	}
	if table := learnedTable(learned); table != "" {
		prompt += polishTableLead + table
	}
	out, err := chatOnce(ctx, cfg, prompt, text)
	if err != nil {
		return "", err
	}
	return cleanPolishOutput(out), nil
}

// chatOnce 在润色接口上发一轮非流式对话，返回助手回复的原文
func chatOnce(ctx context.Context, cfg Config, system, user string) (string, error) {
	cfg.normalize()
	body := map[string]any{
		"model": cfg.PolishModel, "temperature": 0.3, "stream": false,
		"messages": []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
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
	resp, err := directClient(polishMaxWait).Do(req)
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
	return result.Choices[0].Message.Content, nil
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

func polishVoice(ctx context.Context, cfg Config, source string, learned ...learnedWord) (string, int64) {
	cfg.normalize()
	if cfg.PolishStyle == "off" || cfg.PolishAPIKey == "" || utf8.RuneCountInString(strings.TrimSpace(source)) < polishMinRunes {
		return applyCorrections(source, learned), 0 // 不润色，词库里的纠错照样用
	}
	started := time.Now()
	polishCtx, cancel := context.WithTimeout(ctx, polishDeadline(source))
	defer cancel()
	result, err := polish(polishCtx, cfg, source, learned...)
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
		return applyCorrections(source, learned), 0 // 没润色成：耗时记 0，历史和统计就不会把它当成润色过
	}
	result = applyCorrections(guardPolish(source, result, learned), learned)
	return result, elapsed
}

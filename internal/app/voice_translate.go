package app

import "strings"

func voicePolishPrompt(cfg Config) string {
	style := cfg.PolishStyle
	if cfg.VoiceTranslate && style == "off" {
		style = "light"
	}
	prompt := polishBasePrompt + polishStyles[style]
	if cfg.VoiceTranslate {
		lines := strings.Split(prompt, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "- 不翻译，原话什么语言就输出什么语言；") {
				lines[i] = "- 先按这些规则和档位整理原话，再只输出翻译成 " + cfg.VoiceTarget + " 的译文；自然地道，代码、路径、人名、数字原样保留；不解释，不输出原文。翻译要求优先于档位中保留原话用词的要求。"
			}
		}
		prompt = strings.Join(lines, "\n")
	}
	return prompt
}

func voiceLanguageCode(target string) string {
	codes := map[string]string{
		"English": "EN", "Japanese": "JA", "Korean": "KO", "French": "FR",
		"German": "DE", "Spanish": "ES", "Russian": "RU", "Simplified Chinese": "ZH",
		"Traditional Chinese": "ZH", "Portuguese": "PT", "Italian": "IT", "Arabic": "AR",
		"Thai": "TH", "Vietnamese": "VI", "Indonesian": "ID", "Hindi": "HI",
		"Malay": "MS", "Turkish": "TR", "Dutch": "NL", "Polish": "PL",
	}
	if code := codes[target]; code != "" {
		return code
	}
	runes := []rune(strings.TrimSpace(target))
	return strings.ToUpper(string(runes[:min(2, len(runes))]))
}

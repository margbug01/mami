package app

import (
	"path/filepath"
	"strings"
)

// 按程序自动换档：在给 AI 写指令的地方用「提示词」档，在聊天软件里用「轻润色」，其余用用户选的档位。
// shortcut: 名单写死，浏览器只认标题里的站名；revisit when 用户要自己加程序
var (
	promptApps = map[string]bool{
		"cursor.exe": true, "code.exe": true, "code - insiders.exe": true, "windsurf.exe": true, "trae.exe": true,
		"kiro.exe": true, "zed.exe": true, "alma.exe": true, "claude.exe": true, "chatgpt.exe": true,
		"windowsterminal.exe": true, "idea64.exe": true, "pycharm64.exe": true, "goland64.exe": true, "webstorm64.exe": true,
	}
	chatApps = map[string]bool{
		"weixin.exe": true, "wechat.exe": true, "qq.exe": true, "wxwork.exe": true, "dingtalk.exe": true,
		"feishu.exe": true, "lark.exe": true, "telegram.exe": true, "whatsapp.exe": true, "discord.exe": true, "slack.exe": true,
	}
	browserApps = map[string]bool{"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "brave.exe": true, "arc.exe": true, "vivaldi.exe": true, "opera.exe": true}
	// 浏览器标题里出现这些站名就当在给 AI 写东西（小写比较）
	aiSites = []string{"chatgpt", "claude", "gemini", "deepseek", "kimi", "豆包", "通义", "千问", "qwen", "perplexity", "grok", "copilot", "ai studio", "poe", "元宝", "智谱", "chatglm", "mistral"}
)

// autoPolishStyle 用户选的档位 base 在这个程序里该换成什么；关了润色就一直关
func autoPolishStyle(exe, title, base string) string {
	if base == "off" {
		return base
	}
	exe = strings.ToLower(filepath.Base(exe))
	switch {
	case promptApps[exe]:
		return "prompt"
	case chatApps[exe]:
		return "light"
	case browserApps[exe]:
		t := strings.ToLower(title)
		for _, s := range aiSites {
			if strings.Contains(t, s) {
				return "prompt"
			}
		}
	}
	return base
}

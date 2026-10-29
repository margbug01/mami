package app

import "testing"

func TestAutoPolishStyle(t *testing.T) {
	for _, c := range []struct{ exe, title, base, want string }{
		{nativeApp(`C:\Users\x\AppData\Local\Programs\cursor\Cursor.exe`, "/Applications/Cursor.app"), "main.go", "standard", "prompt"},
		{nativeApp(`C:\Program Files\Tencent\Weixin\Weixin.exe`, "/Applications/WeChat.app"), "微信", "standard", "light"},
		{nativeApp(`C:\Program Files\Google\Chrome\Application\chrome.exe`, "/Applications/Google Chrome.app"), "Claude - Google Chrome", "light", "prompt"},
		{nativeApp(`C:\Windows\notepad.exe`, "/System/Applications/TextEdit.app"), "", "structured", "structured"},
		{"Weixin.exe", "微信", "standard", "light"},
		{"chrome.exe", "ChatGPT - Google Chrome", "standard", "prompt"},
		{"msedge.exe", "新闻 - Microsoft Edge", "structured", "structured"},
		{"notepad.exe", "", "light", "light"},
		{"Cursor.exe", "", "off", "off"},
	} {
		if got := autoPolishStyle(c.exe, c.title, c.base); got != c.want {
			t.Errorf("%s %q %s → %s, want %s", c.exe, c.title, c.base, got, c.want)
		}
	}
	cfg := defaultConfig()
	cfg.PolishStyle = "prompt"
	cfg.normalize()
	if cfg.PolishStyle != "prompt" || polishStyles["prompt"] == "" || polishLabel("prompt") == "" {
		t.Fatal("prompt style not wired")
	}
}

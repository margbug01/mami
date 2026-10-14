package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	BaseURL          string   `json:"base_url"`
	APIKey           string   `json:"api_key"`
	Model            string   `json:"model"`
	TargetForChinese string   `json:"target_for_chinese"`
	TargetForOther   string   `json:"target_for_other"`
	Hotkey           string   `json:"hotkey"`
	PopupHotkey      string   `json:"popup_hotkey"`
	ShowButton       bool     `json:"show_button"`
	RequireIBeam     bool     `json:"require_ibeam"`
	OnlyInInputs     bool     `json:"only_in_inputs"`
	ShowInBlindApps  bool     `json:"show_in_blind_apps"` // 微信这类认不出输入框的程序：照样弹
	ButtonAction     string   `json:"button_action"`      // 点翻译按钮后：card / result / replace
	ButtonTimeoutMs  int      `json:"button_timeout_ms"`
	SelectAllIfEmpty bool     `json:"select_all_if_empty"`
	MaxChars         int      `json:"max_chars"`
	TimeoutSec       int      `json:"timeout_sec"`
	Proxy            string   `json:"proxy"`
	Temperature      *float64 `json:"temperature"`
	ExtraPrompt      string   `json:"extra_prompt"`
	ExcludeProcesses []string `json:"exclude_processes"`
	Language         string   `json:"language"` // 界面语言：auto / zh / en / ja
	Theme            string   `json:"theme"`    // 外观：auto / light / dark
}

func defaultConfig() Config {
	return Config{
		BaseURL:          "https://index-translate.bilibili.com/v1",
		Model:            "Index-Translate-35B-A3B",
		TargetForChinese: "English",
		TargetForOther:   "Simplified Chinese",
		Hotkey:           "Alt+T",
		PopupHotkey:      "Alt+Shift+T",
		ShowButton:       true,
		RequireIBeam:     true,
		OnlyInInputs:     true,
		ShowInBlindApps:  true,
		ButtonAction:     actionCard,
		ButtonTimeoutMs:  4000,
		SelectAllIfEmpty: true,
		MaxChars:         6000,
		TimeoutSec:       30,
		ExcludeProcesses: []string{},
		Language:         "auto",
		Theme:            "auto",
	}
}

// 点翻译按钮后怎么展示
const (
	actionCard    = "card"    // 先弹「翻译中…」卡片，结果出来填进去
	actionResult  = "result"  // 不弹加载卡片，结果出来再弹
	actionReplace = "replace" // 直接替换选中文字，出错才弹卡片
)

func (c *Config) normalize() {
	switch c.Language {
	case "zh", "zh-TW", "en", "ja", "ko", "es", "fr", "de", "ru":
	default:
		c.Language = "auto"
	}
	switch c.Theme {
	case "light", "dark":
	default:
		c.Theme = "auto"
	}
	switch c.ButtonAction {
	case actionCard, actionResult, actionReplace:
	default:
		c.ButtonAction = actionCard
	}
	if c.MaxChars <= 0 {
		c.MaxChars = 6000
	}
	if c.TimeoutSec <= 0 {
		c.TimeoutSec = 30
	}
	if c.ButtonTimeoutMs <= 0 {
		c.ButtonTimeoutMs = 4000
	}
	if c.ExcludeProcesses == nil {
		c.ExcludeProcesses = []string{}
	}
}

// loadConfig 读配置；文件不存在时返回默认值，created=true（不落盘，等设置窗口保存）。
func loadConfig(path string) (cfg Config, created bool, err error) {
	cfg = defaultConfig()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		created = true
	} else if err != nil {
		return cfg, false, err
	} else {
		b = bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")) // 记事本可能存成带 BOM
		if err := json.Unmarshal(b, &cfg); err != nil {
			return cfg, false, fmt.Errorf(T("%s 格式不对：%w"), filepath.Base(path), err)
		}
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		for _, k := range []string{"MAMI_API_KEY", "OPENAI_API_KEY"} {
			if v := os.Getenv(k); v != "" {
				cfg.APIKey = v
				break
			}
		}
	}
	cfg.normalize()
	return cfg, created, nil
}

// saveConfig 先写临时文件再改名，写一半崩了也不会留下坏配置。
func saveConfig(path string, cfg Config) error {
	cfg.normalize()
	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// splitList 把设置窗口里「a.exe, b.exe」这种写法拆成列表
func splitList(s string) []string {
	out := []string{}
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' || r == ';' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

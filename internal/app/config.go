package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	PolishStyle       string  `json:"polish_style"`
	PolishBaseURL     string  `json:"polish_base_url"`
	PolishModel       string  `json:"polish_model"`
	PolishAPIKey      string  `json:"polish_api_key,omitempty"`
	PolishAPIKeyDPAPI string  `json:"polish_api_key_dpapi,omitempty"`
	VoiceEnabled      bool    `json:"voice_enabled"`
	PolishAuto        bool    `json:"polish_auto"` // 按程序自动换润色档位（AI 工具用提示词档、聊天软件用轻润色）
	VoiceHotkey       string  `json:"voice_hotkey"`
	VoiceStyle        string  `json:"voice_style"`     // 胶囊样式：bars 渐变波柱 / orb 果冻球 / glass 磨砂玻璃 / pixel 复古像素 / mercury 水银 / islands 三岛连桥 / leds 分段电平
	VoiceOrbSize      float64 `json:"voice_orb_size"`  // 果冻球直径（DIP），40~96
	VoiceTone         string  `json:"voice_tone"`      // 浮窗底色：dark / light / auto（跟 Mami 主题），跟主题分开选
	VoiceSound        bool    `json:"voice_sound"`     // 开始、结束、取消的提示音
	VoiceLiveText     bool    `json:"voice_live_text"` // 说话时胶囊里实时出字（只有流式识别模型有中间结果）
	VoiceLearn        bool    `json:"voice_learn"`     // 贴入后用户手改的词自动记成热词和纠错表
	ASRBaseURL        string  `json:"asr_base_url"`
	ASRModel          string  `json:"asr_model"`
	ASRAPIKey         string  `json:"asr_api_key,omitempty"`
	ASRAPIKeyDPAPI    string  `json:"asr_api_key_dpapi,omitempty"`
	// SavedKeys 各接口地址用过的 Key（keyFor 归一），切服务商再切回来能自动填上；内存里明文，存盘逐个 DPAPI
	SavedKeys        map[string]string `json:"saved_keys_dpapi,omitempty"`
	ASRHotwords      []string          `json:"asr_hotwords"`
	BaseURL          string            `json:"base_url"`
	APIKey           string            `json:"api_key,omitempty"` // 只在内存里是明文；存盘时加密进 api_key_dpapi
	APIKeyDPAPI      string            `json:"api_key_dpapi,omitempty"`
	Model            string            `json:"model"`
	TargetForChinese string            `json:"target_for_chinese"`
	TargetForOther   string            `json:"target_for_other"`
	Hotkey           string            `json:"hotkey"`
	PopupHotkey      string            `json:"popup_hotkey"`
	ShowButton       bool              `json:"show_button"`
	RequireIBeam     bool              `json:"require_ibeam"`
	OnlyInInputs     bool              `json:"only_in_inputs"`
	ShowInBlindApps  bool              `json:"show_in_blind_apps"` // 微信这类认不出输入框的程序：照样弹
	ButtonAction     string            `json:"button_action"`      // 点翻译按钮后：card / result / replace
	ButtonTimeoutMs  int               `json:"button_timeout_ms"`
	SelectAllIfEmpty bool              `json:"select_all_if_empty"`
	MaxChars         int               `json:"max_chars"`
	TimeoutSec       int               `json:"timeout_sec"`
	Proxy            string            `json:"proxy"`
	Temperature      *float64          `json:"temperature"`
	ExtraPrompt      string            `json:"extra_prompt"`
	ExcludeProcesses []string          `json:"exclude_processes"`
	DisableHotkeys   bool              `json:"disable_hotkeys"` // 总开关：true 时一个全局快捷键都不注册
	Language         string            `json:"language"`        // 界面语言：auto / zh / en / ja
	Theme            string            `json:"theme"`           // 外观：auto / light / dark
	StreamCard       bool              `json:"stream_card"`     // 卡片里边翻边显示（关掉就整段出来再显示）
	History          bool              `json:"history"`         // 保存翻译记录到 history/*.jsonl
	HistoryDays      int               `json:"history_days"`    // 记录保留几天，0 = 永久
}

func defaultConfig() Config {
	return Config{
		PolishStyle:      "standard",
		PolishAuto:       true,
		PolishBaseURL:    "https://api.deepseek.com/v1",
		PolishModel:      "deepseek-flash",
		VoiceHotkey:      "RAlt",
		VoiceStyle:       "bars",
		VoiceTone:        "dark",
		VoiceSound:       true,
		VoiceLearn:       true,
		VoiceOrbSize:     voiceOrbW,
		ASRBaseURL:       "https://api.stepfun.com/v1",
		ASRModel:         "stepaudio-3-asr-max",
		ASRHotwords:      []string{},
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
		HistoryDays:      30,
	}
}

// 点翻译按钮后怎么展示
const (
	actionCard    = "card"    // 先弹「翻译中…」卡片，结果出来填进去
	actionResult  = "result"  // 不弹加载卡片，结果出来再弹
	actionReplace = "replace" // 直接替换选中文字，出错才弹卡片
)

func (c *Config) normalize() {
	switch c.PolishStyle {
	case "off", "light", "standard", "structured", "prompt":
	default:
		c.PolishStyle = "standard"
	}
	c.PolishBaseURL = strings.TrimRight(strings.TrimSpace(c.PolishBaseURL), "/")
	if c.PolishBaseURL == "" {
		c.PolishBaseURL = "https://api.deepseek.com/v1"
	}
	c.PolishModel = strings.TrimSpace(c.PolishModel)
	if c.PolishModel == "" {
		c.PolishModel = "deepseek-flash"
	}
	c.PolishAPIKey = strings.TrimSpace(c.PolishAPIKey)
	c.VoiceHotkey = strings.TrimSpace(c.VoiceHotkey)
	if c.VoiceHotkey != "Off" && voiceKey(c.VoiceHotkey) == 0 && !strings.Contains(c.VoiceHotkey, "+") {
		c.VoiceHotkey = "RAlt" // 组合键是否有效由注册时报错
	}
	if c.VoiceOrbSize <= 0 {
		c.VoiceOrbSize = voiceOrbW
	}
	c.VoiceOrbSize = math.Round(min(96, max(40, c.VoiceOrbSize)))
	switch c.VoiceStyle {
	case "bars", "orb", "glass", "pixel", "mercury", "islands", "leds":
	default:
		c.VoiceStyle = "bars"
	}
	switch c.VoiceTone {
	case "light", "auto":
	default:
		c.VoiceTone = "dark"
	}
	c.ASRBaseURL = strings.TrimRight(strings.TrimSpace(c.ASRBaseURL), "/")
	if c.ASRBaseURL == "" {
		c.ASRBaseURL = "https://api.stepfun.com/v1"
	}
	c.ASRModel = strings.TrimSpace(c.ASRModel)
	if c.ASRModel == "" {
		c.ASRModel = "stepaudio-3-asr-max"
	}
	c.ASRAPIKey = strings.TrimSpace(c.ASRAPIKey)
	if c.ASRHotwords == nil {
		c.ASRHotwords = []string{}
	}
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
	if c.HistoryDays < 0 {
		c.HistoryDays = 30
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
	polishPlain := strings.TrimSpace(cfg.PolishAPIKey) != ""
	if !polishPlain && cfg.PolishAPIKeyDPAPI != "" {
		if k, err := unprotectKey(cfg.PolishAPIKeyDPAPI); err == nil {
			cfg.PolishAPIKey = k
		} else {
			log.Printf("polish key: can't decrypt: %v", err)
		}
	}
	cfg.PolishAPIKeyDPAPI = ""
	asrPlain := strings.TrimSpace(cfg.ASRAPIKey) != ""
	if !asrPlain && cfg.ASRAPIKeyDPAPI != "" {
		if k, err := unprotectKey(cfg.ASRAPIKeyDPAPI); err == nil {
			cfg.ASRAPIKey = k
		} else {
			log.Printf("asr key: can't decrypt: %v", err)
		}
	}
	cfg.ASRAPIKeyDPAPI = ""
	saved := map[string]string{}
	for b, enc := range cfg.SavedKeys {
		if k, err := unprotectKey(enc); err == nil {
			saved[b] = k
		}
	}
	cfg.SavedKeys = saved
	plainInFile := strings.TrimSpace(cfg.APIKey) != ""
	if !plainInFile && cfg.APIKeyDPAPI != "" {
		// 手动往 api_key 里填了明文的话以明文为准；解不开（配置是从别的电脑/账户拷来的）就当没填
		if k, err := unprotectKey(cfg.APIKeyDPAPI); err == nil {
			cfg.APIKey = k
		} else {
			log.Printf("api key: can't decrypt (config from another PC or account?): %v", err)
		}
	}
	cfg.APIKeyDPAPI = ""
	if plainInFile || asrPlain || polishPlain {
		// 旧版本存的明文 key：马上加密存回去，不用等下次在设置里点保存
		if err := saveConfig(path, cfg); err != nil {
			log.Printf("api key: encrypt plain key in config: %v", err)
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

// keyFor 记 Key 用的地址：去空白、去尾斜杠、小写
func keyFor(base string) string {
	return strings.ToLower(strings.TrimRight(strings.TrimSpace(base), "/"))
}

// saveConfig 先写临时文件再改名，写一半崩了也不会留下坏配置。
func saveConfig(path string, cfg Config) error {
	cfg.normalize()
	// 正在用的三把也记进去；新建 map，不改调用方手里那份
	saved := map[string]string{}
	for b, k := range cfg.SavedKeys {
		saved[b] = k
	}
	for _, p := range [][2]string{{cfg.BaseURL, cfg.APIKey}, {cfg.ASRBaseURL, cfg.ASRAPIKey}, {cfg.PolishBaseURL, cfg.PolishAPIKey}} {
		if k := strings.TrimSpace(p[1]); k != "" && keyFor(p[0]) != "" {
			saved[keyFor(p[0])] = k
		}
	}
	cfg.SavedKeys = map[string]string{}
	for b, k := range saved {
		if enc, err := protectKey(k); err == nil {
			cfg.SavedKeys[b] = enc
		}
	}
	cfg.PolishAPIKeyDPAPI = ""
	if cfg.PolishAPIKey != "" {
		enc, err := protectKey(cfg.PolishAPIKey)
		if err != nil {
			return fmt.Errorf("Polish DPAPI: %w", err)
		}
		cfg.PolishAPIKey, cfg.PolishAPIKeyDPAPI = "", enc
	}
	cfg.ASRAPIKeyDPAPI = ""
	if cfg.ASRAPIKey != "" {
		enc, err := protectKey(cfg.ASRAPIKey)
		if err != nil {
			return fmt.Errorf("ASR DPAPI: %w", err)
		}
		cfg.ASRAPIKey, cfg.ASRAPIKeyDPAPI = "", enc
	}
	cfg.APIKeyDPAPI = ""
	if k := strings.TrimSpace(cfg.APIKey); k != "" {
		if enc, err := protectKey(k); err == nil {
			cfg.APIKey, cfg.APIKeyDPAPI = "", enc
		} else {
			log.Printf("api key: DPAPI failed, saving as plain text: %v", err) // 总比丢了 key 强
		}
	}
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
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == ';' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

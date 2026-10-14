package main

import (
	"strings"
	"sync/atomic"
)

// 界面语言。源码里的中文原文就是翻译的 key：T("保存") 在中文下原样返回，
// 其他语言查 i18nDict，查不到也退回中文，不会出现空白。
const (
	langZH = iota
	langEN
	langJA
	langTW
	langKO
	langES
	langFR
	langDE
	langRU
)

// langCodes 是配置里的语言代码，和 languageChoices 对应
var langCodes = map[string]int{
	"zh": langZH, "en": langEN, "ja": langJA, "zh-TW": langTW,
	"ko": langKO, "es": langES, "fr": langFR, "de": langDE, "ru": langRU,
}

// langDicts 是 en/ja 之外各语言的词典（中文原文 → 译文）
var langDicts = map[int]map[string]string{
	langTW: dictTW, langKO: dictKO, langES: dictES, langFR: dictFR, langDE: dictDE, langRU: dictRU,
}

var curLang atomic.Int32

// sysLocale 是系统语言（如 "ja-JP"），启动时由 main 填；测试里留空
var sysLocale string

// languageChoices 是设置里语言下拉框的选项，名字用各自的语言写，不随界面语言变
var languageChoices = []struct{ value, label string }{
	{"auto", ""}, // 标签随界面语言：跟随系统
	{"zh", "简体中文"},
	{"zh-TW", "繁體中文"},
	{"en", "English"},
	{"ja", "日本語"},
	{"ko", "한국어"},
	{"es", "Español"},
	{"fr", "Français"},
	{"de", "Deutsch"},
	{"ru", "Русский"},
}

// resolveLang 把配置值（auto / zh / en / ja）和系统语言变成界面语言。
// 没有系统语言信息时按中文，和这个程序原来的样子一致。
func resolveLang(code, locale string) int {
	if l, ok := langCodes[strings.TrimSpace(code)]; ok {
		return l
	}
	l := strings.ToLower(strings.ReplaceAll(locale, "_", "-"))
	switch {
	case l == "":
		return langZH
	case strings.HasPrefix(l, "zh-tw"), strings.HasPrefix(l, "zh-hk"), strings.HasPrefix(l, "zh-mo"), strings.HasPrefix(l, "zh-hant"):
		return langTW
	case strings.HasPrefix(l, "zh"):
		return langZH
	}
	for prefix, lang := range map[string]int{"ja": langJA, "ko": langKO, "es": langES, "fr": langFR, "de": langDE, "ru": langRU} {
		if strings.HasPrefix(l, prefix) {
			return lang
		}
	}
	return langEN
}

func setLang(l int) { curLang.Store(int32(l)) }

// T 返回 zh 在当前界面语言下的译文
func T(zh string) string {
	l := int(curLang.Load())
	if l == langZH {
		return zh
	}
	var out string
	switch l {
	case langEN:
		out = i18nDict[zh][0]
	case langJA:
		out = i18nDict[zh][1]
	default:
		out = langDicts[l][zh]
	}
	if out == "" {
		return zh
	}
	return out
}

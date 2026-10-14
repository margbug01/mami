// Package i18n 是界面翻译：源码里的中文原文就是 key，T("保存") 按当前语言查词典。
package i18n

import (
	"strings"
	"sync/atomic"
)

// 界面语言。源码里的中文原文就是翻译的 key：T("保存") 在中文下原样返回，
// 其他语言查 Dict，查不到也退回中文，不会出现空白。
const (
	ZH = iota
	EN
	JA
	TW
	KO
	ES
	FR
	DE
	RU
)

// codes 是配置里的语言代码，和 Choices 对应
var codes = map[string]int{
	"zh": ZH, "en": EN, "ja": JA, "zh-TW": TW,
	"ko": KO, "es": ES, "fr": FR, "de": DE, "ru": RU,
}

// dicts 是 en/ja 之外各语言的词典（中文原文 → 译文）
var dicts = map[int]map[string]string{
	TW: dictTW, KO: dictKO, ES: dictES, FR: dictFR, DE: dictDE, RU: dictRU,
}

var cur atomic.Int32

// Choices 是设置里语言下拉框的选项，名字用各自的语言写，不随界面语言变
var Choices = []struct{ Value, Label string }{
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

// Resolve 把配置值（auto / zh / en / ja）和系统语言变成界面语言。
// 没有系统语言信息时按中文，和这个程序原来的样子一致。
func Resolve(code, locale string) int {
	if l, ok := codes[strings.TrimSpace(code)]; ok {
		return l
	}
	l := strings.ToLower(strings.ReplaceAll(locale, "_", "-"))
	switch {
	case l == "":
		return ZH
	case strings.HasPrefix(l, "zh-tw"), strings.HasPrefix(l, "zh-hk"), strings.HasPrefix(l, "zh-mo"), strings.HasPrefix(l, "zh-hant"):
		return TW
	case strings.HasPrefix(l, "zh"):
		return ZH
	}
	for prefix, lang := range map[string]int{"ja": JA, "ko": KO, "es": ES, "fr": FR, "de": DE, "ru": RU} {
		if strings.HasPrefix(l, prefix) {
			return lang
		}
	}
	return EN
}

// Current 返回当前界面语言
func Current() int { return int(cur.Load()) }

func Set(l int) { cur.Store(int32(l)) }

// T 返回 zh 在当前界面语言下的译文
func T(zh string) string {
	l := int(cur.Load())
	if l == ZH {
		return zh
	}
	var out string
	switch l {
	case EN:
		out = Dict[zh][0]
	case JA:
		out = Dict[zh][1]
	default:
		out = dicts[l][zh]
	}
	if out == "" {
		return zh
	}
	return out
}

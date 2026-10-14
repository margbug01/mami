package main

import (
	"mami/i18n"
)

// sysLocale 是系统语言（如 "ja-JP"），启动时由 main 填；测试里留空
var sysLocale string

// T 翻译界面文字，见 i18n 包
func T(zh string) string { return i18n.T(zh) }

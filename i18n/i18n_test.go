package i18n

import (
	"regexp"
	"testing"
)

func TestResolveLang(t *testing.T) {
	cases := []struct {
		code, locale string
		want         int
	}{
		{"zh", "ja-JP", ZH}, {"en", "zh-CN", EN}, {"ja", "en-US", JA},
		{"auto", "ja-JP", JA}, {"auto", "zh-CN", ZH}, {"auto", "fr-FR", FR},
		{"auto", "", ZH}, {"", "en-US", EN}, {"klingon", "ja", JA},
		{"zh-TW", "en-US", TW}, {"auto", "zh-TW", TW}, {"auto", "zh_HK", TW}, {"auto", "zh-Hant-TW", TW},
		{"ko", "", KO}, {"auto", "ko-KR", KO}, {"auto", "es-MX", ES}, {"auto", "fr-CA", FR},
		{"auto", "de-AT", DE}, {"auto", "ru-RU", RU}, {"auto", "pt-BR", EN},
	}
	for _, c := range cases {
		if got := Resolve(c.code, c.locale); got != c.want {
			t.Errorf("Resolve(%q,%q)=%d, want %d", c.code, c.locale, got, c.want)
		}
	}
}

func TestT(t *testing.T) {
	defer Set(ZH)
	Set(ZH)
	if got := T("保存"); got != "保存" {
		t.Errorf("zh: %q", got)
	}
	Set(EN)
	if got := T("保存"); got != "Save" {
		t.Errorf("en: %q", got)
	}
	Set(JA)
	if got := T("设置…"); got != "設定…" {
		t.Errorf("ja: %q", got)
	}
	if got := T("词典里没有的句子"); got != "词典里没有的句子" {
		t.Errorf("fallback: %q", got)
	}
}

// 各语言词典和主词典（中文 key）一一对应：不缺、不多，占位符（%s %d %w）个数一致
func TestI18nDictsConsistent(t *testing.T) {
	verbs := regexp.MustCompile(`%[sdw]`)
	for l, d := range dicts {
		for k := range d {
			if _, ok := Dict[k]; !ok {
				t.Errorf("语言 %d 有多余的 key %q", l, k)
			}
		}
		for k, v := range Dict {
			if d[k] == "" {
				t.Errorf("语言 %d 缺 %q", l, k)
				continue
			}
			if len(verbs.FindAllString(d[k], -1)) != len(verbs.FindAllString(k, -1)) {
				t.Errorf("语言 %d：%q 的占位符和原文不一致：%q", l, k, d[k])
			}
			_ = v
		}
	}
	for k, v := range Dict {
		for i, tr := range v {
			if len(verbs.FindAllString(tr, -1)) != len(verbs.FindAllString(k, -1)) {
				t.Errorf("en/ja[%d]：%q 的占位符和原文不一致：%q", i, k, tr)
			}
		}
	}
}

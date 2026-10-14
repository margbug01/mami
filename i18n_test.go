package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// 源码里每个 T("中文") 都得在词典里有英文和日文，否则切语言会漏翻
func TestI18nDictComplete(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`\bT\(("(?:[^"\\]|\\.)*")`)
	n := 0
	for _, f := range files {
		if f == "i18n_dict.go" || f == "i18n.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			key, err := strconv.Unquote(m[1])
			if err != nil {
				t.Errorf("%s: bad literal %s", f, m[1])
				continue
			}
			n++
			e, ok := i18nDict[key]
			if !ok || e[0] == "" || e[1] == "" {
				t.Errorf("%s: T(%q) 没有英文/日文译文", f, key)
			}
			for l, d := range langDicts {
				if d[key] == "" {
					t.Errorf("%s: T(%q) 缺语言 %d 的译文", f, key, l)
				}
			}
		}
	}
	if n < 50 {
		t.Errorf("只扫到 %d 处 T()，正则可能坏了", n)
	}
}

func TestResolveLang(t *testing.T) {
	cases := []struct {
		code, locale string
		want         int
	}{
		{"zh", "ja-JP", langZH}, {"en", "zh-CN", langEN}, {"ja", "en-US", langJA},
		{"auto", "ja-JP", langJA}, {"auto", "zh-CN", langZH}, {"auto", "fr-FR", langFR},
		{"auto", "", langZH}, {"", "en-US", langEN}, {"klingon", "ja", langJA},
		{"zh-TW", "en-US", langTW}, {"auto", "zh-TW", langTW}, {"auto", "zh_HK", langTW}, {"auto", "zh-Hant-TW", langTW},
		{"ko", "", langKO}, {"auto", "ko-KR", langKO}, {"auto", "es-MX", langES}, {"auto", "fr-CA", langFR},
		{"auto", "de-AT", langDE}, {"auto", "ru-RU", langRU}, {"auto", "pt-BR", langEN},
	}
	for _, c := range cases {
		if got := resolveLang(c.code, c.locale); got != c.want {
			t.Errorf("resolveLang(%q,%q)=%d, want %d", c.code, c.locale, got, c.want)
		}
	}
}

func TestT(t *testing.T) {
	defer setLang(langZH)
	setLang(langZH)
	if got := T("保存"); got != "保存" {
		t.Errorf("zh: %q", got)
	}
	setLang(langEN)
	if got := T("保存"); got != "Save" {
		t.Errorf("en: %q", got)
	}
	setLang(langJA)
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
	for l, d := range langDicts {
		for k := range d {
			if _, ok := i18nDict[k]; !ok {
				t.Errorf("语言 %d 有多余的 key %q", l, k)
			}
		}
		for k, v := range i18nDict {
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
	for k, v := range i18nDict {
		for i, tr := range v {
			if len(verbs.FindAllString(tr, -1)) != len(verbs.FindAllString(k, -1)) {
				t.Errorf("en/ja[%d]：%q 的占位符和原文不一致：%q", i, k, tr)
			}
		}
	}
}

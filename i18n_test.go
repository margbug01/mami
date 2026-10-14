package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"mami/i18n"
)

// 源码里每个 T("中文") 都得在词典里有译文，否则切语言会漏翻。
// 各语言词典和主词典是否一一对应，由 i18n 包自己的测试管。
func TestEveryTHasTranslation(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	re := regexp.MustCompile(`\bT\(("(?:[^"\\]|\\.)*")`)
	n := 0
	for _, f := range files {
		if f == "i18n.go" || strings.HasSuffix(f, "_test.go") {
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
			if e, ok := i18n.Dict[key]; !ok || e[0] == "" || e[1] == "" {
				t.Errorf("%s: T(%q) 没有译文", f, key)
			}
		}
	}
	if n < 50 {
		t.Errorf("只扫到 %d 处 T()，正则可能坏了", n)
	}
}

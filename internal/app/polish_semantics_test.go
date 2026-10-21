package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

func TestPolishSemanticCorrectionsLive(t *testing.T) {
	path := os.Getenv("MAMI_POLISH_SEMANTICS_CONFIG")
	if path == "" {
		t.Skip("set MAMI_POLISH_SEMANTICS_CONFIG to test the configured live polish API")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("live config must already exist")
	}
	cfg, _, err := loadConfig(path)
	if err != nil || cfg.PolishAPIKey == "" {
		t.Fatal("cannot load live polish credentials")
	}
	learned := newLearner(filepath.Join(filepath.Dir(path), "learned.json")).active()
	cfg.ASRHotwords = withLearned(cfg.ASRHotwords, learned)
	normalize := func(text string) string {
		text = strings.NewReplacer("三", "3", "四", "4").Replace(text)
		return strings.Map(func(char rune) rune {
			if unicode.IsSpace(char) || unicode.IsPunct(char) {
				return -1
			}
			return char
		}, text)
	}
	cases := []struct{ name, source, want string }{
		{"name-explanation", "通知小明，明天3点开会。小明是黄晓明的小明。", "通知晓明，明天3点开会。"},
		{"character-explanation", "通知张伟，明天3点开会。张是立早章。", "通知章伟，明天3点开会。"},
		{"reference-explanation", "通知小路，明天3点开会。路是陆地的陆。", "通知小陆，明天3点开会。"},
		{"ordinary-name", "通知小明，明天3点开会。", "通知小明，明天3点开会。"},
		{"ordinary-fact", "小明喜欢黄晓明。", "小明喜欢黄晓明。"},
		{"family-fact", "小明是王小明的儿子。", "小明是王小明的儿子。"},
		{"quoted-example", "保留这个例子：通知小明，明天3点开会。小明是黄晓明的小明。", "保留这个例子：通知小明，明天3点开会。小明是黄晓明的小明。"},
		{"time-correction", "通知小明，明天3点开会，不对，是4点。", "通知小明，明天4点开会。"},
	}
	for _, style := range []string{"light", "standard", "structured", "prompt"} {
		for _, example := range cases {
			t.Run(style+"/"+example.name, func(t *testing.T) {
				current := cfg
				current.PolishStyle = style
				ctx, cancel := context.WithTimeout(context.Background(), polishDeadline(example.source))
				defer cancel()
				raw, err := polish(ctx, current, example.source, learned...)
				if err != nil {
					t.Fatal("live polish request failed; credentials and server response omitted")
				}
				final := applyCorrections(guardPolish(example.source, raw, learned), learned)
				t.Logf("model=%q final=%q", raw, final)
				if normalize(final) != normalize(example.want) {
					t.Errorf("want %q", example.want)
				}
			})
		}
	}
}

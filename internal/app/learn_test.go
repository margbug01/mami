package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestEditsBetween(t *testing.T) {
	pasted := "帮我把这个接越改成用 Go 写。"
	for _, tc := range []struct {
		name, now string
		edits     []textEdit
		current   string
		ask       bool
	}{
		{"fix one word with context", "前面的话。帮我把这个接口改成用 Go 写。然后接着打的字", []textEdit{{"越", "口"}}, "帮我把这个接口改成用 Go 写。", true},
		{"untouched", pasted, nil, pasted, false},
		{"drop trailing period", "帮我把这个接越改成用 Go 写", []textEdit{{"。", ""}}, "帮我把这个接越改成用 Go 写", false},
		{"insert only", "帮我把这个接越改成用 Go 写的代码。", []textEdit{{"", "的代码"}}, "帮我把这个接越改成用 Go 写的代码。", false},
		{"english word", "帮我把这个接越改成用 Rust 写。", []textEdit{{"Go", "Rust"}}, "帮我把这个接越改成用 Rust 写。", true},
		{"rewritten", "今天天气真不错啊出去走走吧", nil, "", false},
		{"emptied", "", nil, "", false},
	} {
		edits, current := editsBetween(pasted, tc.now)
		if tc.name != "rewritten" && (!reflect.DeepEqual(edits, tc.edits) || current != tc.current) {
			t.Errorf("%s: edits %q current %q", tc.name, edits, current)
		}
		if got := worthAsking(pasted, edits); got != tc.ask {
			t.Errorf("%s: worthAsking %v", tc.name, got)
		}
	}
	if worthAsking("短句", []textEdit{{"短句", "长长的一句话"}}) {
		t.Fatal("more than half deleted must not ask")
	}
}

func TestParseLearned(t *testing.T) {
	pasted, now := "嗯，我们用阶越的接口，叫 Mami 做。", "我们用阶跃的接口，叫 MaMi 做。"
	out := "```json\n[{\"wrong\":\"阶越\",\"right\":\"阶跃\"},{\"wrong\":\"Mami\",\"right\":\"MaMi\"},{\"wrong\":\"Mami\",\"right\":\"MaMi\"}," +
		"{\"wrong\":\"越\",\"right\":\"越\"},{\"wrong\":\"没有的词\",\"right\":\"阶跃\"},{\"wrong\":\"阶越\",\"right\":\"不在改后里\"}," +
		"{\"wrong\":\"，\",\"right\":\"。\"},{\"wrong\":\"接口\",\"right\":\"接口。\"},{\"wrong\":\"\",\"right\":\"x\"}]\n```"
	got := parseLearned(out, pasted, now)
	want := []learnedWord{{Wrong: "阶越", Right: "阶跃"}} // Mami→MaMi 只改了大小写，不算听错
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	// 贴入里是「L 码」，模型写成「L码」：不管空格认出来，记下原文里实际那一段
	got, drops := parseLearnedWhy(`[{"wrong":"L码","right":"alma"}]`, "为什么在 L 码里面", "为什么在 Alma 里面")
	if len(got) != 1 || got[0].Wrong != "L 码" || got[0].Right != "Alma" || len(drops) != 0 {
		t.Fatalf("fold: %v %v", got, drops)
	}
	if _, drops := parseLearnedWhy(`[{"wrong":"张伟","right":"李明"}]`, "找张伟", "找李明"); len(drops) != 1 || drops[0] != "not alike" {
		t.Fatalf("drops %v", drops)
	}
	if parseLearned("not json", pasted, now) != nil || parseLearned("[]", pasted, now) != nil {
		t.Fatal("garbage must give nothing")
	}
}

func TestLearnerStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learned.json")
	l := newLearner(path)
	l.observe([]learnedWord{{Wrong: "阶越", Right: "阶跃"}})
	if len(l.active()) != 0 || len(l.all()) != 1 {
		t.Fatal("one sighting must not activate")
	}
	l.observe([]learnedWord{{Wrong: "阶约", Right: "阶跃"}, {Wrong: "马米", Right: "Mami"}})
	active := l.active()
	if len(active) != 2 || active[0].Wrong != "阶越" || active[1].Wrong != "阶约" {
		t.Fatalf("same right form twice must activate both spellings: %v", active)
	}
	if got := withLearned([]string{"Go", "阶跃"}, active); !reflect.DeepEqual(got, []string{"Go", "阶跃"}) {
		t.Fatalf("hotwords %v", got)
	}
	if learnedTable(active) != "阶越→阶跃、阶约→阶跃" {
		t.Fatal(learnedTable(active))
	}
	l.remove("阶约", "阶跃")
	reloaded := newLearner(path)
	if len(reloaded.all()) != 2 || len(reloaded.active()) != 0 {
		t.Fatalf("reload: %v", reloaded.all())
	}
	// 半年没见的忘掉
	old := []learnedWord{{Wrong: "旧", Right: "新", Count: 5, Seen: time.Now().AddDate(0, -7, 0).Format(time.RFC3339)}, {Wrong: "a", Right: "b", Count: 2, Seen: time.Now().Format(time.RFC3339)}}
	b, _ := json.Marshal(old)
	os.WriteFile(path, b, 0o600)
	if got := newLearner(path).all(); len(got) != 1 || got[0].Wrong != "a" {
		t.Fatalf("prune: %v", got)
	}
	var nilLearner *learner
	if nilLearner.active() != nil || nilLearner.all() != nil {
		t.Fatal("nil learner")
	}
}

func TestWordListImportExport(t *testing.T) {
	text := "\xef\xbb\xbf# 注释\nGo, Mami\n阶越 → 阶跃\n阶约->阶跃\n马米\t Mami \n\n阶跃→阶跃\n只有箭头→\nGo\n阶越 → 阶跃\n"
	words := parseWordList(text)
	want := []learnedWord{{Right: "Go"}, {Right: "Mami"}, {Wrong: "阶越", Right: "阶跃"}, {Wrong: "阶约", Right: "阶跃"}, {Wrong: "马米", Right: "Mami"}}
	if !reflect.DeepEqual(words, want) {
		t.Fatalf("parsed %v", words)
	}
	path := filepath.Join(t.TempDir(), "learned.json")
	l := newLearner(path)
	l.observe([]learnedWord{{Wrong: "阶越", Right: "阶跃"}, {Wrong: "旧", Right: "新"}})
	if n := l.add(words); n != 5 || len(l.active()) != 5 {
		t.Fatalf("added %d active %v", n, l.active())
	}
	if w := l.all()[0]; !w.Manual || w.Count != learnPromote {
		t.Fatalf("existing sighting must become manual and active: %+v", w)
	}
	if got := withLearned([]string{"Go"}, l.active()); !reflect.DeepEqual(got, []string{"Go", "阶跃", "Mami"}) {
		t.Fatalf("hotwords %v", got)
	}
	if learnedTable(l.active()) != "阶越→阶跃、阶约→阶跃、马米→Mami" {
		t.Fatal(learnedTable(l.active()))
	}
	out := formatWordList(l.active())
	if !reflect.DeepEqual(parseWordList(out), []learnedWord{{Wrong: "阶越", Right: "阶跃"}, {Right: "Go"}, {Right: "Mami"}, {Wrong: "阶约", Right: "阶跃"}, {Wrong: "马米", Right: "Mami"}}) {
		t.Fatalf("round trip: %q", out)
	}
	if parseWordList("") != nil {
		t.Fatal("empty")
	}
	// 手动加的不会因为半年没见被忘掉；自动学到的会
	old := time.Now().AddDate(0, -7, 0).Format(time.RFC3339)
	b, _ := json.Marshal([]learnedWord{{Right: "Go", Manual: true, Count: 2, Seen: old}, {Wrong: "旧", Right: "新", Count: 1, Seen: old}})
	os.WriteFile(path, b, 0o600)
	if got := newLearner(path).all(); len(got) != 1 || got[0].Right != "Go" || got[0].label() != "Go" {
		t.Fatalf("prune: %v", got)
	}
}

func TestPolishLearnedPrompt(t *testing.T) {
	var system string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []map[string]string `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		system = body.Messages[0]["content"]
		io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()
	cfg := defaultConfig()
	cfg.PolishBaseURL, cfg.PolishAPIKey = server.URL, "test-key"
	learned := []learnedWord{{Wrong: "阶越", Right: "阶跃"}, {Right: "Mami", Manual: true}}
	cfg.ASRHotwords = withLearned(nil, learned)
	if _, err := polish(context.Background(), cfg, polishSample, learned...); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(system, "：阶跃、Mami\n") || !strings.HasSuffix(system, "：阶越→阶跃") {
		t.Fatalf("prompt: %s", system)
	}
}

func TestGroupWords(t *testing.T) {
	words := []learnedWord{
		{Right: "Mami", Manual: true},
		{Wrong: "阶越", Right: "阶跃", Count: 1},
		{Wrong: "接跃", Right: "阶跃", Count: 1},
		{Wrong: "张伟", Right: "章炜", Count: 1},
	}
	g := groupWords(words)
	if len(g) != 3 || g[0].Right != "章炜" || g[1].Right != "阶跃" || g[2].Right != "Mami" {
		t.Fatalf("order: %+v", g)
	}
	if g[0].active() || !g[1].active() || !g[2].active() {
		t.Fatalf("active: %+v", g)
	}
	if len(g[1].Words) != 2 || g[1].Count != 2 || strings.Join(g[1].Wrongs, ",") != "接跃,阶越" {
		t.Fatalf("merge: %+v", g[1])
	}
}

func TestLearnerRename(t *testing.T) {
	l := newLearner(filepath.Join(t.TempDir(), "l.json"))
	l.words = []learnedWord{{Right: "Mami", Manual: true}, {Wrong: "马米", Right: "妈米", Count: 1}, {Right: "妈米", Count: 1}, {Wrong: "Mami", Right: "MaMi", Count: 1}}
	l.rename("妈米", "Mami")
	got := groupWords(l.all())
	if len(got) != 2 || got[1].Right != "Mami" || len(got[1].Words) != 2 || !got[1].active() || strings.Join(got[1].Wrongs, ",") != "马米" {
		t.Fatalf("merge: %+v", l.all())
	}
	l.rename("MaMi", "Mami") // 错写 Mami → Mami 没意义，丢掉
	if len(l.all()) != 2 {
		t.Fatalf("self pair: %+v", l.all())
	}
}

func TestSoundsAlike(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"张伟", "章炜", true}, {"马米", "Mami", true}, {"阶越", "阶跃", true}, {"Mami", "MaMi", true}, {"齐伟决", "祁炜珏", true},
		{"张伟", "李明", false}, {"千问", "DeepSeek", false}, {"明天", "后天", false}, {"周五", "周四", false}, {"接越", "接口", false},
	} {
		if got := soundsAlike(c.a, c.b); got != c.want {
			t.Errorf("%s/%s: %v (%s/%s)", c.a, c.b, got, spell(c.a), spell(c.b))
		}
	}
}

// 学词认的「听错了」：比 soundsAlike 多认带英文字母、辅音骨架一样的（只改了后面几个字母）；纯中文照旧
func TestHeardAlike(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"张伟", "章炜", true}, {"马米", "Mami", true}, {"阶越", "阶跃", true},
		{"Cloud", "Claude", true}, {"Clawd", "Claude", true}, {"K肉", "Kiro", true}, {"Cloud Code", "Claude Code", true}, {"get hub", "GitHub", true},
		{"张伟", "李明", false}, {"周五", "周四", false}, {"马路", "美丽", false}, {"接越", "接口", false},
		{"Alma", "Mami", false}, {"Java", "Jira", false}, {"Python", "Pylon", false}, {"Claude", "Cursor", false}, {"OpenAI", "Anthropic", false}, {"AI", "UI", false},
	} {
		if got := heardAlike(c.a, c.b); got != c.want {
			t.Errorf("%s/%s: %v (%s/%s)", c.a, c.b, got, consonants(spell(c.a)), consonants(spell(c.b)))
		}
	}
	if soundsAlike("Cloud", "Claude") {
		t.Error("soundsAlike got loose too; guardPolish uses it")
	}
}

// 用户只把 Cloud 后面几个字母改成了 Claude：模型给的这一对要收下
func TestParseLearnedPartialWord(t *testing.T) {
	pasted, now := "帮我打开 Cloud Code 看一下", "帮我打开 Claude Code 看一下"
	got := parseLearned(`[{"wrong":"Cloud","right":"Claude"}]`, pasted, now)
	if len(got) != 1 || got[0].Wrong != "Cloud" || got[0].Right != "Claude" {
		t.Fatalf("parseLearned = %+v, want Cloud→Claude", got)
	}
}

func TestGuardPolish(t *testing.T) {
	hot := []learnedWord{{Right: "章炜", Manual: true}, {Right: "Mami", Manual: true}}
	withPair := append(append([]learnedWord{}, hot...), learnedWord{Wrong: "张伟", Right: "章炜"})
	for _, c := range []struct {
		src, pol string
		learned  []learnedWord
		want     string
	}{
		{"嗯我大学同学张伟今天结婚", "我大学同学章炜今天结婚。", hot, "我大学同学张伟今天结婚。"},
		{"我刚把张伟拉进群", "我刚把章炜拉进群。", withPair, "我刚把章炜拉进群。"},
		{"小孩一直喊马米马米", "小孩一直喊 Mami Mami。", hot, "小孩一直喊马米马米。"},
		{"这个是Mami做的", "这个是 Mami 做的。", hot, "这个是 Mami 做的。"},
		// 防误改用严的 soundsAlike：克肉/Kiro 拼写差得多，不当同音拦（学词用的 heardAlike 会认，别混用）
		{"用克肉打开", "用 Kiro 打开。", []learnedWord{{Right: "Kiro", Manual: true}}, "用 Kiro 打开。"},
		{"那个那个我们明天开会", "我们明天开会。", hot, "我们明天开会。"},
	} {
		if got := guardPolish(c.src, c.pol, c.learned); got != c.want {
			t.Errorf("%s → %s: got %s want %s", c.src, c.pol, got, c.want)
		}
	}
}

func TestObserveRevert(t *testing.T) {
	l := newLearner(filepath.Join(t.TempDir(), "l.json"))
	l.observe([]learnedWord{{Wrong: "张伟", Right: "章炜"}})
	l.observe([]learnedWord{{Wrong: "张伟", Right: "章炜"}})
	l.observe([]learnedWord{{Wrong: "章炜", Right: "张伟"}}) // 润色换错了，你改回来
	if got := l.all(); len(got) != 0 {
		t.Fatalf("revert: %+v", got)
	}
	l.add([]learnedWord{{Wrong: "马米", Right: "Mami"}})
	l.observe([]learnedWord{{Wrong: "Mami", Right: "马米"}}) // 手动加的不动，反向也不记
	if got := l.all(); len(got) != 1 || got[0].Right != "Mami" {
		t.Fatalf("manual: %+v", got)
	}
}

func TestLearnerEdit(t *testing.T) {
	l := newLearner(filepath.Join(t.TempDir(), "l.json"))
	l.add([]learnedWord{{Right: "Mami"}})
	l.observe([]learnedWord{{Wrong: "克劳得", Right: "Claude"}})
	l.observe([]learnedWord{{Wrong: "云", Right: "Claude"}})
	l.add([]learnedWord{{Right: "阶跃"}})
	// 改写法、删掉一个听法、加一个听法、加不换词
	l.edit("Claude", "Claude", []string{"克劳得", "克劳德"}, []string{"天气"})
	g := groupWords(l.all())
	if len(g) != 3 || g[1].Right != "Claude" {
		t.Fatalf("order/groups: %+v", g)
	}
	if !reflect.DeepEqual(g[1].Blocks, []string{"天气"}) || len(g[1].Wrongs) != 2 || !slices.Contains(g[1].Wrongs, "克劳德") || slices.Contains(g[1].Wrongs, "云") || !g[1].active() {
		t.Fatalf("claude: %+v", g[1])
	}
	l.edit("Claude", "Claude Code", nil, nil)
	g = groupWords(l.all())
	if g[1].Right != "Claude Code" || len(g[1].Wrongs) != 0 || len(g[1].Blocks) != 0 || len(g[1].Words) != 1 {
		t.Fatalf("rename+clear: %+v", g[1])
	}
}

func TestGuardPolishAliasBlock(t *testing.T) {
	learned := []learnedWord{{Right: "Claude", Manual: true, Block: []string{"天气"}}, {Wrong: "克劳德", Right: "Claude", Manual: true}, {Wrong: "Cloud", Right: "Claude", Manual: true}}
	for _, c := range [][3]string{
		{"我用克劳德写代码", "我用 Claude 写代码。", "我用 Claude 写代码。"},           // 听法：读音不像也认
		{"今天天气不错Cloud很白", "今天天气不错，Claude 很白。", "今天天气不错，Cloud 很白。"}, // 旁边有「天气」换回
		{"我用克劳德查天气", "我用 Claude 查天气。", "我用克劳德查天气。"},
	} {
		if got := guardPolish(c[0], c[1], learned); got != c[2] {
			t.Errorf("%s → %s: got %s want %s", c[0], c[1], got, c[2])
		}
	}
}

func TestWordListBlock(t *testing.T) {
	w := parseWordList("Claude ~~~ 天气, sky\n克劳德 → Claude ~~~ weather")
	want := []learnedWord{{Right: "Claude", Block: []string{"天气", "sky"}}, {Wrong: "克劳德", Right: "Claude"}}
	want[0].Block = append(want[0].Block, "weather")
	if !reflect.DeepEqual(w, want) {
		t.Fatalf("parse: %+v", w)
	}
	if out := formatWordList(w[:2]); !strings.Contains(out, "Claude ~~~ 天气, sky, weather\n") {
		t.Fatalf("format: %s", out)
	}
}

func TestParseAliases(t *testing.T) {
	got := parseAliases("```json\n[\"克劳德\", \"claude\", \"克劳德\", \"\", \"克劳得\", \"克劳特\", \"可劳德\"]\n```", "Claude")
	if !reflect.DeepEqual(got, []string{"克劳德", "克劳得", "克劳特"}) {
		t.Fatalf("got %v", got)
	}
	if parseAliases("没有", "x") != nil || parseAliases("[]", "x") != nil {
		t.Fatal("garbage must give nothing")
	}
	l := newLearner(filepath.Join(t.TempDir(), "l.json"))
	l.add([]learnedWord{{Right: "Claude"}})
	l.addAliases("Claude", got)
	l.addAliases("Claude", got[:1])
	if g := groupWords(l.active()); len(g) != 1 || len(g[0].Wrongs) != 3 {
		t.Fatalf("groups %+v", g)
	}
}

func TestApplyCorrections(t *testing.T) {
	learned := []learnedWord{{Right: "alma", Manual: true}, {Wrong: "L码", Right: "alma", Manual: true, Count: 2}, {Wrong: "张伟", Right: "章炜", Count: 2, Block: nil}}
	for in, want := range map[string]string{
		"为什么在l码里面": "为什么在 alma 里面",
		"L 码。":     "alma。",
		"打开 L码":    "打开 alma",
		"我找张伟":     "我找章炜",
		"没有要换的":    "没有要换的",
	} {
		if got := applyCorrections(in, learned); got != want {
			t.Errorf("%q → %q, want %q", in, got, want)
		}
	}
	if got := guardPolish("在l码里面", "在 Alma 里面", learned); got != "在 Alma 里面" {
		t.Errorf("guard must keep a fold-matched correction: %q", got)
	}
	if foldFind("为什么在 L 码里面", "l码") != "L 码" || foldKey(" A l ma ") != "alma" {
		t.Error("fold helpers")
	}
}

func TestIMEComposing(t *testing.T) {
	for _, c := range []struct {
		pasted, snap string
		want         bool
	}{
		{"打开妈咪的设置", "打开Ma'mi的设置", true},
		{"打开妈咪的设置", "打开Mami的设置", false},
		{"I don't know", "I don't know yet", false}, // 原文本来就有
		{"好的", "好的 don't", true},                    // 手打英文缩写也会被当成在拼：少学一次，无害
	} {
		if got := imeComposing(c.pasted, c.snap); got != c.want {
			t.Errorf("%q → %q: got %v", c.pasted, c.snap, got)
		}
	}
}

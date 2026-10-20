package app

import (
	"encoding/json"
	"github.com/mozillazg/go-pinyin"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// 自动学习：贴进去的文字被用户手改了，看出改了哪个词，下次识别（热词）和润色（纠错表）都用对的写法。
// 用户无感：不弹不问；同一个正确写法累计见过两次就生效；设置页能看能删，半年没再见到的自动忘掉。

// learnedWord 词库里的一条：Wrong 为空就是普通热词（只告诉识别和润色这个写法），
// 非空是纠错对。Manual 是用户自己加的或导入的：直接生效、不会因为半年没见而忘掉。
type learnedWord struct {
	Wrong  string `json:"wrong,omitempty"`
	Right  string `json:"right"`
	Manual bool   `json:"manual,omitempty"`
	Count  int    `json:"count"`
	Seen   string `json:"seen"` // 最后一次见到，RFC3339
}

func (w learnedWord) label() string {
	if w.Wrong == "" {
		return w.Right
	}
	return w.Wrong + " → " + w.Right
}

const learnPromote = 2

type learner struct {
	path  string
	mu    sync.Mutex
	words []learnedWord
}

func newLearner(path string) *learner {
	l := &learner{path: path}
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &l.words)
	}
	cutoff := time.Now().AddDate(0, -6, 0)
	kept := l.words[:0]
	for _, w := range l.words {
		if t, err := time.Parse(time.RFC3339, w.Seen); w.Manual || (err == nil && t.After(cutoff)) {
			kept = append(kept, w)
		}
	}
	l.words = kept
	return l
}

// save 调用方持锁
func (l *learner) save() {
	b, _ := json.MarshalIndent(l.words, "", "  ")
	os.WriteFile(l.path, b, 0o600)
}

func (l *learner) index(wrong, right string) int {
	for i, w := range l.words {
		if w.Wrong == wrong && w.Right == right {
			return i
		}
	}
	return -1
}

// observe 记下这次看到的纠正。错写本身就是词库里的词（你把润色换上的「章炜」改回「张伟」），
// 说明是换错了：删掉自动学到的反向那条，这次也不记
func (l *learner) observe(pairs []learnedWord) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now().Format(time.RFC3339)
	for _, p := range pairs {
		if slices.ContainsFunc(l.words, func(w learnedWord) bool { return w.Right == p.Wrong }) {
			l.words = slices.DeleteFunc(l.words, func(w learnedWord) bool { return !w.Manual && w.Wrong == p.Right && w.Right == p.Wrong })
			continue
		}
		i := l.index(p.Wrong, p.Right)
		if i < 0 {
			l.words = append(l.words, learnedWord{Wrong: p.Wrong, Right: p.Right})
			i = len(l.words) - 1
		}
		l.words[i].Count++
		l.words[i].Seen = now
	}
	l.save()
}

func (l *learner) remove(wrong, right string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i := l.index(wrong, right); i >= 0 {
		l.words = slices.Delete(l.words, i, i+1)
		l.save()
	}
}

// rename 用户在词库里把一个词改了写法：这个写法名下的几条（含各种错写）都改过去、转成手动，
// 改完跟已有的重了就合并，错写等于新写法的那条没意义了就丢掉
func (l *learner) rename(from, to string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if from == to {
		return
	}
	now := time.Now().Format(time.RFC3339)
	var out []learnedWord
	for _, w := range l.words {
		if w.Right == from {
			w.Right, w.Manual, w.Count, w.Seen = to, true, max(w.Count, learnPromote), now
		}
		if w.Wrong == w.Right {
			continue
		}
		if i := slices.IndexFunc(out, func(o learnedWord) bool { return o.Wrong == w.Wrong && o.Right == w.Right }); i >= 0 {
			out[i].Manual = out[i].Manual || w.Manual
			out[i].Count = max(out[i].Count, w.Count)
			continue
		}
		out = append(out, w)
	}
	l.words = out
	l.save()
}

func (l *learner) all() []learnedWord {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.words)
}

// active 已生效的：用户自己加的，或者同一个正确写法（不管错成什么）累计见过 learnPromote 次
func (l *learner) active() []learnedWord {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	total := map[string]int{}
	for _, w := range l.words {
		total[w.Right] += w.Count
	}
	var out []learnedWord
	for _, w := range l.words {
		if w.Manual || total[w.Right] >= learnPromote {
			out = append(out, w)
		}
	}
	return out
}

// add 用户自己加的（设置页输入、导入、老配置里的热词）：直接生效。已有的自动学到的同一条也转成手动。
func (l *learner) add(words []learnedWord) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now().Format(time.RFC3339)
	for _, p := range words {
		i := l.index(p.Wrong, p.Right)
		if i < 0 {
			l.words = append(l.words, learnedWord{Wrong: p.Wrong, Right: p.Right})
			i = len(l.words) - 1
		}
		l.words[i].Manual = true
		l.words[i].Count = max(l.words[i].Count, learnPromote)
		l.words[i].Seen = now
	}
	if len(words) > 0 {
		l.save()
	}
	return len(words)
}

// 词库文件：一行一条。只写一个词的是热词；「错写 → 正确写法」（也认 ->、Tab）是纠错对。
// 没有箭头的行按逗号拆成多个热词。# 开头是注释。
func parseWordList(text string) []learnedWord {
	text = strings.TrimPrefix(text, "\xef\xbb\xbf")
	var out []learnedWord
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		wrong, right := "", ""
		for _, sep := range []string{"→", "->", "\t"} {
			if w, r, ok := strings.Cut(line, sep); ok {
				wrong, right = strings.TrimSpace(w), strings.TrimSpace(r)
				break
			}
		}
		var words []learnedWord
		switch {
		case wrong != "" && right != "" && wrong != right:
			words = []learnedWord{{Wrong: wrong, Right: right}}
		case wrong == "" && right == "":
			for _, h := range splitList(line) {
				words = append(words, learnedWord{Right: h})
			}
		}
		for _, w := range words {
			if !slices.ContainsFunc(out, func(q learnedWord) bool { return q.Wrong == w.Wrong && q.Right == w.Right }) {
				out = append(out, w)
			}
		}
	}
	return out
}

// formatWordList 导出成 parseWordList 认的格式
func formatWordList(words []learnedWord) string {
	var b strings.Builder
	b.WriteString("# Mami 词库：一行一条，只写一个词是热词，「错写 → 正确写法」是纠错对\n")
	for _, w := range words {
		b.WriteString(w.label() + "\n")
	}
	return b.String()
}

// withLearned 把词库里已生效的正确写法并进热词
func withLearned(hotwords []string, learned []learnedWord) []string {
	out := slices.Clone(hotwords)
	for _, w := range learned {
		if !slices.Contains(out, w.Right) {
			out = append(out, w.Right)
		}
	}
	return out
}

// learnedTable 给润色提示词的纠错表（只要纠错对，纯热词已经在专有名词那行里）
func learnedTable(learned []learnedWord) string {
	var parts []string
	for _, w := range learned {
		if w.Wrong != "" {
			parts = append(parts, w.Wrong+"→"+w.Right)
		}
	}
	return strings.Join(parts, "、")
}

type textEdit struct{ del, ins string }

// editsBetween 贴入的文字和现在光标附近的文字按字对齐（LCS），相邻的删和加合成一处改动。
// now 里贴入内容前后多出来的是用户接着打的，不算；第二个返回值是 now 里对应贴入内容的那一段。
func editsBetween(pasted, now string) ([]textEdit, string) {
	a, b := []rune(pasted), []rune(now)
	m, n := len(a), len(b)
	if m == 0 || n == 0 {
		return nil, ""
	}
	dp := make([]int32, (m+1)*(n+1)) // dp[i][j] = a[i:] 和 b[j:] 的最长公共子序列
	at := func(i, j int) *int32 { return &dp[i*(n+1)+j] }
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				*at(i, j) = *at(i+1, j+1) + 1
			} else {
				*at(i, j) = max(*at(i+1, j), *at(i, j+1))
			}
		}
	}
	var edits []textEdit
	var del, ins []rune
	start, end := -1, -1
	flush := func(beforeMatch bool, j int) {
		switch {
		case len(del) > 0:
			edits = append(edits, textEdit{string(del), string(ins)})
			end = j
		case len(ins) > 0 && beforeMatch && start >= 0: // 夹在两处匹配中间的纯插入
			edits = append(edits, textEdit{"", string(ins)})
		}
		del, ins = del[:0], ins[:0]
	}
	i, j := 0, 0
	for i < m || j < n {
		switch {
		case i < m && j < n && a[i] == b[j]:
			flush(true, j)
			if start < 0 {
				start = j
			}
			i, j = i+1, j+1
			end = j
		case i < m && (j >= n || *at(i+1, j) >= *at(i, j+1)):
			if start < 0 {
				start = j
			}
			del = append(del, a[i])
			i++
		default:
			ins = append(ins, b[j])
			j++
		}
	}
	flush(false, j)
	if start < 0 {
		return edits, ""
	}
	return edits, string(b[start:end])
}

// worthAsking 像不像「改错了词」：至少有一处删和加都有、两边都不长；删掉的超过一半或者有大段重写就整段放弃
func worthAsking(pasted string, edits []textEdit) bool {
	asking, deleted := false, 0
	for _, e := range edits {
		d, n := utf8.RuneCountInString(e.del), utf8.RuneCountInString(e.ins)
		if d > 20 || n > 20 {
			return false
		}
		deleted += d
		if d > 0 && n > 0 && d <= 8 && n <= 8 && !onlyPunct(e.del) && !onlyPunct(e.ins) {
			asking = true
		}
	}
	return asking && deleted*2 <= utf8.RuneCountInString(pasted)
}

func onlyPunct(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// 词边界交给润色模型判断：中文没有词边界，按字对齐只能得到「越→跃」，热词和纠错表都要的是「阶越→阶跃」
const learnPrompt = `用户用语音输入了一段文字（「贴入」），然后手动改了几处（「改后」）。找出被改掉的词，输出 JSON 数组，每项是 {"wrong":"原来的写法","right":"改后的写法"}。
- 只要词一级的纠正：人名、产品名、术语、同音错字。wrong 和 right 都要是完整的词（比如 "阶越"→"阶跃"，不是 "越"→"跃"），不要带上没改的前后文。
- 只收「听错了」的：wrong 和 right 读音相同或很接近（张伟/章炜、马米/Mami、阶越/阶跃）。换了一个人、换了一个东西、改了意思的（张伟→李明、千问→DeepSeek、明天→后天）读音不像，不算。
- 标点、语气词、语序、句式这类改动不算；拿不准就不要。
- 只输出 JSON 数组，没有就输出 []。`

// parseLearned 校验模型抽出来的词对：读音得像（soundsAlike），wrong 得真在贴入文本里、right 得真在改后文本里、wrong 在改后文本里已经不见了，都不超过 12 个字
func parseLearned(out, pasted, now string) []learnedWord {
	var raw []struct {
		Wrong string `json:"wrong"`
		Right string `json:"right"`
	}
	if err := json.Unmarshal([]byte(cleanOutput(out)), &raw); err != nil {
		return nil
	}
	var pairs []learnedWord
	for _, p := range raw {
		w, r := strings.TrimSpace(p.Wrong), strings.TrimSpace(p.Right)
		if w == "" || r == "" || w == r || utf8.RuneCountInString(w) > 12 || utf8.RuneCountInString(r) > 12 || onlyPunct(w) || onlyPunct(r) {
			continue
		}
		if !strings.Contains(pasted, w) || !strings.Contains(now, r) || strings.Contains(now, w) || !soundsAlike(w, r) {
			continue
		}
		if !slices.ContainsFunc(pairs, func(q learnedWord) bool { return q.Wrong == w && q.Right == r }) {
			pairs = append(pairs, learnedWord{Wrong: w, Right: r})
		}
	}
	return pairs
}

// soundsAlike 自动学习只收「听错了」的：两边转成不带声调的拼音（英文转小写），
// 编辑距离不超过较长一边的四分之一才算读音像（周五/周四差 2/6 也要挡住）。张伟/章炜、马米/Mami 能过，
// 张伟/李明、千问/DeepSeek 这种换了人换了东西的过不了。
// shortcut: 多音字只取默认读音、英文名的中文音译（Claude/克劳德）也过不了，宁可少学；revisit when 用户反馈该学的没学到
func soundsAlike(a, b string) bool {
	pa, pb := spell(a), spell(b)
	if pa == "" || pb == "" {
		return false
	}
	return editDistance(pa, pb)*4 <= max(len(pa), len(pb))
}

var pinyinArgs = pinyin.NewArgs()

func spell(s string) string {
	var sb strings.Builder
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Han, r):
			if p := pinyin.LazyPinyin(string(r), pinyinArgs); len(p) > 0 {
				sb.WriteString(p[0])
			}
		case r < utf8.RuneSelf && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			sb.WriteRune(unicode.ToLower(r))
		}
	}
	return sb.String()
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// hunk 一段对齐结果：same 时 del==ins，否则是原文 del 被改成了 ins
type hunk struct {
	same     bool
	del, ins string
}

// diffHunks 字级 LCS，把 a→b 切成「相同 / 改动」交替的几段，拼 ins 就是 b、拼 del 就是 a
func diffHunks(a, b string) []hunk {
	x, y := []rune(a), []rune(b)
	m, n := len(x), len(y)
	dp := make([]int32, (m+1)*(n+1))
	at := func(i, j int) *int32 { return &dp[i*(n+1)+j] }
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if x[i] == y[j] {
				*at(i, j) = *at(i+1, j+1) + 1
			} else {
				*at(i, j) = max(*at(i+1, j), *at(i, j+1))
			}
		}
	}
	var out []hunk
	var del, ins, same []rune
	flushDiff := func() {
		if len(del) > 0 || len(ins) > 0 {
			out = append(out, hunk{del: string(del), ins: string(ins)})
			del, ins = nil, nil
		}
	}
	flushSame := func() {
		if len(same) > 0 {
			out = append(out, hunk{same: true, del: string(same), ins: string(same)})
			same = nil
		}
	}
	i, j := 0, 0
	for i < m || j < n {
		switch {
		case i < m && j < n && x[i] == y[j]:
			flushDiff()
			same = append(same, x[i])
			i, j = i+1, j+1
		case i < m && (j >= n || *at(i+1, j) >= *at(i, j+1)):
			flushSame()
			del = append(del, x[i])
			i++
		default:
			flushSame()
			ins = append(ins, y[j])
			j++
		}
	}
	flushDiff()
	flushSame()
	return out
}

// guardPolish 润色拿名词表把读音像的词换成了词库里的词（同学张伟→章炜），
// 只有纠错表里有生效的「张伟→章炜」才留下，没凭据的换回原文的写法。润色的其他改动不动。
func guardPolish(source, polished string, learned []learnedWord) string {
	vocab := map[string]bool{}
	pair := map[[2]string]bool{}
	for _, w := range learned {
		vocab[w.Right] = true
		if w.Wrong != "" {
			pair[[2]string{w.Wrong, w.Right}] = true
		}
	}
	if len(vocab) == 0 {
		return polished
	}
	var sb strings.Builder
	for _, h := range diffHunks(source, polished) {
		ins := h.ins
		if !h.same && h.del != "" {
			for v := range vocab {
				if !strings.Contains(ins, v) || strings.Contains(h.del, v) {
					continue
				}
				if w := homophoneIn(h.del, v); w != "" && !pair[[2]string{w, v}] {
					for range strings.Count(ins, v) {
						ins = unswap(ins, v, w)
					}
				}
			}
		}
		sb.WriteString(ins)
	}
	return sb.String()
}

// homophoneIn 在 s 里找一段跟 v 读音像的，找不到返回空
func homophoneIn(s, v string) string {
	r := []rune(s)
	n := utf8.RuneCountInString(v)
	for size := max(1, n-2); size <= min(len(r), n+2); size++ {
		for i := 0; i+size <= len(r); i++ {
			if c := string(r[i : i+size]); soundsAlike(c, v) {
				return c
			}
		}
	}
	return ""
}

// unswap 把 ins 里的 v 换回原来的 w；w 是汉字时顺手去掉润色给英文词加的两边空格
func unswap(ins, v, w string) string {
	if r, _ := utf8.DecodeRuneInString(w); unicode.Is(unicode.Han, r) {
		ins = strings.Replace(ins, " "+v+" ", v, 1)
		ins = strings.Replace(ins, " "+v, v, 1)
		ins = strings.Replace(ins, v+" ", v, 1)
	}
	return strings.Replace(ins, v, w, 1)
}

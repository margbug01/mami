package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

// rewriteSample 一段长文口述的三个版本：识别原话 → 润色后贴入 → 你改完的样子。
// 先只攒着；以后后台从这里总结你的说话习惯（第二阶段，还没做）。
type rewriteSample struct {
	Time    string `json:"time"`
	App     string `json:"app,omitempty"`
	Raw     string `json:"raw"`     // ASR 原话
	Pasted  string `json:"pasted"`  // 润色后贴进去的
	Final   string `json:"final"`   // 你改完的
	Changed int    `json:"changed"` // 改了几个字（删+加）
}

// 短句（聊天）只抽改错的词，不存样本；这几个数以后按数据再调
const (
	sampleMinRunes = 40                   // 贴入至少这么长才存
	sampleMaxRunes = 1500                 // 再长不存：每次停手都要逐字对齐，太长了一次要几十 MB
	sampleKeep     = 500                  // 最多留这么多条，旧的先删
	sampleMaxAge   = 180 * 24 * time.Hour // 半年前的删掉
)

// sampleLog 样本存在 samples.jsonl（一行一条，只在本机，跟翻译记录一样是明文）
type sampleLog struct {
	mu   sync.Mutex
	path string
	n    int // 条数，设置页每帧要显示，别每帧读文件
}

func newSampleLog(path string) *sampleLog {
	s := &sampleLog{path: path}
	s.n = len(s.load())
	return s
}

func (s *sampleLog) load() []rewriteSample {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil
	}
	var out []rewriteSample
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var x rewriteSample
		if json.Unmarshal(sc.Bytes(), &x) == nil && x.Final != "" {
			out = append(out, x)
		}
	}
	return out
}

func (s *sampleLog) add(x rewriteSample, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keep []rewriteSample
	for _, old := range s.load() {
		if t, err := time.Parse(time.RFC3339, old.Time); err == nil && now.Sub(t) > sampleMaxAge {
			continue
		}
		keep = append(keep, old)
	}
	keep = append(keep, x)
	keep = keep[max(0, len(keep)-sampleKeep):]
	var buf bytes.Buffer
	for _, k := range keep {
		line, _ := json.Marshal(k)
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	s.n = len(keep)
	return nil
}

func (s *sampleLog) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}

func (s *sampleLog) clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.n = 0
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// sampleOf 贴入的和读到的文字对得上（没改的字至少六成）才算同一段话；
// 对不上多半是已经发出去、框里换成了别的内容。返回改后那段和改了几个字。
func sampleOf(pasted, snap string) (final string, changed int, ok bool) {
	edits, now := editsBetween(pasted, snap)
	if now == "" {
		return "", 0, false
	}
	n, del := utf8.RuneCountInString(pasted), 0
	for _, e := range edits {
		del += utf8.RuneCountInString(e.del)
		changed += utf8.RuneCountInString(e.del) + utf8.RuneCountInString(e.ins)
	}
	return now, changed, (n-del)*10 >= 6*n
}

// worthSample 长文、改过、改后还对得上原文才存
func worthSample(pasted, final string, changed int) bool {
	n := utf8.RuneCountInString(pasted)
	return n >= sampleMinRunes && n <= sampleMaxRunes && changed > 0 && final != ""
}

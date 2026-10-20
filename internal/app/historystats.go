package app

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// historyStats 是「记录」页用的统计：每天几次（热力图）、每个小时几次、常用程序、用法和耗时。
// 来源是 history/*.jsonl；开记录之前的翻译只能从日志里补次数和耗时（没有原文、程序、用法）。
type historyStats struct {
	days             map[string]int // "2006-01-02" → 次数
	hours            [24]int
	total            int
	errors           int
	fromLog          int // 其中有多少条是从日志里补的
	apps             map[string]int
	used             int // 复制过或替换过
	unused           int // 什么都没做就关了
	sumMs            int64
	nMs              int64
	sumFirst, nFirst int64
	voice            []historyEntry // 语音记录，新的在前
	loaded           time.Time
}

func newHistoryStats() *historyStats {
	return &historyStats{days: map[string]int{}, apps: map[string]int{}, loaded: time.Now()}
}

func (s *historyStats) add(t time.Time, totalMs int64, failed bool) {
	s.total++
	s.days[t.Format("2006-01-02")]++
	s.hours[t.Hour()]++
	if failed {
		s.errors++
	} else if totalMs > 0 {
		s.sumMs += totalMs
		s.nMs++
	}
}

// loadHistoryStats 读记录目录，再从日志里补上记录开始之前的翻译
func loadHistoryStats(dir string, logs []string) *historyStats {
	s := newHistoryStats()
	var earliest time.Time
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	acted := map[string]string{} // id → 最终算「用了」还是「没用」
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 64<<10), 8<<20)
		for sc.Scan() {
			var e historyEntry
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				continue
			}
			switch e.Type {
			case "translation":
				t, err := time.Parse(time.RFC3339, e.Time)
				if err != nil {
					continue
				}
				t = t.Local()
				if earliest.IsZero() || t.Before(earliest) {
					earliest = t
				}
				s.add(t, e.TotalMs, e.Error != "" && !e.Incomplete)
				if e.App != "" {
					s.apps[e.App]++
				}
				if e.FirstMs > 0 {
					s.sumFirst += e.FirstMs
					s.nFirst++
				}
			case "voice":
				e.Messages = nil
				s.voice = append(s.voice, e)
			case "action":
				if e.Action == "copy" || e.Action == "replace" {
					acted[e.ID] = "used"
				} else if acted[e.ID] == "" {
					acted[e.ID] = "unused"
				}
			}
		}
		fh.Close()
	}
	for _, v := range acted {
		if v == "used" {
			s.used++
		} else {
			s.unused++
		}
	}
	for _, l := range logs {
		s.addFromLog(l, earliest)
	}
	// RFC3339 同一时区下按字符串排就是按时间排
	sort.SliceStable(s.voice, func(i, j int) bool { return s.voice[i].Time > s.voice[j].Time })
	return s
}

// voiceFinal 贴出去的那段：润色后的文字，没润色（或旧记录）就是识别原文
func voiceFinal(e historyEntry) string {
	if e.Translation != "" {
		return e.Translation
	}
	return e.Source
}

// typingCharsPerMin 估「省下的时间」用的打字速度：中文日常打字大约每分钟 50 字
const typingCharsPerMin = 50

type voiceSummary struct {
	total, today, chars int
	savedMs             int64 // 打这些字要的时间减去说话+识别+润色的时间，每条不低于 0
	polishMs, nPolish   int64
}

func summarizeVoice(list []historyEntry, now time.Time) voiceSummary {
	var v voiceSummary
	day := now.Format("2006-01-02")
	for _, e := range list {
		n := len([]rune(strings.TrimSpace(voiceFinal(e))))
		v.total++
		v.chars += n
		if t, err := time.Parse(time.RFC3339, e.Time); err == nil && t.In(now.Location()).Format("2006-01-02") == day {
			v.today++
		}
		v.savedMs += max(0, int64(n)*60000/typingCharsPerMin-e.TotalMs)
		if e.PolishMs > 0 {
			v.polishMs += e.PolishMs
			v.nPolish++
		}
	}
	return v
}

// 2026/10/14 12:48:51.678885 translate #10 ok in 983ms
// 2026/10/14 12:48:51.678885 translate #11 failed after 30s: ...
var logTranslateRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2})\.\d+ translate #\d+ (ok in|failed after) (\S+)`)

// addFromLog 只补 before 之前的（之后的已经在记录里了）；before 为零值就全补
func (s *historyStats) addFromLog(path string, before time.Time) {
	fh, err := os.Open(path)
	if err != nil {
		return
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		m := logTranslateRe.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		t, err := time.ParseInLocation("2006/01/02 15:04:05", m[1], time.Local)
		if err != nil || (!before.IsZero() && !t.Before(before)) {
			continue
		}
		d, _ := time.ParseDuration(strings.TrimSuffix(m[3], ":"))
		s.add(t, d.Milliseconds(), m[2] == "failed after")
		s.fromLog++
	}
}

// voiceActivity 语音记录按天、按小时数次数，给「记录」页语音 tab 的热力图和时段柱子用
func voiceActivity(list []historyEntry, loc *time.Location) (days map[string]int, hours [24]int) {
	days = map[string]int{}
	for _, e := range list {
		t, err := time.Parse(time.RFC3339, e.Time)
		if err != nil {
			continue
		}
		t = t.In(loc)
		days[t.Format("2006-01-02")]++
		hours[t.Hour()]++
	}
	return days, hours
}

func (s *historyStats) maxDay() int {
	m := 0
	for _, n := range s.days {
		m = max(m, n)
	}
	return m
}

func (s *historyStats) activeDays() int { return len(s.days) }

type countItem struct {
	key string
	n   int
}

// topHours 次数最多的几个小时，次数相同按时间先后
func (s *historyStats) topHours(k int) []countItem {
	var out []countItem
	for h, n := range s.hours {
		if n > 0 {
			out = append(out, countItem{itoa2(h), n})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].n > out[j].n })
	return out[:min(k, len(out))]
}

func (s *historyStats) topApps(k int) []countItem {
	var out []countItem
	for a, n := range s.apps {
		out = append(out, countItem{a, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].n != out[j].n {
			return out[i].n > out[j].n
		}
		return out[i].key < out[j].key
	})
	return out[:min(k, len(out))]
}

func itoa2(n int) string {
	return string([]byte{byte('0' + n/10), byte('0' + n%10)})
}

package app

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func readHistory(t *testing.T, dir string) []historyEntry {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	var out []historyEntry
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			t.Fatal(err)
		}
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			var e historyEntry
			if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
				t.Fatalf("bad line %q: %v", sc.Text(), err)
			}
			out = append(out, e)
		}
		fh.Close()
	}
	return out
}

func TestHistoryRecord(t *testing.T) {
	dir := t.TempDir()
	a := &App{hist: &history{dir: dir}}
	cfg := defaultConfig()
	cfg.Model, cfg.ExtraPrompt = "hy-mt2-pro", "这是写给 AI 的提示词"
	a.cfg = cfg
	tr, _ := newTranslator(cfg)

	// 没开记录：什么都不写
	a.recordTranslation(cfg, tr, historyEntry{ID: "x"}, "你好", "English")
	if got := readHistory(t, dir); len(got) != 0 {
		t.Fatalf("history off but wrote %d lines", len(got))
	}

	cfg.History = true
	a.cfg = cfg
	id := historyID(time.Now(), 7)
	a.recordTranslation(cfg, tr, historyEntry{ID: id, Translation: "Hello", TotalMs: 900}, "你好", "English")
	a.recordAction(id, "copy")
	got := readHistory(t, dir)
	if len(got) != 2 || got[0].Type != "translation" || got[1].Type != "action" || got[1].Action != "copy" || got[1].ID != id {
		t.Fatalf("got %+v", got)
	}
	// 记下的是真正发出去的提示词：Hy-MT 模板 + 额外要求 + 原文
	msg := got[0].Messages
	if len(msg) != 1 || !strings.Contains(msg[0]["content"], "这是写给 AI 的提示词") || !strings.HasSuffix(msg[0]["content"], "你好") {
		t.Fatalf("messages %+v", msg)
	}
	if got[0].Source != "你好" || got[0].Model != "hy-mt2-pro" || got[0].Target != "English" {
		t.Fatalf("entry %+v", got[0])
	}
	if fi, _ := os.Stat(filepath.Join(dir, time.Now().Format("2006-01-02")+".jsonl")); fi == nil {
		t.Fatal("no file for today")
	}
}

func TestHistoryDismissOnlyIfUntouched(t *testing.T) {
	dir := t.TempDir()
	a := &App{hist: &history{dir: dir}}
	a.cfg = defaultConfig()
	a.cfg.History = true
	a.pop = popupState{status: statusDone, histID: "a"}
	a.noteDismiss("close")
	a.noteDismiss("dismiss") // 已经记过 close，不再记
	a.pop = popupState{status: statusDone, histID: "b", acted: true}
	a.noteDismiss("dismiss") // 复制过了，不算没用
	a.pop = popupState{status: statusError, histID: "c"}
	a.noteDismiss("dismiss") // 错误卡片不记
	got := readHistory(t, dir)
	if len(got) != 1 || got[0].ID != "a" || got[0].Action != "close" {
		t.Fatalf("got %+v", got)
	}
}

func TestHistoryPrune(t *testing.T) {
	dir := t.TempDir()
	h := &history{dir: dir}
	old := time.Now().AddDate(0, 0, -40).Format("2006-01-02") + ".jsonl"
	recent := time.Now().AddDate(0, 0, -3).Format("2006-01-02") + ".jsonl"
	for _, f := range []string{old, recent, "notes.jsonl"} {
		os.WriteFile(filepath.Join(dir, f), []byte("{}\n"), 0o600)
	}
	h.prune(0) // 永久保留
	h.prune(30)
	left, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if len(left) != 2 {
		t.Fatalf("left %v", left)
	}
	for _, f := range left {
		if filepath.Base(f) == old {
			t.Fatalf("old file not pruned: %v", left)
		}
	}
}

func TestHistoryStats(t *testing.T) {
	dir := t.TempDir()
	at := func(s string) time.Time {
		v, _ := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
		return v
	}
	write := func(e historyEntry) {
		ts, _ := time.Parse(time.RFC3339, e.Time)
		f, _ := os.OpenFile(filepath.Join(dir, ts.Local().Format("2006-01-02")+".jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		b, _ := json.Marshal(e)
		f.Write(append(b, '\n'))
		f.Close()
	}
	rfc := func(s string) string { return at(s).Format(time.RFC3339) }
	write(historyEntry{Type: "translation", ID: "1", Time: rfc("2026-10-14 13:24"), App: "Alma.exe", TotalMs: 1800, FirstMs: 1200})
	write(historyEntry{Type: "action", ID: "1", Action: "copy"})
	write(historyEntry{Type: "action", ID: "1", Action: "dismiss"}) // 复制过再关，仍算用了
	write(historyEntry{Type: "translation", ID: "2", Time: rfc("2026-10-14 21:05"), App: "Alma.exe", TotalMs: 1000})
	write(historyEntry{Type: "action", ID: "2", Action: "close"})
	write(historyEntry{Type: "translation", ID: "3", Time: rfc("2026-10-15 21:30"), App: "chrome.exe", Error: "HTTP 504"})

	// 日志：开记录之前的两条要补，之后那条已经在记录里了，不能重复算
	logf := filepath.Join(dir, "mami.log")
	os.WriteFile(logf, []byte(
		"2026/10/13 22:03:12.936265 translate #1 ok in 7.655s\n"+
			"2026/10/13 22:04:00.000000 translate #2 failed after 30s: context deadline exceeded\n"+
			"2026/10/13 22:05:00.000000 translate #3: 12 chars → English (replace=false)\n"+
			"2026/10/14 13:24:05.000000 translate #4 ok in 1.8s\n"), 0o600)

	s := loadHistoryStats(dir, []string{logf, filepath.Join(dir, "missing.log")})
	if s.total != 5 || s.fromLog != 2 || s.errors != 2 {
		t.Fatalf("total=%d fromLog=%d errors=%d", s.total, s.fromLog, s.errors)
	}
	if s.days["2026-10-13"] != 2 || s.days["2026-10-14"] != 2 || s.days["2026-10-15"] != 1 || s.maxDay() != 2 || s.activeDays() != 3 {
		t.Fatalf("days %v", s.days)
	}
	if s.hours[21] != 2 || s.hours[22] != 2 || s.hours[13] != 1 {
		t.Fatalf("hours %v", s.hours)
	}
	if top := s.topHours(1); top[0].key != "21" || top[0].n != 2 {
		t.Fatalf("topHours %v", top) // 21 和 22 一样多，取早的
	}
	if s.used != 1 || s.unused != 1 || s.apps["Alma.exe"] != 2 || s.topApps(1)[0].key != "Alma.exe" {
		t.Fatalf("used=%d unused=%d apps=%v", s.used, s.unused, s.apps)
	}
	// 平均耗时只算成功的：7.655 + 1.8 + 1.0
	if s.nMs != 3 || s.sumMs != 7655+1800+1000 || s.nFirst != 1 {
		t.Fatalf("ms %d/%d first %d", s.sumMs, s.nMs, s.nFirst)
	}
}

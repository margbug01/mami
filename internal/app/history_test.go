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

//go:build windows

package app

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestHistoryDeleteEntry(t *testing.T) {
	dir := t.TempDir()
	h := &history{dir: dir}
	now := time.Now().Format(time.RFC3339)
	h.write(historyEntry{ID: "remove", Type: "voice", Time: now, Source: "delete this"})
	h.write(historyEntry{ID: "keep", Type: "voice", Time: now, Source: "keep this"})
	h.write(historyEntry{ID: "translation", Type: "translation", Time: now, Source: "translation"})
	other := filepath.Join(dir, "2000-01-01.jsonl")
	original := "{\"type\":\"action\",\"id\":\"remove\",\"action\":\"copy\"}\ninvalid line\n"
	if err := os.WriteFile(other, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := len(loadHistoryStats(dir, nil).voice); got != 2 {
		t.Fatalf("before: %d", got)
	}
	if err := h.deleteEntry("remove"); err != nil {
		t.Fatal(err)
	}
	stats := loadHistoryStats(dir, nil)
	if len(stats.voice) != 1 || stats.voice[0].ID != "keep" || stats.total != 1 {
		t.Fatalf("after: voice=%v translations=%d", stats.voice, stats.total)
	}
	data, err := os.ReadFile(other)
	if err != nil || string(data) != "invalid line\n" {
		t.Fatalf("must preserve malformed lines: %q, %v", data, err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil || strings.Contains(string(data), "delete this") || strings.Contains(string(data), "\"remove\"") {
			t.Fatalf("deleted content remains in %s", path)
		}
	}
	if err := h.deleteEntry("remove"); err != nil {
		t.Fatal(err)
	}
	if err := h.deleteEntry("keep"); err != nil || len(loadHistoryStats(dir, nil).voice) != 0 {
		t.Fatalf("last voice deletion: %v", err)
	}
	if err := h.deleteEntry(""); err == nil {
		t.Fatal("empty ID must be rejected")
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, ".history-delete-*"))
	if len(leftovers) != 0 {
		t.Fatal("temporary files left behind")
	}
}

func TestHistoryDeleteDeadline(t *testing.T) {
	h := &history{dir: t.TempDir()}
	h.write(historyEntry{ID: "expire", Type: "voice", Time: time.Now().Format(time.RFC3339)})
	h.queueDelete("expire", 10*time.Millisecond)
	deadline := time.Now().Add(2 * time.Second)
	for {
		pending, revision, failed := h.deletionState()
		if len(pending) == 0 {
			if revision != 1 || failed || len(loadHistoryStats(h.dir, nil).voice) != 0 {
				t.Fatal("deadline did not remove record")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timer did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	h.undoDeletes()
	if len(loadHistoryStats(h.dir, nil).voice) != 0 {
		t.Fatal("expired delete must not be undone")
	}
}

func TestHistoryDeleteUndo(t *testing.T) {
	a := &App{hist: &history{dir: t.TempDir()}}
	now := time.Now()
	entry := historyEntry{ID: "sample", Type: "voice", Time: now.Format(time.RFC3339), Source: "通知晓明，明天三点开会。"}
	a.hist.write(entry)
	defer a.hist.undoDeletes()
	a.set = settingsState{draft: defaultConfig(), page: pageHist}
	tt := ui.NewTester(a.settingsView, settingsW, 1000)
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	pending, _, failed := a.hist.deletionState()
	if !pending[entry.ID] || failed || len(loadHistoryStats(a.hist.dir, nil).voice) != 1 {
		t.Fatal("deletion must wait for undo deadline")
	}
	if _, ok := tt.Find(entry.Source); ok {
		t.Fatal("pending card still visible")
	}
	if path := os.Getenv("MAMI_DELETE_SNAP"); path != "" {
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, tt.Image()); err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
	if err := tt.Click("撤销"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	pending, _, _ = a.hist.deletionState()
	if len(pending) != 0 || len(loadHistoryStats(a.hist.dir, nil).voice) != 1 {
		t.Fatal("undo must preserve record")
	}
	if _, ok := tt.Find(entry.Source); !ok {
		t.Fatal("undo did not restore card")
	}
	if path := os.Getenv("MAMI_DELETE_SNAP"); path != "" {
		card := ui.NewTester(func(c *ui.Context) { a.voiceCard(c, entry, now) }, 620, 120)
		card.Move(500, 20)
		card.Frame()
		file, err := os.Create(path + ".card.png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(file, card.Image()); err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
}

func TestHistoryDeleteUndoBatch(t *testing.T) {
	h := &history{dir: t.TempDir()}
	for _, id := range []string{"first", "second"} {
		h.write(historyEntry{ID: id, Type: "voice", Time: time.Now().Format(time.RFC3339)})
		h.queueDelete(id, 20*time.Millisecond)
	}
	h.undoDeletes()
	time.Sleep(40 * time.Millisecond)
	pending, revision, failed := h.deletionState()
	if len(pending) != 0 || revision != 0 || failed || len(loadHistoryStats(h.dir, nil).voice) != 2 {
		t.Fatal("undo did not cancel every pending deletion")
	}
}

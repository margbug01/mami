package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

func (h *history) deleteEntry(id string) error {
	if h == nil || h.dir == "" || id == "" {
		return errors.New("missing history entry")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.deleteEntryLocked(id)
}

func (h *history) deleteEntryLocked(id string) error {
	files, err := filepath.Glob(filepath.Join(h.dir, "*.jsonl"))
	if err != nil {
		return err
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var kept bytes.Buffer
		removed := false
		for _, line := range bytes.SplitAfter(data, []byte{'\n'}) {
			var entry historyEntry
			if json.Unmarshal(bytes.TrimSpace(line), &entry) == nil && entry.ID == id {
				removed = true
				continue
			}
			kept.Write(line)
		}
		if !removed {
			continue
		}
		if err := replaceHistoryFile(path, kept.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func (h *history) queueDelete(id string, delay time.Duration) {
	if h == nil || h.dir == "" || id == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.pendingDeletes == nil {
		h.pendingDeletes = make(map[string]*time.Timer)
	}
	if _, exists := h.pendingDeletes[id]; exists {
		return
	}
	h.deleteFailed = false
	var timer *time.Timer
	timer = time.AfterFunc(delay, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if h.pendingDeletes[id] != timer {
			return
		}
		h.deleteFailed = h.deleteEntryLocked(id) != nil || h.deleteFailed
		delete(h.pendingDeletes, id)
		h.deleteRevision++
	})
	h.pendingDeletes[id] = timer
}

func (h *history) undoDeletes() {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, timer := range h.pendingDeletes {
		timer.Stop()
		delete(h.pendingDeletes, id)
	}
}

func (h *history) deletionState() (map[string]bool, uint64, bool) {
	pending := make(map[string]bool)
	if h == nil {
		return pending, 0, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for id := range h.pendingDeletes {
		pending[id] = true
	}
	return pending, h.deleteRevision, h.deleteFailed
}

func replaceHistoryFile(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".history-delete-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), path)
}

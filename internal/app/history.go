package app

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// 翻译记录：每次翻译写一行 JSON 到 history/2026-10-14.jsonl（按本地日期分文件）。
// 两种行：
//   {"type":"translation", ...}  翻译本身：原文、发给模型的完整 messages、译文、耗时、出错
//   {"type":"action","id":...}   之后对这条译文做了什么：copy / replace / close / dismiss
// 用户之后可以把这些文件交给 AI，看哪些提示词被模型执行了而不是翻译、额外要求有没有被遵守。

type historyEntry struct {
	PolishStyle string              `json:"polish_style,omitempty"`
	PolishMs    int64               `json:"polish_ms,omitempty"`
	Type        string              `json:"type"`
	ID          string              `json:"id"`
	Time        string              `json:"time"`
	App         string              `json:"app,omitempty"`
	Model       string              `json:"model,omitempty"`
	BaseURL     string              `json:"base_url,omitempty"`
	Target      string              `json:"target,omitempty"`
	Mode        string              `json:"mode,omitempty"` // card / replace（快捷键或按钮直接替换）
	Source      string              `json:"source,omitempty"`
	Translation string              `json:"translation,omitempty"`
	Messages    []map[string]string `json:"messages,omitempty"`
	FirstMs     int64               `json:"first_ms,omitempty"` // 收到第一段译文用了多久（流式）
	TotalMs     int64               `json:"total_ms,omitempty"`
	Error       string              `json:"error,omitempty"`
	Incomplete  bool                `json:"incomplete,omitempty"` // 连接中途断了，只有一半译文
	Action      string              `json:"action,omitempty"`
}

type history struct {
	mu  sync.Mutex
	dir string
}

func (h *history) write(e historyEntry) {
	if h == nil || h.dir == "" {
		return
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if err := os.MkdirAll(h.dir, 0o700); err != nil {
		log.Println("history:", err)
		return
	}
	name := filepath.Join(h.dir, time.Now().Format("2006-01-02")+".jsonl")
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		log.Println("history:", err)
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

// prune 删掉 days 天以前的记录文件；days<=0 表示永久保留
func (h *history) prune(days int) {
	if h == nil || h.dir == "" || days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	files, _ := filepath.Glob(filepath.Join(h.dir, "*.jsonl"))
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, f := range files {
		day := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		if len(day) == 10 && day < cutoff {
			if err := os.Remove(f); err == nil {
				log.Printf("history: removed %s (older than %d days)", filepath.Base(f), days)
			}
		}
	}
}

func historyID(t time.Time, seq int64) string {
	return t.Format("20060102-150405") + "-" + strconv.FormatInt(seq, 10)
}

// pruneLoop 启动时清一次过期记录，之后每 6 小时一次
func (a *App) pruneLoop() {
	for {
		cfg, _ := a.config()
		a.hist.prune(cfg.HistoryDays)
		time.Sleep(6 * time.Hour)
	}
}

func (a *App) openHistory() {
	os.MkdirAll(a.hist.dir, 0o700)
	mygo.Shell.OpenPath(a.hist.dir)
}

// recordTranslation 在翻译 goroutine 里调用
func (a *App) recordTranslation(cfg Config, tr *Translator, e historyEntry, text, target string) {
	if !cfg.History {
		return
	}
	e.Type = "translation"
	e.Model, e.BaseURL, e.Target, e.Source = cfg.Model, cfg.BaseURL, target, text
	e.Messages = tr.Messages(text, target)
	a.hist.write(e)
}

// recordAction 记下对某条译文做了什么。copy/replace 每次都记；close/dismiss 只在什么都没做就关掉时记（见 popupState.acted）
func (a *App) recordAction(id, action string) {
	if id == "" {
		return
	}
	if cfg, _ := a.config(); !cfg.History {
		return
	}
	a.hist.write(historyEntry{Type: "action", ID: id, Time: time.Now().Format(time.RFC3339), Action: action})
}

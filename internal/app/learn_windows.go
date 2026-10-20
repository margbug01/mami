//go:build windows

package app

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// 贴入之后盯一会儿：用户在同一个窗口里打字、停下 0.8 秒，就读光标附近的文字和贴入的比；
// 按了 Enter 马上比；切走窗口或 30 秒后不盯了。一次口述最多问模型一次。
type learnWatch struct {
	mu       sync.Mutex
	gen      uint64
	fg       uintptr // 0 = 没在盯
	pasted   string
	cfg      Config
	deadline time.Time
	debounce *time.Timer
}

func (a *App) learnStart(fg uintptr, pasted string, cfg Config) {
	if a.learn == nil || !cfg.VoiceLearn || cfg.PolishAPIKey == "" {
		return
	}
	w := &a.learnW
	w.mu.Lock()
	defer w.mu.Unlock()
	w.gen++
	w.fg, w.pasted, w.cfg, w.deadline = fg, pasted, cfg, time.Now().Add(30*time.Second)
	if w.debounce != nil {
		w.debounce.Stop()
	}
}

// learnTyped 钩子线程报来的真实按键（onMouse 里调用）
func (a *App) learnTyped(vk uint32) {
	w := &a.learnW
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.fg == 0 {
		return
	}
	if time.Now().After(w.deadline) || foreground() != w.fg {
		w.fg = 0
		return
	}
	gen := w.gen
	if w.debounce != nil {
		w.debounce.Stop()
	}
	if vk == vkReturn {
		go a.learnCheck(gen)
		return
	}
	w.debounce = time.AfterFunc(800*time.Millisecond, func() { a.learnCheck(gen) })
}

func (a *App) learnCheck(gen uint64) {
	w := &a.learnW
	w.mu.Lock()
	if w.gen != gen || w.fg == 0 {
		w.mu.Unlock()
		return
	}
	fg, pasted, cfg := w.fg, w.pasted, w.cfg
	w.mu.Unlock()
	if foreground() != fg {
		return
	}
	n := utf8.RuneCountInString(pasted)
	before, after, ok := caretText(min(4000, 2*n+200), min(2000, n+200))
	if !ok {
		return
	}
	edits, now := editsBetween(pasted, before+after)
	if !worthAsking(pasted, edits) {
		return
	}
	w.mu.Lock()
	if w.gen != gen {
		w.mu.Unlock()
		return
	}
	w.fg = 0 // 这次口述只问一次
	w.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), polishMaxWait)
	defer cancel()
	out, err := chatOnce(ctx, cfg, learnPrompt, "贴入：\n"+pasted+"\n\n改后：\n"+now)
	if err != nil {
		reason, _, _ := strings.Cut(err.Error(), ":") // 正文可能回显原文，只记状态
		log.Printf("learn: %s", reason)
		return
	}
	pairs := parseLearned(out, pasted, now)
	if len(pairs) == 0 {
		log.Printf("learn: %d edit(s), nothing usable", len(edits))
		return
	}
	a.learn.observe(pairs)
	log.Printf("learn: %s", learnedTable(pairs))
}

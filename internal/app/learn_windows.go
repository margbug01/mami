//go:build windows

package app

import (
	"context"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// 贴入之后盯着：用户在同一个窗口里打字，每停手 0.3 秒就读一次光标附近的文字存成快照。
// 改词：停手 2 秒或按 Enter 时拿快照和贴入的比，一次口述最多问模型一次。Enter 不能当场读：
// 聊天框里 Enter 就是发送，读到时框已经清空了，所以用按下前那张。
// 长文样本：一直盯到切走窗口、停手 60 秒或满 10 分钟，存最后一张还对得上贴入内容的快照
// （发出去以后框空了或换了内容就对不上，自然停在发送前那一刻）。
type learnWatch struct {
	mu       sync.Mutex
	gen      uint64
	fg       uintptr // 0 = 没在盯
	raw      string  // 识别原话（润色前）
	pasted   string
	snap     string // 最近一次读到的光标附近文字（还没读到时为空）
	best     string // 最近一张还对得上贴入内容的快照
	asked    bool   // 这次口述已经问过模型改词了
	cfg      Config
	deadline time.Time // 停手超过这个时间就收尾
	hardEnd  time.Time
	snapT    *time.Timer
	checkT   *time.Timer
	endT     *time.Timer
}

const (
	learnSnapAfter  = 300 * time.Millisecond
	learnCheckAfter = 2 * time.Second
	learnIdleEnd    = 60 * time.Second
	learnMaxWatch   = 10 * time.Minute // 写长 prompt 可能改好几分钟
)

func (a *App) learnStart(fg uintptr, raw, pasted string, cfg Config) {
	if a.learn == nil || !cfg.VoiceLearn {
		return
	}
	w := &a.learnW
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopTimers()
	w.gen++
	now := time.Now()
	w.fg, w.raw, w.pasted, w.snap, w.best, w.asked, w.cfg = fg, raw, pasted, "", "", false, cfg
	w.deadline, w.hardEnd = now.Add(learnIdleEnd), now.Add(learnMaxWatch)
	gen := w.gen
	w.endT = time.AfterFunc(learnIdleEnd, func() { a.learnFinish(gen) })
}

func (w *learnWatch) stopTimers() {
	for _, t := range []*time.Timer{w.snapT, w.checkT, w.endT} {
		if t != nil {
			t.Stop()
		}
	}
}

// learnTyped 钩子线程报来的真实按键（onMouse 里调用）
func (a *App) learnTyped(vk uint32) {
	w := &a.learnW
	w.mu.Lock()
	if w.fg == 0 {
		w.mu.Unlock()
		return
	}
	gen := w.gen
	now := time.Now()
	if now.After(w.deadline) || now.After(w.hardEnd) || foreground() != w.fg {
		w.mu.Unlock()
		go a.learnFinish(gen) // 切走了：用之前的快照收尾
		return
	}
	w.stopTimers()
	w.deadline = now.Add(learnIdleEnd)
	w.endT = time.AfterFunc(min(learnIdleEnd, time.Until(w.hardEnd)), func() { a.learnFinish(gen) })
	if vk == vkReturn {
		snap := w.snap
		w.mu.Unlock()
		go a.learnJudge(gen, snap) // 用按 Enter 之前那张快照；长文还接着盯（Enter 也可能只是换行）
		return
	}
	w.snapT = time.AfterFunc(learnSnapAfter, func() { a.learnSnap(gen) })
	w.checkT = time.AfterFunc(learnCheckAfter, func() {
		w.mu.Lock()
		snap := w.snap
		w.mu.Unlock()
		a.learnJudge(gen, snap)
	})
	w.mu.Unlock()
}

// learnSnap 读一次光标附近的文字存起来
func (a *App) learnSnap(gen uint64) {
	w := &a.learnW
	w.mu.Lock()
	if w.gen != gen || w.fg == 0 {
		w.mu.Unlock()
		return
	}
	fg, pasted := w.fg, w.pasted
	n := utf8.RuneCountInString(pasted)
	w.mu.Unlock()
	if foreground() != fg {
		return
	}
	before, after, ok := caretText(min(8000, 2*n+400), min(4000, n+400))
	if !ok {
		log.Printf("learn: can't read text at caret")
		return
	}
	snap := before + after
	same := false
	if n >= sampleMinRunes && n <= sampleMaxRunes {
		_, _, same = sampleOf(pasted, snap)
	}
	w.mu.Lock()
	if w.gen == gen {
		w.snap = snap
		if same {
			w.best = snap
		}
	}
	w.mu.Unlock()
}

// learnJudge 拿快照和贴入的比，是改词就问模型（一次口述只问一次）
func (a *App) learnJudge(gen uint64, snap string) {
	w := &a.learnW
	w.mu.Lock()
	if w.gen != gen || w.fg == 0 || w.asked || snap == "" {
		w.mu.Unlock()
		return
	}
	pasted, cfg := w.pasted, w.cfg
	edits, now := editsBetween(pasted, snap)
	if !worthAsking(pasted, edits) {
		w.mu.Unlock()
		log.Printf("learn: %d edit(s), not a word fix", len(edits))
		return
	}
	w.asked = true
	w.mu.Unlock()
	a.learnWords(cfg, pasted, now, len(edits))
}

func (a *App) learnWords(cfg Config, pasted, now string, nEdits int) {
	if cfg.PolishAPIKey == "" {
		log.Printf("learn: off (no polish key)")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), polishMaxWait)
	defer cancel()
	out, err := chatOnce(ctx, cfg, learnPrompt, "贴入：\n"+pasted+"\n\n改后：\n"+now)
	if err != nil {
		reason, _, _ := strings.Cut(err.Error(), ":") // 正文可能回显原文，只记状态
		log.Printf("learn: %s", reason)
		return
	}
	pairs, drops := parseLearnedWhy(out, pasted, now)
	if len(pairs) == 0 {
		log.Printf("learn: %d edit(s), nothing usable (model gave %d; dropped: %s)", nEdits, len(drops), strings.Join(drops, ", "))
		return
	}
	a.learn.observe(pairs)
	log.Printf("learn: %s", learnedTable(pairs))
}

// learnFinish 不盯了：长文改过就存一条样本
func (a *App) learnFinish(gen uint64) {
	w := &a.learnW
	w.mu.Lock()
	if w.gen != gen || w.fg == 0 {
		w.mu.Unlock()
		return
	}
	fg, raw, pasted, best := w.fg, w.raw, w.pasted, w.best
	w.fg = 0
	w.stopTimers()
	w.mu.Unlock()
	if a.samples == nil || best == "" {
		return
	}
	final, changed, _ := sampleOf(pasted, best)
	if !worthSample(pasted, final, changed) {
		return
	}
	x := rewriteSample{Time: time.Now().Format(time.RFC3339), App: filepath.Base(processName(windowPID(fg))), Raw: raw, Pasted: pasted, Final: final, Changed: changed}
	if err := a.samples.add(x, time.Now()); err != nil {
		log.Printf("samples: %v", err)
		return
	}
	log.Printf("samples: kept one (%d chars, %d changed)", utf8.RuneCountInString(pasted), changed)
}

package app

import (
	"log"
	"strings"
	"time"
)

// 抓选区、贴文字的流程两个平台一样，只是底下的剪贴板和模拟按键各自实现
// （clipboard.go / worker.go 是 Windows，clipboard_darwin.go 是 macOS）。

// paste 用剪贴板 + 粘贴键（一般是 Ctrl+V，见 sendPaste）把文字贴进当前焦点，0.7 秒后把原来的剪贴板还回去
// （期间要是别人又往剪贴板里写了东西，就不还原，免得覆盖掉）。
func (w *worker) paste(text string, allow ...func(owner uintptr) bool) {
	w.do(func(owner uintptr) {
		snap, ok := snapshotClipboard(owner)
		if len(allow) > 0 && !allow[0](owner) {
			return
		}
		if !setClipboardText(owner, text, true) {
			return
		}
		sendPaste()
		if !ok {
			return
		}
		seq := clipSeq()
		time.AfterFunc(700*time.Millisecond, func() {
			w.do(func(owner uintptr) {
				if clipSeq() == seq {
					restoreClipboard(owner, snap)
				}
			})
		})
	})
}

func waitClipChange(seq uint32, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if clipSeq() != seq {
			pumpSleep(20 * time.Millisecond) // 有的程序分几次写入多种格式
			return true
		}
		pumpSleep(10 * time.Millisecond)
	}
	// 卡顿时一次循环就能睡过 deadline：出门前再看一眼，晚到的复制也算
	if clipSeq() != seq {
		pumpSleep(20 * time.Millisecond)
		return true
	}
	return false
}

// waitModifiersUp 等用户松开快捷键里的修饰键，不然模拟的 Ctrl+C 会变成 Ctrl+Alt+C
func waitModifiersUp(timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !keyDown(VK_SHIFT) && !keyDown(VK_CONTROL) && !keyDown(VK_MENU) && !keyDown(VK_LWIN) && !keyDown(VK_RWIN) {
			return
		}
		pumpSleep(10 * time.Millisecond)
	}
}

// grabSelection 用模拟 Ctrl+C 取当前选中的文字，取完把剪贴板原样还回去。
// 没选中时只在确实是输入框里才全选：焦点在聊天记录、网页正文上按 Ctrl+A 会把整页几千字都选上。
// noInput：想全选但焦点不是输入框，所以没全选
func grabSelection(owner uintptr, selectAllIfEmpty bool) (text string, selectedAll, noInput bool) {
	snap, _ := snapshotClipboard(owner)
	snapSeq := clipSeq()
	changed := false

	seq := clipSeq()
	start := time.Now()
	var in inputState
	if selectAllIfEmpty {
		in = focusedInput()
		// UIA 说不清的老式程序/Qt（微信这类）：有系统输入光标也算输入框
		in.input = in.input || systemCaret(foreground())
	}
	if selectAllIfEmpty && in.input && in.empty {
		// 输入框明确没选中：Ctrl+C 不会改剪贴板，等它就是白等 600ms，直接走下面的全选
		log.Printf("grab: nothing selected (uia, %v), select all", time.Since(start).Round(time.Millisecond))
	} else {
		sendCtrl('C')
		sent := time.Since(start)
		if waitClipChange(seq, 600*time.Millisecond) {
			changed = true
			text = getClipboardText(owner)
		}
		log.Printf("grab: ctrl+c changed=%v after %v (sendinput %v), %d chars, seq %d→%d", changed, time.Since(start).Round(time.Millisecond), sent.Round(time.Millisecond), len([]rune(text)), seq, clipSeq())
	}
	if strings.TrimSpace(text) == "" && selectAllIfEmpty {
		if !in.input {
			log.Printf("grab: nothing selected and focus is not an input box, not selecting all")
			noInput = true
		} else {
			sendCtrl('A')
			selectedAll = true
			pumpSleep(40 * time.Millisecond)
			seq = clipSeq()
			sendCtrl('C')
			if waitClipChange(seq, 600*time.Millisecond) {
				changed = true
				text = getClipboardText(owner)
			}
			log.Printf("grab: select all → %d chars", len([]rune(text)))
		}
	}
	// 超时之后剪贴板才被写（程序卡了一下）也得还原，不然用户原来的剪贴板就被顶掉了
	if changed || clipSeq() != snapSeq {
		restoreClipboard(owner, snap)
	}
	return text, selectedAll, noInput
}

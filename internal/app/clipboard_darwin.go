//go:build darwin

package app

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/purego/objc"
)

// macOS 的剪贴板（NSPasteboard）和模拟按键。剪贴板不需要 owner 窗口，
// worker 就是一条按顺序干活的 goroutine；owner 参数两边签名一致，这里恒为 0。

type worker struct {
	mu   sync.Mutex
	jobs []func(owner uintptr)
	wake chan struct{}
}

var theWorker *worker

func startWorker() (*worker, error) {
	w := &worker{wake: make(chan struct{}, 1)}
	go func() {
		for range w.wake {
			w.drain()
		}
	}()
	theWorker = w
	return w, nil
}

func (w *worker) drain() {
	for {
		w.mu.Lock()
		if len(w.jobs) == 0 {
			w.mu.Unlock()
			return
		}
		job := w.jobs[0]
		w.jobs = w.jobs[1:]
		w.mu.Unlock()
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("worker job panic: %v", r)
				}
			}()
			job(0)
		}()
	}
}

// do 把 job 排到 worker 上执行，不等它完成
func (w *worker) do(job func(owner uintptr)) {
	w.mu.Lock()
	w.jobs = append(w.jobs, job)
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// clipItem 剪贴板里的一项（一个 NSPasteboardItem 的全部类型和数据）
type clipItem struct {
	types []string
	data  [][]byte
}

const (
	pbString    = "public.utf8-plain-text"
	pbTransient = "org.nspasteboard.TransientType" // 剪贴板管理工具约定：带这个类型的内容不进历史
)

func pasteboard() objc.ID { return send(class("NSPasteboard"), "generalPasteboard") }

func clipSeq() uint32 {
	var n uint32
	withPool(func() { n = uint32(send(pasteboard(), "changeCount")) })
	return n
}

func pumpSleep(d time.Duration) { time.Sleep(d) }

func snapshotClipboard(uintptr) ([]clipItem, bool) {
	var items []clipItem
	withPool(func() {
		arr := send(pasteboard(), "pasteboardItems")
		for i := 0; arr != 0 && i < int(send(arr, "count")); i++ {
			pi := send(arr, "objectAtIndex:", uintptr(i))
			types := send(pi, "types")
			var it clipItem
			for j := 0; j < int(send(types, "count")); j++ {
				t := send(types, "objectAtIndex:", uintptr(j))
				d := send(pi, "dataForType:", uintptr(t))
				if d == 0 || send(d, "length") > 64<<20 {
					continue
				}
				it.types = append(it.types, goString(t))
				it.data = append(it.data, goBytes(d))
			}
			if len(it.types) > 0 {
				items = append(items, it)
			}
		}
	})
	return items, true
}

// writeItems 清空剪贴板写入 items；transient 时给第一项加上「不进历史」的标记
func writeItems(items []clipItem, transient bool) bool {
	ok := false
	withPool(func() {
		pb := pasteboard()
		send(pb, "clearContents")
		arr := send(class("NSMutableArray"), "array")
		for i, it := range items {
			pi := send(send(send(class("NSPasteboardItem"), "alloc"), "init"), "autorelease")
			for j, t := range it.types {
				send(pi, "setData:forType:", uintptr(nsData(it.data[j])), uintptr(nsString(t)))
			}
			if transient && i == 0 {
				send(pi, "setData:forType:", uintptr(nsData(nil)), uintptr(nsString(pbTransient)))
			}
			send(arr, "addObject:", uintptr(pi))
		}
		ok = len(items) == 0 || sendBool(pb, "writeObjects:", uintptr(arr))
	})
	return ok
}

func restoreClipboard(_ uintptr, items []clipItem) { writeItems(items, true) }

// setClipboardText 写入文本；temp=true 时不进剪贴板管理工具的历史
func setClipboardText(_ uintptr, s string, temp bool) bool {
	s = strings.ReplaceAll(s, "\x00", "")
	return writeItems([]clipItem{{types: []string{pbString}, data: [][]byte{[]byte(s)}}}, temp)
}

func getClipboardText(uintptr) string {
	var s string
	withPool(func() { s = goString(send(pasteboard(), "stringForType:", uintptr(nsString(pbString)))) })
	return s
}

// sendCtrl macOS 上是 ⌘+键（复制、粘贴、全选、撤销）。
// shortcut: 按 ANSI 键位发键码，Dvorak 这类布局下 ⌘C 会变成别的键；revisit when 有用户用非 QWERTY 布局
func sendCtrl(key uint16) {
	if code, ok := vkToMac[uint32(key)]; ok {
		postKey(code, flagCmd)
	}
}

// sendPaste macOS 上的终端、聊天软件都认 ⌘V
func sendPaste() { sendCtrl('V') }

// collapseSelection 按一下 →：全选留下的选区收到末尾
func collapseSelection() { postKey(vkToMac[0x27], 0) }

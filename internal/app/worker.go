//go:build windows

package app

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// worker 是一条自带消息窗口的 OS 线程，专门做「碰别的程序」的事：
// 模拟 Ctrl+C / Ctrl+V、快照和还原剪贴板。剪贴板必须有一个 owner 窗口，
// 而且这个窗口所在线程得一直处理消息（还原剪贴板后别人复制时，系统会同步
// 给 owner 发 WM_DESTROYCLIPBOARD），所以不能放在 MyGo 的 UI 线程上卡着。
type worker struct {
	hwnd uintptr
	mu   sync.Mutex
	jobs []func(owner uintptr)
}

const wmWorkerJob = WM_APP + 1

var theWorker *worker

func startWorker() (*worker, error) {
	w := &worker{}
	ready := make(chan error)
	go func() {
		runtime.LockOSThread()
		inst, _, _ := pGetModuleHandleW.Call(0)
		wc := WNDCLASSEXW{
			CbSize:        uint32(unsafe.Sizeof(WNDCLASSEXW{})),
			LpfnWndProc:   syscall.NewCallback(workerProc),
			HInstance:     inst,
			LpszClassName: u16("MamiWorker"),
		}
		if r, _, e := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
			ready <- fmt.Errorf("RegisterClassEx: %v", e)
			return
		}
		h, _, e := pCreateWindowExW.Call(0, uintptr(unsafe.Pointer(u16("MamiWorker"))), 0, 0, 0, 0, 0, 0, HWND_MESSAGE, 0, inst, 0)
		if h == 0 {
			ready <- fmt.Errorf("CreateWindowEx: %v", e)
			return
		}
		w.hwnd = h
		theWorker = w
		ready <- nil
		var m MSG
		for {
			r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	return w, <-ready
}

func workerProc(hwnd, msg, wp, lp uintptr) uintptr {
	if msg == wmWorkerJob && theWorker != nil {
		theWorker.drain()
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
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
			job(w.hwnd)
		}()
	}
}

// do 把 job 排到 worker 线程上执行，不等它完成
func (w *worker) do(job func(owner uintptr)) {
	w.mu.Lock()
	w.jobs = append(w.jobs, job)
	w.mu.Unlock()
	pPostMessageW.Call(w.hwnd, wmWorkerJob, 0, 0)
}

// paste 用剪贴板 + 粘贴键（一般是 Ctrl+V，见 sendPaste）把文字贴进当前焦点，0.7 秒后把原来的剪贴板还回去
// （期间要是别人又往剪贴板里写了东西，就不还原，免得覆盖掉）。
func (w *worker) paste(text string, allow ...func(owner uintptr) bool) {
	w.do(func(owner uintptr) {
		snap, ok := snapshotClipboard(owner)
		if len(allow) > 0 && !allow[0](owner) {
			return
		}
		if !setClipboardText(owner, toCRLF(text), true) {
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

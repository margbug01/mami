//go:build windows

package app

import (
	"log"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"
)

// 闲下来就把内存还给系统。Mami 大部分时间只是挂着等热键，用过的内存却不会自己回去：
//   - Go 用剩的堆不急着还（一次口述、开一次设置、剪贴板里有张截图时粘贴一次，都会把堆顶上去）；
//     闲了 30 秒就 FreeOSMemory，把空着的堆还回去
//   - 显卡驱动、渲染这些原生内存的页大多用一下就不碰了；闲了 3 分钟让 Windows 把它们换出工作集
//     （任务管理器里的数字掉到二三十 MB；下次用时从内存里捡回来，几毫秒，不碰磁盘）
// 另外内存比上次记的多了 64MB 就在日志里记一行，以后再涨能对上是什么时候、涨的是 Go 还是原生

const (
	memReleaseIdle = 30 * time.Second
	memTrimIdle    = 3 * time.Minute
	memLogStep     = 64 << 20
)

var (
	lastBusy                  atomic.Int64 // 最近一次有东西在屏幕上动（UnixNano）
	pGetCurrentProcess        = kernel32.NewProc("GetCurrentProcess")
	pSetProcessWorkingSetSize = kernel32.NewProc("SetProcessWorkingSetSize")
	pK32GetProcessMemoryInfo  = kernel32.NewProc("K32GetProcessMemoryInfo")
)

// markBusy 胶囊、设置、翻译卡片这些出现或变化时调：从这一刻起重新算闲了多久（任意线程）
func markBusy() { lastBusy.Store(time.Now().UnixNano()) }

// processMemoryCounters PROCESS_MEMORY_COUNTERS_EX
type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
	privateUsage               uintptr
}

func processMemory() (private, workingSet uintptr) {
	proc, _, _ := pGetCurrentProcess.Call()
	c := processMemoryCounters{cb: uint32(unsafe.Sizeof(processMemoryCounters{}))}
	if r, _, _ := pK32GetProcessMemoryInfo.Call(proc, uintptr(unsafe.Pointer(&c)), uintptr(c.cb)); r == 0 {
		return 0, 0
	}
	return c.privateUsage, c.workingSetSize
}

// memoryLoop 每 30 秒看一眼：闲够了就还内存，涨多了就记日志
func (a *App) memoryLoop() {
	markBusy()
	var released, trimmed int64 // 已经为哪一次 busy 还过 / 换出过
	var base uintptr            // 上次记日志时的内存（之后掉下去就跟着往下，涨回来 64MB 再记）
	for range time.Tick(30 * time.Second) {
		busy := lastBusy.Load()
		idle := time.Since(time.Unix(0, busy))
		if busy != released && idle >= memReleaseIdle {
			released = busy
			debug.FreeOSMemory()
		}
		if busy != trimmed && idle >= memTrimIdle && a.setHwnd.Load() == 0 { // 设置开着时人可能正在点，别让它卡一下
			trimmed = busy
			proc, _, _ := pGetCurrentProcess.Call()
			pSetProcessWorkingSetSize.Call(proc, ^uintptr(0), ^uintptr(0)) // (-1, -1)：把工作集清空，页留在内存里随用随取
		}
		private, ws := processMemory()
		switch {
		case private == 0:
		case base == 0 || private >= base+memLogStep:
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			log.Printf("mem: %dMB private, %dMB working set (go heap %dMB in use, %dMB held)",
				private>>20, ws>>20, ms.HeapInuse>>20, (ms.HeapSys-ms.HeapReleased)>>20)
			base = private
		case private < base:
			base = private
		}
	}
}

// settingsClosed 设置窗口关了：只给设置页用的东西放掉——预览和检查用的那几份脚本、样式列表、
// 没在用的流光配色纹理。浮窗自己的那份脚本和当前配色留着，下次说话不用重新准备（界面线程）
func (a *App) settingsClosed(cfg Config) {
	w := &a.capWin
	for key := range w.styles {
		if !strings.HasSuffix(key, "/") { // "文件夹/" 是浮窗那份
			delete(w.styles, key)
		}
	}
	w.list, w.listAt = nil, time.Time{}
	keep := flowPaletteIndex(cfg.VoiceFlowPalette)
	for i := range flowViews {
		if i != keep {
			flowViews[i] = [2]flowRenderer{}
		}
	}
	customOpenSkill = ""
	markBusy()
}

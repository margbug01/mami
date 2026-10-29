//go:build windows

package app

import "unsafe"

var (
	pGetCurrentProcess        = kernel32.NewProc("GetCurrentProcess")
	pSetProcessWorkingSetSize = kernel32.NewProc("SetProcessWorkingSetSize")
	pK32GetProcessMemoryInfo  = kernel32.NewProc("K32GetProcessMemoryInfo")
)

// memLabels processMemory 两个数在日志里的叫法
var memLabels = [2]string{"private", "working set"}

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

// trimWorkingSet 把工作集清空，页留在内存里随用随取
func trimWorkingSet() {
	proc, _, _ := pGetCurrentProcess.Call()
	pSetProcessWorkingSetSize.Call(proc, ^uintptr(0), ^uintptr(0)) // (-1, -1)
}

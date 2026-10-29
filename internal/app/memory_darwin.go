//go:build darwin

package app

import (
	"os"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// memLabels processMemory 两个数在日志里的叫法：phys_footprint 就是活动监视器里的「内存」
var memLabels = [2]string{"footprint", "resident"}

// rusageInfoV0 struct rusage_info_v0（<sys/resource.h>）
type rusageInfoV0 struct {
	uuid           [16]byte
	userTime       uint64
	systemTime     uint64
	pkgIdleWkups   uint64
	interruptWkups uint64
	pageins        uint64
	wiredSize      uint64
	residentSize   uint64
	physFootprint  uint64
	procStart      uint64
	procExit       uint64
}

var (
	procPidRusage     func(pid int32, flavor int32, info unsafe.Pointer) int32
	procPidRusageOnce sync.Once
)

func processMemory() (footprint, resident uintptr) {
	procPidRusageOnce.Do(func() { // 用到才找：包级 init 的顺序按文件名，别依赖 darwin.go 先跑
		if lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW); err == nil {
			purego.RegisterLibFunc(&procPidRusage, lib, "proc_pid_rusage")
		}
	})
	if procPidRusage == nil {
		return 0, 0
	}
	var info rusageInfoV0
	if procPidRusage(int32(os.Getpid()), 0 /* RUSAGE_INFO_V0 */, unsafe.Pointer(&info)) != 0 {
		return 0, 0
	}
	return uintptr(info.physFootprint), uintptr(info.residentSize)
}

// trimWorkingSet macOS 没有「把工作集清空」这回事，内存紧了系统自己会压缩、换出
func trimWorkingSet() {}

package app

import (
	"os"
	"slices"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// 谁在出声：CoreAudio 的进程对象（macOS 14.2 起有），IsRunningOutput 就是在往输出设备送声音，不用权限。
// 怎么停：MediaRemote 的 MRMediaRemoteSendCommand。macOS 15.4 起第三方程序读不到「正在播放」的信息了，
// 但发暂停、播放还管用（27.2 上实测 MySpotify 能停能放，WebKit 替网页里的播放器接了这条命令）
var (
	mediaOnce  sync.Once
	caPropSize func(obj uint32, addr unsafe.Pointer, qsize uint32, q uintptr, size *uint32) int32
	caPropData func(obj uint32, addr unsafe.Pointer, qsize uint32, q uintptr, size *uint32, data unsafe.Pointer) int32
	mrSend     func(cmd uint32, opts uintptr) bool
	respPID    func(pid int32) int32 // 帮别人干活的进程（WebKit 的 GPU 进程）→ 那个 app
)

func loadMedia() {
	mediaOnce.Do(func() {
		reg := func(fn any, lib uintptr, name string) {
			if lib == 0 {
				return
			}
			if sym, err := purego.Dlsym(lib, name); err == nil && sym != 0 {
				purego.RegisterFunc(fn, sym)
			}
		}
		open := func(path string) uintptr {
			h, _ := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			return h
		}
		ca := open("/System/Library/Frameworks/CoreAudio.framework/CoreAudio")
		reg(&caPropSize, ca, "AudioObjectGetPropertyDataSize")
		reg(&caPropData, ca, "AudioObjectGetPropertyData")
		reg(&mrSend, open("/System/Library/PrivateFrameworks/MediaRemote.framework/MediaRemote"), "MRMediaRemoteSendCommand")
		reg(&respPID, open("/usr/lib/libSystem.B.dylib"), "responsibility_get_pid_responsible_for_pid")
	})
}

func fourCC(s string) uint32 {
	return uint32(s[0])<<24 | uint32(s[1])<<16 | uint32(s[2])<<8 | uint32(s[3])
}

type caAddress struct{ selector, scope, element uint32 }

func platformMediaPlaying() []int32 {
	loadMedia()
	if caPropSize == nil || caPropData == nil {
		return nil
	}
	list := caAddress{fourCC("prs#"), fourCC("glob"), 0} // kAudioHardwarePropertyProcessObjectList
	var n uint32
	if caPropSize(1, unsafe.Pointer(&list), 0, 0, &n) != 0 || n < 4 { // 1 = kAudioObjectSystemObject；老系统没有这个属性
		return nil
	}
	ids := make([]uint32, n/4)
	if caPropData(1, unsafe.Pointer(&list), 0, 0, &n, unsafe.Pointer(&ids[0])) != 0 {
		return nil
	}
	self := int32(os.Getpid())
	var out []int32
	for _, id := range ids[:n/4] {
		var running uint32
		size := uint32(4)
		if a := (caAddress{fourCC("piro"), fourCC("glob"), 0}); caPropData(id, unsafe.Pointer(&a), 0, 0, &size, unsafe.Pointer(&running)) != 0 || running == 0 {
			continue
		}
		var pid int32
		size = 4
		if a := (caAddress{fourCC("ppid"), fourCC("glob"), 0}); caPropData(id, unsafe.Pointer(&a), 0, 0, &size, unsafe.Pointer(&pid)) != 0 || pid <= 0 {
			continue
		}
		if respPID != nil {
			if p := respPID(pid); p > 0 {
				pid = p
			}
		}
		if pid != self && !slices.Contains(out, pid) {
			out = append(out, pid)
		}
	}
	return out
}

func platformMediaSend(cmd int) bool {
	loadMedia()
	return mrSend != nil && mrSend(uint32(cmd), 0)
}

//go:build darwin

package app

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

// 录音用 AudioQueue：16kHz 单声道 16 位，和 Windows 那边一样每 100ms 一块。
// 回调排在自己线程的 run loop 上（不让系统线程直接回调进 Go）。

type audioFormat struct {
	SampleRate       float64
	FormatID         uint32
	FormatFlags      uint32
	BytesPerPacket   uint32
	FramesPerPacket  uint32
	BytesPerFrame    uint32
	ChannelsPerFrame uint32
	BitsPerChannel   uint32
	Reserved         uint32
}

// audioBuffer AudioQueueBuffer 的前几个字段
type audioBuffer struct {
	Capacity uint32
	_        uint32
	Data     uintptr
	Size     uint32
}

var (
	aqNewInput      func(format *audioFormat, cb, user, runLoop, mode uintptr, flags uint32, out *uintptr) int32
	aqAllocate      func(aq uintptr, size uint32, out *uintptr) int32
	aqEnqueue       func(aq, buf uintptr, n uint32, descs uintptr) int32
	aqStart         func(aq, at uintptr) int32
	aqStop          func(aq uintptr, immediate bool) int32
	aqDispose       func(aq uintptr, immediate bool) int32
	cfRunLoopRunIn  func(mode uintptr, seconds float64, returnAfterSource bool) int32
	kCFRunLoopDefMd uintptr

	aqCallback = purego.NewCallback(aqInput)
	aqSinks    sync.Map // 录音代号 → chan []byte
	aqSeq      atomic.Uintptr
)

func init() {
	reg := purego.RegisterLibFunc
	reg(&aqNewInput, libAudio, "AudioQueueNewInput")
	reg(&aqAllocate, libAudio, "AudioQueueAllocateBuffer")
	reg(&aqEnqueue, libAudio, "AudioQueueEnqueueBuffer")
	reg(&aqStart, libAudio, "AudioQueueStart")
	reg(&aqStop, libAudio, "AudioQueueStop")
	reg(&aqDispose, libAudio, "AudioQueueDispose")
	reg(&cfRunLoopRunIn, libCF, "CFRunLoopRunInMode")
	kCFRunLoopDefMd = dlvar(libCF, "kCFRunLoopDefaultMode")
}

// aqInput 一块录满了：拷走，再把缓冲还给队列
func aqInput(user, aq, buf, start uintptr, n uint32, descs uintptr) uintptr {
	b := (*audioBuffer)(cPointer(buf))
	if v, ok := aqSinks.Load(user); ok && b.Size > 0 {
		data := append([]byte(nil), unsafe.Slice((*byte)(cPointer(b.Data)), b.Size)...)
		select {
		case v.(chan []byte) <- data:
		default:
		}
	}
	aqEnqueue(aq, buf, 0, 0) // 停了以后会失败，不要紧
	return 0
}

// hasMic 有没有能录音的输入设备。Mac mini、Mac Studio 自己没有麦克风：什么都没接时 AudioQueue 照样建得起来、
// 开得起来，只是一块数据都不来，按了触发键就像没反应。测试里换成假的
var hasMic = func() bool {
	ok := false
	withPool(func() {
		ok = send(class("AVCaptureDevice"), "defaultDeviceWithMediaType:", dlvar(libAVF, "AVMediaTypeAudio")) != 0
	})
	return ok
}

func recordVoice(ctx context.Context, stop <-chan struct{}, started func(), chunk func(data []byte, level float64)) (pcm []byte, err error) {
	if !hasMic() {
		return nil, errors.New(T("没找到麦克风：接上耳机、AirPods 或 USB 麦克风再试"))
	}
	switch micPermission() {
	case micDenied:
		return nil, errors.New(T("没有麦克风权限：系统设置 → 隐私与安全性 → 麦克风，打开 Mami"))
	}
	id := aqSeq.Add(1)
	sink := make(chan []byte, 64)
	aqSinks.Store(id, sink)
	defer aqSinks.Delete(id)

	// 队列建在专门的线程上，它的 run loop 收回调；done 关掉后线程退出
	type made struct {
		q   uintptr
		err error
	}
	ready, done := make(chan made, 1), make(chan struct{})
	defer close(done)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		format := audioFormat{SampleRate: 16000, FormatID: 0x6C70636D, FormatFlags: 0x4 | 0x8, // 'lpcm'，有符号整数、紧凑
			BytesPerPacket: 2, FramesPerPacket: 1, BytesPerFrame: 2, ChannelsPerFrame: 1, BitsPerChannel: 16}
		var q uintptr
		if st := aqNewInput(&format, aqCallback, id, cfRunLoopGetCurrent(), kCFRunLoopCommonModes, 0, &q); st != 0 {
			ready <- made{err: fmt.Errorf(T("麦克风操作失败：%s（%d）%s"), "AudioQueueNewInput", st, "")}
			return
		}
		for range 4 {
			var b uintptr
			if st := aqAllocate(q, 3200, &b); st != 0 {
				aqDispose(q, true)
				ready <- made{err: fmt.Errorf(T("麦克风操作失败：%s（%d）%s"), "AudioQueueAllocateBuffer", st, "")}
				return
			}
			aqEnqueue(q, b, 0, 0)
		}
		ready <- made{q: q}
		for {
			select {
			case <-done:
				aqDispose(q, true)
				return
			default:
				if cfRunLoopRunIn(kCFRunLoopDefMd, 0.05, false) == 1 { // kCFRunLoopRunFinished：没挂上源，别空转
					time.Sleep(10 * time.Millisecond)
				}
			}
		}
	}()
	m := <-ready
	if m.err != nil {
		return nil, m.err
	}
	q := m.q
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if st := aqStart(q, 0); st != 0 {
		return nil, fmt.Errorf(T("麦克风操作失败：%s（%d）%s"), "AudioQueueStart", st, "")
	}
	defer aqStop(q, true)
	if started != nil {
		started()
	}
	take := func(data []byte) {
		pcm = append(pcm, data...)
		if chunk != nil {
			chunk(data, pcmRMS(data))
		}
	}
	limit := time.NewTimer(5 * time.Minute)
	defer limit.Stop()
loop:
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-stop:
			break loop
		case <-limit.C:
			break loop
		case data := <-sink:
			take(data)
		}
	}
	aqStop(q, true) // 同步停：返回后不会再有回调
	for {
		select {
		case data := <-sink:
			take(data)
			continue
		default:
		}
		break
	}
	if len(pcm) > 5*60*32000 {
		pcm = pcm[:5*60*32000]
	}
	return pcm, nil
}

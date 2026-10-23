//go:build windows

package app

import (
	"context"
	"fmt"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var winmm = syscall.NewLazyDLL("winmm.dll")
var (
	pWaveInOpen      = winmm.NewProc("waveInOpen")
	pWaveInPrepare   = winmm.NewProc("waveInPrepareHeader")
	pWaveInAdd       = winmm.NewProc("waveInAddBuffer")
	pWaveInStart     = winmm.NewProc("waveInStart")
	pWaveInStop      = winmm.NewProc("waveInStop")
	pWaveInReset     = winmm.NewProc("waveInReset")
	pWaveInUnprepare = winmm.NewProc("waveInUnprepareHeader")
	pWaveInClose     = winmm.NewProc("waveInClose")
	pWaveInError     = winmm.NewProc("waveInGetErrorTextW")
)

type waveFormat struct {
	Tag, Channels      uint16
	Rate, BytesPerSec  uint32
	Align, Bits, Extra uint16
}

type waveHeader struct {
	Data             uintptr
	Length, Recorded uint32
	User             uintptr
	Flags, Loops     uint32
	Next, Reserved   uintptr
}

type waveBuffer struct {
	header   *waveHeader
	memory   uintptr
	prepared bool
}

func waveError(op string, code uintptr) error {
	var text [256]uint16
	pWaveInError.Call(code, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)))
	return fmt.Errorf(T("麦克风操作失败：%s（%d）%s"), op, code, syscall.UTF16ToString(text[:]))
}

// 驱动异步持有的头和 PCM 都用系统内存，不让 Go GC 移动或提前回收。
// chunk 每收到一块（100ms）就调一次，data 指向驱动缓冲区，回调里要用就得拷走。
func recordVoice(ctx context.Context, stop <-chan struct{}, started func(), chunk func(data []byte, level float64)) (pcm []byte, err error) {
	format := waveFormat{Tag: 1, Channels: 1, Rate: 16000, BytesPerSec: 32000, Align: 2, Bits: 16}
	var handle uintptr
	if code, _, _ := pWaveInOpen.Call(uintptr(unsafe.Pointer(&handle)), ^uintptr(0)&0xffffffff, uintptr(unsafe.Pointer(&format)), 0, 0, 0); code != 0 {
		return nil, waveError("waveInOpen", code)
	}
	buffers := make([]waveBuffer, 4)
	defer func() {
		// reset 先归还所有缓冲；卸载/关闭失败时宁可保留内存，也不能让驱动访问已释放地址。
		reset, _, _ := pWaveInReset.Call(handle)
		safe := reset == 0
		if reset != 0 && err == nil {
			err = waveError("waveInReset", reset)
		}
		for _, buffer := range buffers {
			if buffer.prepared {
				code, _, _ := pWaveInUnprepare.Call(handle, uintptr(unsafe.Pointer(buffer.header)), unsafe.Sizeof(waveHeader{}))
				if code != 0 {
					safe = false
					if err == nil {
						err = waveError("waveInUnprepareHeader", code)
					}
				}
			}
		}
		code, _, _ := pWaveInClose.Call(handle)
		if code != 0 {
			safe = false
			if err == nil {
				err = waveError("waveInClose", code)
			}
		}
		if safe {
			for _, buffer := range buffers {
				if buffer.memory != 0 {
					pGlobalFree.Call(buffer.memory)
				}
			}
		}
	}()
	for index := range buffers {
		memory, _, _ := pGlobalAlloc.Call(0x40, unsafe.Sizeof(waveHeader{})+3200)
		if memory == 0 {
			return nil, fmt.Errorf("waveIn: out of memory")
		}
		header := (*waveHeader)(nativePointer(memory))
		header.Data, header.Length = memory+unsafe.Sizeof(waveHeader{}), 3200
		buffers[index] = waveBuffer{header: header, memory: memory}
		if code, _, _ := pWaveInPrepare.Call(handle, memory, unsafe.Sizeof(waveHeader{})); code != 0 {
			return nil, waveError("waveInPrepareHeader", code)
		}
		buffers[index].prepared = true
		if code, _, _ := pWaveInAdd.Call(handle, memory, unsafe.Sizeof(waveHeader{})); code != 0 {
			return nil, waveError("waveInAddBuffer", code)
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if code, _, _ := pWaveInStart.Call(handle); code != 0 {
		return nil, waveError("waveInStart", code)
	}
	if started != nil {
		started()
	}
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	limit := time.NewTimer(5 * time.Minute)
	defer limit.Stop()
	next := 0
	collect := func(requeue bool) error {
		for count := 0; count < len(buffers); count++ {
			buffer := buffers[next]
			header := buffer.header
			if atomic.LoadUint32(&header.Flags)&1 == 0 {
				break
			}
			next = (next + 1) % len(buffers)
			data := unsafe.Slice((*byte)(nativePointer(header.Data)), min(header.Recorded, header.Length))
			pcm = append(pcm, data...)
			if chunk != nil {
				chunk(data, pcmRMS(data))
			}
			if requeue {
				if code, _, _ := pWaveInAdd.Call(handle, buffer.memory, unsafe.Sizeof(waveHeader{})); code != 0 {
					return waveError("waveInAddBuffer", code)
				}
			}
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-stop:
			goto finish
		case <-limit.C:
			goto finish
		case <-ticker.C:
			if err = collect(true); err != nil {
				return nil, err
			}
		}
	}
finish:
	if code, _, _ := pWaveInStop.Call(handle); code != 0 {
		return nil, waveError("waveInStop", code)
	}
	if code, _, _ := pWaveInReset.Call(handle); code != 0 {
		return nil, waveError("waveInReset", code)
	}
	if err = collect(false); err != nil {
		return nil, err
	}
	if len(pcm) > 5*60*32000 {
		pcm = pcm[:5*60*32000]
	}
	return pcm, nil
}

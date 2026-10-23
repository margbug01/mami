//go:build darwin

package app

import (
	"sync"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo"
)

// 提示音：每段合成好的 WAV 建一个 NSSound 常驻，播的时候先停再从头放
var (
	soundsMu sync.Mutex
	sounds   = map[*byte]objc.ID{}
)

func playSound(wav []byte) {
	if len(wav) == 0 {
		return
	}
	go mygo.RunOnMain(func() {
		withPool(func() {
			soundsMu.Lock()
			snd, ok := sounds[&wav[0]]
			if !ok {
				snd = send(send(class("NSSound"), "alloc"), "initWithData:", uintptr(nsData(wav)))
				sounds[&wav[0]] = snd
			}
			soundsMu.Unlock()
			if snd != 0 {
				send(snd, "stop")
				send(snd, "play")
			}
		})
	})
}

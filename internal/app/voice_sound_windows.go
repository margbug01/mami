//go:build windows

package app

import "unsafe"

var pPlaySound = winmm.NewProc("PlaySoundW")

func playSound(wav []byte) {
	const sndAsync, sndNoDefault, sndMemory = 0x1, 0x2, 0x4
	pPlaySound.Call(uintptr(unsafe.Pointer(&wav[0])), 0, sndAsync|sndNoDefault|sndMemory)
}

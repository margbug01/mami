package app

import (
	"encoding/binary"
	"math"
)

// 提示音在启动时合成好放在内存里，playSound 异步播放，不用带音频文件
var (
	soundStart = chime(0.24, 820, 920) // 嘚嘚：两下轻敲，第二下略高
	soundStop  = chime(0.24, 920, 820) // 嘚嘚：第二下略低
	soundAbort = chime(0.14, 700)      // 取消：一下更轻的
)

// chime 一下一下的轻敲：每下隔 80ms，1ms 起音、约 26ms 就衰减完，
// 叠一个 2.76 倍的不成谐波泛音，听起来像敲木头而不是铃铛；生成 22.05kHz 单声道 WAV
func chime(volume float64, freqs ...float64) []byte {
	const rate, step, tail = 22050, 0.08, 0.12
	n := int((step*float64(len(freqs)-1) + tail) * rate)
	pcm := make([]byte, 44+2*n)
	for i := 0; i < n; i++ {
		t, v := float64(i)/rate, 0.0
		for k, f := range freqs {
			u := t - step*float64(k)
			if u < 0 {
				continue
			}
			env := math.Min(1, u/0.001) * math.Exp(-u/0.026)
			v += env * (math.Sin(2*math.Pi*f*u) + 0.35*math.Exp(-u/0.006)*math.Sin(2*math.Pi*2.76*f*u))
		}
		binary.LittleEndian.PutUint16(pcm[44+2*i:], uint16(int16(math.Max(-1, math.Min(1, v*volume))*32767)))
	}
	copy(pcm, "RIFF")
	binary.LittleEndian.PutUint32(pcm[4:], uint32(36+2*n))
	copy(pcm[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(pcm[16:], 16)
	binary.LittleEndian.PutUint16(pcm[20:], 1)
	binary.LittleEndian.PutUint16(pcm[22:], 1)
	binary.LittleEndian.PutUint32(pcm[24:], rate)
	binary.LittleEndian.PutUint32(pcm[28:], rate*2)
	binary.LittleEndian.PutUint16(pcm[32:], 2)
	binary.LittleEndian.PutUint16(pcm[34:], 16)
	copy(pcm[36:], "data")
	binary.LittleEndian.PutUint32(pcm[40:], uint32(2*n))
	return pcm
}

package app

import (
	"encoding/binary"
	"math"
)

// pcmRMS 16 位小端单声道 PCM 的音量（0~1）
func pcmRMS(data []byte) float64 {
	if len(data) < 2 {
		return 0
	}
	var sum float64
	for index := 0; index+1 < len(data); index += 2 {
		sample := float64(int16(binary.LittleEndian.Uint16(data[index:]))) / 32768
		sum += sample * sample
	}
	return math.Sqrt(sum / float64(len(data)/2))
}

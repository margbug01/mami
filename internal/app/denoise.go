package app

import (
	"encoding/binary"
	"math"
	"math/cmplx"
)

const (
	dnFrame = 512 // 32ms @16k
	dnHop   = dnFrame / 2
	dnBins  = dnFrame/2 + 1
	dnLowK  = 3 // 3*31.25Hz：再往下只有风扇、桌面震动，没人声
)

// denoiser 流式降噪：512 点 STFT、半重叠、sqrt-Hann 窗，输出比输入晚 16ms。
// 噪声谱用开头几帧起步，之后只在某个频点「看着像噪声」时才跟着更新；增益是决策导向的维纳滤波。
// 增益留了下限 floor，不压到底：识别模型更怕压出来的「水声」和被吃掉的辅音，底噪反倒没那么怕。
type denoiser struct {
	floor   float64
	alpha   float64 // 决策导向平滑：越大越少「水声」，但字头越容易被削
	mix     float64 // 掺回原声的比例（等价于增益压缩成 mix+(1-mix)·G）
	win     [dnFrame]float64
	hist    [dnFrame]float64 // 最近一帧输入
	ola     [dnFrame]float64 // overlap-add 累加
	noise   [dnBins]float64
	prevG2P [dnBins]float64
	buf     [dnFrame]complex128
	pending []float64
	frames  int
	skip    int // 对齐用：前 dnHop 个输出是延迟，丢掉
	nIn     int
	nOut    int
}

func newDenoiser(floor float64) *denoiser {
	d := &denoiser{floor: floor, alpha: 0.98, skip: dnHop}
	for n := range d.win {
		d.win[n] = math.Sqrt(0.5 - 0.5*math.Cos(2*math.Pi*float64(n)/dnFrame))
	}
	return d
}

// process 吃 16 位 PCM，吐出已经降好噪的那部分（长度跟输入不一定相同，flush 后总长对齐）
func (d *denoiser) process(pcm []byte) []byte {
	for i := 0; i+1 < len(pcm); i += 2 {
		d.pending = append(d.pending, float64(int16(binary.LittleEndian.Uint16(pcm[i:])))/32768)
	}
	d.nIn += len(pcm) / 2
	return d.drain()
}

// flush 用静音把最后不满一帧的部分推出来，总输出长度 = 总输入长度
func (d *denoiser) flush() []byte {
	var out []byte
	for d.nOut < d.nIn {
		d.pending = append(d.pending, make([]float64, dnHop)...)
		out = append(out, d.drain()...)
	}
	if extra := (d.nOut - d.nIn) * 2; extra > 0 {
		out = out[:len(out)-extra]
		d.nOut = d.nIn
	}
	return out
}

func (d *denoiser) drain() []byte {
	var out []byte
	for len(d.pending) >= dnHop {
		copy(d.hist[:], d.hist[dnHop:])
		copy(d.hist[dnHop:], d.pending[:dnHop])
		d.pending = d.pending[dnHop:]
		d.step()
		from := min(d.skip, dnHop)
		d.skip -= from
		for _, v := range d.ola[from:dnHop] {
			s := int16(max(-32768, min(32767, math.Round(v*32768))))
			out = binary.LittleEndian.AppendUint16(out, uint16(s))
		}
		d.nOut += dnHop - from
		copy(d.ola[:], d.ola[dnHop:])
		clear(d.ola[dnHop:])
	}
	return out
}

func (d *denoiser) step() {
	for n := range d.buf {
		d.buf[n] = complex(d.hist[n]*d.win[n], 0)
	}
	fft(d.buf[:], false)
	const initFrames = 8
	alpha := d.alpha
	d.frames++
	for k := 0; k < dnBins; k++ {
		p := real(d.buf[k])*real(d.buf[k]) + imag(d.buf[k])*imag(d.buf[k])
		switch {
		case d.frames <= initFrames:
			d.noise[k] += p / initFrames
		case p < d.noise[k]:
			d.noise[k] = 0.9*d.noise[k] + 0.1*p
		case p < 3*d.noise[k]:
			d.noise[k] = 0.98*d.noise[k] + 0.02*p
		default:
			d.noise[k] *= 1.001 // 噪声慢慢变大时也跟得上（约每秒 +3%）
		}
		nz := max(d.noise[k], 1e-12)
		if d.frames <= initFrames {
			nz = max(d.noise[k]*initFrames/float64(d.frames), 1e-12)
		}
		post := p / nz
		prior := alpha*d.prevG2P[k]/nz + (1-alpha)*max(post-1, 0)
		g := max(prior/(1+prior), d.floor)
		if k < dnLowK {
			g = d.floor
		}
		d.prevG2P[k] = g * g * p
		g = d.mix + (1-d.mix)*g
		d.buf[k] *= complex(g, 0)
		if k > 0 && k < dnFrame/2 {
			d.buf[dnFrame-k] *= complex(g, 0)
		}
	}
	fft(d.buf[:], true)
	for n := range d.ola {
		d.ola[n] += real(d.buf[n]) * d.win[n]
	}
}

// denoisePCM 整段一次降完（测试和非流式用）
func denoisePCM(pcm []byte, floor float64) []byte {
	d := newDenoiser(floor)
	return append(d.process(pcm), d.flush()...)
}

// fft 原地基 2 FFT；inv 时带 1/n
func fft(a []complex128, inv bool) {
	n := len(a)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			a[i], a[j] = a[j], a[i]
		}
	}
	for l := 2; l <= n; l <<= 1 {
		ang := 2 * math.Pi / float64(l)
		if !inv {
			ang = -ang
		}
		wl := cmplx.Rect(1, ang)
		for i := 0; i < n; i += l {
			w := complex(1, 0)
			for k := 0; k < l/2; k++ {
				u, v := a[i+k], a[i+k+l/2]*w
				a[i+k], a[i+k+l/2] = u+v, u-v
				w *= wl
			}
		}
	}
	if inv {
		for i := range a {
			a[i] /= complex(float64(n), 0)
		}
	}
}

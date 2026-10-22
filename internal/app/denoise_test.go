package app

import (
	"encoding/binary"
	"math"
	"math/rand"
	"testing"
)

func pcmOf(x []float64) []byte {
	var b []byte
	for _, v := range x {
		b = binary.LittleEndian.AppendUint16(b, uint16(int16(max(-32768, min(32767, math.Round(v*32768))))))
	}
	return b
}

func floatsOf(b []byte) []float64 {
	x := make([]float64, len(b)/2)
	for i := range x {
		x[i] = float64(int16(binary.LittleEndian.Uint16(b[2*i:]))) / 32768
	}
	return x
}

func energy(x []float64) (e float64) {
	for _, v := range x {
		e += v * v
	}
	return
}

func TestDenoiseIdentityAndLength(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	x := make([]float64, 16000+123)
	for i := range x {
		x[i] = 0.3 * math.Sin(float64(i)*0.05) * r.Float64()
	}
	// floor=1 时增益恒为 1，应原样还原（逐块喂，块长故意不对齐）
	d := newDenoiser(1)
	in := pcmOf(x)
	var out []byte
	for i := 0; i < len(in); i += 3202 {
		out = append(out, d.process(in[i:min(i+3202, len(in))])...)
	}
	out = append(out, d.flush()...)
	if len(out) != len(in) {
		t.Fatalf("len %d want %d", len(out), len(in))
	}
	y := floatsOf(out)
	for i := range x {
		if math.Abs(y[i]-x[i]) > 2.0/32768 {
			t.Fatalf("sample %d: %f vs %f", i, y[i], x[i])
		}
	}
}

func TestDenoiseSuppressesNoiseKeepsTone(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	x := make([]float64, 32000)
	for i := range x {
		x[i] = 0.02 * r.NormFloat64()
		if i >= 16000 {
			x[i] += 0.2 * math.Sin(2*math.Pi*1000*float64(i)/16000)
		}
	}
	y := floatsOf(denoisePCM(pcmOf(x), 0.3))
	noiseIn, noiseOut := energy(x[8000:16000]), energy(y[8000:16000])
	if noiseOut > 0.3*noiseIn {
		t.Errorf("noise kept %.2f of energy", noiseOut/noiseIn)
	}
	toneIn, toneOut := energy(x[20000:]), energy(y[20000:])
	if toneOut < 0.8*toneIn {
		t.Errorf("tone kept only %.2f of energy", toneOut/toneIn)
	}
}

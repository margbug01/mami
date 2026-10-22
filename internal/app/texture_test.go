package app

import (
	"bytes"
	"image"
	"testing"
)

func rowLuma(img *image.RGBA, y int) float64 {
	var sum float64
	for x := range img.Bounds().Dx() {
		i := img.PixOffset(x, y)
		sum += 0.2126*float64(img.Pix[i]) + 0.7152*float64(img.Pix[i+1]) + 0.0722*float64(img.Pix[i+2])
	}
	return sum / float64(img.Bounds().Dx())
}

func TestTexture(t *testing.T) {
	a, b := paintTexture(mistLight, 90, 60), paintTexture(mistLight, 90, 60)
	if a.Bounds().Dx() != 90 || a.Bounds().Dy() != 60 {
		t.Fatalf("size %v", a.Bounds())
	}
	if !bytes.Equal(a.Pix, b.Pix) {
		t.Fatal("texture not deterministic")
	}
	if top, bottom := rowLuma(a, 0), rowLuma(a, 59); top <= bottom+8 {
		t.Fatalf("top row should be clearly lighter: top %.0f bottom %.0f", top, bottom)
	}

	// 色团：中心不透明，往外只减不增，边上没了
	prev := blobAlpha(0)
	if prev != 1 || blobAlpha(1) != 0 {
		t.Fatalf("blob ends %v %v", prev, blobAlpha(1))
	}
	for d := float32(0.01); d < 1; d += 0.01 {
		if v := blobAlpha(d); v > prev {
			t.Fatalf("blob alpha rises at %.2f", d)
		} else {
			prev = v
		}
	}

	// 颗粒：固定种子每次一样，相邻像素确实有深有浅
	g1, g2 := grainTile(0.2, 0.2), grainTile(0.2, 0.2)
	if !bytes.Equal(g1.Pix, g2.Pix) {
		t.Fatal("grain not deterministic")
	}
	var black, white, changes int
	for i := 0; i < len(g1.Pix); i += 4 {
		if g1.Pix[i] == 0 {
			black++
		} else {
			white++
		}
		if i > 0 && g1.Pix[i+3] != g1.Pix[i-1] {
			changes++
		}
	}
	if n := grainSize * grainSize; black < n/3 || white < n/3 || changes < n/2 {
		t.Fatalf("grain too flat: black %d white %d changes %d", black, white, changes)
	}
}

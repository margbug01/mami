package app

import (
	"image"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

const flowSize = 48
const flowPixels = 64
const flowFrame = time.Second / 30

type flowRenderer struct {
	texture *image.RGBA
	frames  [2]*image.RGBA
	bitmap  *ui.Bitmap
	last    time.Time
	slot    int
}

var flowViews [len(flowPalettes)][2]flowRenderer
var flowStarted = time.Now()

func flowHash(x, y float64) float64 {
	value := uint32(int64(x))*374761393 + uint32(int64(y))*668265263
	value = (value ^ (value >> 13)) * 1274126177
	return float64(value^(value>>16)) / 4294967295
}

func flowNoise(x, y float64) float64 {
	cellX, cellY := math.Floor(x), math.Floor(y)
	localX, localY := x-cellX, y-cellY
	blendX, blendY := localX*localX*(3-2*localX), localY*localY*(3-2*localY)
	top := flowHash(cellX, cellY)*(1-blendX) + flowHash(cellX+1, cellY)*blendX
	bottom := flowHash(cellX, cellY+1)*(1-blendX) + flowHash(cellX+1, cellY+1)*blendX
	return top*(1-blendY) + bottom*blendY
}

func flowFBM(x, y float64) float64 {
	total, amplitude := 0.0, 0.5
	for octave := 0; octave < 4; octave++ {
		total += amplitude * flowNoise(x, y)
		x, y = 1.6*x+1.2*y+17.1, -1.2*x+1.6*y+9.2
		amplitude *= 0.5
	}
	return total / 0.9375
}

func flowSimplex(x, y float64) float64 {
	const skew = 0.36602540378
	const unskew = 0.21132486540
	cellX, cellY := math.Floor(x+(x+y)*skew), math.Floor(y+(x+y)*skew)
	firstX, firstY := x-cellX+(cellX+cellY)*unskew, y-cellY+(cellX+cellY)*unskew
	cornerX, cornerY := 0.0, 1.0
	if firstX > firstY {
		cornerX, cornerY = 1, 0
	}
	contribution := func(dx, dy, gx, gy float64) float64 {
		weight := math.Max(0, 0.5-dx*dx-dy*dy)
		angle := flowHash(gx, gy) * 2 * math.Pi
		return weight * weight * weight * weight * (math.Cos(angle)*dx + math.Sin(angle)*dy)
	}
	return 70 * (contribution(firstX, firstY, cellX, cellY) + contribution(firstX-cornerX+unskew, firstY-cornerY+unskew, cellX+cornerX, cellY+cornerY) + contribution(firstX-1+2*unskew, firstY-1+2*unskew, cellX+1, cellY+1))
}

func flowTexture(id string) *image.RGBA {
	palette := flowPalettes[flowPaletteIndex(id)]
	texture := image.NewRGBA(image.Rect(0, 0, 512, 512))
	spots := [][6]float64{{110, 102, 145, 143, 255, 244}, {365, 140, 125, 61, 229, 255}, {340, 410, 170, 4, 46, 139}, {75, 340, 110, 87, 255, 255}}
	for index := range spots {
		copy(spots[index][3:], palette.spots[index][:])
	}
	for row := 0; row < 512; row++ {
		for col := 0; col < 512; col++ {
			rgb := palette.base
			for _, spot := range spots {
				dx, dy := float64(col)-spot[0], float64(row)-spot[1]
				alpha := math.Exp(-(dx*dx+dy*dy)/(2*spot[2]*spot[2])) * 0.94
				for channel := 0; channel < 3; channel++ {
					rgb[channel] = rgb[channel]*(1-alpha) + spot[channel+3]*alpha
				}
			}
			offset := texture.PixOffset(col, row)
			grain := (flowHash(float64(col), float64(row)) - 0.5) * 3.5
			for channel := 0; channel < 3; channel++ {
				texture.Pix[offset+channel] = uint8(math.Max(0, math.Min(255, rgb[channel]+grain)))
			}
			texture.Pix[offset+3] = 255
		}
	}
	return texture
}

func renderFlow(target, texture *image.RGBA, seconds float64) {
	size := target.Bounds().Dx()
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			offset := target.PixOffset(col, row)
			pointX, pointY := (float64(col)+0.5)*2/float64(size)-1, 1-(float64(row)+0.5)*2/float64(size)
			radius := math.Hypot(pointX, pointY)
			mask := math.Max(0, math.Min(1, (1-radius)*float64(size)/2))
			mask = mask * mask * (3 - 2*mask)
			if mask == 0 {
				clear(target.Pix[offset : offset+4])
				continue
			}
			bulge := math.Pow(math.Sqrt(math.Max(0, 1-radius*radius)), 1.1)
			sphereX, sphereY := pointX/(1+bulge), pointY/(1+bulge)
			domainX, domainY := sphereX*3.25, sphereY*3.25
			clockX, clockY := seconds*0.095, -seconds*0.073
			firstX := flowFBM(domainX+clockX, domainY+clockY)
			firstY := flowFBM(domainX+5.2-clockX, domainY+1.3-clockY)
			warpX, warpY := domainX+1.35*(firstX-0.5), domainY+1.35*(firstY-0.5)
			secondX := flowFBM(warpX+1.7+clockX*0.65, warpY+9.2+clockY*0.65)
			secondY := flowFBM(warpX+8.3-clockX*0.55, warpY+2.8-clockY*0.55)
			flow := flowFBM(domainX+1.35*(secondX-0.5)+clockX*0.4, domainY+1.35*(secondY-0.5)+clockY*0.4)*2 - 1
			fade := math.Min(1, radius/0.32)
			radial := flow * 0.65 * fade * fade * (3 - 2*fade) / math.Max(radius, 0.0001)
			warpedX, warpedY := sphereX+pointX*radial, sphereY+pointY*radial
			turn := 0.24*math.Sin(seconds*0.14) + 0.16*math.Sin(sphereY*2+seconds*0.19)
			sine, cosine := math.Sincos(turn)
			driftX := flowSimplex(sphereX*0.8+seconds*0.032, sphereY*0.8+4.7) * 0.15
			driftY := flowSimplex(sphereX*0.8+8.1, sphereY*0.8-seconds*0.027) * 0.15
			sampleX := math.Max(0, math.Min(511, (0.5+(cosine*warpedX+sine*warpedY+driftX+math.Sin(seconds*0.17)*0.16)*0.46)*511))
			sampleY := math.Max(0, math.Min(511, (0.5-(-sine*warpedX+cosine*warpedY+driftY+math.Sin(seconds*0.13+1.2)*0.16)*0.46)*511))
			left, top := int(sampleX), int(sampleY)
			fractionX, fractionY := sampleX-float64(left), sampleY-float64(top)
			for channel := 0; channel < 3; channel++ {
				at := func(x, y int) float64 { return float64(texture.Pix[texture.PixOffset(x, y)+channel]) }
				upper := at(left, top)*(1-fractionX) + at(min(511, left+1), top)*fractionX
				lower := at(left, min(511, top+1))*(1-fractionX) + at(min(511, left+1), min(511, top+1))*fractionX
				target.Pix[offset+channel] = uint8((upper*(1-fractionY) + lower*fractionY) * mask)
			}
			target.Pix[offset+3] = uint8(mask * 255)
		}
	}
}

func drawFlowOrb(p *ui.Painter, r ui.Rect, level float32, now time.Time, recognizing bool, palette string) {
	index := 0
	if recognizing {
		index = 1
	}
	view := &flowViews[flowPaletteIndex(palette)][index]
	if view.texture == nil {
		view.texture = flowTexture(palette)
		for slot := range view.frames {
			view.frames[slot] = image.NewRGBA(image.Rect(0, 0, flowPixels, flowPixels))
		}
	}
	if view.bitmap == nil || now.Sub(view.last) >= flowFrame {
		view.slot = 1 - view.slot
		frame := view.frames[view.slot]
		renderFlow(frame, view.texture, now.Sub(flowStarted).Seconds())
		if view.bitmap == nil {
			view.bitmap = ui.NewBitmap(frame)
		} else {
			view.bitmap.Update(frame)
		}
		view.last = now
	}
	diameter := float32(flowSize) * (0.92 + 0.08*min(1, max(0, level)))
	if recognizing {
		diameter = flowSize * 0.96
	}
	diameter = min(diameter, min(r.W, r.H))
	box := ui.Rect{X: r.X + (r.W-diameter)/2, Y: r.Y + (r.H-diameter)/2, W: diameter, H: diameter}
	p.ImageRegion(view.bitmap, ui.Rect{}, box, ui.ImageOptions{})
	p.After(flowFrame)
}

package app

import "math"

type dockRect struct{ X, Y, W, H int }

type dockPosition struct {
	Edge     string  `json:"edge"`
	Fraction float64 `json:"fraction"`
	Monitor  string  `json:"monitor"`
}

func dockFraction(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0.5
	}
	return math.Max(0, math.Min(1, value))
}

func dockSnap(area dockRect, x, y int) dockPosition {
	edges := []string{"left", "right", "top", "bottom"}
	distances := []int{absDock(x - area.X), absDock(x - area.X - area.W), absDock(y - area.Y), absDock(y - area.Y - area.H)}
	nearest := 0
	for index := 1; index < len(distances); index++ {
		if distances[index] < distances[nearest] {
			nearest = index
		}
	}
	fraction := float64(y-area.Y-dockLong/2) / float64(max(1, area.H-dockLong))
	if nearest >= 2 {
		fraction = float64(x-area.X-dockLong/2) / float64(max(1, area.W-dockLong))
	}
	return dockPosition{Edge: edges[nearest], Fraction: dockFraction(fraction)}
}

func absDock(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

// 收起时小条的厚度和长度（DIP）
const dockThick, dockLong = 20, 64

func dockPlacement(area dockRect, position dockPosition, expanded bool, panelHeight int) dockRect {
	fraction := dockFraction(position.Fraction)
	vertical := position.Edge != "top" && position.Edge != "bottom"
	width, height := dockThick, dockLong
	if !vertical {
		width, height = dockLong, dockThick
	}
	if expanded {
		width, height = min(196, area.W), min(panelHeight, area.H)
	}
	result := dockRect{W: width, H: height}
	if vertical {
		result.X = area.X
		if position.Edge != "left" {
			result.X += area.W - width
		}
		result.Y = area.Y + int(fraction*float64(max(0, area.H-dockLong)))
	} else {
		result.X = area.X + int(fraction*float64(max(0, area.W-dockLong)))
		result.Y = area.Y
		if position.Edge == "bottom" {
			result.Y += area.H - height
		}
	}
	result.X = max(area.X, min(result.X, area.X+area.W-width))
	result.Y = max(area.Y, min(result.Y, area.Y+area.H-height))
	return result
}

package app

import (
	"math"
	"testing"
)

func TestDockGeometry(t *testing.T) {
	area := dockRect{X: -1920, Y: 30, W: 1920, H: 1050}
	for _, test := range []struct {
		x, y int
		edge string
	}{{-1920, 500, "left"}, {0, 500, "right"}, {-500, 30, "top"}, {-500, 1080, "bottom"}, {-1900, 500, "left"}, {-400, 1060, "bottom"}} {
		position := dockSnap(area, test.x, test.y)
		if position.Edge != test.edge {
			t.Fatalf("snap %d,%d: got %s want %s", test.x, test.y, position.Edge, test.edge)
		}
	}
	for _, edge := range []string{"left", "right", "top", "bottom"} {
		for _, fraction := range []float64{-1, 0, 0.5, 1, 2, math.NaN(), math.Inf(1)} {
			for _, expanded := range []bool{false, true} {
				rect := dockPlacement(area, dockPosition{Edge: edge, Fraction: fraction}, expanded, 396)
				if rect.X < area.X || rect.Y < area.Y || rect.X+rect.W > area.X+area.W || rect.Y+rect.H > area.Y+area.H {
					t.Fatalf("out of bounds: %+v", rect)
				}
				if edge == "left" && rect.X != area.X || edge == "right" && rect.X+rect.W != area.X+area.W || edge == "top" && rect.Y != area.Y || edge == "bottom" && rect.Y+rect.H != area.Y+area.H {
					t.Fatalf("not flush: %s %+v", edge, rect)
				}
			}
		}
	}
}

func TestDockFraction(t *testing.T) {
	for _, test := range []struct{ value, want float64 }{
		{-1, 0}, {0, 0}, {0.25, 0.25}, {1, 1}, {2, 1},
		{math.NaN(), 0.5}, {math.Inf(1), 0.5}, {math.Inf(-1), 0.5},
	} {
		if got := dockFraction(test.value); got != test.want {
			t.Errorf("fraction(%v) = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestDockPanelPlacement(t *testing.T) {
	area := dockRect{X: -1000, Y: 30, W: 1000, H: 800}
	for _, test := range []struct {
		edge     string
		fraction float64
		want     dockRect
	}{
		{"left", 0.5, dockRect{-1000, 398, 196, 228}},
		{"right", 0.5, dockRect{-196, 398, 196, 228}},
		{"top", 0.5, dockRect{-532, 30, 196, 228}},
		{"bottom", 0.5, dockRect{-532, 602, 196, 228}},
		{"right", 1, dockRect{-196, 602, 196, 228}},
		{"bottom", 1, dockRect{-196, 602, 196, 228}},
	} {
		if got := dockPlacement(area, dockPosition{Edge: test.edge, Fraction: test.fraction}, true, 228); got != test.want {
			t.Errorf("panel %s/%v = %+v, want %+v", test.edge, test.fraction, got, test.want)
		}
	}
}

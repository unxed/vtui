package vtui

import (
	"math"
	"testing"
)

func TestPixelToCell(t *testing.T) {
	tests := []struct {
		px, cell int
		want     int16
	}{
		{0, 9, 0},
		{17, 9, 1},
		{18, 9, 2},
		{-5, 9, 0},
		{-18, 9, -2},
		{100, 0, 0},
		{100, -3, 0},
		{math.MaxInt32, 1, math.MaxInt16},
		{math.MinInt32, 1, math.MinInt16},
	}
	for _, tt := range tests {
		if got := pixelToCell(tt.px, tt.cell); got != tt.want {
			t.Errorf("pixelToCell(%d, %d) = %d, want %d", tt.px, tt.cell, got, tt.want)
		}
	}
}

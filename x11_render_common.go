//go:build linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || solaris || illumos

package vtui

import (
	"image"
	"time"
)

// glyphKey используется для кэширования отрисованных символов
type glyphKey struct {
	ch uint64
	fg uint32
	bg uint32
	w  int
}

// renderStats собирает статистику производительности графического вывода
type renderStats struct {
	frameCount int
	totalDraw  time.Duration
	totalFlush time.Duration
	totalRows  int
	dirtyRows  int
	glyphs     int
	putImages  int
	lastReport time.Time
}

// clearFrameMargins paints opaque black over the part of img outside the
// gridW x gridH pixel area the cell grid covers: the partial right column and
// the partial bottom row of a window that is not a whole number of cells.
//
// The renderers draw whole cells only, so nothing else ever paints there. A
// frame can still land in them: the FrameManager renders on its own goroutine,
// so after the window shrinks it may render once more for the previous, larger
// grid before it learns the new size. That frame is clipped by the image and
// fills the margin with the last cells of the larger layout, and the smaller
// frames that follow cover only their own cells, so a nearly whole bottom row
// (a second key bar) stayed on screen (f4 #283).
func clearFrameMargins(img *image.RGBA, gridW, gridH int) {
	if img == nil {
		return
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if gridW < 0 {
		gridW = 0
	}
	if gridH < 0 {
		gridH = 0
	}
	fill := func(x0, y0, x1, y1 int) {
		for y := y0; y < y1; y++ {
			row := img.Pix[y*img.Stride : y*img.Stride+w*4]
			for x := x0 * 4; x < x1*4; x += 4 {
				row[x], row[x+1], row[x+2], row[x+3] = 0, 0, 0, 255
			}
		}
	}
	if gridW < w {
		fill(gridW, 0, w, min(gridH, h))
	}
	if gridH < h {
		fill(0, gridH, w, h)
	}
}

//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64)

package vtui

import (
	"fmt"
	"os"
	"time"
	"unicode/utf8"
)

// gogpuProfile is opt-in per-frame accounting for the gogpu renderer.
//
// It exists to answer the one question guesswork keeps getting wrong: when a
// panel redraw feels sluggish, where does the time actually go? gg queues most
// work, but not all of it — GPURenderContext.FillPath clones the path and
// tessellates it at draw time, not at flush time, so a screen full of colour
// spans pays for itself inside canvas.Draw. Splitting the frame into
// background fills, box-drawing fills, text and the final Render is what tells
// us which of those to attack.
//
//	VTUI_GOGPU_PROFILE=1  one line per frame into the debug log
//	VTUI_GOGPU_PROFILE=2  also echo it to stdout
var (
	gogpuProfileEnabled = os.Getenv("VTUI_GOGPU_PROFILE") != ""
	gogpuProfileStdout  = os.Getenv("VTUI_GOGPU_PROFILE") == "2"
	gogpuProfileFrame   uint64
)

type gogpuFrameStats struct {
	spans    int
	bgFills  int
	boxChars int
	strings  int
	glyphs   int

	bgTime     time.Duration
	boxTime    time.Duration
	textTime   time.Duration
	drawTime   time.Duration
	renderTime time.Duration
}

// gogpuProfNow and gogpuProfSince collapse to a single branch when profiling
// is off, which is what lets the instrumentation sit in the per-cell loop.
func gogpuProfNow() time.Time {
	if !gogpuProfileEnabled {
		return time.Time{}
	}
	return time.Now()
}

func gogpuProfSince(t time.Time) time.Duration {
	if !gogpuProfileEnabled {
		return 0
	}
	return time.Since(t)
}

func gogpuRuneCount(s string) int { return utf8.RuneCountInString(s) }

func gogpuProfMs(d time.Duration) float64 { return float64(d.Microseconds()) / 1000.0 }

func (s *gogpuFrameStats) report(cols, rows int) {
	gogpuProfileFrame++
	msg := fmt.Sprintf(
		"GOGPU_PROF: frame=%d grid=%dx%d spans=%d bgFills=%d box=%d strings=%d glyphs=%d "+
			"draw=%.2fms (bg=%.2f box=%.2f text=%.2f other=%.2f) render=%.2fms total=%.2fms",
		gogpuProfileFrame, cols, rows,
		s.spans, s.bgFills, s.boxChars, s.strings, s.glyphs,
		gogpuProfMs(s.drawTime),
		gogpuProfMs(s.bgTime), gogpuProfMs(s.boxTime), gogpuProfMs(s.textTime),
		gogpuProfMs(s.drawTime-s.bgTime-s.boxTime-s.textTime),
		gogpuProfMs(s.renderTime),
		gogpuProfMs(s.drawTime+s.renderTime),
	)
	DebugLog("%s", msg)
	if gogpuProfileStdout {
		fmt.Fprintln(os.Stdout, msg)
	}
}

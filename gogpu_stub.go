//go:build freebsd || dragonfly || openbsd || netbsd

package vtui

import "fmt"

// GogpuRenderer является заглушкой для платформ, где gogpu бекенд отключен.
// Это необходимо для успешной компиляции проверок типов в framemanager.go.
type GogpuRenderer struct{}

func (r *GogpuRenderer) Render(buf, shadow []CharInfo, width, height int, forceRedraw bool) {}
func (r *GogpuRenderer) SetCursor(x, y int, visible bool, shape CursorShape)               {}
func (r *GogpuRenderer) SetPalette(palette *[256]uint32)                                    {}
func (r *GogpuRenderer) Flush()                                                            {}

// RunGogpuHost — заглушка функции запуска для BSD.
func RunGogpuHost(cols, rows int, setupApp func()) error {
	return fmt.Errorf("gogpu backend is not supported on BSDs")
}
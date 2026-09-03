//go:build !(linux || windows || darwin) || android || !(amd64 || arm64)

package vtui

import "fmt"

// EbitenRenderer is a stub for platforms where the Ebitengine backend is not
// built, so that type switches elsewhere in the package still compile.
type EbitenRenderer struct{}

func (r *EbitenRenderer) Render(buf, shadow []CharInfo, width, height int, forceRedraw bool) {}
func (r *EbitenRenderer) SetCursor(x, y int, visible bool, shape CursorShape)                {}
func (r *EbitenRenderer) SetPalette(palette *[256]uint32)                                    {}
func (r *EbitenRenderer) SetWindowTitle(title string)                                        {}
func (r *EbitenRenderer) ResizeWindow(cols, rows int)                                        {}
func (r *EbitenRenderer) Flush()                                                             {}

func (r *EbitenRenderer) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
}

// RunEbitenHost reports that this platform has no Ebitengine backend.
//
// The cut is not arbitrary: Ebitengine reaches the system through purego, and
// purego supports 64-bit Linux, Windows and macOS. On 32-bit ARM and on the
// remaining BSDs and Solaris there is no cgo-free path, and pulling in cgo is
// exactly what this backend exists to avoid.
func RunEbitenHost(cols, rows int, fontName string, fontSize float64, setupApp func()) error {
	return fmt.Errorf("ebiten backend is not supported on this platform")
}

package vtui

import (
	"unsafe"
)

// Win32 Window Styles
const (
	wsOverlapped       = 0x00000000
	wsPopup            = 0x80000000
	wsChild            = 0x40000000
	wsMinimize         = 0x20000000
	wsVisible          = 0x10000000
	wsDisabled         = 0x08000000
	wsClipSiblings     = 0x04000000
	wsClipChildren     = 0x02000000
	wsMaximize         = 0x01000000
	wsCaption          = 0x00C00000
	wsBorder           = 0x00800000
	wsDlgFrame         = 0x00400000
	wsVScroll          = 0x00200000
	wsHScroll          = 0x00100000
	wsSysMenu          = 0x00080000
	wsThickFrame       = 0x00040000
	wsGroup            = 0x00020000
	wsTabStop          = 0x00010000
	wsMinimizeBox      = 0x00020000
	wsMaximizeBox      = 0x00010000
	wsOverlappedWindow = wsOverlapped | wsCaption | wsSysMenu | wsThickFrame | wsMinimizeBox | wsMaximizeBox
)

// Win32 Extended Window Styles
const (
	wsExAcceptFiles = 0x00000010
	wsExAppWindow   = 0x00040000
)

// Win32 Class Styles
const (
	csVRedraw = 0x0001
	csHRedraw = 0x0002
	csDblClks = 0x0008
)

// Win32 Window Messages
const (
	wmDestroy         = 0x0002
	wmSize            = 0x0005
	wmSetFocus        = 0x0007
	wmKillFocus       = 0x0008
	wmInputLangChange = 0x0051
	wmPaint           = 0x000F
	wmClose           = 0x0010
	wmQuit            = 0x0012
	wmEraseBkgnd      = 0x0014
	wmSetCursor       = 0x0020
	wmDropFiles       = 0x0233
	wmKeyDown         = 0x0100
	wmKeyUp           = 0x0101
	wmChar            = 0x0102
	wmSysKeyDown      = 0x0104
	wmSysKeyUp        = 0x0105
	wmSysChar         = 0x0106
	wmLButtonDown     = 0x0201
	wmLButtonUp       = 0x0202
	wmLButtonDblClk   = 0x0203
	wmRButtonDown     = 0x0204
	wmRButtonUp       = 0x0205
	wmRButtonDblClk   = 0x0206
	wmMButtonDown     = 0x0207
	wmMButtonUp       = 0x0208
	wmMButtonDblClk   = 0x0209
	wmMouseMove       = 0x0200
	wmMouseWheel      = 0x020A
	wmPerformDragDrop = 0x0400 + 101
	wmPerformResize   = wmPerformDragDrop + 1
	wmSysCommand      = 0x0112
)

// WM_SYSCOMMAND commands.
const (
	scMaximize = 0xF030
	scRestore  = 0xF120
)

const (
	swpNoMove     = 0x0002
	swpNoSize     = 0x0001
	swpNoZOrder   = 0x0004
	swpNoActivate = 0x0010
)

// Win32 DIB and GDI Constants
const (
	biRGB        = 0
	dibRGBColors = 0
	srcCopy      = 0x00CC0020
)

type win32BitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type win32BitmapInfo struct {
	bmiHeader win32BitmapInfoHeader
	bmiColors [1]uint32
}

func makeTopDownDIBInfo(w, h int) win32BitmapInfo {
	return win32BitmapInfo{
		bmiHeader: win32BitmapInfoHeader{
			biSize:        uint32(unsafe.Sizeof(win32BitmapInfoHeader{})),
			biWidth:       int32(w),
			biHeight:      -int32(h), // negative height indicates top-down DIB
			biPlanes:      1,
			biBitCount:    32,
			biCompression: biRGB,
		},
	}
}

// rgbaToBGRA converts standard Go RGBA pixel rows to 32-bit Win32 BGRA pixels.
func rgbaToBGRA(dst, src []byte, lineBytes int) {
	for i := 0; i+3 < lineBytes && i+3 < len(src) && i+3 < len(dst); i += 4 {
		dst[i] = src[i+2]   // B
		dst[i+1] = src[i+1] // G
		dst[i+2] = src[i]   // R
		dst[i+3] = 255      // A
	}
}

// pixelRect is a half-open pixel rectangle [x0,x1) x [y0,y1).
type pixelRect struct {
	x0, y0, x1, y1 int
}

// frameMarginRects returns the parts of a clientW x clientH client area that a
// frameW x frameH frame blitted at the origin leaves uncovered: the partial
// cell column on the right and the partial cell row at the bottom when the
// window size is not a whole number of cells, or a wider band while the
// frame still has the size of a smaller grid.
//
// Nothing else paints those pixels (WM_ERASEBKGND is suppressed once a frame
// has been shown), so whatever an earlier paint left there -- for instance a
// frame composed for the maximized size and blitted, clipped, right after the
// window was restored -- stays visible until they are cleared (f4 #283).
func frameMarginRects(clientW, clientH, frameW, frameH int) []pixelRect {
	if clientW <= 0 || clientH <= 0 {
		return nil
	}
	frameW = max(frameW, 0)
	frameH = max(frameH, 0)
	var rects []pixelRect
	if frameW < clientW {
		rects = append(rects, pixelRect{frameW, 0, clientW, clientH})
	}
	if frameH < clientH {
		rects = append(rects, pixelRect{0, frameH, min(frameW, clientW), clientH})
	}
	return rects
}

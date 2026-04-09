package vtui

// ProgressBar displays a completion percentage using block characters.
type ProgressBar struct {
	ScreenObject
	Percent int
}

func NewProgressBar(x, y, w int) *ProgressBar {
	pb := &ProgressBar{}
	pb.SetPosition(x, y, x+w-1, y)
	return pb
}

func (pb *ProgressBar) SetPercent(p int) {
	if p < 0 { p = 0 }
	if p > 100 { p = 100 }
	pb.Percent = p
}

func (pb *ProgressBar) Show(scr *ScreenBuf) {
	pb.ScreenObject.Show(scr)
	pb.DisplayObject(scr)
}

func (pb *ProgressBar) DisplayObject(scr *ScreenBuf) {
	if !pb.IsVisible() { return }

	width := pb.X2 - pb.X1 + 1
	if width <= 0 { return }

	fillW := (pb.Percent * width) / 100

	// Use Dialog Edit colors for the bar itself (usually Cyan)
	// and Dialog Text for the background.
	bgAttr := Palette[ColDialogText]
	fillAttr := Palette[ColDialogEdit]
	if pb.IsDisabled() {
		bgAttr = DimColor(bgAttr)
		fillAttr = DimColor(fillAttr)
	}

	// 1. Draw filled part (Full blocks)
	if fillW > 0 {
		scr.FillRect(pb.X1, pb.Y1, pb.X1+fillW-1, pb.Y1, '█', fillAttr)
	}

	// 2. Draw remaining part (Light shade)
	if fillW < width {
		scr.FillRect(pb.X1+fillW, pb.Y1, pb.X2, pb.Y1, '░', bgAttr)
	}
}
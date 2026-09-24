package vtui

import "testing"

func TestAutoLayoutPinEdges(t *testing.T) {
	useAutoLayoutTestScreen(t)
	label := NewLabel(0, 0, "edges", nil)
	layout := NewAutoLayout(10, 5, 30, 12)
	layout.PinEdges(label, Margins{Left: 2, Top: 1, Right: 3, Bottom: 2}).Apply()
	x1, y1, x2, y2 := label.GetPosition()
	if x1 != 12 || y1 != 6 || x2 != 36 || y2 != 14 {
		t.Fatalf("position = (%d,%d)-(%d,%d)", x1, y1, x2, y2)
	}
}

func TestAutoLayoutFillHeight(t *testing.T) {
	useAutoLayoutTestScreen(t)
	label := NewLabel(0, 0, "height", nil)
	layout := NewAutoLayout(4, 3, 20, 10)
	layout.FillHeight(label, 2, 1).PinLeft(label, 0).Apply()
	_, y1, _, y2 := label.GetPosition()
	if y1 != 5 || y2 != 11 {
		t.Fatalf("vertical position = %d-%d", y1, y2)
	}
}

func TestAutoLayoutAlignLeft(t *testing.T) {
	useAutoLayoutTestScreen(t)
	a := NewLabel(0, 0, "a", nil)
	b := NewLabel(8, 0, "b", nil)
	layout := NewAutoLayout(0, 0, 30, 10)
	layout.AlignLeft(a, b).Apply()
	if a.X1 != b.X1 {
		t.Fatalf("left edges = %d and %d", a.X1, b.X1)
	}
}

func TestAutoLayoutAlignRight(t *testing.T) {
	useAutoLayoutTestScreen(t)
	a := NewLabel(0, 0, "long", nil)
	b := NewLabel(8, 0, "b", nil)
	layout := NewAutoLayout(0, 0, 30, 10)
	layout.AlignRight(a, b).Apply()
	if a.X2 != b.X2 {
		t.Fatalf("right edges = %d and %d", a.X2, b.X2)
	}
}

func TestAutoLayoutAlignBottom(t *testing.T) {
	useAutoLayoutTestScreen(t)
	a := NewLabel(0, 0, "a", nil)
	b := NewLabel(0, 5, "b", nil)
	layout := NewAutoLayout(0, 0, 30, 10)
	layout.AlignBottom(a, b).Apply()
	if a.Y2 != b.Y2 {
		t.Fatalf("bottom edges = %d and %d", a.Y2, b.Y2)
	}
}

func TestAutoLayoutSameWidth(t *testing.T) {
	useAutoLayoutTestScreen(t)
	a := NewLabel(0, 0, "wide", nil)
	b := NewLabel(0, 0, "b", nil)
	layout := NewAutoLayout(0, 0, 30, 10)
	layout.SameWidth(a, b).Apply()
	if a.X2-a.X1 != b.X2-b.X1 {
		t.Fatalf("widths = %d and %d", a.X2-a.X1, b.X2-b.X1)
	}
}

func TestAutoLayoutSameHeight(t *testing.T) {
	useAutoLayoutTestScreen(t)
	a := NewLabel(0, 0, "a", nil)
	b := NewLabel(0, 0, "b", nil)
	layout := NewAutoLayout(0, 0, 30, 10)
	layout.SameHeight(a, b).Apply()
	if a.Y2-a.Y1 != b.Y2-b.Y1 {
		t.Fatalf("heights = %d and %d", a.Y2-a.Y1, b.Y2-b.Y1)
	}
}

func TestAutoLayoutCenterVertical(t *testing.T) {
	useAutoLayoutTestScreen(t)
	label := NewLabel(0, 0, "center", nil)
	layout := NewAutoLayout(0, 0, 20, 10)
	layout.CenterVertical(label).Apply()
	if label.Y1+label.Y2 != layout.Y1+layout.Y2 {
		t.Fatalf("not centered: label %d+%d, bounds %d+%d", label.Y1, label.Y2, layout.Y1, layout.Y2)
	}
}

func TestAutoLayoutSetMinHeight(t *testing.T) {
	useAutoLayoutTestScreen(t)
	label := NewLabel(0, 0, "min", nil)
	layout := NewAutoLayout(0, 0, 20, 10)
	layout.SetMinHeight(label, 4).PinTop(label, 0).Apply()
	if got := label.Y2 - label.Y1 + 1; got < 4 {
		t.Fatalf("height = %d, want at least 4", got)
	}
}

func TestAutoLayoutApportionHeights(t *testing.T) {
	useAutoLayoutTestScreen(t)
	a := NewLabel(0, 0, "a", nil)
	b := NewLabel(0, 0, "b", nil)
	c := NewLabel(0, 0, "c", nil)
	layout := NewAutoLayout(0, 0, 20, 12)
	layout.PinTop(a, 0).StackVertical(0, a, b, c).ApportionHeights(12, a, b, c).Apply()
	total := a.Y2 - a.Y1 + 1 + b.Y2 - b.Y1 + 1 + c.Y2 - c.Y1 + 1
	if total != 12 {
		t.Fatalf("height total = %d, want 12", total)
	}
}

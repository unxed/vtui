package vtui

import (
	"github.com/unxed/vtinput"
	"time"
)

// MouseClickType represents the type of mouse click event.
type MouseClickType int

const (
	MouseClickNone MouseClickType = iota
	MouseClickLeft
	MouseClickMiddle
	MouseClickRight
	MouseClickDoubleLeft
	MouseClickDoubleRight
)

// MouseDragState tracks the state of a mouse drag operation.
type MouseDragState struct {
	// IsActive indicates whether a drag operation is in progress.
	IsActive bool
	// StartX and StartY are the coordinates where the drag started.
	StartX, StartY int
	// CurrentX and CurrentY are the current mouse coordinates during the drag.
	CurrentX, CurrentY int
	// Button indicates which button initiated the drag (FromLeft1stButtonPressed, etc.).
	Button uint32
	// StartTime is when the drag operation began.
	StartTime time.Time
}

// MouseWheelDirection represents the direction of mouse wheel movement.
type MouseWheelDirection int

const (
	MouseWheelNone MouseWheelDirection = iota
	MouseWheelUp
	MouseWheelDown
)

// MouseHandler provides utilities for processing mouse events with click,
// drag-select, and wheel support. It coordinates with golang.org/x/term
// for terminal state management.
type MouseHandler struct {
	// LastClickTime tracks the time of the last mouse click for double-click detection.
	LastClickTime time.Time
	// LastClickX and LastClickY track the position of the last click.
	LastClickX, LastClickY int
	// LastClickButton tracks which button was last clicked.
	LastClickButton uint32
	// ClickCount increments for consecutive clicks in the same location.
	ClickCount int
	// DoubleClickThreshold is the maximum time (in milliseconds) between clicks
	// to be considered a double-click. Default is 300ms.
	DoubleClickThreshold time.Duration
	// DoubleClickDistance is the maximum pixel distance for clicks to be
	// considered at the same location for double-click purposes. Default is 3 pixels.
	DoubleClickDistance int

	// DragState tracks the current drag operation, if any.
	DragState *MouseDragState
	// DragStartThreshold is the minimum distance (in pixels) the mouse must move
	// before a drag operation is initiated. Default is 2 pixels.
	DragStartThreshold int
	// IsDragging is set to true once the drag threshold is exceeded.
	IsDragging bool

	// WheelScrollLines specifies how many lines to scroll per wheel event.
	// Default is 3 lines per wheel tick.
	WheelScrollLines int

	// CurrentMouseX and CurrentMouseY track the current mouse position.
	CurrentMouseX, CurrentMouseY int
	// MousePositionKnown indicates whether the mouse position has been established.
	MousePositionKnown bool
}

// NewMouseHandler creates a new MouseHandler with sensible defaults.
func NewMouseHandler() *MouseHandler {
	return &MouseHandler{
		DoubleClickThreshold: 300 * time.Millisecond,
		DoubleClickDistance:  3,
		DragStartThreshold:   2,
		WheelScrollLines:     3,
		DragState:            &MouseDragState{},
	}
}

// GetClickType analyzes a mouse event and returns the click type.
// This method integrates with golang.org/x/term by tracking state
// across raw terminal input events.
func (mh *MouseHandler) GetClickType(e *vtinput.InputEvent) MouseClickType {
	if e.Type != vtinput.MouseEventType || e.ButtonState == 0 || !IsMousePress(e) {
		return MouseClickNone
	}

	mx, my := int(e.MouseX), int(e.MouseY)
	now := time.Now()
	dx := mx - mh.LastClickX
	dy := my - mh.LastClickY

	// Check if this is a double-click
	if mh.LastClickButton == e.ButtonState &&
		(now.Sub(mh.LastClickTime)) <= mh.DoubleClickThreshold &&
		dx*dx+dy*dy <= mh.DoubleClickDistance*mh.DoubleClickDistance {
		mh.ClickCount++
	} else {
		mh.ClickCount = 1
	}

	mh.LastClickTime = now
	mh.LastClickX = mx
	mh.LastClickY = my
	mh.LastClickButton = e.ButtonState

	if mh.ClickCount >= 2 {
		mh.ClickCount = 0 // Reset for next potential triple-click
		switch e.ButtonState {
		case vtinput.FromLeft1stButtonPressed:
			return MouseClickDoubleLeft
		case vtinput.RightmostButtonPressed:
			return MouseClickDoubleRight
		}
	}

	switch e.ButtonState {
	case vtinput.FromLeft1stButtonPressed:
		return MouseClickLeft
	case vtinput.FromLeft2ndButtonPressed:
		return MouseClickMiddle
	case vtinput.RightmostButtonPressed:
		return MouseClickRight
	}

	return MouseClickNone
}

// BeginDrag initiates a drag operation at the given coordinates.
func (mh *MouseHandler) BeginDrag(x, y int, button uint32) {
	mh.DragState.IsActive = true
	mh.DragState.StartX = x
	mh.DragState.StartY = y
	mh.DragState.CurrentX = x
	mh.DragState.CurrentY = y
	mh.DragState.Button = button
	mh.DragState.StartTime = time.Now()
	mh.IsDragging = false
}

// UpdateDrag updates the current drag position and returns true if the
// drag threshold has been exceeded (drag has begun).
func (mh *MouseHandler) UpdateDrag(x, y int) bool {
	if !mh.DragState.IsActive {
		return false
	}

	mh.DragState.CurrentX = x
	mh.DragState.CurrentY = y

	if !mh.IsDragging {
		dx := x - mh.DragState.StartX
		dy := y - mh.DragState.StartY
		distance := dx*dx + dy*dy
		threshold := mh.DragStartThreshold * mh.DragStartThreshold

		if distance >= threshold {
			mh.IsDragging = true
		}
	}

	return mh.IsDragging
}

// EndDrag terminates the drag operation.
func (mh *MouseHandler) EndDrag() {
	mh.DragState.IsActive = false
	mh.IsDragging = false
}

// GetDragSelection returns the selection coordinates for the current drag,
// normalized to ensure x1 <= x2 and y1 <= y2.
func (mh *MouseHandler) GetDragSelection() (x1, y1, x2, y2 int) {
	if !mh.DragState.IsActive {
		return 0, 0, 0, 0
	}

	x1, x2 = mh.DragState.StartX, mh.DragState.CurrentX
	y1, y2 = mh.DragState.StartY, mh.DragState.CurrentY

	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}

	return x1, y1, x2, y2
}

// GetWheelDirection returns the wheel scroll direction from a mouse event.
func (mh *MouseHandler) GetWheelDirection(e *vtinput.InputEvent) MouseWheelDirection {
	if e.Type != vtinput.MouseEventType {
		return MouseWheelNone
	}

	switch {
	case e.WheelDirection > 0:
		return MouseWheelUp
	case e.WheelDirection < 0:
		return MouseWheelDown
	default:
		return MouseWheelNone
	}
}

// GetWheelLines returns the number of lines to scroll based on the wheel event
// and the configured WheelScrollLines setting.
func (mh *MouseHandler) GetWheelLines(e *vtinput.InputEvent) int {
	if e.Type != vtinput.MouseEventType {
		return 0
	}

	if e.WheelDirection > 0 {
		return mh.WheelScrollLines
	} else if e.WheelDirection < 0 {
		return -mh.WheelScrollLines
	}
	return 0
}

// UpdateMousePosition updates the current mouse position from an event.
func (mh *MouseHandler) UpdateMousePosition(e *vtinput.InputEvent) {
	if e.Type == vtinput.MouseEventType {
		mh.CurrentMouseX = int(e.MouseX)
		mh.CurrentMouseY = int(e.MouseY)
		mh.MousePositionKnown = true
	}
}

// IsClickInBounds checks if a click occurred within the specified bounds.
func (mh *MouseHandler) IsClickInBounds(x1, y1, x2, y2 int) bool {
	if !mh.MousePositionKnown {
		return false
	}
	return mh.CurrentMouseX >= x1 && mh.CurrentMouseX <= x2 &&
		mh.CurrentMouseY >= y1 && mh.CurrentMouseY <= y2
}

// IsDragInBounds checks if the current drag is within the specified bounds.
func (mh *MouseHandler) IsDragInBounds(x1, y1, x2, y2 int) bool {
	if !mh.IsDragging {
		return false
	}
	sx1, sy1, sx2, sy2 := mh.GetDragSelection()
	return !(sx2 < x1 || sx1 > x2 || sy2 < y1 || sy1 > y2)
}

// ResetClickState clears the click detection state without affecting drag state.
func (mh *MouseHandler) ResetClickState() {
	mh.ClickCount = 0
	mh.LastClickButton = 0
	mh.LastClickTime = time.Time{}
}

// ResetDragState clears the drag state without affecting click detection.
func (mh *MouseHandler) ResetDragState() {
	mh.EndDrag()
	mh.DragState = &MouseDragState{}
}

// Reset clears all mouse state.
func (mh *MouseHandler) Reset() {
	mh.ResetClickState()
	mh.ResetDragState()
	mh.MousePositionKnown = false
}

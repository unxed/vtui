# Mouse Support in vtui

This document describes the comprehensive mouse event handling system in vtui, including click detection, drag-select functionality, wheel scrolling, and integration with `golang.org/x/term`.

## Overview

vtui provides a multi-layered mouse event handling system:

1. **Low-level Event Helpers** (`mouse_gesture.go`): Fast predicates for classifying raw mouse events
2. **MouseHandler** (`mouse_handler.go`): Stateful event analyzer for clicks, drags, and wheel events
3. **Terminal Integration** (`mouse_terminal_integration.go`): Coordination with `golang.org/x/term` for raw mode

## Architecture

### Raw Event Helpers

The `mouse_gesture.go` module provides pure predicates that classify individual mouse events:

```go
// Check if an event is a new mouse button press (not motion, not wheel)
if IsMousePress(event) { ... }

// Check if an event is a mouse button release
if IsMouseRelease(event) { ... }

// Check for specific button presses
if IsLeftMouseButton(event) { ... }
if IsMiddleMouseButton(event) { ... }
if IsRightMouseButton(event) { ... }

// Check for wheel events
if IsMouseWheelEvent(event) { ... }

// Check for double-clicks (in raw protocol flags)
if IsDoubleClick(event) { ... }

// Get coordinates or test bounds
x, y := GetMouseCoordinates(event)
if IsMouseEventInBounds(event, x1, y1, x2, y2) { ... }
```

### MouseHandler

The `MouseHandler` provides stateful analysis of mouse events across multiple calls. It tracks:

- **Click state**: Detects single/double clicks with configurable timing and distance thresholds
- **Drag state**: Tracks drag operations from press through motion to release
- **Wheel events**: Maps wheel direction to scroll line counts
- **Mouse position**: Maintains current known position

#### Click Detection

```go
handler := NewMouseHandler()

// Configure double-click detection
handler.DoubleClickThreshold = 300 * time.Millisecond  // Default
handler.DoubleClickDistance = 3                         // pixels, default

// Analyze clicks
clickType := handler.GetClickType(event)
switch clickType {
case MouseClickLeft:
    // Handle left click
case MouseClickMiddle:
    // Handle middle click
case MouseClickRight:
    // Handle right click
case MouseClickDoubleLeft:
    // Handle double left click
}
```

#### Drag-Select Operations

```go
// Start a drag when the user presses a button
handler.BeginDrag(int(event.MouseX), int(event.MouseY), event.ButtonState)

// Track the drag as the mouse moves
isDragging := handler.UpdateDrag(int(event.MouseX), int(event.MouseY))

// When truly dragging (exceeded threshold), get the selection
if isDragging {
    x1, y1, x2, y2 := handler.GetDragSelection()
    // Selection is normalized: x1 <= x2, y1 <= y2
}

// End the drag on mouse release
if IsMouseRelease(event) {
    handler.EndDrag()
}
```

The handler automatically manages a configurable drag-start threshold:

```go
handler.DragStartThreshold = 2  // pixels, default
// The drag doesn't begin until the mouse moves this many pixels from start
```

#### Wheel Scrolling

```go
// Get the wheel direction and amount
lines := handler.GetWheelLines(event)  // ±WheelScrollLines
if lines < 0 {
    // Scroll up
    listbox.PageUp()
} else if lines > 0 {
    // Scroll down
    listbox.PageDown()
}

// Or check direction explicitly
dir := handler.GetWheelDirection(event)
switch dir {
case MouseWheelUp:
    // Scroll up
case MouseWheelDown:
    // Scroll down
}
```

### Terminal Integration

The `MouseTerminalIntegration` layer coordinates with `golang.org/x/term` for terminal state management:

```go
mti := NewMouseTerminalIntegration()

// Check if running in a terminal
if !mti.IsTerminal() {
    return errors.New("not a terminal")
}

// Enable raw mode (uses term.MakeRaw)
if err := mti.EnableRawMode(); err != nil {
    return err
}
defer mti.DisableRawMode()

// Use the handler through the integration layer
handler := mti.GetMouseHandler()

// Process mouse events...
```

## Typical Usage Patterns

### Pattern 1: Simple Click Handling in a Widget

```go
func (w *MyWidget) ProcessMouse(e *vtinput.InputEvent) bool {
    if !w.HitTest(int(e.MouseX), int(e.MouseY)) {
        return false
    }

    if IsMousePress(e) {
        if IsLeftMouseButton(e) {
            w.OnClick()
            return true
        }
    }
    return false
}
```

### Pattern 2: Drag-Select in a Text Editor

```go
type TextEditor struct {
    mouseHandler *MouseHandler
    selectionX1, selectionY1, selectionX2, selectionY2 int
    isDragging bool
}

func (te *TextEditor) ProcessMouse(e *vtinput.InputEvent) bool {
    te.mouseHandler.UpdateMousePosition(e)

    if IsMousePress(e) && IsLeftMouseButton(e) {
        te.mouseHandler.BeginDrag(int(e.MouseX), int(e.MouseY), e.ButtonState)
        return true
    }

    if IsMouseMotion(e) && te.mouseHandler.DragState.IsActive {
        if te.mouseHandler.UpdateDrag(int(e.MouseX), int(e.MouseY)) {
            te.isDragging = true
            x1, y1, x2, y2 := te.mouseHandler.GetDragSelection()
            te.SelectText(x1, y1, x2, y2)
            return true
        }
    }

    if IsMouseRelease(e) && te.isDragging {
        te.mouseHandler.EndDrag()
        te.isDragging = false
        return true
    }

    return false
}
```

### Pattern 3: Scroll Wheel in a ListBox

```go
func (lb *ListBox) ProcessMouse(e *vtinput.InputEvent) bool {
    if IsMouseWheelEvent(e) {
        lines := lb.mouseHandler.GetWheelLines(e)
        if lines > 0 {
            lb.ScrollDown(lines)
        } else if lines < 0 {
            lb.ScrollUp(-lines)
        }
        return true
    }
    // ... other mouse handling
}
```

## Configuration

### MouseHandler Defaults

- `DoubleClickThreshold`: 300 milliseconds
- `DoubleClickDistance`: 3 pixels
- `DragStartThreshold`: 2 pixels
- `WheelScrollLines`: 3 lines per wheel tick

All of these can be customized:

```go
handler := NewMouseHandler()
handler.DoubleClickThreshold = 200 * time.Millisecond
handler.WheelScrollLines = 5
```

## Terminal Raw Mode

When using `MouseTerminalIntegration`, ensure the proper initialization order:

1. Create the integration
2. Check `IsTerminal()`
3. Enable raw mode with `EnableRawMode()`
4. Initialize FrameManager (which calls `vtinput.EnableProtocols()`)
5. Process events
6. Disable raw mode on shutdown

The integration layer documents how `golang.org/x/term`'s raw mode APIs (`term.MakeRaw`, `term.Restore`) work with vtui's input protocol setup (handled by `vtinput.EnableProtocols()` in the terminal initialization sequence).

## Testing

Run the mouse-related tests:

```bash
go test -v -run "Mouse"
```

Key test suites:
- `TestMouseHandler_*`: MouseHandler functionality
- `TestMouseGestureHelper*`: Raw event helpers
- `TestMouseTerminal*`: Terminal integration
- Existing gesture tests in `mouse_gesture_test.go`

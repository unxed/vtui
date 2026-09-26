package vtui

import (
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

type fakeStatusBar struct {
	text      string
	activated int
}

func (b *fakeStatusBar) Label(maxW int) string { return b.text }
func (b *fakeStatusBar) Activate()             { b.activated++ }

func keyEvent(vk uint16) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
}

func TestChatWindow_Layout(t *testing.T) {
	cw := NewChatWindow(0, 0, 79, 23, "")
	if cw.Input.Y1 != 19 || cw.Input.Y2 != 22 {
		t.Fatalf("expected input rows 19-22 in a tall window, got %d-%d", cw.Input.Y1, cw.Input.Y2)
	}

	short := NewChatWindow(0, 0, 79, 8, "")
	if short.Input.Y1 != 6 || short.Input.Y2 != 7 {
		t.Fatalf("expected a 2-row input in a short window, got %d-%d", short.Input.Y1, short.Input.Y2)
	}
}

func TestChatWindow_UnfocusedIgnoresKeys(t *testing.T) {
	cw := NewChatWindow(0, 0, 79, 23, "")
	if cw.ProcessKey(keyEvent(vtinput.VK_DOWN)) {
		t.Fatal("an unfocused ChatWindow should not consume keys")
	}
}

func TestChatWindow_TabPassesThrough(t *testing.T) {
	cw := NewChatWindow(0, 0, 79, 23, "")
	cw.SetFocus(true)
	if cw.ProcessKey(keyEvent(vtinput.VK_TAB)) {
		t.Fatal("ChatWindow should not consume Tab, so a host can switch focus elsewhere")
	}
}

func TestChatWindow_EnterSendsAndClearsInput(t *testing.T) {
	cw := NewChatWindow(0, 0, 79, 23, "")
	cw.SetFocus(true)
	cw.Input.SetText("hello there")

	var sent string
	cw.OnSend = func(text string) { sent = text }

	if !cw.ProcessKey(keyEvent(vtinput.VK_RETURN)) {
		t.Fatal("Enter should be handled")
	}
	if sent != "hello there" {
		t.Fatalf("OnSend got %q", sent)
	}
	if cw.Input.GetText() != "" {
		t.Fatalf("input should be cleared after sending, got %q", cw.Input.GetText())
	}
}

func TestChatWindow_EnterOnBlankInputDoesNotSend(t *testing.T) {
	cw := NewChatWindow(0, 0, 79, 23, "")
	cw.SetFocus(true)
	cw.Input.SetText("   ")

	called := false
	cw.OnSend = func(string) { called = true }
	cw.ProcessKey(keyEvent(vtinput.VK_RETURN))
	if called {
		t.Fatal("OnSend should not fire for blank/whitespace-only input")
	}
}

func TestChatWindow_ShiftEnterInsertsNewline(t *testing.T) {
	cw := NewChatWindow(0, 0, 79, 23, "")
	cw.SetFocus(true)
	cw.Input.SetText("line one")
	cw.Input.SetCursorPos(0, len("line one"))

	called := false
	cw.OnSend = func(string) { called = true }

	e := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_RETURN, ControlKeyState: vtinput.ShiftPressed,
	}
	if !cw.ProcessKey(e) {
		t.Fatal("Shift+Enter should be handled")
	}
	if called {
		t.Fatal("Shift+Enter must not send")
	}
	if cw.Input.LineCount() != 2 {
		t.Fatalf("Shift+Enter should insert a newline, got %d lines", cw.Input.LineCount())
	}
}

func TestChatWindow_StatusBarFocusNavigation(t *testing.T) {
	cw := NewChatWindow(0, 0, 79, 23, "")
	bar := &fakeStatusBar{text: "3 files attached"}
	cw.StatusBar = bar
	cw.SetFocus(true)

	if !cw.ProcessKey(keyEvent(vtinput.VK_UP)) {
		t.Fatal("Up from input row 0 should be handled")
	}
	if !cw.StatusBarFocused() {
		t.Fatal("expected the status bar to have focus")
	}

	if !cw.ProcessKey(keyEvent(vtinput.VK_DOWN)) {
		t.Fatal("Down from the status bar should be handled")
	}
	if cw.StatusBarFocused() {
		t.Fatal("expected focus back on the input box")
	}

	// Enter on the bar activates it.
	cw.ProcessKey(keyEvent(vtinput.VK_UP))
	if !cw.StatusBarFocused() {
		t.Fatal("expected the status bar to have focus again")
	}
	cw.ProcessKey(keyEvent(vtinput.VK_RETURN))
	if bar.activated != 1 {
		t.Fatalf("Enter on the status bar should call Activate once, got %d", bar.activated)
	}
}

func TestChatWindow_StatusBarHidesWhenLabelEmpty(t *testing.T) {
	cw := NewChatWindow(0, 0, 79, 23, "")
	cw.StatusBar = &fakeStatusBar{text: ""}
	cw.SetFocus(true)

	cw.ProcessKey(keyEvent(vtinput.VK_UP))
	if cw.StatusBarFocused() {
		t.Fatal("an empty-label status bar should not take focus")
	}
}

func TestChatWindow_OnStatusBarKeyHook(t *testing.T) {
	cw := NewChatWindow(0, 0, 79, 23, "")
	cw.StatusBar = &fakeStatusBar{text: "apply patch"}
	cw.SetFocus(true)
	cw.ProcessKey(keyEvent(vtinput.VK_UP))

	var gotKey uint16
	cw.OnStatusBarKey = func(e *vtinput.InputEvent) bool {
		gotKey = e.VirtualKeyCode
		return e.VirtualKeyCode == vtinput.VK_F3
	}
	if !cw.ProcessKey(keyEvent(vtinput.VK_F3)) {
		t.Fatal("OnStatusBarKey should mark F3 as handled")
	}
	if gotKey != vtinput.VK_F3 {
		t.Fatalf("OnStatusBarKey did not see the key, got %d", gotKey)
	}
}

func TestChatWindow_RenderingAndLinkNavigation(t *testing.T) {
	cw := NewChatWindow(0, 0, 33, 23, "")
	cw.SelfLabel = "You"
	cw.PeerLabel = "Bot"
	cw.URLSchemes = []string{"ai://"}
	cw.Turns = []ChatTurn{
		{Role: ChatRoleSelf, Text: "hi", Time: time.Time{}},
		{Role: ChatRolePeer, Text: "See [context](ai://ctx/readme.txt) and ai://out/result.go too.", Time: time.Time{}},
	}
	cw.SetFocus(true)

	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	cw.Show(scr)

	visible := cw.VisibleLinks()
	if len(visible) == 0 {
		t.Fatal("Show should have collected visible links")
	}
	var targets []string
	for _, l := range visible {
		targets = append(targets, l.Target)
	}
	joined := strings.Join(targets, "\n")
	for _, want := range []string{"ai://ctx/readme.txt", "ai://out/result.go"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("visible links %v missing %q", targets, want)
		}
	}

	// Found "Bot" header rendered somewhere.
	foundHeader := false
	for y := 0; y < 25; y++ {
		if strings.Contains(ScreenRow(scr, y, 0, 33), "Bot") {
			foundHeader = true
			break
		}
	}
	if !foundHeader {
		t.Fatal("peer header label was not rendered")
	}

	// Move focus onto the first link from the input box.
	cw.Input.SetCursorPos(0, 0)
	if !cw.ProcessKey(keyEvent(vtinput.VK_UP)) {
		t.Fatal("Up from input row 0 should move focus onto a link")
	}
	if !cw.LinkFocused() {
		t.Fatal("expected a link to have focus")
	}

	var activated string
	cw.OnActivateLink = func(target string) { activated = target }
	cw.ProcessKey(keyEvent(vtinput.VK_RETURN))
	if activated == "" {
		t.Fatal("Enter on a focused link should call OnActivateLink")
	}

	var altTarget string
	cw.OnLinkAltKey = func(target string, e *vtinput.InputEvent) bool {
		altTarget = target
		return true
	}
	cw.ProcessKey(keyEvent(vtinput.VK_F5))
	if altTarget != activated {
		t.Fatalf("F5 should fire OnLinkAltKey for the focused link, got %q want %q", altTarget, activated)
	}

	if !cw.ProcessKey(keyEvent(vtinput.VK_ESCAPE)) {
		t.Fatal("Escape should be handled while a link has focus")
	}
	if cw.LinkFocused() {
		t.Fatal("Escape should return focus to the input box")
	}
}

func TestChatWindow_ExtraLink(t *testing.T) {
	cw := NewChatWindow(0, 0, 40, 23, "")
	cw.Turns = []ChatTurn{{Role: ChatRolePeer, Text: "```go:out.go\ncode\n```"}}
	cw.ExtraLink = func(line string) (label, target string, ok bool) {
		p := strings.TrimSpace(line)
		if !strings.HasPrefix(p, "```") {
			return "", "", false
		}
		colon := strings.Index(p, ":")
		if colon == -1 {
			return "", "", false
		}
		name := strings.TrimSpace(p[colon+1:])
		return "ai://out/" + name, "ai://out/" + name, true
	}
	cw.SetFocus(true)

	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	cw.Show(scr)

	found := false
	for _, l := range cw.visibleLinks {
		if l.Target == "ai://out/out.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ExtraLink synthetic link not found among %v", cw.visibleLinks)
	}
}

func TestChatWindow_MouseLinkActivationAndWheel(t *testing.T) {
	cw := NewChatWindow(0, 0, 40, 23, "")
	cw.SetFocus(true)
	cw.visibleLinks = []ChatLink{{Row: 1, Col: 1, Width: 5, Target: "ai://ctx/readme.txt"}}

	var activated string
	cw.OnActivateLink = func(target string) { activated = target }

	e := &vtinput.InputEvent{
		Type: vtinput.MouseEventType, KeyDown: true,
		MouseX: 2, MouseY: 2, ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.DoubleClick,
	}
	if !cw.ProcessMouse(e) {
		t.Fatal("mouse click on a link should be handled")
	}
	if !cw.LinkFocused() {
		t.Fatal("clicking a link should focus it")
	}
	if activated != "ai://ctx/readme.txt" {
		t.Fatalf("double-click should activate the link, got %q", activated)
	}

	wheelDown := vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1}
	if !cw.ProcessMouse(&wheelDown) {
		t.Fatal("wheel-down should be handled")
	}
	wheelUp := vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: 1}
	if !cw.ProcessMouse(&wheelUp) {
		t.Fatal("wheel-up should be handled")
	}
	if cw.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType}) {
		t.Fatal("an empty mouse event should not be handled")
	}
}

func TestChatWindow_PageAndSelectionKeys(t *testing.T) {
	cw := NewChatWindow(0, 0, 40, 23, "")
	cw.SetFocus(true)
	cw.Turns = make([]ChatTurn, 0)
	for i := 0; i < 40; i++ {
		cw.Turns = append(cw.Turns, ChatTurn{Role: ChatRolePeer, Text: "line"})
	}
	cw.updateLines()
	cw.topPos = 20

	if !cw.ProcessKey(keyEvent(vtinput.VK_PRIOR)) || cw.topPos < 0 {
		t.Fatal("PageUp was not handled correctly")
	}
	if !cw.ProcessKey(keyEvent(vtinput.VK_NEXT)) {
		t.Fatal("PageDown was not handled")
	}

	shiftLeft := &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_LEFT, ControlKeyState: vtinput.ShiftPressed,
	}
	if !cw.ProcessKey(shiftLeft) {
		t.Fatal("Shift+Left should be handled (passed through to Input)")
	}
}

func TestChatWindow_BusyLabel(t *testing.T) {
	cw := NewChatWindow(0, 0, 40, 23, "")
	cw.Busy = true
	cw.BusyLabel = "typing"
	cw.updateLines()

	found := false
	for _, l := range cw.lines {
		var sb strings.Builder
		for _, c := range l.cells {
			if c.Char != WideCharFiller {
				sb.WriteRune(rune(c.Char))
			}
		}
		if strings.Contains(sb.String(), "typing") {
			found = true
		}
	}
	if !found {
		t.Fatal("busy label was not appended to the rendered lines")
	}
}

func TestChatWindow_ScrollToBottom(t *testing.T) {
	cw := NewChatWindow(0, 0, 40, 23, "")
	for i := 0; i < 60; i++ {
		cw.Turns = append(cw.Turns, ChatTurn{Role: ChatRolePeer, Text: "line"})
	}
	cw.ScrollToBottom()
	h := cw.Input.Y1 - cw.Y1 - 2
	maxTop := len(cw.lines) - h
	if cw.topPos != maxTop {
		t.Fatalf("ScrollToBottom left topPos %d, want %d", cw.topPos, maxTop)
	}
}

func TestChatWindow_VisibleLinksIsACopy(t *testing.T) {
	cw := NewChatWindow(0, 0, 40, 23, "")
	cw.visibleLinks = []ChatLink{{Row: 0, Col: 0, Width: 1, Target: "a"}}

	got := cw.VisibleLinks()
	if len(got) != 1 || got[0].Target != "a" {
		t.Fatalf("VisibleLinks = %v", got)
	}
	got[0].Target = "mutated"
	if cw.visibleLinks[0].Target != "a" {
		t.Fatal("VisibleLinks should return a copy, not the internal slice")
	}
}

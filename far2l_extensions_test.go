package vtui

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestFar2lClipboard_Disabled(t *testing.T) {
	oldEnabled := Far2lEnabled
	t.Cleanup(func() { Far2lEnabled = oldEnabled })
	Far2lEnabled = false
	ok := SetFar2lClipboard("test")
	if ok {
		t.Error("SetFar2lClipboard should return false when Far2lEnabled is false")
	}

	_, ok = GetFar2lClipboard()
	if ok {
		t.Error("GetFar2lClipboard should return false when Far2lEnabled is false")
	}
}

func TestFar2lInteract_Timeout(t *testing.T) {
	oldEnabled, oldFM := Far2lEnabled, FrameManager
	t.Cleanup(func() {
		Far2lEnabled = oldEnabled
		FrameManager = oldFM
	})
	Far2lEnabled = true
	// Init minimal FrameManager
	FrameManager = &frameManager{}
	FrameManager.EventChan = make(chan *vtinput.InputEvent)
	// RedrawChan needs buffer to not block
	FrameManager.RedrawChan = make(chan struct{}, 1)

	stk := &vtinput.Far2lStack{}
	stk.PushU8('w') // request window size

	// Start interaction that waits for reply
	start := time.Now()
	// We wrap it in a goroutine because it blocks
	var reply *vtinput.Far2lStack
	done := make(chan bool)
	go func() {
		reply = Far2lInteract(stk, true)
		done <- true
	}()

	// Wait for timeout (we use short timeout in mock or just check time elapsed)
	select {
	case <-done:
		if reply != nil {
			t.Error("Expected nil reply on timeout")
		}
		if time.Since(start) < 100*time.Millisecond {
			t.Error("Timeout happened too fast")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Interaction did not timeout, it hung")
	}
}

func TestFar2lInteract_Success(t *testing.T) {
	oldEnabled, oldFM := Far2lEnabled, FrameManager
	oldID := far2lIDCounter.Load()
	t.Cleanup(func() {
		Far2lEnabled = oldEnabled
		FrameManager = oldFM
		far2lIDCounter.Store(oldID)
	})
	Far2lEnabled = true
	far2lIDCounter.Store(0)
	idToWait := uint8(0)

	// Mocking the interaction loop
	localFm := &frameManager{}
	localFm.EventChan = make(chan *vtinput.InputEvent, 1)
	localFm.injectedEvents = make([]*vtinput.InputEvent, 0)
	FrameManager = localFm

	stk := &vtinput.Far2lStack{}
	stk.PushU8('w')

	go func() {
		// Wait for the ID to be assigned by Far2lInteract
		for far2lIDCounter.Load() == 0 {
			time.Sleep(10 * time.Millisecond)
		}
		idToWait = uint8(far2lIDCounter.Load())

		// Prepare reply
		resp := vtinput.Far2lStack{}
		resp.PushU16(24) // height
		resp.PushU16(80) // width
		resp.PushU8(idToWait)

		localFm.EventChan <- &vtinput.InputEvent{
			Type:         vtinput.Far2lEventType,
			Far2lCommand: "reply",
			Far2lData:    resp,
		}
	}()

	// The event loop pumping is now handled automatically by WaitFar2lResponse.
	// We just call the interaction, and the background goroutine we started above
	// will push the reply into the EventChan, which WaitFar2lResponse will dispatch.
	reply := Far2lInteract(stk, true)

	if reply == nil {
		t.Fatal("Interaction failed to receive reply")
	}

	if w := reply.PopU16(); w != 80 {
		t.Errorf("Unexpected data in reply: %d", w)
	}
}

func TestFar2lInteract_NoEventStealing(t *testing.T) {
	oldEnabled, oldFM := Far2lEnabled, FrameManager
	oldID := far2lIDCounter.Load()
	t.Cleanup(func() {
		Far2lEnabled = oldEnabled
		FrameManager = oldFM
		far2lIDCounter.Store(oldID)
	})
	Far2lEnabled = true
	far2lIDCounter.Store(0)

	localFm := &frameManager{}
	localFm.Init(NewSilentScreenBuf())
	defer localFm.Shutdown()
	localFm.EventChan = make(chan *vtinput.InputEvent, 10)
	FrameManager = localFm

	// 1. Prepare a standard keypress event
	kp := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_A,
		Char:           'a',
	}

	// Track dispatching via a mock frame. The frame runs on the goroutine
	// pumping WaitFar2lResponse, so the flag is atomic.
	var received atomic.Bool
	frame := &mockFrame{
		onProcessKey: func(e *vtinput.InputEvent) bool {
			if e == kp {
				received.Store(true)
			}
			return true
		},
	}
	localFm.Push(frame)

	stk := &vtinput.Far2lStack{}
	stk.PushU8('w')

	// 2. Queue a keypress, then start Far2lInteract, which blocks in
	// WaitFar2lResponse and has to pump that keypress while it waits.
	localFm.EventChan <- kp
	var reply *vtinput.Far2lStack
	done := make(chan struct{})
	go func() {
		reply = Far2lInteract(stk, true)
		close(done)
	}()

	// 3. Wait -- on the conditions themselves, not on sleeps a slow runner
	// can outlast -- until the waiter is registered and the keypress has
	// been dispatched, and only then let the reply arrive.
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !cond() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			time.Sleep(time.Millisecond)
		}
	}
	idToWait := uint8(0)
	waitFor("the far2l waiter to register", func() bool {
		localFm.far2lMu.Lock()
		defer localFm.far2lMu.Unlock()
		for id := range localFm.pendingFar2l {
			idToWait = id
			return true
		}
		return false
	})
	waitFor("the keypress to be dispatched", received.Load)

	resp := vtinput.Far2lStack{}
	resp.PushU16(24) // height
	resp.PushU16(80) // width
	resp.PushU8(idToWait)

	// Queued rather than dispatched from here, so the waiting goroutine
	// dispatches it itself, as it would a real reply.
	localFm.EventChan <- &vtinput.InputEvent{
		Type:         vtinput.Far2lEventType,
		Far2lCommand: "reply",
		Far2lData:    resp,
	}

	// 4. Wait for the interaction to complete
	select {
	case <-done:
		if reply == nil {
			t.Fatal("Interaction failed on valid reply")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Interaction timed out / hung")
	}
}
func TestIssue117_WaitFar2lResponse_TaskPumping(t *testing.T) {
	oldFM := FrameManager
	t.Cleanup(func() { FrameManager = oldFM })
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()
	FrameManager = fm

	taskExecuted := false
	fm.PostTask(func() {
		taskExecuted = true
	})

	// Simulate the reply coming in asynchronously after some task pumping
	go func() {
		time.Sleep(50 * time.Millisecond)
		resp := vtinput.Far2lStack{}
		resp.PushU8(42) // ID
		fm.dispatchEvent(&vtinput.InputEvent{
			Type:         vtinput.Far2lEventType,
			Far2lCommand: "reply",
			Far2lData:    resp,
		}, false)
	}()

	// Wait with a 2-second timeout
	res := fm.WaitFar2lResponse(42, 2*time.Second)

	if res == nil {
		t.Fatal("Expected valid reply, got nil (timeout)")
	}
	if !taskExecuted {
		t.Error("Issue #117: WaitFar2lResponse failed to pump and execute background UI tasks!")
	}
}

func TestIssue117_WaitFar2lResponse_TimeoutCleanup(t *testing.T) {
	oldFM := FrameManager
	t.Cleanup(func() { FrameManager = oldFM })
	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	defer fm.Shutdown()
	FrameManager = fm

	// Wait for a non-existent reply with a short 50ms timeout
	res := fm.WaitFar2lResponse(99, 50*time.Millisecond)

	if res != nil {
		t.Fatalf("Expected nil reply on timeout, got %v", res)
	}

	// Verify that the waiter was safely cleaned up from the map
	fm.far2lMu.Lock()
	_, ok := fm.pendingFar2l[99]
	fm.far2lMu.Unlock()

	if ok {
		t.Error("Issue #117: WaitFar2lResponse failed to unregister the waiter after a timeout!")
	}

	// Simulate a very late reply arriving after the timeout cleanup.
	// It should be safely ignored and must NOT panic.
	resp := vtinput.Far2lStack{}
	resp.PushU8(99)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Late reply caused a panic after waiter cleanup: %v", r)
		}
	}()

	fm.dispatchEvent(&vtinput.InputEvent{
		Type:         vtinput.Far2lEventType,
		Far2lCommand: "reply",
		Far2lData:    resp,
	}, false)
}

// A terminal acknowledges the far2l extensions once, when the protocols are
// announced. Init must not put the manager back to "never negotiated": a
// second Init -- the session picker handing over to the main screen, a host
// that inits for its own buffer -- would otherwise leave the process without
// the extensions for the rest of the session, because no second
// acknowledgement is ever sent (f4#922).
func TestFar2lNegotiationSurvivesReInit(t *testing.T) {
	oldEnabled := Far2lEnabled
	Far2lEnabled = false
	t.Cleanup(func() { Far2lEnabled = oldEnabled })

	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	if far2lEnabledFor(fm) {
		t.Fatal("far2l must stay off until a terminal acknowledges it")
	}

	fm.dispatchEvent(&vtinput.InputEvent{Type: vtinput.Far2lEventType, Far2lCommand: "ok"}, false)
	if !far2lEnabledFor(fm) {
		t.Fatal("the acknowledgement did not enable far2l")
	}

	fm.Init(NewSilentScreenBuf())
	if !far2lEnabledFor(fm) {
		t.Fatal("a second Init dropped the negotiated far2l state")
	}
}

// The negotiated state must not follow the process into a native window: an
// APC request there waits out its whole timeout for a reply nobody will send.
func TestFar2lIsOffBehindANativeWindow(t *testing.T) {
	oldEnabled := Far2lEnabled
	Far2lEnabled = false
	t.Cleanup(func() { Far2lEnabled = oldEnabled })
	withTerminalClipboard(t, false)

	fm := &frameManager{}
	fm.Init(NewSilentScreenBuf())
	fm.dispatchEvent(&vtinput.InputEvent{Type: vtinput.Far2lEventType, Far2lCommand: "ok"}, false)
	if !far2lEnabledFor(fm) {
		t.Fatal("the acknowledgement did not enable far2l")
	}

	DisableTerminalClipboard()
	if far2lEnabledFor(fm) {
		t.Fatal("far2l stayed on with no terminal behind the application")
	}
}

// ResetFar2lNegotiation is the lever for a process that changes the terminal
// under itself, such as a session daemon adopting a new client's PTY.
func TestResetFar2lNegotiation(t *testing.T) {
	oldEnabled, oldFM := Far2lEnabled, FrameManager
	Far2lEnabled = false
	fm := &frameManager{}
	FrameManager = fm
	t.Cleanup(func() {
		Far2lEnabled = oldEnabled
		FrameManager = oldFM
	})

	fm.Init(NewSilentScreenBuf())
	fm.dispatchEvent(&vtinput.InputEvent{Type: vtinput.Far2lEventType, Far2lCommand: "ok"}, false)
	if !far2lEnabledFor(fm) {
		t.Fatal("the acknowledgement did not enable far2l")
	}

	ResetFar2lNegotiation()
	if far2lEnabledFor(fm) {
		t.Fatal("far2l survived an explicit reset")
	}

	fm.Init(NewSilentScreenBuf())
	if far2lEnabledFor(fm) {
		t.Fatal("Init brought far2l back after the negotiation was forgotten")
	}
}

// Far2lNegotiated is the public form of the acknowledgement: false with no
// FrameManager and before the terminal answered, true after, and false again
// after an explicit reset and behind a native window.
func TestFar2lNegotiated(t *testing.T) {
	oldEnabled, oldFM := Far2lEnabled, FrameManager
	Far2lEnabled = false
	t.Cleanup(func() {
		Far2lEnabled = oldEnabled
		FrameManager = oldFM
	})
	withTerminalClipboard(t, false)

	FrameManager = nil
	if Far2lNegotiated() {
		t.Fatal("negotiated with no FrameManager")
	}

	fm := &frameManager{}
	FrameManager = fm
	fm.Init(NewSilentScreenBuf())
	if Far2lNegotiated() {
		t.Fatal("negotiated before the terminal acknowledged the extensions")
	}

	fm.dispatchEvent(&vtinput.InputEvent{Type: vtinput.Far2lEventType, Far2lCommand: "ok"}, false)
	if !Far2lNegotiated() {
		t.Fatal("not negotiated after the acknowledgement")
	}

	ResetFar2lNegotiation()
	if Far2lNegotiated() {
		t.Fatal("negotiated after an explicit reset")
	}

	fm.dispatchEvent(&vtinput.InputEvent{Type: vtinput.Far2lEventType, Far2lCommand: "ok"}, false)
	DisableTerminalClipboard()
	if Far2lNegotiated() {
		t.Fatal("negotiated behind a native window")
	}
}

// ResetFar2lNegotiation is called from places that may run before any
// FrameManager was ever set up (e.g. a session daemon reacting to a signal
// early during startup); it must not panic on a nil manager.
func TestResetFar2lNegotiation_NilFrameManager(t *testing.T) {
	oldFM := FrameManager
	t.Cleanup(func() { FrameManager = oldFM })
	FrameManager = nil

	ResetFar2lNegotiation()
}

// far2lEnabledFor falls back to the global Far2lEnabled flag both when there
// is no FrameManager at all and when one exists but Init has never run on it
// (far2lConfigured stays false), not only for the negotiated/configured
// branches already covered above.
func TestFar2lEnabledFor_Table(t *testing.T) {
	oldEnabled := Far2lEnabled
	t.Cleanup(func() { Far2lEnabled = oldEnabled })
	withTerminalClipboard(t, false)

	cases := []struct {
		name    string
		fm      *frameManager
		enabled bool
		want    bool
	}{
		{"nil frame manager, global on", nil, true, true},
		{"nil frame manager, global off", nil, false, false},
		{"unconfigured frame manager, global on", &frameManager{}, true, true},
		{"unconfigured frame manager, global off", &frameManager{}, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Far2lEnabled = tc.enabled
			if got := far2lEnabledFor(tc.fm); got != tc.want {
				t.Errorf("far2lEnabledFor() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Far2lInteractTimeout is the exported entry point used by callers that need
// a timeout other than Far2lInteract's fixed 2s. With wait=false it must
// fire the request and return immediately without touching FrameManager.
func TestFar2lInteractTimeout_NoWait(t *testing.T) {
	oldFM := FrameManager
	t.Cleanup(func() { FrameManager = oldFM })
	FrameManager = nil

	stk := &vtinput.Far2lStack{}
	stk.PushU8('w')

	done := make(chan *vtinput.Far2lStack, 1)
	go func() { done <- Far2lInteractTimeout(stk, false, time.Second) }()

	select {
	case reply := <-done:
		if reply != nil {
			t.Errorf("expected a nil reply when wait is false, got %v", reply)
		}
	case <-time.After(time.Second):
		t.Fatal("Far2lInteractTimeout(wait=false) blocked instead of returning immediately")
	}
}

// far2lInteractionPayloadLocked assigns ascending IDs but must never hand out
// the reserved value 0: a uint8 counter wraps from 255 to 0 on every 256th
// call, and 0 is used elsewhere to mean "no ID".
func TestFar2lInteractionPayloadLocked_SkipsReservedZeroID(t *testing.T) {
	oldID := far2lIDCounter.Load()
	t.Cleanup(func() { far2lIDCounter.Store(oldID) })
	far2lIDCounter.Store(255) // the next Add(1) wraps a uint8 to 0

	stk := &vtinput.Far2lStack{}
	stk.PushU8('w')
	id, payload := far2lInteractionPayloadLocked(stk)

	if id == 0 {
		t.Error("far2lInteractionPayloadLocked must never hand out the reserved id 0")
	}
	if id != 1 {
		t.Errorf("got id %d, want 1 (256 wraps to 0, which is skipped for the next value)", id)
	}
	if len(payload) == 0 {
		t.Error("expected a non-empty APC payload")
	}
}

// waitForFar2lID blocks until far2lIDCounter moves past last and returns the
// id assigned to the newest far2l interaction. Tests use it to learn, without
// polling any fixed value, which id a concurrently running SetFar2lClipboard
// or GetFar2lClipboard call is now waiting for a reply on.
func waitForFar2lID(t *testing.T, last uint32) uint8 {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if v := far2lIDCounter.Load(); v != last {
			return uint8(v) //nolint:gosec // mirrors far2lInteractionPayloadLocked's intentional uint32->uint8 wraparound
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for a new far2l interaction id")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// sendFar2lReply delivers a mocked far2l reply for id through fm's event
// channel. build pushes the reply's payload in the same bottom-to-top order
// the real far2l host would (last-pushed is first-popped); the id itself is
// always pushed last, since dispatchEvent pops it first to route the reply.
func sendFar2lReply(fm *frameManager, id uint8, build func(*vtinput.Far2lStack)) {
	stk := &vtinput.Far2lStack{}
	build(stk)
	stk.PushU8(id)
	fm.EventChan <- &vtinput.InputEvent{
		Type:         vtinput.Far2lEventType,
		Far2lCommand: "reply",
		Far2lData:    *stk,
	}
}

// TestSetFar2lClipboard exercises the CLIP_OPEN/CLIP_SETDATA exchange that
// TestFar2lClipboard_Disabled does not reach, since it only ever sees
// Far2lEnabled false.
func TestSetFar2lClipboard(t *testing.T) {
	cases := []struct {
		name       string
		openStatus uint8
		setStatus  uint8 // only sent/checked when openStatus == 1
		wantOK     bool
	}{
		{name: "open and setdata succeed", openStatus: 1, setStatus: 1, wantOK: true},
		{name: "host rejects CLIP_OPEN", openStatus: 0, wantOK: false},
		{name: "host rejects CLIP_SETDATA", openStatus: 1, setStatus: 0, wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldEnabled, oldFM := Far2lEnabled, FrameManager
			oldID := far2lIDCounter.Load()
			t.Cleanup(func() {
				Far2lEnabled = oldEnabled
				FrameManager = oldFM
				far2lIDCounter.Store(oldID)
			})
			Far2lEnabled = true
			far2lIDCounter.Store(0)
			withTerminalClipboard(t, false)

			fm := &frameManager{EventChan: make(chan *vtinput.InputEvent, 4)}
			FrameManager = fm

			resultCh := make(chan bool, 1)
			go func() { resultCh <- SetFar2lClipboard("hello world") }()

			openID := waitForFar2lID(t, 0)
			sendFar2lReply(fm, openID, func(s *vtinput.Far2lStack) {
				s.PushU64(0) // features
				s.PushU8(tc.openStatus)
			})

			if tc.openStatus == 1 {
				setID := waitForFar2lID(t, uint32(openID))
				sendFar2lReply(fm, setID, func(s *vtinput.Far2lStack) {
					s.PushU64(42) // dataID
					s.PushU8(tc.setStatus)
				})
			}

			select {
			case ok := <-resultCh:
				if ok != tc.wantOK {
					t.Errorf("SetFar2lClipboard() = %v, want %v", ok, tc.wantOK)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("SetFar2lClipboard did not return")
			}
		})
	}
}

type far2lGetResult struct {
	text string
	ok   bool
}

// TestGetFar2lClipboard exercises the CLIP_OPEN/CLIP_GETDATA exchange,
// including the trailing-NUL trim on the returned text and the "opened but
// no data available" case, none of which TestFar2lClipboard_Disabled reaches.
func TestGetFar2lClipboard(t *testing.T) {
	zero := uint32(0)
	noData := uint32(0xFFFFFFFF)

	cases := []struct {
		name        string
		openStatus  uint8
		data        []byte
		explicitLen *uint32 // overrides len(data) when set; nil derives it
		wantOK      bool
		wantText    string
	}{
		{
			name:       "open and getdata succeed, trailing NULs trimmed",
			openStatus: 1,
			data:       []byte("clip\x00\x00"),
			wantOK:     true,
			wantText:   "clip",
		},
		{
			name:       "host rejects CLIP_OPEN",
			openStatus: 0,
			wantOK:     false,
			wantText:   "",
		},
		{
			name:        "host reports no data available",
			openStatus:  1,
			explicitLen: &noData,
			wantOK:      true,
			wantText:    "",
		},
		{
			name:        "host reports zero-length data",
			openStatus:  1,
			explicitLen: &zero,
			wantOK:      true,
			wantText:    "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			oldEnabled, oldFM := Far2lEnabled, FrameManager
			oldID := far2lIDCounter.Load()
			t.Cleanup(func() {
				Far2lEnabled = oldEnabled
				FrameManager = oldFM
				far2lIDCounter.Store(oldID)
			})
			Far2lEnabled = true
			far2lIDCounter.Store(0)
			withTerminalClipboard(t, false)

			fm := &frameManager{EventChan: make(chan *vtinput.InputEvent, 4)}
			FrameManager = fm

			resultCh := make(chan far2lGetResult, 1)
			go func() {
				text, ok := GetFar2lClipboard()
				resultCh <- far2lGetResult{text, ok}
			}()

			openID := waitForFar2lID(t, 0)
			sendFar2lReply(fm, openID, func(s *vtinput.Far2lStack) {
				s.PushU64(0) // features, cleared and ignored by the caller
				s.PushU8(tc.openStatus)
			})

			if tc.openStatus == 1 {
				length := uint32(len(tc.data)) //nolint:gosec // tc.data is a small constant test fixture
				if tc.explicitLen != nil {
					length = *tc.explicitLen
				}
				getID := waitForFar2lID(t, uint32(openID))
				sendFar2lReply(fm, getID, func(s *vtinput.Far2lStack) {
					s.PushU64(7) // dataID
					if length != 0xFFFFFFFF && length > 0 {
						s.PushBytes(tc.data)
					}
					s.PushU32(length)
				})
			}

			select {
			case res := <-resultCh:
				if res.ok != tc.wantOK || res.text != tc.wantText {
					t.Errorf("GetFar2lClipboard() = (%q, %v), want (%q, %v)", res.text, res.ok, tc.wantText, tc.wantOK)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("GetFar2lClipboard did not return")
			}
		})
	}
}

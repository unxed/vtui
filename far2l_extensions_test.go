package vtui

import (
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

	// Track dispatching via a mock frame
	received := false
	frame := &mockFrame{
		onProcessKey: func(e *vtinput.InputEvent) bool {
			if e == kp {
				received = true
			}
			return true
		},
	}
	localFm.Push(frame)

	stk := &vtinput.Far2lStack{}
	stk.PushU8('w')

	// 2. Start Far2lInteract (which will block in WaitFar2lResponse)
	var reply *vtinput.Far2lStack
	done := make(chan bool)
	go func() {
		// Put the event into the queue AFTER Far2lInteract starts waiting
		time.Sleep(20 * time.Millisecond)
		localFm.EventChan <- kp

		reply = Far2lInteract(stk, true)
		done <- true
	}()

	// 3. Now simulate the dispatcher receiving the reply
	// We wait a bit to ensure WaitFar2lResponse has processed the kp event.
	time.Sleep(100 * time.Millisecond)

	idToWait := uint8(far2lIDCounter.Load())

	resp := vtinput.Far2lStack{}
	resp.PushU16(24) // height
	resp.PushU16(80) // width
	resp.PushU8(idToWait)

	replyEvent := &vtinput.InputEvent{
		Type:         vtinput.Far2lEventType,
		Far2lCommand: "reply",
		Far2lData:    resp,
	}

	// Manually dispatch the reply to satisfy the waiter
	localFm.dispatchEvent(replyEvent, false)

	// 4. Wait for the interaction to complete
	select {
	case <-done:
		if reply == nil {
			t.Fatal("Interaction failed on valid reply")
		}
		// Verify that the keypress event was DISPATCHED by the WaitFar2lResponse pump
		if !received {
			t.Error("KeyPress event was not dispatched during Far2lInteract wait!")
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

package vtui

import (
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestFar2lClipboard_Disabled(t *testing.T) {
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
	Far2lEnabled = true
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
		for far2lIDCounter == 0 {
			time.Sleep(10 * time.Millisecond)
		}
		idToWait = far2lIDCounter

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

	reply := Far2lInteract(stk, true)

	if reply == nil {
		t.Fatal("Interaction failed to receive reply")
	}

	if w := reply.PopU16(); w != 80 {
		t.Errorf("Unexpected data in reply: %d", w)
	}
}
//go:build linux && !android && (amd64 || arm64)

package vtui

import (
	"errors"
	"sync/atomic"
	"testing"
)

func TestWaylandPresentWakeCoalescesAndRearms(t *testing.T) {
	var sends atomic.Int32
	wake := waylandPresentWake{}
	send := func() error {
		sends.Add(1)
		return nil
	}

	wake.request(send)
	wake.request(send)
	if got := sends.Load(); got != 1 {
		t.Fatalf("coalesced sync requests = %d, want 1", got)
	}
	if !wake.done() {
		t.Fatal("completed sync did not report a pending redraw")
	}
	if wake.done() {
		t.Fatal("completed sync reported a second redraw")
	}

	wake.request(send)
	if got := sends.Load(); got != 2 {
		t.Fatalf("rearmed sync requests = %d, want 2", got)
	}
}

func TestWaylandPresentWakeRetriesAfterSendFailure(t *testing.T) {
	wake := waylandPresentWake{}
	fail := true
	send := func() error {
		if fail {
			fail = false
			return errors.New("test send failure")
		}
		return nil
	}

	wake.request(send)
	wake.request(send)
	if !wake.done() {
		t.Fatal("retry request did not leave a pending redraw")
	}
}

func TestWaylandPresentWakeIgnoresRequestsAfterClose(t *testing.T) {
	wake := waylandPresentWake{}
	wake.close()
	sent := false
	wake.request(func() error {
		sent = true
		return nil
	})
	if sent {
		t.Fatal("closed wake sent a sync request")
	}
	if wake.done() {
		t.Fatal("closed wake reported a redraw")
	}
}

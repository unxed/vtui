package vtui

import (
	"encoding/base64"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unxed/vtinput"
)

// ClipboardAccessManager interfaces with the host application to determine
// if the remote terminal is allowed to interact with the clipboard.
type ClipboardAccessManager interface {
	Authorize(clientID string) int // 1=Allow, 0=Deny, -1=FallbackLocal
}

var (
	GlobalClipboardAccessManager ClipboardAccessManager
	Far2lEnabled                 bool
	far2lInteractMu              sync.Mutex
	far2lIDCounter               atomic.Uint32
)

// Far2lInteract sends a request to the terminal emulator and optionally waits for a reply.
func Far2lInteract(stk *vtinput.Far2lStack, wait bool) *vtinput.Far2lStack {
	return far2lInteractTimeout(FrameManager, stk, wait, 2*time.Second)
}

func Far2lInteractTimeout(stk *vtinput.Far2lStack, wait bool, timeout time.Duration) *vtinput.Far2lStack {
	return far2lInteractTimeout(FrameManager, stk, wait, timeout)
}

func far2lInteractTimeout(fm *frameManager, stk *vtinput.Far2lStack, wait bool, timeout time.Duration) *vtinput.Far2lStack {
	far2lInteractMu.Lock()
	defer far2lInteractMu.Unlock()

	id, payload := far2lInteractionPayloadLocked(stk)
	DebugLog("VTUI_FAR2L_INTERACT: Sending ID=%d, payload_len=%d, wait=%v", id, len(payload), wait)
	_, _ = os.Stdout.Write(payload)

	if wait && fm != nil {
		return fm.WaitFar2lResponse(id, timeout)
	}
	DebugLog("VTUI_FAR2L: Processed without waiting for ID=%d", id)
	return nil
}

// far2lInteractionPayload returns a complete APC request and allocates its
// response ID. Renderers use it to put asynchronous requests into the frame
// being composed, keeping far2l bytes ordered with the text they accompany.
// The caller must not wait for the response while composing a frame.
func far2lInteractionPayload(stk *vtinput.Far2lStack) []byte {
	far2lInteractMu.Lock()
	defer far2lInteractMu.Unlock()
	_, payload := far2lInteractionPayloadLocked(stk)
	return payload
}

func far2lInteractionPayloadLocked(stk *vtinput.Far2lStack) (uint8, []byte) {
	id := uint8(far2lIDCounter.Add(1))
	if id == 0 {
		id = uint8(far2lIDCounter.Add(1))
	}
	stk.PushU8(id)
	b64 := base64.StdEncoding.EncodeToString(*stk)
	payload := make([]byte, 0, len(b64)+10)
	payload = append(payload, "\x1b_far2l:"...)
	payload = append(payload, b64...)
	payload = append(payload, '\x07')
	return id, payload
}

// SetFar2lClipboard attempts to set the clipboard using far2l extensions.
// clientID far2l terminal requires 32-256 chars for security handshake.
const clientID = "vtui-stateful-terminal-client-persistent-id-32chars"

func SetFar2lClipboard(text string) bool {
	fm := FrameManager
	enabled := far2lEnabledFor(fm)
	DebugLog("VTUI_FAR2L: SetFar2lClipboard attempt, Enabled=%v, text_len=%d", enabled, len(text))
	if !enabled {
		return false
	}
	// 1. Open
	stk := &vtinput.Far2lStack{}
	stk.PushString(clientID)
	stk.PushU8('o') // FARTTY_INTERACT_CLIP_OPEN
	stk.PushU8('c') // FARTTY_INTERACT_CLIPBOARD
	DebugLog("VTUI_FAR2L: Requesting CLIP_OPEN with ID %q...", clientID)
	reply := far2lInteractTimeout(fm, stk, true, 15*time.Second)

	if reply != nil {
		status := reply.PopU8()
		// FARTTY_INTERACT_CLIP_OPEN also returns uint64 features as second pop (bottom of stack)
		features := reply.PopU64()
		DebugLog("VTUI_FAR2L: CLIP_OPEN reply: status=%d, features=0x%X", status, features)

		if status == 1 {
			// 2. SetData
			stk = &vtinput.Far2lStack{}
			// IMPORTANT: Push in REVERSE order for C++ Pop (LIFO)
			// C++ expects: Pop(fmt), Pop(len), Pop(data)
			stk.PushBytes([]byte(text))
			stk.PushU32(uint32(len(text)))
			stk.PushU32(1)  // CF_TEXT
			stk.PushU8('s') // FARTTY_INTERACT_CLIP_SETDATA
			stk.PushU8('c') // FARTTY_INTERACT_CLIPBOARD

			DebugLog("VTUI_FAR2L: Requesting CLIP_SETDATA...")
			setReply := far2lInteractTimeout(fm, stk, true, 15*time.Second)

			success := false
			if setReply != nil {
				setStatus := setReply.PopU8()
				DebugLog("VTUI_FAR2L: CLIP_SETDATA status=%d", setStatus)
				if setStatus == 1 {
					success = true
					// CLIP_SETDATA also returns uint64 dataID if successful
					_ = setReply.PopU64()
				}
			}

			// 3. Close
			stk = &vtinput.Far2lStack{}
			stk.PushU8('c') // FARTTY_INTERACT_CLIP_CLOSE
			stk.PushU8('c') // FARTTY_INTERACT_CLIPBOARD
			far2lInteractTimeout(fm, stk, false, 2*time.Second)

			return success
		}
	}
	return false
}

// GetFar2lClipboard attempts to read the clipboard using far2l extensions.
func GetFar2lClipboard() (string, bool) {
	fm := FrameManager
	enabled := far2lEnabledFor(fm)
	DebugLog("VTUI_FAR2L: GetFar2lClipboard attempt, Enabled=%v", enabled)
	if !enabled {
		return "", false
	}

	stk := &vtinput.Far2lStack{}
	stk.PushString(clientID)
	stk.PushU8('o')
	stk.PushU8('c')
	reply := far2lInteractTimeout(fm, stk, true, 15*time.Second)

	if reply != nil {
		status := reply.PopU8()
		_ = reply.PopU64() // Clear stack
		if status == 1 {
			stk = &vtinput.Far2lStack{}
			// C++ expects: Pop(fmt)
			stk.PushU32(1)  // CF_TEXT
			stk.PushU8('g') // FARTTY_INTERACT_CLIP_GETDATA
			stk.PushU8('c') // FARTTY_INTERACT_CLIPBOARD
			getReply := far2lInteractTimeout(fm, stk, true, 15*time.Second)

			res := ""
			if getReply != nil {
				// C++ sends: Push(id), Push(data), Push(len)
				// We must pop in reverse: len, data, id

				l := getReply.PopU32()
				if l != 0xFFFFFFFF && l > 0 {
					data := getReply.PopBytes(int(l))
					// CRITICAL: Trim trailing NUL bytes from C-style clipboard string
					for len(data) > 0 && data[len(data)-1] == 0 {
						data = data[:len(data)-1]
					}
					res = string(data)
					DebugLog("VTUI_FAR2L: CLIP_GETDATA success, clean_len=%d", len(res))
				}

				_ = getReply.PopU64() // dataID
			}

			stk = &vtinput.Far2lStack{}
			stk.PushU8('c')
			stk.PushU8('c')
			far2lInteractTimeout(fm, stk, false, 2*time.Second)

			return res, true
		}
	}
	return "", false
}

func far2lEnabledFor(fm *frameManager) bool {
	if fm != nil && fm.far2lConfigured.Load() {
		return fm.far2lEnabled.Load()
	}
	return Far2lEnabled
}

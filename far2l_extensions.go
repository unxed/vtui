package vtui

import (
	"encoding/base64"
	"os"
	"sync"
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
	far2lIDCounter               uint8
)

// Far2lInteract sends a request to the terminal emulator and optionally waits for a reply.
func Far2lInteract(stk *vtinput.Far2lStack, wait bool) *vtinput.Far2lStack {
	return Far2lInteractTimeout(stk, wait, 2*time.Second)
}

func Far2lInteractTimeout(stk *vtinput.Far2lStack, wait bool, timeout time.Duration) *vtinput.Far2lStack {
	far2lInteractMu.Lock()
	defer far2lInteractMu.Unlock()

	far2lIDCounter++
	if far2lIDCounter == 0 {
		far2lIDCounter = 1
	}
	id := far2lIDCounter
	stk.PushU8(id)

	b64 := base64.StdEncoding.EncodeToString(*stk)
	DebugLog("VTUI_FAR2L_INTERACT: Sending ID=%d, payload_len=%d, wait=%v", id, len(b64), wait)
	os.Stdout.WriteString("\x1b_far2l:" + b64 + "\x07")

	if wait && FrameManager != nil {
		return FrameManager.WaitForFar2lReply(id, timeout)
	}
	DebugLog("VTUI_FAR2L: Received response for ID=%d", id)
	return nil
}

// SetFar2lClipboard attempts to set the clipboard using far2l extensions.
// clientID far2l terminal requires 32-256 chars for security handshake.
const clientID = "vtui-stateful-terminal-client-persistent-id-32chars"

func SetFar2lClipboard(text string) bool {
	DebugLog("VTUI_FAR2L: SetFar2lClipboard attempt, Enabled=%v, text_len=%d", Far2lEnabled, len(text))
	if !Far2lEnabled {
		return false
	}
	// 1. Open
	stk := &vtinput.Far2lStack{}
	stk.PushString(clientID)
	stk.PushU8('o') // FARTTY_INTERACT_CLIP_OPEN
	stk.PushU8('c') // FARTTY_INTERACT_CLIPBOARD
	DebugLog("VTUI_FAR2L: Requesting CLIP_OPEN with ID %q...", clientID)
	reply := Far2lInteractTimeout(stk, true, 15*time.Second)

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
			setReply := Far2lInteractTimeout(stk, true, 15*time.Second)

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
			Far2lInteract(stk, false)

			return success
		}
	}
	return false
}

// GetFar2lClipboard attempts to read the clipboard using far2l extensions.
func GetFar2lClipboard() (string, bool) {
	DebugLog("VTUI_FAR2L: GetFar2lClipboard attempt, Enabled=%v", Far2lEnabled)
	if !Far2lEnabled {
		return "", false
	}

	stk := &vtinput.Far2lStack{}
	stk.PushString(clientID)
	stk.PushU8('o')
	stk.PushU8('c')
	reply := Far2lInteractTimeout(stk, true, 15*time.Second)

	if reply != nil {
		status := reply.PopU8()
		_ = reply.PopU64() // Clear stack
		if status == 1 {
			stk = &vtinput.Far2lStack{}
			// C++ expects: Pop(fmt)
			stk.PushU32(1)  // CF_TEXT
			stk.PushU8('g') // FARTTY_INTERACT_CLIP_GETDATA
			stk.PushU8('c') // FARTTY_INTERACT_CLIPBOARD
			getReply := Far2lInteractTimeout(stk, true, 15*time.Second)

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
			Far2lInteract(stk, false)

			return res, true
		}
	}
	return "", false
}

package vtui

import (
	"encoding/base64"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unxed/vtinput"
)

// far2l clipboard protocol constants (VTExts 5.7). The CLIP_OPEN reply carries
// a bitmask of what the host supports; a text longer than 16 KiB is sent in
// chunks when the host reports FARTTY_FEATCLIP_CHUNKED_SET.
const (
	farttyFeatClipDataID     = 0x00000001
	farttyFeatClipChunkedSet = 0x00000002

	farttyClipChunkThreshold = 0x4000 // > this many bytes go in chunks
	farttyClipChunkSize      = 0x4000 // size of one CLIP_SETDATACHUNK piece
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
	// far2lClipboardLocal records that the terminal answered -1 to CLIP_OPEN
	// ("use your own clipboard"). For the rest of the process the far2l
	// clipboard is not tried again; Set/Get fall through to the own clipboard.
	far2lClipboardLocal atomic.Bool
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
// The clipboard handshake uses far2lClientID (32-256 chars, see far2l_clientid.go).
func SetFar2lClipboard(text string) bool {
	fm := FrameManager
	enabled := far2lEnabledFor(fm)
	DebugLog("VTUI_FAR2L: SetFar2lClipboard attempt, Enabled=%v, text_len=%d", enabled, len(text))
	if !enabled {
		return false
	}
	if far2lClipboardLocal.Load() {
		return false
	}
	// 1. Open
	stk := &vtinput.Far2lStack{}
	stk.PushString(far2lClientID())
	stk.PushU8('o') // FARTTY_INTERACT_CLIP_OPEN
	stk.PushU8('c') // FARTTY_INTERACT_CLIPBOARD
	DebugLog("VTUI_FAR2L: Requesting CLIP_OPEN with ID %q...", far2lClientID())
	reply := far2lInteractTimeout(fm, stk, true, 15*time.Second)

	if reply != nil {
		status := reply.PopU8()
		// FARTTY_INTERACT_CLIP_OPEN also returns uint64 features as second pop (bottom of stack)
		features := reply.PopU64()
		DebugLog("VTUI_FAR2L: CLIP_OPEN reply: status=%d, features=0x%X", status, features)

		if status == 1 {
			success := far2lClipSetData(fm, text, features)

			// 3. Close
			stk = &vtinput.Far2lStack{}
			stk.PushU8('c') // FARTTY_INTERACT_CLIP_CLOSE
			stk.PushU8('c') // FARTTY_INTERACT_CLIPBOARD
			far2lInteractTimeout(fm, stk, false, 2*time.Second)

			return success
		}
		if int8(status) == -1 { //nolint:gosec // the CLIP_OPEN status is an int8 on the wire; -1 means "use your own clipboard"
			// The terminal asked us to use our own clipboard for the session.
			far2lClipboardLocal.Store(true)
		}
	}
	return false
}

// far2lClipChunkPlan splits total bytes into CLIP_SETDATACHUNK pieces and a
// final CLIP_SETDATA remainder. All but the remainder go in chunks of
// farttyClipChunkSize; the remainder is 1..farttyClipChunkSize bytes, as
// VTExts 5.7.3 requires (when total is a multiple of the chunk size the last
// full chunk is left for CLIP_SETDATA). It is used only when total exceeds
// farttyClipChunkThreshold.
func far2lClipChunkPlan(total int) (chunks, restStart int) {
	chunks = total / farttyClipChunkSize
	if total%farttyClipChunkSize == 0 {
		chunks--
	}
	if chunks < 0 {
		chunks = 0
	}
	restStart = chunks * farttyClipChunkSize
	return chunks, restStart
}

// far2lClipSetData sends the text with CLIP_SETDATA, or, when the host
// reported FARTTY_FEATCLIP_CHUNKED_SET and the text is longer than 16 KiB,
// first with CLIP_SETDATACHUNK pieces. It returns the CLIP_SETDATA success.
func far2lClipSetData(fm *frameManager, text string, features uint64) bool {
	if features&farttyFeatClipChunkedSet != 0 && len(text) > farttyClipChunkThreshold {
		chunks, restStart := far2lClipChunkPlan(len(text))
		for i := 0; i < chunks; i++ {
			piece := text[i*farttyClipChunkSize : (i+1)*farttyClipChunkSize]
			stk := &vtinput.Far2lStack{}
			// Pop order for CLIP_SETDATACHUNK: size>>8, data.
			stk.PushBytes([]byte(piece))
			stk.PushU16(uint16(farttyClipChunkSize >> 8))
			stk.PushU8('S') // FARTTY_INTERACT_CLIP_SETDATACHUNK
			stk.PushU8('c') // FARTTY_INTERACT_CLIPBOARD
			// Only every 16th chunk is sent with an ID and waited for.
			wait := i%16 == 0
			DebugLog("VTUI_FAR2L: Requesting CLIP_SETDATACHUNK #%d (wait=%v)", i, wait)
			far2lInteractTimeout(fm, stk, wait, 15*time.Second)
		}
		text = text[restStart:]
	}

	stk := &vtinput.Far2lStack{}
	// IMPORTANT: Push in REVERSE order for C++ Pop (LIFO)
	// C++ expects: Pop(fmt), Pop(len), Pop(data)
	stk.PushBytes([]byte(text))
	stk.PushU32(uint32(len(text))) //nolint:gosec // the protocol carries the length as uint32; SetClipboard caps the text at 2 MiB
	stk.PushU32(1)                 // CF_TEXT
	stk.PushU8('s')                // FARTTY_INTERACT_CLIP_SETDATA
	stk.PushU8('c')                // FARTTY_INTERACT_CLIPBOARD

	DebugLog("VTUI_FAR2L: Requesting CLIP_SETDATA (len=%d)...", len(text))
	setReply := far2lInteractTimeout(fm, stk, true, 15*time.Second)
	if setReply == nil {
		return false
	}
	setStatus := setReply.PopU8()
	DebugLog("VTUI_FAR2L: CLIP_SETDATA status=%d", setStatus)
	if setStatus == 1 {
		// CLIP_SETDATA also returns uint64 dataID if successful
		_ = setReply.PopU64()
		return true
	}
	return false
}

// GetFar2lClipboard attempts to read the clipboard using far2l extensions.
// GetFar2lClipboard attempts to read the clipboard using far2l extensions.
func GetFar2lClipboard() (string, bool) {
	fm := FrameManager
	enabled := far2lEnabledFor(fm)
	DebugLog("VTUI_FAR2L: GetFar2lClipboard attempt, Enabled=%v", enabled)
	if !enabled {
		return "", false
	}
	if far2lClipboardLocal.Load() {
		return "", false
	}

	stk := &vtinput.Far2lStack{}
	stk.PushString(far2lClientID())
	stk.PushU8('o')
	stk.PushU8('c')
	reply := far2lInteractTimeout(fm, stk, true, 15*time.Second)

	if reply != nil {
		status := reply.PopU8()
		_ = reply.PopU64()      // Clear stack
		if int8(status) == -1 { //nolint:gosec // the CLIP_OPEN status is an int8 on the wire; -1 means "use your own clipboard"
			// The terminal asked us to use our own clipboard for the session.
			far2lClipboardLocal.Store(true)
		}
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

// ResetFar2lNegotiation forgets that a terminal acknowledged the extensions.
// A process that changes the terminal under itself -- a session daemon taking
// over a new client's PTY -- calls this before re-announcing the protocols, so
// a client that never answers is not talked to in far2l anyway.
func ResetFar2lNegotiation() {
	fm := FrameManager
	if fm == nil {
		return
	}
	fm.far2lNegotiated.Store(false)
	fm.far2lEnabled.Store(Far2lEnabled)
}

// Far2lNegotiated reports whether the terminal behind this process has
// acknowledged the far2l extensions: it answered the announcement, so an APC
// request sent to it is understood rather than ignored or printed. Unlike the
// Far2lEnabled startup default, which only says the application is willing
// to try, this is true only after the acknowledgement, and false again behind
// a native window or after ResetFar2lNegotiation. Code that starts a protocol
// of its own on this channel (a far2l DND binding, say) checks it first.
func Far2lNegotiated() bool {
	fm := FrameManager
	return fm != nil && !noTerminalBehind.Load() && fm.far2lNegotiated.Load()
}

func far2lEnabledFor(fm *frameManager) bool {
	// A native window has no terminal to answer an APC request, and every
	// far2l clipboard call would spend its full timeout waiting for a reply
	// that cannot arrive. This matters because the acknowledgement now
	// survives Init: a process that started in a terminal and then opened a
	// GUI window must not keep the negotiated state.
	if noTerminalBehind.Load() {
		return false
	}
	if fm != nil {
		if fm.far2lNegotiated.Load() {
			return true
		}
		if fm.far2lConfigured.Load() {
			return fm.far2lEnabled.Load()
		}
	}
	return Far2lEnabled
}

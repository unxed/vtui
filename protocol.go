package vtui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// DownMessage represents any command sent from the host application to the vtui kernel.
type DownMessage struct {
	Op       string     `json:"op"`
	Seq      int        `json:"seq,omitempty"`
	Version  int        `json:"version,omitempty"`
	Features []string   `json:"features,omitempty"`
	FrameID  string     `json:"frameId,omitempty"`
	Tree     *VuiNode   `json:"tree,omitempty"`
	Ops      []PatchOp  `json:"ops,omitempty"`
	ID       string     `json:"id,omitempty"`
	Method   string     `json:"method,omitempty"`
	Args     any        `json:"args,omitempty"`
	From     int        `json:"from,omitempty"`
	Rows     [][]string `json:"rows,omitempty"`
	Title    string     `json:"title,omitempty"`
	Text     string     `json:"text,omitempty"`
	Buttons  []string   `json:"buttons,omitempty"`
}

// PatchOp represents an atomic mutation operation on the widget tree.
type PatchOp struct {
	Kind     string         `json:"kind"` // "set" | "insert" | "remove" | "move"
	ID       string         `json:"id,omitempty"`
	Props    map[string]any `json:"props,omitempty"`
	ParentID string         `json:"parentId,omitempty"`
	Index    int            `json:"index,omitempty"`
	Node     *VuiNode       `json:"node,omitempty"`
}

// UpMessage represents an event or reply sent from the vtui kernel to the host application.
type UpMessage struct {
	Op       string   `json:"op"`
	ReplyTo  int      `json:"replyTo,omitempty"`
	Version  int      `json:"version,omitempty"`
	Size     [2]int   `json:"size,omitempty"`
	Backend  string   `json:"backend,omitempty"`
	Features []string `json:"features,omitempty"`
	Cmd      int      `json:"cmd,omitempty"`
	SrcID    string   `json:"srcId,omitempty"`
	ID       string   `json:"id,omitempty"`
	Value    any      `json:"value,omitempty"`
	Index    int      `json:"index,omitempty"`
	FrameID  string   `json:"frameId,omitempty"`
	Result   int      `json:"result,omitempty"`
	From     int      `json:"from,omitempty"`
	To       int      `json:"to,omitempty"`
	W        int      `json:"w,omitempty"`
	H        int      `json:"h,omitempty"`
	Need     [2]int   `json:"need,omitempty"`
	Code     string   `json:"code,omitempty"`
	Message  string   `json:"message,omitempty"`
}

// ProtocolSession manages the JSON Lines communication stream with a host application.
type ProtocolSession struct {
	mu            sync.Mutex
	in            io.Reader
	out           io.Writer
	fm            *frameManager
	mountedFrames map[string]Frame
	tracing       bool
	closed        bool
	throttleMap   map[string]time.Time
	recordFile    *os.File
	recordStart   time.Time
	eventMu       sync.Mutex
	eventClosed   bool
	eventWG       sync.WaitGroup
	closeDone     chan struct{}
}

// NewProtocolSession creates a new protocol session over the given I/O streams.
func NewProtocolSession(in io.Reader, out io.Writer, fm *frameManager) *ProtocolSession {
	if fm == nil {
		fm = FrameManager
	}
	recPath := os.Getenv("VTUI_RECORD")
	var recF *os.File
	if recPath != "" {
		if f, err := os.Create(recPath); err == nil {
			recF = f
		}
	}

	ps := &ProtocolSession{
		in:            in,
		out:           out,
		fm:            fm,
		mountedFrames: make(map[string]Frame),
		tracing:       os.Getenv("VTUI_TRACE") == "1",
		throttleMap:   make(map[string]time.Time),
		recordFile:    recF,
		recordStart:   time.Now(),
		closeDone:     make(chan struct{}),
	}

	// Wire FrameManager event sink to protocol output
	fm.SetEventSink(func(ev UIEvent) {
		ps.handleUIEvent(ev)
	})

	return ps
}

func (ps *ProtocolSession) send(msg UpMessage) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.closed {
		return io.ErrClosedPipe
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if ps.tracing {
		fmt.Fprintf(os.Stderr, "[VTUI_TRACE:UP] %s\n", string(data))
	}
	if ps.recordFile != nil {
		elapsed := time.Since(ps.recordStart).Seconds()
		recLine, _ := json.Marshal(map[string]any{
			"time": elapsed,
			"dir":  "up",
			"msg":  msg,
		})
		_, _ = ps.recordFile.Write(append(recLine, '\n'))
		_ = ps.recordFile.Sync()
	}
	_, err = ps.out.Write(append(data, '\n'))
	return err
}

func (ps *ProtocolSession) recordDown(line []byte) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.closed || ps.recordFile == nil {
		return
	}
	var raw any
	_ = json.Unmarshal(line, &raw)
	recLine, _ := json.Marshal(map[string]any{
		"time": time.Since(ps.recordStart).Seconds(),
		"dir":  "down",
		"msg":  raw,
	})
	_, _ = ps.recordFile.Write(append(recLine, '\n'))
	_ = ps.recordFile.Sync()
}

func (ps *ProtocolSession) handleUIEvent(ev UIEvent) {
	ps.eventMu.Lock()
	if ps.eventClosed {
		ps.eventMu.Unlock()
		return
	}
	ps.eventWG.Add(1)
	ps.eventMu.Unlock()
	go func() {
		defer ps.eventWG.Done()
		switch ev.Kind {
		case "command":
			_ = ps.send(UpMessage{
				Op:    "command",
				Cmd:   ev.Cmd,
				SrcID: ev.SrcID,
			})
		case "changed":
			ps.mu.Lock()
			last, ok := ps.throttleMap[ev.SrcID]
			now := time.Now()
			if ok && now.Sub(last) < 16*time.Millisecond {
				ps.mu.Unlock()
				return
			}
			ps.throttleMap[ev.SrcID] = now
			ps.mu.Unlock()

			_ = ps.send(UpMessage{
				Op:    "changed",
				ID:    ev.SrcID,
				Value: ev.Value.S,
			})
		case "closed":
			_ = ps.send(UpMessage{
				Op:      "closed",
				FrameID: ev.SrcID,
				Result:  ev.Index,
			})
		case "resize":
			_ = ps.send(UpMessage{
				Op: "resize",
				W:  ev.Index,
				H:  ev.Value.I,
			})
		case "key":
			_ = ps.send(UpMessage{
				Op:  "key",
				Cmd: ev.Cmd,
			})
		}
	}()
}

// Serve reads and processes lines from the transport until EOF or quit.
func (ps *ProtocolSession) Serve() error {
	scanner := bufio.NewScanner(ps.in)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		if ps.tracing {
			fmt.Fprintf(os.Stderr, "[VTUI_TRACE:DOWN] %s\n", string(line))
		}
		ps.recordDown(line)

		var msg DownMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			_ = ps.send(UpMessage{
				Op:      "error",
				Code:    "INVALID_JSON",
				Message: err.Error(),
			})
			continue
		}

		if err := ps.handleMessage(&msg); err != nil {
			if err == io.EOF {
				break
			}
			_ = ps.send(UpMessage{
				Op:      "error",
				ReplyTo: msg.Seq,
				Code:    "HANDLER_ERROR",
				Message: err.Error(),
			})
		}
	}

	ps.Close()
	return scanner.Err()
}

func (ps *ProtocolSession) handleMessage(msg *DownMessage) error {
	switch msg.Op {
	case "hello":
		w, h, backend := 80, 25, "ansi"
		if ps.fm != nil {
			if err := ps.fm.callOnUI(func() error {
				if ps.fm.scr != nil {
					w, h = ps.fm.scr.width, ps.fm.scr.height
				}
				backend = ps.fm.GetBackendName()
				return nil
			}); err != nil {
				return err
			}
		}
		return ps.send(UpMessage{
			Op:       "welcome",
			ReplyTo:  msg.Seq,
			Version:  1,
			Size:     [2]int{w, h},
			Backend:  backend,
			Features: []string{"vui", "patch", "virtual_lists", "autolayout", "describe"},
		})

	case "describe":
		var vocab any
		data, err := os.ReadFile("vocabulary.json")
		if err == nil {
			_ = json.Unmarshal(data, &vocab)
		}
		return ps.send(UpMessage{
			Op:      "description",
			ReplyTo: msg.Seq,
			Value:   vocab,
		})

	case "mount":
		if msg.Tree == nil {
			return fmt.Errorf("mount: missing tree")
		}
		frameID := msg.FrameID
		if frameID == "" {
			frameID = msg.Tree.ID
		}
		return ps.fm.callOnUI(func() error {
			doc := &VuiDocument{
				VuiVersion: 1,
				Root:       msg.Tree,
			}
			win, err := LoadVuiDocument(doc)
			if err != nil {
				return fmt.Errorf("mount error: %w", err)
			}
			if frameID != "" {
				win.SetID(frameID)
			}
			ps.mountedFrames[frameID] = win
			ps.fm.Push(win)
			ps.fm.Redraw()
			return nil
		})

	case "patch":
		return ps.fm.callOnUI(func() error { return ps.applyPatch(msg) })

	case "call":
		return ps.fm.callOnUI(func() error { return ps.handleCall(msg) })

	case "message":
		buttons := msg.Buttons
		if len(buttons) == 0 {
			buttons = []string{"&Ok"}
		}
		return ps.fm.callOnUI(func() error {
			dlg := createMessageDialog(msg.Title, msg.Text, buttons, legacyKindFromTitle(msg.Title))
			if msg.FrameID != "" {
				dlg.SetID(msg.FrameID)
			}
			ps.fm.Push(dlg)
			ps.fm.Redraw()
			return nil
		})

	case "close":
		return ps.fm.callOnUI(func() error {
			if f, ok := ps.mountedFrames[msg.FrameID]; ok {
				f.Close()
				delete(ps.mountedFrames, msg.FrameID)
				ps.fm.Redraw()
			}
			return nil
		})

	case "quit":
		// Shutdown takes uiOwnershipMu itself before Run starts, so routing it
		// through callOnUI would deadlock on that non-reentrant mutex.
		ps.fm.Shutdown()
		return io.EOF

	default:
		return fmt.Errorf("unknown operation: %q", msg.Op)
	}
}

func (ps *ProtocolSession) applyPatch(msg *DownMessage) error {
	frame, ok := ps.mountedFrames[msg.FrameID]
	if !ok {
		frame = ps.fm.GetTopFrame()
	}
	if frame == nil {
		return fmt.Errorf("patch: frame %q not found", msg.FrameID)
	}

	for _, op := range msg.Ops {
		switch op.Kind {
		case "set":
			el, found := ps.fm.Lookup(frameMatchesIDString(frame), op.ID)
			if !found {
				return fmt.Errorf("patch set: element %q not found", op.ID)
			}
			if pa, ok := el.(PropertyAccess); ok {
				for k, v := range op.Props {
					if err := pa.SetProperty(k, toPropValue(v)); err != nil {
						return fmt.Errorf("patch set %s.%s: %w", op.ID, k, err)
					}
				}
			}

		case "remove":
			el, found := ps.fm.Lookup(frameMatchesIDString(frame), op.ID)
			if found {
				if g, ok := el.GetOwner().(*Group); ok {
					for i, child := range g.items {
						if child == el {
							g.items = append(g.items[:i], g.items[i+1:]...)
							break
						}
					}
				}
			}

		case "insert":
			parent, found := ps.fm.Lookup(frameMatchesIDString(frame), op.ParentID)
			if !found {
				parent = frame.(UIElement)
			}
			if container, ok := parent.(interface{ AddItem(UIElement) }); ok {
				childEl, err := buildNode(op.Node, make(map[string]UIElement), &[]struct {
					label    *Text
					targetID string
				}{})
				if err != nil {
					return err
				}
				container.AddItem(childEl)
			}
		}
	}

	ps.fm.Redraw()
	return nil
}

func (ps *ProtocolSession) handleCall(msg *DownMessage) error {
	el, found := ps.fm.Lookup("", msg.ID)
	if !found {
		return fmt.Errorf("call: element %q not found", msg.ID)
	}

	switch msg.Method {
	case "focus":
		el.SetFocus(true)
	case "selectAll":
		if e, ok := el.(*Edit); ok {
			e.SelectAll()
		}
	}
	ps.fm.Redraw()
	return nil
}

func frameMatchesIDString(f Frame) string {
	if so, ok := f.(interface{ ID() string }); ok {
		return so.ID()
	}
	return ""
}

// Close terminates the protocol session and tears down the UI safely.
func (ps *ProtocolSession) Close() {
	ps.eventMu.Lock()
	ps.eventClosed = true
	ps.eventMu.Unlock()

	ps.mu.Lock()
	if ps.closed {
		done := ps.closeDone
		ps.mu.Unlock()
		<-done
		return
	}
	ps.closed = true
	if ps.recordFile != nil {
		_ = ps.recordFile.Close()
		ps.recordFile = nil
	}
	ps.mu.Unlock()
	ps.eventWG.Wait()

	if ps.fm != nil {
		ps.fm.SetEventSink(nil)
		ps.fm.shutdownAndWait()
	}
	close(ps.closeDone)
}

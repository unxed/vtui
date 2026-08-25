package vtui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"
)

// VuiNode describes a single widget node in .vui JSON format.
type VuiNode struct {
	Type     string         `json:"type"`
	ID       string         `json:"id,omitempty"`
	Props    map[string]any `json:"props,omitempty"`
	Layout   *VuiLayoutDef  `json:"layout,omitempty"`
	Children []*VuiNode     `json:"children,omitempty"`
	Row      int            `json:"row,omitempty"`
	Col      int            `json:"col,omitempty"`
	RowSpan  int            `json:"rowSpan,omitempty"`
	ColSpan  int            `json:"colSpan,omitempty"`
}

// VuiLayoutDef describes layout container settings in .vui JSON format.
type VuiLayoutDef struct {
	Type    string `json:"type"`
	Spacing any    `json:"spacing,omitempty"`
	Margins []int  `json:"margins,omitempty"`
	Align   string `json:"align,omitempty"`
}

// VuiConnection describes a signal-to-command or signal-to-emit connection.
type VuiConnection struct {
	From    string `json:"from"`
	Signal  string `json:"signal"`
	Command any    `json:"command,omitempty"`
	Emit    bool   `json:"emit,omitempty"`
}

// VuiDocument describes a complete .vui interface document.
type VuiDocument struct {
	VuiVersion  int               `json:"vuiVersion"`
	Root        *VuiNode          `json:"root"`
	Connections []VuiConnection   `json:"connections,omitempty"`
	TabOrder    []string          `json:"tabOrder,omitempty"`
	Palette     map[string]string `json:"palette,omitempty"`
}

// LoadDialog loads and instantiates a dialog/window tree from reader.
func LoadDialog(r io.Reader) (*Window, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	var doc VuiDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return LoadVuiDocument(&doc)
}

// LoadDialogFile loads and instantiates a dialog/window tree from a .vui file path.
// If VTUI_WATCH=1 is set, it starts an automatic reload watcher preserving widget states.
func LoadDialogFile(path string) (*Window, error) {
	win, err := loadDialogFileOnce(path)
	if err != nil {
		return nil, err
	}

	if os.Getenv("VTUI_WATCH") == "1" {
		watchVuiFile(path, win)
	}

	return win, nil
}

func loadDialogFileOnce(path string) (*Window, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return LoadDialog(f)
}

func watchVuiFile(path string, targetWin *Window) {
	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	lastMtime := fi.ModTime()
	fm := FrameManager

	go func() {
		for {
			time.Sleep(200 * time.Millisecond)
			if fm == nil || fm.IsShutdown() {
				return
			}
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastMtime) {
				lastMtime = info.ModTime()
				DebugLog("VUI: Hot reloading %s...", path)
				fm.PostTask(func() {
					stateMap := make(map[string]any)
					walk(targetWin, func(el UIElement) bool {
						id := el.GetId()
						if id != "" {
							if dc, ok := el.(DataControl); ok {
								stateMap[id] = dc.GetData()
							}
						}
						return true
					})

					newWin, err := loadDialogFileOnce(path)
					if err == nil && newWin != nil {
						walk(newWin, func(el UIElement) bool {
							id := el.GetId()
							if id != "" {
								if val, ok := stateMap[id]; ok {
									if dc, ok := el.(DataControl); ok {
										dc.SetData(val)
									}
								}
							}
							return true
						})
						targetWin.SetPosition(newWin.X1, newWin.Y1, newWin.X2, newWin.Y2)
						targetWin.rootGroup = newWin.rootGroup
						targetWin.rootGroup.SetOwner(targetWin)
						fm.Redraw()
					}
				})
			}
		}
	}()
}

// LayoutContainerElement is an interface for containers that hold child elements with a layout.
type LayoutContainerElement struct {
	Group
	layoutType string
	spacing    int
	margins    Margins
	align      string
}

func (l *LayoutContainerElement) ApplyLayout() {
	ApplyLayoutTree(l, l.layoutType, l.spacing, l.margins, l.align, l.GetChildren())
}

func (l *LayoutContainerElement) SizeSpecH() SizeSpec {
	h, _ := ComputeContainerSizeHint(l.GetChildren(), l.layoutType, l.spacing, l.margins)
	return h
}

func (l *LayoutContainerElement) SizeSpecV() SizeSpec {
	_, v := ComputeContainerSizeHint(l.GetChildren(), l.layoutType, l.spacing, l.margins)
	return v
}

// LoadVuiDocument constructs a live Window tree from a parsed VuiDocument.
func LoadVuiDocument(doc *VuiDocument) (*Window, error) {
	if doc == nil || doc.Root == nil {
		return nil, fmt.Errorf("vtui: empty vui document")
	}

	elementsByID := make(map[string]UIElement)
	var buddyLinks []struct {
		label    *Text
		targetID string
	}

	rootEl, err := buildNode(doc.Root, elementsByID, &buddyLinks)
	if err != nil {
		return nil, err
	}

	win, ok := rootEl.(*Window)
	if !ok {
		return nil, fmt.Errorf("vtui: root node must be Dialog or Window, got %T", rootEl)
	}

	// Resolve buddy links
	for _, bl := range buddyLinks {
		if target, ok := elementsByID[bl.targetID]; ok {
			bl.label.FocusLink = target
		}
	}

	// Apply connections
	for _, conn := range doc.Connections {
		el, ok := elementsByID[conn.From]
		if !ok {
			continue
		}
		cmdID := resolveCommandID(conn.Command)
		if cmdID != 0 {
			if so, ok := el.(interface{ SetProperty(string, PropValue) error }); ok {
				_ = so.SetProperty("command", PropValInt(cmdID))
			}
		}
	}

	// Calculate AutoSize & Centering
	autoSize := false
	center := true
	if doc.Root.Props != nil {
		if v, ok := doc.Root.Props["autoSize"].(bool); ok {
			autoSize = v
		}
		if v, ok := doc.Root.Props["center"].(bool); ok {
			center = v
		}
	}

	winW := 0
	winH := 0
	if doc.Root.Props != nil {
		if w, ok := doc.Root.Props["w"].(float64); ok {
			winW = int(w)
		} else if w, ok := doc.Root.Props["width"].(float64); ok {
			winW = int(w)
		}
		if h, ok := doc.Root.Props["h"].(float64); ok {
			winH = int(h)
		} else if h, ok := doc.Root.Props["height"].(float64); ok {
			winH = int(h)
		}
	}

	layoutType := "VBox"
	spacing := 1
	margins := Margins{Top: 1, Right: 2, Bottom: 1, Left: 2}
	align := "fill"
	if doc.Root.Layout != nil {
		if doc.Root.Layout.Type != "" {
			layoutType = doc.Root.Layout.Type
		}
		if s, ok := doc.Root.Layout.Spacing.(float64); ok {
			spacing = int(s)
		}
		if len(doc.Root.Layout.Margins) == 4 {
			margins = Margins{
				Top:    doc.Root.Layout.Margins[0],
				Right:  doc.Root.Layout.Margins[1],
				Bottom: doc.Root.Layout.Margins[2],
				Left:   doc.Root.Layout.Margins[3],
			}
		}
		if doc.Root.Layout.Align != "" {
			align = doc.Root.Layout.Align
		}
	}

	if autoSize {
		hSpec, vSpec := ComputeContainerSizeHint(win.GetChildren(), layoutType, spacing, margins)
		calcW := hSpec.Hint + 2
		calcH := vSpec.Hint + 2
		if winW > 0 && calcW < winW {
			calcW = winW
		}
		if winH > 0 && calcH < winH {
			calcH = winH
		}
		winW = calcW
		winH = calcH
		win.MinW = hSpec.Min + 2
		win.MinH = vSpec.Min + 2
	} else {
		if winW <= 0 {
			winW = win.X2 - win.X1 + 1
		}
		if winH <= 0 {
			winH = win.Y2 - win.Y1 + 1
		}
	}

	scrW, scrH := 80, 25
	if FrameManager != nil && FrameManager.scr != nil {
		scrW = FrameManager.scr.width
		scrH = FrameManager.scr.height
	}

	x1, y1 := 0, 0
	if center {
		x1 = (scrW - winW) / 2
		y1 = (scrH - winH) / 2
	}
	win.SetPosition(x1, y1, x1+winW-1, y1+winH-1)

	// Apply layout
	ApplyLayoutTree(win.rootGroup, layoutType, spacing, margins, align, win.GetChildren())

	return win, nil
}

func buildNode(node *VuiNode, registry map[string]UIElement, buddyLinks *[]struct {
	label    *Text
	targetID string
}) (UIElement, error) {
	if node == nil {
		return nil, nil
	}

	var el UIElement
	if node.Type == "Group" && node.Layout != nil {
		spacing := 1
		margins := Margins{}
		align := "fill"
		if s, ok := node.Layout.Spacing.(float64); ok {
			spacing = int(s)
		}
		if len(node.Layout.Margins) == 4 {
			margins = Margins{
				Top:    node.Layout.Margins[0],
				Right:  node.Layout.Margins[1],
				Bottom: node.Layout.Margins[2],
				Left:   node.Layout.Margins[3],
			}
		}
		if node.Layout.Align != "" {
			align = node.Layout.Align
		}
		container := &LayoutContainerElement{
			Group:      *NewGroup(0, 0, 10, 10),
			layoutType: node.Layout.Type,
			spacing:    spacing,
			margins:    margins,
			align:      align,
		}
		el = container
	} else {
		var err error
		el, err = NewByType(node.Type)
		if err != nil {
			return nil, err
		}
	}

	if node.ID != "" {
		el.SetID(node.ID)
		registry[node.ID] = el
	}

	if pa, ok := el.(PropertyAccess); ok && node.Props != nil {
		for k, v := range node.Props {
			propVal := toPropValue(v)
			_ = pa.SetProperty(k, propVal)
			if k == "buddy" && node.Type == "Label" {
				if lbl, ok := el.(*Text); ok {
					*buddyLinks = append(*buddyLinks, struct {
						label    *Text
						targetID string
					}{label: lbl, targetID: fmt.Sprintf("%v", v)})
				}
			}
		}
	}

	if container, ok := el.(interface{ AddItem(UIElement) }); ok {
		for _, childNode := range node.Children {
			childEl, err := buildNode(childNode, registry, buddyLinks)
			if err != nil {
				return nil, err
			}
			if childEl != nil {
				container.AddItem(childEl)
			}
		}
	}

	return el, nil
}

func toPropValue(v any) PropValue {
	switch val := v.(type) {
	case string:
		return PropValString(val)
	case bool:
		return PropValBool(val)
	case float64:
		return PropValInt(int(val))
	case int:
		return PropValInt(val)
	case []any:
		var list []string
		for _, item := range val {
			list = append(list, fmt.Sprintf("%v", item))
		}
		return PropValStringList(list)
	default:
		return PropValString(fmt.Sprintf("%v", v))
	}
}

func resolveCommandID(cmd any) int {
	if cmd == nil {
		return 0
	}
	switch v := cmd.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		switch v {
		case "CmOk", "CmOK":
			return CmOK
		case "CmCancel":
			return CmCancel
		case "CmQuit":
			return CmQuit
		case "CmDefault":
			return CmDefault
		case "CmClose":
			return CmClose
		case "CmZoom":
			return CmZoom
		case "CmResize":
			return CmResize
		case "CmNext":
			return CmNext
		case "CmPrev":
			return CmPrev
		case "CmHelp":
			return CmHelp
		default:
			if n, err := strconv.Atoi(v); err == nil {
				return n
			}
		}
	}
	return 0
}

package vtui

import (
	"fmt"
	"github.com/mattn/go-runewidth"
	"github.com/unxed/vtinput"
	"strings"
)

const SemanticSceneVersion = 2

// SemanticSceneAdapter позволяет приложению (например, f4) модифицировать
// сгенерированную сцену перед ее отправкой рендереру.
type SemanticSceneAdapter func(ctx *SemanticContext, baseScene map[string]any) map[string]any

var AppSceneAdapter SemanticSceneAdapter

// SemanticID генерирует уникальный ID для элемента.
func SemanticID(v any) string {
	if v == nil {
		return ""
	}
	if el, ok := v.(UIElement); ok {
		if id := el.GetId(); id != "" {
			return "id:" + id
		}
	}
	return fmt.Sprintf("%T:%p", v, v)
}

// ExportSemanticScene обходит все текущие экраны и фреймы, собирая полное дерево.
func (fm *frameManager) ExportSemanticScene() map[string]any {
	if fm == nil || fm.scr == nil {
		return nil
	}

	ctx := &SemanticContext{
		Width:        fm.scr.width,
		Height:       fm.scr.height,
		ActiveScreen: fm.ActiveIdx,
	}

	fm.SyncCurrentScreen()
	screens := make([]map[string]any, 0, len(fm.Screens))
	for i, screen := range fm.Screens {
		frames := make([]map[string]any, 0, len(screen.Frames))
		for _, frame := range screen.Frames {
			if node := semanticFrame(ctx, frame); node != nil {
				frames = append(frames, node)
			}
		}
		screens = append(screens, map[string]any{
			"index":       i,
			"active":      i == fm.ActiveIdx,
			"title":       screen.GetTitle(),
			"progress":    screen.GetProgress(),
			"attention":   screen.NeedsAttention(),
			"transparent": screen.Transparent,
			"frames":      frames,
		})
	}

	scene := map[string]any{
		"type":         "scene",
		"version":      SemanticSceneVersion,
		"width":        fm.scr.width,
		"height":       fm.scr.height,
		"activeScreen": fm.ActiveIdx,
		"screens":      screens,
	}

	if fm.ActiveIdx >= 0 && fm.ActiveIdx < len(screens) {
		scene["frames"] = screens[fm.ActiveIdx]["frames"]
	}

	if mb := fm.GetActiveMenuBar(); mb != nil {
		scene["menuBar"] = semanticMenuBar(mb)
	}
	if fm.KeyBar != nil {
		scene["keyBar"] = semanticKeyBar(fm.KeyBar)
	}
	if fm.currentToast != nil {
		scene["toast"] = map[string]any{"message": fm.currentToast.Message}
	}
	if len(fm.Screens) > 1 {
		scene["workspaceCount"] = len(fm.Screens)
	}

	if AppSceneAdapter != nil {
		if adapted := AppSceneAdapter(ctx, scene); adapted != nil {
			return adapted
		}
	}
	return scene
}

// HandleSemanticAction глобально маршрутизирует семантические действия во vtui
func (fm *frameManager) HandleSemanticAction(action map[string]any) bool {
	if fm == nil || action == nil {
		return false
	}
	if kind, _ := action["kind"].(string); kind == "command" {
		return fm.EmitCommand(semanticInt(action["command"]), action["args"])
	}
	if semanticString(action["action"]) == "menu_bar_activate" || semanticString(action["action"]) == "menuBar.activate" {
		if mb := fm.GetActiveMenuBar(); mb != nil {
			idx := semanticInt(action["index"])
			if idx >= 0 && idx < len(mb.Items) {
				mb.Active = true
				mb.ActivateSubMenu(idx)
				fm.Redraw()
				return true
			}
		}
	}

	activeIdx := fm.ActiveIdx
	frames := fm.GetActiveFrames(activeIdx)

	target := semanticString(action["target"])
	for i := len(frames) - 1; i >= 0; i-- {
		if target == "" {
			if h, ok := frames[i].(SemanticActionHandler); ok && h.HandleSemanticAction(action) {
				fm.Redraw()
				return true
			}
		} else {
			if h, ok := frames[i].(SemanticActionHandler); ok {
				if h.HandleSemanticAction(action) {
					fm.Redraw()
					return true
				}
			}
		}
	}
	return false
}

func semanticFrame(ctx *SemanticContext, frame Frame) map[string]any {
	if frame == nil {
		return nil
	}
	if sp, ok := frame.(SemanticProvider); ok {
		if node := sp.SemanticNode(ctx); node != nil {
			return node
		}
	}

	x1, y1, x2, y2 := frame.GetPosition()
	base := map[string]any{
		"id":       SemanticID(frame),
		"title":    strings.TrimSpace(frame.GetTitle()),
		"type":     int(frame.GetType()),
		"x":        x1,
		"y":        y1,
		"w":        x2 - x1 + 1,
		"h":        y2 - y1 + 1,
		"modal":    frame.IsModal(),
		"busy":     frame.IsBusy(),
		"progress": frame.GetProgress(),
		"shadow":   frame.HasShadow(),
	}

	base["kind"] = "fallback"
	base["fallback"] = true
	base["reason"] = fmt.Sprintf("unsupported frame %T", frame)
	return base
}

// Рекурсивный экспорт потомков для контейнеров
func semanticChildren(ctx *SemanticContext, children []UIElement) []map[string]any {
	var nodes []map[string]any
	for _, child := range children {
		if child == nil {
			continue
		}
		if sp, ok := child.(SemanticProvider); ok {
			if node := sp.SemanticNode(ctx); node != nil {
				nodes = append(nodes, node)
				continue
			}
		}
		x1, y1, x2, y2 := child.GetPosition()
		nodes = append(nodes, map[string]any{
			"id":       SemanticID(child),
			"kind":     "widget",
			"x":        x1,
			"y":        y1,
			"w":        x2 - x1 + 1,
			"h":        y2 - y1 + 1,
			"visible":  child.IsVisible(),
			"focused":  child.IsFocused(),
			"disabled": child.IsDisabled(),
		})
	}
	return nodes
}

func handleSemanticChildrenAction(children []UIElement, target string, action map[string]any) bool {
	for _, child := range children {
		if SemanticID(child) == target {
			if h, ok := child.(SemanticActionHandler); ok {
				return h.HandleSemanticAction(action)
			}
		}
		if c, ok := child.(Container); ok {
			if handleSemanticChildrenAction(c.GetChildren(), target, action) {
				return true
			}
		}
	}
	return false
}

// --- Реализация семантики для базовых компонентов vtui ---

func (w *Window) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := w.GetPosition()
	kind := "window"
	if w.Modal {
		kind = "dialog"
	}
	node := map[string]any{
		"id":        SemanticID(w),
		"kind":      kind,
		"title":     strings.TrimSpace(w.GetTitle()),
		"x":         x1,
		"y":         y1,
		"w":         x2 - x1 + 1,
		"h":         y2 - y1 + 1,
		"modal":     w.Modal,
		"busy":      w.IsBusy(),
		"progress":  w.GetProgress(),
		"shadow":    w.HasShadow(),
		"showClose": w.ShowClose,
		"showZoom":  w.ShowZoom,
	}
	if w.rootGroup != nil {
		node["children"] = semanticChildren(ctx, w.rootGroup.GetChildren())
	}
	return node
}

func (w *Window) HandleSemanticAction(action map[string]any) bool {
	target := semanticString(action["target"])
	if SemanticID(w) == target {
		switch semanticString(action["action"]) {
		case "close", "dialog.close", "window.close":
			w.Close()
			return true
		}
	}
	if w.rootGroup != nil {
		return w.rootGroup.HandleSemanticAction(action)
	}
	return false
}

func (g *Group) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := g.GetPosition()
	return map[string]any{
		"id":       SemanticID(g),
		"kind":     "group",
		"x":        x1,
		"y":        y1,
		"w":        x2 - x1 + 1,
		"h":        y2 - y1 + 1,
		"visible":  g.IsVisible(),
		"focused":  g.IsFocused(),
		"children": semanticChildren(ctx, g.GetChildren()),
	}
}

func (g *Group) HandleSemanticAction(action map[string]any) bool {
	target := semanticString(action["target"])
	if SemanticID(g) == target {
		switch semanticString(action["action"]) {
		case "focus", "control.focus":
			g.SetFocus(true)
			return true
		}
	}

	for _, child := range g.items {
		if SemanticID(child) == target {
			if h, ok := child.(SemanticActionHandler); ok {
				return h.HandleSemanticAction(action)
			}
		}
		if c, ok := child.(Container); ok {
			if handleSemanticChildrenAction(c.GetChildren(), target, action) {
				return true
			}
		}
	}
	return false
}

func (b *Button) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := b.GetPosition()
	return map[string]any{
		"id":       SemanticID(b),
		"kind":     "button",
		"x":        x1,
		"y":        y1,
		"w":        x2 - x1 + 1,
		"h":        y2 - y1 + 1,
		"visible":  b.IsVisible(),
		"focused":  b.IsFocused(),
		"disabled": b.IsDisabled(),
		"text":     b.cleanText,
		"hotkey":   stringOrEmpty(b.hotkey),
		"default":  b.IsDefault,
	}
}

func (b *Button) HandleSemanticAction(action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "activate", "control.activate":
		return b.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	case "focus", "control.focus":
		b.SetFocus(true)
		return true
	}
	return false
}

func (cb *Checkbox) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := cb.GetPosition()
	return map[string]any{
		"id":         SemanticID(cb),
		"kind":       "checkbox",
		"x":          x1,
		"y":          y1,
		"w":          x2 - x1 + 1,
		"h":          y2 - y1 + 1,
		"visible":    cb.IsVisible(),
		"focused":    cb.IsFocused(),
		"disabled":   cb.IsDisabled(),
		"text":       cb.cleanText,
		"hotkey":     stringOrEmpty(cb.hotkey),
		"state":      cb.State,
		"threeState": cb.ThreeState,
	}
}

func (cb *Checkbox) HandleSemanticAction(action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "toggle", "control.toggle":
		cb.Toggle()
		return true
	case "focus", "control.focus":
		cb.SetFocus(true)
		return true
	}
	return false
}

func (cg *CheckGroup) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := cg.GetPosition()
	return map[string]any{
		"id":         SemanticID(cg),
		"kind":       "checkGroup",
		"x":          x1,
		"y":          y1,
		"w":          x2 - x1 + 1,
		"h":          y2 - y1 + 1,
		"visible":    cg.IsVisible(),
		"focused":    cg.IsFocused(),
		"disabled":   cg.IsDisabled(),
		"items":      cg.Items,
		"states":     cg.States,
		"focusIndex": cg.focusIdx,
		"columns":    cg.Columns,
	}
}

func (cg *CheckGroup) HandleSemanticAction(action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "select", "control.select":
		idx := semanticInt(action["index"])
		if idx >= 0 && idx < len(cg.Items) {
			cg.focusIdx = idx
			cg.States[idx] = !cg.States[idx]
			cg.NotifyChange()
			return true
		}
	case "focus", "control.focus":
		cg.SetFocus(true)
		return true
	}
	return false
}

func (rg *RadioGroup) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := rg.GetPosition()
	return map[string]any{
		"id":         SemanticID(rg),
		"kind":       "radioGroup",
		"x":          x1,
		"y":          y1,
		"w":          x2 - x1 + 1,
		"h":          y2 - y1 + 1,
		"visible":    rg.IsVisible(),
		"focused":    rg.IsFocused(),
		"disabled":   rg.IsDisabled(),
		"items":      rg.Items,
		"selected":   rg.Selected,
		"focusIndex": rg.focusIdx,
		"columns":    rg.Columns,
	}
}

func (rg *RadioGroup) HandleSemanticAction(action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "select", "control.select":
		idx := semanticInt(action["index"])
		if idx >= 0 && idx < len(rg.Items) {
			rg.focusIdx = idx
			if rg.Selected != idx {
				rg.Selected = idx
				if rg.OnChange != nil {
					rg.OnChange(idx)
				}
				rg.FireAction(nil, idx)
			}
			return true
		}
	case "focus", "control.focus":
		rg.SetFocus(true)
		return true
	}
	return false
}

func (cb *ComboBox) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := cb.GetPosition()
	var items []map[string]any
	if cb.Menu != nil {
		for i, item := range cb.Menu.Items {
			clean, hotkey, _ := ParseAmpersandString(item.Text)
			items = append(items, map[string]any{
				"index":     i,
				"text":      clean,
				"rawText":   item.Text,
				"hotkey":    stringOrEmpty(hotkey),
				"shortcut":  item.Shortcut,
				"command":   item.Command,
				"separator": item.Separator,
			})
		}
	}

	return map[string]any{
		"id":           SemanticID(cb),
		"kind":         "comboBox",
		"x":            x1,
		"y":            y1,
		"w":            x2 - x1 + 1,
		"h":            y2 - y1 + 1,
		"visible":      cb.IsVisible(),
		"focused":      cb.IsFocused(),
		"disabled":     cb.IsDisabled(),
		"text":         cb.Edit.GetText(),
		"dropdownOnly": cb.DropdownOnly,
		"items":        items,
		"selected":     cb.Menu.SelectPos,
	}
}

func (cb *ComboBox) HandleSemanticAction(action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "select", "control.select":
		idx := semanticInt(action["index"])
		if cb.Menu != nil && idx >= 0 && idx < len(cb.Menu.Items) {
			cb.Menu.SetSelectPos(idx)
			cb.Edit.SetText(cb.Menu.Items[idx].Text)
			if cb.Menu.OnAction != nil {
				cb.Menu.OnAction(idx)
			}
			return true
		}
	case "focus", "control.focus":
		cb.SetFocus(true)
		return true
	}
	return false
}

func (e *Edit) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := e.GetPosition()
	return map[string]any{
		"id":             SemanticID(e),
		"kind":           "edit",
		"x":              x1,
		"y":              y1,
		"w":              x2 - x1 + 1,
		"h":              y2 - y1 + 1,
		"visible":        e.IsVisible(),
		"focused":        e.IsFocused(),
		"disabled":       e.IsDisabled(),
		"text":           e.GetText(),
		"cursor":         e.curPos,
		"left":           e.leftPos,
		"password":       e.PasswordMode,
		"selectionStart": e.selStart,
		"selectionEnd":   e.selEnd,
		"history":        e.ShowHistoryButton,
	}
}

func (e *Edit) HandleSemanticAction(action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "set_text", "control.setText":
		e.SetText(semanticString(action["text"]))
		if e.OnTextChange != nil {
			e.OnTextChange(e.GetText())
		}
		return true
	case "insert_text", "control.insertText":
		e.InsertString(semanticString(action["text"]))
		return true
	case "focus", "control.focus":
		e.SetFocus(true)
		return true
	}
	return false
}

func (pb *ProgressBar) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := pb.GetPosition()
	return map[string]any{
		"id":      SemanticID(pb),
		"kind":    "progressBar",
		"x":       x1,
		"y":       y1,
		"w":       x2 - x1 + 1,
		"h":       y2 - y1 + 1,
		"visible": pb.IsVisible(),
		"percent": pb.Percent,
	}
}

func (sb *ScrollBar) SemanticNode(ctx *SemanticContext) map[string]any {
	x1, y1, x2, y2 := sb.GetPosition()
	return map[string]any{
		"id":      SemanticID(sb),
		"kind":    "scrollBar",
		"x":       x1,
		"y":       y1,
		"w":       x2 - x1 + 1,
		"h":       y2 - y1 + 1,
		"visible": sb.IsVisible(),
		"value":   sb.Value,
		"min":     sb.Min,
		"max":     sb.Max,
	}
}

func (sb *ScrollBar) HandleSemanticAction(action map[string]any) bool {
	switch semanticString(action["action"]) {
	case "scroll", "control.scroll":
		val := semanticInt(action["value"])
		sb.scroll(val)
		return true
	}
	return false
}

func (m *VMenu) HandleSemanticAction(action map[string]any) bool {
	target := semanticString(action["target"])
	if SemanticID(m) == target {
		switch semanticString(action["action"]) {
		case "close", "menu.close":
			m.Close()
			return true
		case "menu_activate", "menu.activate":
			idx := semanticInt(action["index"])
			if idx >= 0 && idx < len(m.Items) && !m.Items[idx].Separator {
				m.SetSelectPos(idx)
				return m.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
			}
		}
	}
	return false
}

func semanticMenuBar(mb *MenuBar) map[string]any {
	if mb == nil {
		return nil
	}
	items := make([]map[string]any, 0, len(mb.Items))
	for i, item := range mb.Items {
		clean, hotkey, _ := ParseAmpersandString(item.Label)
		itemX := mb.GetItemX(i)

		itemW := 0
		if i < len(mb.Items)-1 {
			itemW = mb.GetItemX(i+1) - itemX
		} else {
			itemW = runewidth.StringWidth("  " + clean + "  ")
		}

		subItems := make([]map[string]any, 0, len(item.SubItems))
		for j, sub := range item.SubItems {
			subClean, subHotkey, _ := ParseAmpersandString(sub.Text)
			subItems = append(subItems, map[string]any{
				"index":     j,
				"text":      subClean,
				"rawText":   sub.Text,
				"hotkey":    stringOrEmpty(subHotkey),
				"shortcut":  sub.Shortcut,
				"command":   sub.Command,
				"separator": sub.Separator,
			})
		}

		items = append(items, map[string]any{
			"index":    i,
			"x":        itemX,
			"w":        itemW,
			"text":     clean,
			"rawText":  item.Label,
			"hotkey":   stringOrEmpty(hotkey),
			"command":  item.Command,
			"disabled": false,
			"items":    subItems,
		})
	}
	x1, y1, x2, y2 := mb.GetPosition()
	return map[string]any{
		"id":       SemanticID(mb),
		"kind":     "menuBar",
		"x":        x1,
		"y":        y1,
		"w":        x2 - x1 + 1,
		"h":        y2 - y1 + 1,
		"active":   mb.Active,
		"selected": mb.SelectPos,
		"items":    items,
	}
}

func semanticKeyBar(kb *KeyBar) map[string]any {
	labels := kb.Normal
	modifier := "normal"
	if kb.shiftState {
		labels = kb.Shift
		modifier = "shift"
	} else if kb.ctrlState {
		labels = kb.Ctrl
		modifier = "ctrl"
	} else if kb.altState {
		labels = kb.Alt
		modifier = "alt"
	}
	items := make([]map[string]any, 0, len(labels))
	for i, label := range labels {
		items = append(items, map[string]any{
			"index": i,
			"key":   fmt.Sprintf("F%d", i+1),
			"text":  label,
		})
	}
	x1, y1, x2, y2 := kb.GetPosition()
	return map[string]any{
		"id":       SemanticID(kb),
		"kind":     "keyBar",
		"x":        x1,
		"y":        y1,
		"w":        x2 - x1 + 1,
		"h":        y2 - y1 + 1,
		"visible":  kb.IsVisible(),
		"modifier": modifier,
		"items":    items,
	}
}

func stringOrEmpty(r rune) string {
	if r == 0 {
		return ""
	}
	return string(r)
}

func semanticString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func semanticInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

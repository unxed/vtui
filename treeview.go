package vtui

import (
	"strings"

	"github.com/unxed/vtinput"
)

// TreeNode represents a single item in the TreeView.
type TreeNode struct {
	Text     string
	Children []*TreeNode
	Expanded bool
	Data     any

	parent *TreeNode
}

// AddChild adds a child node and sets its parent.
func (n *TreeNode) AddChild(child *TreeNode) {
	child.parent = n
	n.Children = append(n.Children, child)
}

// Parent returns the parent node, or nil if this is the root.
func (n *TreeNode) Parent() *TreeNode {
	return n.parent
}

type flatNode struct {
	node   *TreeNode
	level  int
	isLast []bool // Indicates if the ancestor at each level is the last child
}

// TreeView displays hierarchical data in an expandable tree structure.
type TreeView struct {
	ScrollView
	Root                 *TreeNode
	ShowRoot             bool
	ColorTextIdx         int
	ColorSelectedTextIdx int
	ColorTreeLineIdx     int
	ColorBoxIdx          int

	flatNodes []flatNode
}

func NewTreeView(x, y, w, h int, root *TreeNode) *TreeView {
	tv := &TreeView{
		Root:                 root,
		ShowRoot:             true,
		ColorTextIdx:         ColTableText,
		ColorSelectedTextIdx: ColTableSelectedText,
		ColorTreeLineIdx:     ColTreeLine,
		ColorBoxIdx:          ColTableBox,
	}
	tv.canFocus = true
	tv.ShowScrollBar = true
	tv.InitScrollBar(tv)

	// Define default action: toggle expansion for folders,
	// or trigger user callback for leaves.
	tv.OnAction = func(idx int) {
		if idx < 0 || idx >= len(tv.flatNodes) {
			return
		}
		node := tv.flatNodes[idx].node

		// 1. Fire the action first (Command bubbling)
		tv.FireAction(nil, node)

		// 2. Toggle expansion
		if len(node.Children) > 0 {
			node.Expanded = !node.Expanded
			tv.Flatten()
		}
	}

	tv.SetPosition(x, y, x+w-1, y+h-1)
	tv.Flatten()
	return tv
}

// Flatten rebuilds the internal flat list of visible nodes based on expansion state.
func (t *TreeView) Flatten() {
	t.flatNodes = nil
	if t.Root == nil {
		return
	}

	var traverse func(node *TreeNode, level int, isLast []bool)
	traverse = func(node *TreeNode, level int, isLast []bool) {
		t.flatNodes = append(t.flatNodes, flatNode{
			node:   node,
			level:  level,
			isLast: append([]bool(nil), isLast...),
		})
		if node.Expanded {
			for i, child := range node.Children {
				childIsLast := i == len(node.Children)-1
				traverse(child, level+1, append(isLast, childIsLast))
			}
		}
	}

	if t.ShowRoot {
		traverse(t.Root, 0, []bool{true})
	} else {
		for i, child := range t.Root.Children {
			childIsLast := i == len(t.Root.Children)-1
			traverse(child, 0, []bool{childIsLast})
		}
	}

	t.ItemCount = len(t.flatNodes)
	// Ensure ViewHeight is updated in case ItemCount relies on it, though SetPosition handles it.
	if t.SelectPos >= t.ItemCount {
		t.SelectPos = t.ItemCount - 1
	}
	if t.SelectPos < 0 {
		t.SelectPos = 0
	}
	t.EnsureVisible()
}

func (t *TreeView) Show(scr *ScreenBuf) {
	t.ScreenObject.Show(scr)
	t.DisplayObject(scr)
}

func (t *TreeView) DisplayObject(scr *ScreenBuf) {
	if !t.IsVisible() {
		return
	}

	width := t.X2 - t.X1 + 1
	height := t.Y2 - t.Y1 + 1

	colText := Palette[t.ColorTextIdx]
	colSel := Palette[t.ColorSelectedTextIdx]
	colLine := Palette[t.ColorTreeLineIdx]

	for i := 0; i < height; i++ {
		idx := t.TopPos + i
		currY := t.Y1 + i

		if idx < len(t.flatNodes) {
			fn := t.flatNodes[idx]

			attr := colText
			if idx == t.SelectPos {
				if t.IsFocused() {
					attr = Palette[ColDialogHighlightSelectedButton]
				} else {
					attr = colSel
				}
			}
			if t.IsDisabled() {
				attr = DimColor(attr)
			}

			// Build tree lines
			var sb strings.Builder
			for lvl := 0; lvl < fn.level; lvl++ {
				if fn.isLast[lvl] {
					sb.WriteString("  ")
				} else {
					sb.WriteRune(boxSymbols[bsV]) // '│'
					sb.WriteRune(' ')
				}
			}

			if fn.node != t.Root || !t.ShowRoot {
				if fn.isLast[fn.level] {
					sb.WriteRune(boxSymbols[4])   // '└'
					sb.WriteRune(boxSymbols[bsH]) // '─'
				} else {
					sb.WriteRune(boxSymbols[6])   // '├'
					sb.WriteRune(boxSymbols[bsH]) // '─'
				}
			}

			marker := " "
			if len(fn.node.Children) > 0 {
				if fn.node.Expanded {
					marker = "[-] "
				} else {
					marker = "[+] "
				}
			}

			prefixStr := sb.String()
			textStr := marker + fn.node.Text

			// Clip string if too long
			prefixWidth := StringWidth(prefixStr)
			textWidth := StringWidth(textStr)

			if prefixWidth+textWidth > width {
				textStr = TruncateString(textStr, width-prefixWidth, "")
				textWidth = StringWidth(textStr)
			}

			scr.Write(t.X1, currY, StringToCharInfo(prefixStr, colLine))
			scr.Write(t.X1+prefixWidth, currY, StringToCharInfo(textStr, attr))

			// Fill remaining
			fillWidth := width - prefixWidth - textWidth
			if fillWidth > 0 {
				scr.FillRect(t.X1+prefixWidth+textWidth, currY, t.X2, currY, ' ', attr)
			}
		} else {
			scr.FillRect(t.X1, currY, t.X2, currY, ' ', colText)
		}
	}

	// Scrollbar
	t.DrawScrollBar(scr)
}

func (t *TreeView) ProcessKey(e *vtinput.InputEvent) bool {
	e = TextInputView(e)
	if !e.KeyDown || t.IsDisabled() || len(t.flatNodes) == 0 {
		return false
	}
	fn := t.flatNodes[t.SelectPos]

	switch e.VirtualKeyCode {
	case vtinput.VK_LEFT:
		if fn.node.Expanded && len(fn.node.Children) > 0 {
			fn.node.Expanded = false
			t.Flatten()
			return true
		} else if fn.node.parent != nil {
			for i := t.SelectPos - 1; i >= 0; i-- {
				if t.flatNodes[i].node == fn.node.parent {
					t.SetSelectPos(i)
					break
				}
			}
			return true
		}
	case vtinput.VK_RIGHT:
		if len(fn.node.Children) > 0 {
			if !fn.node.Expanded {
				fn.node.Expanded = true
				t.Flatten()
				return true
			}
			if t.SelectPos < len(t.flatNodes)-1 {
				t.SetSelectPos(t.SelectPos + 1)
				return true
			}
		}
	case vtinput.VK_SPACE:
		if t.OnAction != nil {
			t.OnAction(t.SelectPos)
		}
		return true
	}

	return t.HandleKey(e)
}

func (t *TreeView) ProcessMouse(e *vtinput.InputEvent) bool {
	if t.IsDisabled() {
		return false
	}

	// Custom handling for [+] [-] icon clicks before generic list selection
	if e.Type == vtinput.MouseEventType && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown {
		mx, my := int(e.MouseX), int(e.MouseY)
		clickIdx := t.GetClickIndex(my)
		if clickIdx != -1 {
			fn := t.flatNodes[clickIdx]
			prefixWidth := fn.level*2 + map[bool]int{true: 0, false: 2}[fn.node == t.Root && t.ShowRoot]
			if mx >= t.X1+prefixWidth && mx < t.X1+prefixWidth+3 && len(fn.node.Children) > 0 {
				fn.node.Expanded = !fn.node.Expanded
				t.Flatten()
				t.SetSelectPos(clickIdx)
				return true
			}
		}
	}

	return t.HandleMouse(e)
}

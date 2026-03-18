package vtui

import (
	"os"
)
import (
	"testing"
)

func TestSelectDirDialog_Navigation(t *testing.T) {
	SetDefaultPalette()
	// Use OSVFS on a temp directory for testing
	tmpDir := t.TempDir()
	vfs := NewOSVFS(tmpDir)

	dlg := SelectDirDialog("Test", tmpDir, vfs)

	if dlg == nil {
		t.Fatal("Failed to create SelectDirDialog")
	}

	// Verify Edit field has the path
	var pathEdit *Edit
	for _, item := range dlg.items {
		if e, ok := item.(*Edit); ok {
			pathEdit = e
			break
		}
	}

	if pathEdit == nil || pathEdit.GetText() == "" {
		t.Error("Path Edit field not found or empty")
	}

	// Find the ListBox
	var lb *ListBox
	for _, item := range dlg.items {
		if l, ok := item.(*ListBox); ok {
			lb = l
			break
		}
	}

	if lb == nil {
		t.Fatal("ListBox not found in dialog")
	}

	// Navigation logic check: clicking ".." (index 0)
	if lb.OnChange != nil {
		lb.OnChange(0)
	}

	// After going up from tmpDir, we should be in its parent
	// pathEdit should be updated
	if pathEdit.GetText() == tmpDir {
		t.Error("Path Edit was not updated after navigation")
	}
}

func TestSelectDirDialog_ArrowVsEnter(t *testing.T) {
	SetDefaultPalette()
	tmpDir := t.TempDir()
	vfs := NewOSVFS(tmpDir)

	dlg := SelectDirDialog("Test", tmpDir, vfs)

	var lb *ListBox
	for _, item := range dlg.items {
		if l, ok := item.(*ListBox); ok { lb = l; break }
	}

	initialPath := vfs.GetPath()

	// 1. Simulate Down Arrow (Select index 0, which is "..")
	// This should trigger OnChange but NOT change the VFS path.
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})

	if vfs.GetPath() != initialPath {
		t.Errorf("Path changed on Arrow Key! Expected %s, got %s", initialPath, vfs.GetPath())
	}

	// 2. Simulate Enter (Action)
	// This should change the VFS path.
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	if vfs.GetPath() == initialPath {
		t.Error("Path DID NOT change on Enter Key")
	}
}

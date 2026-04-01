package vtui

import (
	"os"
	"path/filepath"
	"testing"
	"github.com/unxed/vtinput"
)

// testVFS implements VFSMinimal for testing without coupling to f4's VFS.
type testVFS struct { currentPath string }
func (v *testVFS) GetPath() string { return v.currentPath }
func (v *testVFS) SetPath(p string) error { v.currentPath = p; return nil }
func (v *testVFS) ReadDir(p string) ([]VFSItem, error) {
	entries, _ := os.ReadDir(p)
	items := make([]VFSItem, 0)
	for _, e := range entries { items = append(items, VFSItem{Name: e.Name(), IsDir: e.IsDir()}) }
	return items, nil
}
func (v *testVFS) Join(elem ...string) string { return filepath.Join(elem...) }
func (v *testVFS) Dir(p string) string { return filepath.Dir(p) }
func (v *testVFS) Base(p string) string { return filepath.Base(p) }

func TestSelectDirDialog_ArrowVsEnter(t *testing.T) {
	SetDefaultPalette()
	tmpDir := t.TempDir()
	vfs := &testVFS{currentPath: tmpDir}

	dlg := SelectDirDialog("Test", tmpDir, vfs)

	var lb *ListBox
	for _, item := range dlg.rootGroup.items {
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
func TestInputBox_OkCallback(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewScreenBuf())

	received := ""
	onOk := func(s string) { received = s }

	dlg := InputBox("Title", "Prompt", "DefaultValue", onOk)

	// Find Edit and Button
	var edit *Edit
	var okBtn *Button
	for _, item := range dlg.rootGroup.items {
		if e, ok := item.(*Edit); ok { edit = e }
		if b, ok := item.(*Button); ok && b.hotkey == 'o' { okBtn = b }
	}

	if edit == nil || okBtn == nil { t.Fatal("Dialog structure missing components") }

	edit.SetText("NewValue")
	if okBtn.OnClick != nil {
		okBtn.OnClick()
	}

	if received != "NewValue" {
		t.Errorf("Expected 'NewValue', got '%s'", received)
	}
	if !dlg.IsDone() {
		t.Error("Dialog should be finished after Ok")
	}
}

func TestSelectFileDialog_Selection(t *testing.T) {
	SetDefaultPalette()
	tmpDir := t.TempDir()
	vfs := &testVFS{currentPath: tmpDir}

	// Create a dummy file
	os.WriteFile(vfs.Join(tmpDir, "dummy.txt"), []byte("data"), 0644)

	dlg := SelectFileDialog("Title", tmpDir, vfs)

	var lb *ListBox
	var fileEdit *Edit
	editCount := 0
	for _, item := range dlg.rootGroup.items {
		if l, ok := item.(*ListBox); ok {
			lb = l
		}
		if e, ok := item.(*Edit); ok {
			editCount++
			if editCount == 2 { // fileEdit is the second Edit field
				fileEdit = e
	if lb == nil || fileEdit == nil { t.Fatal("SelectFileDialog structure error") }

	// Find dummy.txt in list
	fileIdx := -1
	for i, name := range lb.Items {
		if name == "dummy.txt" { fileIdx = i; break }
	}

	if fileIdx == -1 { t.Fatal("File not found in list") }

	// Change selection to file
	if lb.OnSelect != nil {
		lb.OnSelect(fileIdx)
	}

	if fileEdit.GetText() != "dummy.txt" {
		t.Errorf("File Edit not updated on selection. Got %q", fileEdit.GetText())
	}
}
		}
	}
}

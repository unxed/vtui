package vtui

import (
	"os"
	"path/filepath"
	"testing"
	"context"
	"time"
	"github.com/unxed/vtinput"
)

// testVFS implements VFSMinimal for testing without coupling to f4's VFS.
type testVFS struct { currentPath string }
func (v *testVFS) GetPath() string { return v.currentPath }
func (v *testVFS) SetPath(p string) error { v.currentPath = p; return nil }
func (v *testVFS) ReadDir(ctx context.Context, p string, onChunk func([]VFSItem)) error {
	entries, _ := os.ReadDir(p)
	items := make([]VFSItem, 0)
	for _, e := range entries { items = append(items, VFSItem{Name: e.Name(), IsDir: e.IsDir()}) }
	if len(items) > 0 && onChunk != nil { onChunk(items) }
	return nil
}
func (v *testVFS) Join(elem ...string) string { return filepath.Join(elem...) }
func (v *testVFS) Dir(p string) string { return filepath.Dir(p) }
func (v *testVFS) Base(p string) string { return filepath.Base(p) }

// pumpTasks executes all pending tasks in the FrameManager queue.
func pumpTasks() {
	for {
		select {
		case task := <-FrameManager.TaskChan:
			task()
		default:
			return
		}
	}
}

// waitForCondition waits for a predicate to become true, pumping tasks in between.
func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pumpTasks()
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Timeout waiting for condition in async test")
}

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
	v := &testVFS{currentPath: tmpDir}
	os.WriteFile(filepath.Join(tmpDir, "dummy.txt"), []byte("data"), 0644)

	dlg := SelectFileDialog("Title", tmpDir, v)
	var lb *ListBox
	var fileEdit *Edit
	walk(dlg.rootGroup, func(el UIElement) bool {
		if l, ok := el.(*ListBox); ok { lb = l }
		if t, ok := el.(*Text); ok && el.GetHotkey() == 'f' {
			if e, ok := t.FocusLink.(*Edit); ok { fileEdit = e }
		}
		return true
	})

	// Wait for async loading
	waitForCondition(t, time.Second, func() bool {
		for _, item := range lb.Items { if item == "dummy.txt" { return true } }
		return false
	})

	// Change selection and check if Edit field updates
	lb.SelectName("dummy.txt")
	if lb.OnSelect != nil { lb.OnSelect(lb.SelectPos) }

	if fileEdit.GetText() != "dummy.txt" {
		t.Errorf("File Edit not updated. Got %q", fileEdit.GetText())
	}
}

func TestSelectDirDialog_Filtering(t *testing.T) {
	SetDefaultPalette()
	tmpDir := t.TempDir()
	os.Mkdir(filepath.Join(tmpDir, "subfolder"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "should_be_hidden.txt"), []byte("data"), 0644)

	v := &testVFS{currentPath: tmpDir}
	dlg := SelectDirDialog("Select Dir", tmpDir, v)

	var lb *ListBox
	walk(dlg.rootGroup, func(el UIElement) bool {
		if l, ok := el.(*ListBox); ok { lb = l; return false }; return true
	})

	waitForCondition(t, time.Second, func() bool {
		return len(lb.Items) > 1 // ".." + "subfolder"
	})

	for _, item := range lb.Items {
		if item == "should_be_hidden.txt" {
			t.Error("SelectDirDialog MUST NOT show files, only directories")
		}
	}
}

func TestDialogNavigation_UX(t *testing.T) {
	SetDefaultPalette()
	tmpDir := t.TempDir()
	subPath := filepath.Join(tmpDir, "my_work_folder")
	os.Mkdir(subPath, 0755)

	v := &testVFS{currentPath: tmpDir}
	dlg := SelectFileDialog("UX Test", tmpDir, v)

	var lb *ListBox
	walk(dlg.rootGroup, func(el UIElement) bool {
		if l, ok := el.(*ListBox); ok { lb = l; return false }; return true
	})

	// 1. Enter into "my_work_folder"
	waitForCondition(t, time.Second, func() bool {
		for _, item := range lb.Items { if item == "my_work_folder" { return true } }
		return false
	})
	lb.SelectName("my_work_folder")
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	// 2. Assert path changed and UI updated (list should now contain only "..")
	waitForCondition(t, time.Second, func() bool {
		return v.GetPath() == subPath && len(lb.Items) == 1 && lb.Items[0] == ".."
	})
	if lb.SelectPos != 0 {
		t.Errorf("UX FAIL: Cursor must be on '..' (0) when entering an empty directory. Got pos: %d", lb.SelectPos)
	}

	// 3. Exit back to parent via ".."
	lb.SelectName("..")
	lb.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})

	// 4. Assert path restored and UI updated (cursor should land on "my_work_folder")
	waitForCondition(t, time.Second, func() bool {
		return v.GetPath() == tmpDir && lb.Items[lb.SelectPos] == "my_work_folder"
	})
}

func TestSelectFileDialog_LayoutBestPractice(t *testing.T) {
	SetDefaultPalette()
	FrameManager.Init(NewScreenBuf())
	v := &testVFS{currentPath: "/tmp"}

	// Create dialog (55x20)
	dlg := SelectFileDialog("LayoutTest", "/tmp", v)

	var fileEdit *Edit
	var btnOk *Button
	var lb *ListBox

	walk(dlg.rootGroup, func(el UIElement) bool {
		if t, ok := el.(*Text); ok && el.GetHotkey() == 'f' {
			if e, ok := t.FocusLink.(*Edit); ok { fileEdit = e }
		}
		if b, ok := el.(*Button); ok && b.GetHotkey() == 'o' { btnOk = b }
		if l, ok := el.(*ListBox); ok { lb = l }
		return true
	})

	if fileEdit == nil || btnOk == nil || lb == nil {
		t.Fatal("Required components not found in dialog")
	}

	// 1. Check ListBox stretch
	lx1, _, lx2, _ := lb.GetPosition()
	if lx1 < dlg.X1 || lx2 > dlg.X2 {
		t.Errorf("ListBox bounds invalid: %d..%d", lx1, lx2)
	}

	// 2. Check File Edit stretch
	ex1, _, ex2, _ := fileEdit.GetPosition()
	if ex1 <= dlg.X1+2 {
		t.Errorf("File Edit overlap with label: X1=%d", ex1)
	}
	if ex2 < ex1 {
		t.Errorf("File Edit has negative width: X1=%d, X2=%d", ex1, ex2)
	}

	// 3. Check Button centering
	bx1, _, _, _ := btnOk.GetPosition()
	if bx1 < dlg.X1 {
		t.Errorf("Button out of bounds: X1=%d", bx1)
	}
}
func TestLayout_StandardDialogs_Validity(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	FrameManager.Init(scr)
	v := &testVFS{currentPath: "/tmp"}

	t.Run("SelectFileDialog", func(t *testing.T) {
		dlg := SelectFileDialog("Test", "/tmp", v)
		AssertLayout(t, dlg)
	})

	t.Run("InputBox", func(t *testing.T) {
		dlg := InputBox("Title", "Prompt", "Val", nil)
		AssertLayout(t, dlg)
	})

	t.Run("MessageDialog", func(t *testing.T) {
		dlg := createMessageDialog("Title", "Multi\nLine\nText", []string{"&Ok", "&Cancel"})
		AssertLayout(t, dlg)
	})
}

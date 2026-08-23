//go:build windows

package vtui

import (
	"syscall"
	"testing"
	"unsafe"
)

func TestWin32DnD_DropEffectConversion(t *testing.T) {
	cases := []struct {
		action DropAction
		effect uint32
	}{
		{DropNone, dropEffectNone},
		{DropCopy, dropEffectCopy},
		{DropMove, dropEffectMove},
		{DropLink, dropEffectLink},
	}

	for _, tc := range cases {
		eff := dropActionToDropEffect(tc.action)
		if eff != tc.effect {
			t.Errorf("dropActionToDropEffect(%v) = %d, want %d", tc.action, eff, tc.effect)
		}
		act := dropEffectToDropAction(tc.effect)
		if act != tc.action {
			t.Errorf("dropEffectToDropAction(%d) = %v, want %v", tc.effect, act, tc.action)
		}
	}

	// A mask naming more than one effect does not round-trip, by design. The
	// two directions answer different questions: dropActionToDropEffect turns
	// the set of actions a target permits into the mask that names them, while
	// dropEffectToDropAction reads back the one effect a completed drop
	// performed, which is what DoDragDrop leaves in pdwEffect. Inverting the
	// mask direction is dropEffectToAllowedActions' job -- see the comment on
	// it in win32_droptarget_windows.go.
	const both = dropEffectCopy | dropEffectMove
	if eff := dropActionToDropEffect(DropCopy | DropMove); eff != both {
		t.Errorf("dropActionToDropEffect(copy|move) = %d, want %d", eff, both)
	}
	if act := dropEffectToAllowedActions(both); act != DropCopy|DropMove {
		t.Errorf("dropEffectToAllowedActions(copy|move) = %v, want %v", act, DropCopy|DropMove)
	}
	if act := dropEffectToDropAction(both); act != DropCopy {
		t.Errorf("dropEffectToDropAction(copy|move) = %v, want %v", act, DropCopy)
	}
}

func TestWin32DnD_BuildHDROP(t *testing.T) {
	paths := []string{"C:\\test1.txt", "C:\\test2.txt"}
	hGlobal, err := buildHDROP(paths)
	if err != nil {
		t.Fatalf("buildHDROP failed: %v", err)
	}
	defer procGlobalFree.Call(hGlobal)

	ptr, _, _ := procGlobalLock.Call(hGlobal)
	if ptr == 0 {
		t.Fatal("GlobalLock failed")
	}
	defer procGlobalUnlock.Call(hGlobal)

	df := (*dropFiles)(unsafe.Pointer(ptr))
	if df.pFiles != uint32(unsafe.Sizeof(dropFiles{})) {
		t.Errorf("pFiles offset = %d, want %d", df.pFiles, unsafe.Sizeof(dropFiles{}))
	}
	if df.fWide != 1 {
		t.Errorf("fWide = %d, want 1 (Unicode)", df.fWide)
	}

	rawChars := (*uint16)(unsafe.Pointer(ptr + uintptr(df.pFiles)))
	str1 := syscall.UTF16ToString(unsafe.Slice(rawChars, 100))
	if str1 != "C:\\test1.txt" {
		t.Errorf("first path = %q, want 'C:\\test1.txt'", str1)
	}
}

func TestWin32DnD_DataObjectAndDropSourceLifecycle(t *testing.T) {
	dataObj := newComDataObject([]string{"C:\\file.txt"})
	if dataObj.refCount != 1 {
		t.Errorf("initial dataObj refCount = %d, want 1", dataObj.refCount)
	}

	// QueryInterface
	var pObj uintptr
	hr := comDataObjectQueryInterface(dataObj.toIUnknown(), &iidIDataObject, &pObj)
	if hr != sOK || pObj == 0 {
		t.Fatalf("QueryInterface for IDataObject failed: hr=0x%X", hr)
	}
	if dataObj.refCount != 2 {
		t.Errorf("dataObj refCount after QueryInterface = %d, want 2", dataObj.refCount)
	}
	comDataObjectRelease(dataObj.toIUnknown())

	// QueryGetData
	fe := formatEtc{cfFormat: cfHDROP, tymed: tymedHGLOBAL}
	if hr := comDataObjectQueryGetData(dataObj.toIUnknown(), &fe); hr != sOK {
		t.Errorf("QueryGetData for CF_HDROP returned hr=0x%X, want S_OK", hr)
	}

	feBad := formatEtc{cfFormat: 9999, tymed: tymedHGLOBAL}
	if hr := comDataObjectQueryGetData(dataObj.toIUnknown(), &feBad); hr != dvEFormatEtc {
		t.Errorf("QueryGetData for invalid format returned hr=0x%X, want DV_E_FORMATETC", hr)
	}

	// DropSource
	dropSrc := newComDropSource()
	if hr := comDropSourceQueryContinueDrag(dropSrc.toIUnknown(), 1, 0); hr != dragDropSCancel {
		t.Errorf("QueryContinueDrag on ESC returned hr=0x%X, want DRAGDROP_S_CANCEL", hr)
	}
	if hr := comDropSourceQueryContinueDrag(dropSrc.toIUnknown(), 0, 0); hr != dragDropSDrop {
		t.Errorf("QueryContinueDrag on mouse release returned hr=0x%X, want DRAGDROP_S_DROP", hr)
	}
	if hr := comDropSourceQueryContinueDrag(dropSrc.toIUnknown(), 0, 0x0001); hr != sOK {
		t.Errorf("QueryContinueDrag with LBUTTON held returned hr=0x%X, want S_OK", hr)
	}
}

func TestWin32DnD_EnumeratorSurvivesBeingHandedOut(t *testing.T) {
	// EnumFormatEtc hands its result to a caller that lives outside Go. The
	// only thing keeping that object alive is the pin registry, so the pin
	// has to exist while the enumerator is in flight and be gone once its
	// reference count reaches zero -- otherwise a collection mid-drag turns
	// the enumerator into whatever happens to occupy its address next.
	before := comLiveCount()

	dataObj := newComDataObject([]string{"C:\\file.txt"})
	var pEnum uintptr
	if hr := comDataObjectEnumFormatEtc(dataObj.toIUnknown(), dataDirGet, &pEnum); hr != sOK {
		t.Fatalf("EnumFormatEtc returned hr=0x%X, want S_OK", hr)
	}
	if pEnum == 0 {
		t.Fatal("EnumFormatEtc handed out a null enumerator")
	}
	if comLiveCount() != before+2 {
		t.Fatalf("live COM objects = %d, want %d (data object plus enumerator)",
			comLiveCount(), before+2)
	}

	var fetched uint32
	var got formatEtc
	if hr := comEnumFormatEtcNext(pEnum, 1, &got, &fetched); hr != sOK || fetched != 1 {
		t.Fatalf("Next returned hr=0x%X fetched=%d, want S_OK and 1", hr, fetched)
	}
	if got.cfFormat != cfHDROP {
		t.Errorf("first advertised format = %d, want CF_HDROP", got.cfFormat)
	}

	if n := comEnumFormatEtcRelease(pEnum); n != 0 {
		t.Errorf("enumerator refCount after release = %d, want 0", n)
	}
	if n := comDataObjectRelease(dataObj.toIUnknown()); n != 0 {
		t.Errorf("data object refCount after release = %d, want 0", n)
	}
	if comLiveCount() != before {
		t.Errorf("live COM objects = %d after both releases, want %d", comLiveCount(), before)
	}
}

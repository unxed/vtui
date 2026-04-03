package vtui

import (
	"testing"
)

func TestDynamicText_Update(t *testing.T) {
	SetDefaultPalette()
	counter := 0
	dt := NewDynamicText(0, 0, 10, 0, func() string {
		counter++
		return "val"
	})

	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1)

	// Every Show() call should trigger the callback
	dt.Show(scr)
	if counter != 1 { t.Errorf("Callback not called on first Show, count: %d", counter) }
	if dt.content != "val" { t.Error("Content not updated from callback") }

	dt.Show(scr)
	if counter != 2 { t.Errorf("Callback not called on second Show, count: %d", counter) }
}
package vtui

import (
	"testing"
	"github.com/unxed/vtinput"
)

func TestX11_KeysymMapping(t *testing.T) {
	tests := []struct {
		keysym uint32
		wantVK uint16
	}{
		{0xff51, vtinput.VK_LEFT},
		{0xff8d, vtinput.VK_RETURN},   // KP_Enter
		{0xffb5, vtinput.VK_NUMPAD5}, // KP_5
		{0xffab, vtinput.VK_ADD},     // KP_Add
		{0x0061, vtinput.VK_A},       // 'a'
		{0x0041, vtinput.VK_A},       // 'A'
	}

	for _, tt := range tests {
		got := keysymToVK(tt.keysym)
		if got != tt.wantVK {
			t.Errorf("keysymToVK(0x%x) = 0x%x, want 0x%x", tt.keysym, got, tt.wantVK)
		}
	}
}
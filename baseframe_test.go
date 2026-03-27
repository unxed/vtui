package vtui

import "testing"

func TestBaseFrame_OnResult(t *testing.T) {
	bf := &BaseFrame{}
	result := -100
	bf.OnResult = func(code int) {
		result = code
	}
	bf.SetExitCode(42)
	if result != 42 {
		t.Errorf("OnResult callback failed, expected 42, got %d", result)
	}
}
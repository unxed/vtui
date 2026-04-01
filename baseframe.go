package vtui

// BaseFrame provides a default implementation for the Frame interface.
// Other frames should embed this to avoid boilerplate.
type BaseFrame struct {
	ScreenObject
	Done     bool
	ExitCode int
	Modal    bool
	Number   int
	OnResult func(int)
	Busy     bool
	AttentionSuppressed bool
}

func (bf *BaseFrame) SetExitCode(code int) {
	bf.Done = true
	bf.ExitCode = code
	if bf.OnResult != nil {
		bf.OnResult(code)
	}
}

func (bf *BaseFrame) IsDone() bool         { return bf.Done }
func (bf *BaseFrame) IsBusy() bool         { return bf.Busy }
func (bf *BaseFrame) IsAttentionSuppressed() bool { return bf.AttentionSuppressed }
func (bf *BaseFrame) IsModal() bool        { return bf.Modal }
func (bf *BaseFrame) GetWindowNumber() int { return bf.Number }
func (bf *BaseFrame) SetWindowNumber(n int) { bf.Number = n }
func (bf *BaseFrame) RequestFocus() bool   { return true }
func (bf *BaseFrame) Close()               { bf.SetExitCode(-1) }
func (bf *BaseFrame) HasShadow() bool      { return false }
func (bf *BaseFrame) GetKeyLabels() *KeySet { return nil }
func (bf *BaseFrame) GetMenuBar() *MenuBar  { return nil }
func (bf *BaseFrame) ResizeConsole(w, h int) {}
func (bf *BaseFrame) GetTitle() string       { return "" }
func (bf *BaseFrame) GetProgress() int       { return -1 }


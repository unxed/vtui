package vtui

import (
	"os"
	"path/filepath"
)

// OSVFS implements VFS for the local host operating system.
type OSVFS struct {
	currentPath string
}

func NewOSVFS(initialPath string) *OSVFS {
	abs, _ := filepath.Abs(initialPath)
	return &OSVFS{currentPath: abs}
}

func (v *OSVFS) GetPath() string { return v.currentPath }
func (v *OSVFS) SetPath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil { return err }
	v.currentPath = abs
	return nil
}

func (v *OSVFS) ReadDir(path string) ([]VFSItem, error) {
	entries, err := os.ReadDir(path)
	if err != nil { return nil, err }
	items := make([]VFSItem, 0, len(entries))
	for _, e := range entries {
		info, _ := e.Info()
		items = append(items, VFSItem{
			Name:  e.Name(),
			Size:  info.Size(),
			IsDir: e.IsDir(),
			MTime: info.ModTime(),
		})
	}
	return items, nil
}

func (v *OSVFS) Stat(path string) (VFSItem, error) {
	info, err := os.Stat(path)
	if err != nil { return VFSItem{}, err }
	return VFSItem{
		Name:  info.Name(),
		Size:  info.Size(),
		IsDir: info.IsDir(),
		MTime: info.ModTime(),
	}, nil
}

func (v *OSVFS) Join(elem ...string) string { return filepath.Join(elem...) }
func (v *OSVFS) Abs(path string) (string, error) { return filepath.Abs(path) }
func (v *OSVFS) Base(path string) string { return filepath.Base(path) }
func (v *OSVFS) Dir(path string) string { return filepath.Dir(path) }
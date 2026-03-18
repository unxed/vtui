package vtui

import "time"

// VFSItem represents a generic file or directory entry in any VFS.
type VFSItem struct {
	Name  string
	Size  int64
	IsDir bool
	MTime time.Time
	Mode  string
}

// VFS (Virtual File System) is the interface for any data provider.
type VFS interface {
	GetPath() string
	SetPath(path string) error
	ReadDir(path string) ([]VFSItem, error)
	Stat(path string) (VFSItem, error)
	Join(elem ...string) string
	Abs(path string) (string, error)
	Base(path string) string
	Dir(path string) string
}
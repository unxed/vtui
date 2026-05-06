//go:build linux

package vtui

import (
	"unsafe"

	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/shm"
	"github.com/BurntSushi/xgb/xproto"
	"golang.org/x/sys/unix"
)

var (
	shmId   int
	shmAddr uintptr
	shmData []byte
)

func init() {
	// Allocate 32MB segment (sufficient for a 4K display)
	size := 3840 * 2160 * 4

	r1, _, err := unix.Syscall(unix.SYS_SHMGET, uintptr(unix.IPC_PRIVATE), uintptr(size), uintptr(unix.IPC_CREAT|0600))
	if err != 0 {
		DebugLog("X11: shmget failed: %v", err)
		return
	}
	id := int(r1)

	r1, _, err = unix.Syscall(unix.SYS_SHMAT, uintptr(id), 0, 0)
	if err != 0 {
		unix.Syscall(unix.SYS_SHMCTL, uintptr(id), uintptr(unix.IPC_RMID), 0)
		DebugLog("X11: shmat failed: %v", err)
		return
	}
	addr := r1

	shmId = id
	shmAddr = addr
	shmData = unsafe.Slice((*byte)(unsafe.Pointer(shmAddr)), size)
	DebugLog("X11: Allocated shared memory segment (ID: %d)", shmId)
}
func x11shmInit(conn *xgb.Conn, id int) uint32 {
	if err := shm.Init(conn); err != nil {
		return 0
	}
	seg, err := shm.NewSegId(conn)
	if err != nil {
		return 0
	}
	shm.Attach(conn, seg, uint32(id), false)
	return uint32(seg)
}

func x11shmDetach(conn *xgb.Conn, seg uint32) {
	shm.Detach(conn, shm.Seg(seg))
}

func x11shmPutImage(conn *xgb.Conn, wid xproto.Window, gc xproto.Gcontext, w, h2 uint16, minY, maxY int, seg uint32) {
	shm.PutImage(conn, xproto.Drawable(wid), gc,
		w, h2,
		0, uint16(minY),
		w, uint16(maxY-minY+1),
		0, int16(minY),
		24, 2, 0,
		shm.Seg(seg), 0)
}

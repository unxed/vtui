//go:build openbsd || netbsd || dragonfly || darwin

package vtui

import (
	"github.com/BurntSushi/xgb"
	"github.com/BurntSushi/xgb/xproto"
)

var (
	shmId   int
	shmAddr uintptr
	shmData []byte
)

func x11shmInit(conn *xgb.Conn, id int) uint32 { return 0 }
func x11shmDetach(conn *xgb.Conn, seg uint32)  {}
func x11shmPutImage(conn *xgb.Conn, wid xproto.Window, gc xproto.Gcontext, w, h2 uint16, minY, maxY int, seg uint32) {
}
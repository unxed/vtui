//go:build windows

package vtui

import (
	"strings"
	"time"

	winescape "github.com/unxed/libwinescape/go"
)

// WinescapeGraphicsProbeEnabled opts a Windows build into asking the
// terminal's graphics-protocol questions (da1Sixel, QueryCellSize) over the
// real host tty through libwinescape instead of the Windows Console API,
// when the process is running under Wine.
//
// The Windows Console API path relays the DA1/CSI-16t query and its answer
// through Wine's ConPTY bridge, whose limits here are already documented
// (f4#474, f4's docs/WINE.md: "Wine's ConPTY support isn't good enough").
// With this on -- and only once winescapeAvailable() (winescape.Available()
// by default) has confirmed a working install with its own startup
// self-test, not merely that the package linked -- the probe instead opens
// /dev/tty on the Wine host directly and asks it the same question
// graphics_probe_unix.go already asks natively on Unix, skipping the bridge
// outright.
//
// Off by default: f4 turns this on together with its own opt-in
// UseWinescape/hostmode setting, so nothing here changes unless a caller
// sets it explicitly. When off, or when winescapeAvailable() answers false
// (not under Wine, or Wine's raw-syscall passthrough failed the self-test),
// da1Sixel and QueryCellSize keep using the existing Windows Console API
// path, unchanged.
var WinescapeGraphicsProbeEnabled bool

// winescapeAvailable is winescape.Available(), indirected so a test can pick
// the answer without a real Wine install.
var winescapeAvailable = winescape.Available

// useWinescapeProbe reports whether da1Sixel and QueryCellSize should bypass
// the Windows Console API and talk to the host tty directly through
// libwinescape.
func useWinescapeProbe() bool {
	return WinescapeGraphicsProbeEnabled && winescapeAvailable()
}

// winescapeDA1Sixel is da1Sixel's libwinescape counterpart: it opens
// /dev/tty on the Wine host directly, bypassing the Windows Console API and
// the ConPTY bridge it is relayed through, and asks the terminal the same
// primary-device-attributes question graphics_probe_unix.go asks on native
// Unix.
func winescapeDA1Sixel() bool {
	tty, err := winescape.OpenFile("/dev/tty", winescape.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer tty.Close()

	if _, err := tty.Write([]byte("\x1b[c")); err != nil {
		return false
	}
	answer, ok := winescapeReadAnswer(tty, da1ResponseComplete)
	if !ok {
		return false
	}
	return parseDA1Sixel(answer)
}

// winescapeQueryCellSize is QueryCellSize's libwinescape counterpart.
func winescapeQueryCellSize() (cw, ch int, ok bool) {
	tty, err := winescape.OpenFile("/dev/tty", winescape.O_RDWR, 0)
	if err != nil {
		return 0, 0, false
	}
	defer tty.Close()

	if _, err := tty.Write([]byte("\x1b[16t")); err != nil {
		return 0, 0, false
	}
	answer, done := winescapeReadAnswer(tty, cellSizeResponseComplete)
	if !done {
		return 0, 0, false
	}
	return parseCellSizeResponse(answer)
}

// winescapeProbeBudget is the read budget for a terminal answer, matching
// the Windows Console API and native-Unix probes it stands in for.
const winescapeProbeBudget = 300 * time.Millisecond

// winescapeReadAnswer polls tty for up to winescapeProbeBudget, collecting
// bytes until complete reports the answer whole.
func winescapeReadAnswer(tty *winescape.File, complete func(string) bool) (string, bool) {
	var sb strings.Builder
	buf := make([]byte, 128)
	deadline := time.Now().Add(winescapeProbeBudget)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return sb.String(), false
		}
		n, err := winescape.Poll([]winescape.PollFd{{Fd: int32(tty.Fd()), Events: winescape.POLLIN}}, remaining)
		if err != nil {
			return sb.String(), false
		}
		if n <= 0 {
			continue
		}
		nr, err := tty.Read(buf)
		if nr > 0 {
			sb.Write(buf[:nr])
			if complete(sb.String()) {
				return sb.String(), true
			}
		}
		if err != nil {
			return sb.String(), false
		}
	}
}

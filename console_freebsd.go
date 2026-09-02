//go:build freebsd

package vtui

import "golang.org/x/sys/unix"

// FreeBSD ships two system console drivers. Both drive the same emulator
// (sys/teken), so they accept the same escape sequences, but syscons renders
// the resulting VGA attribute byte differently: in a text mode bit 7 is the
// hardware blink bit.
//
//	sys/dev/syscons/scterm-teken.c, scteken_te_to_sc_attr():
//	    attr = te_to_sc_color[fg&7] | (fg&8) |
//	           ((te_to_sc_color[bg&7] | (bg&8)) << 4);
//	    if (a->ta_format & (TF_BOLD | TF_UNDERLINE)) attr ^= 8;
//	    if (a->ta_format & TF_BLINK)                 attr ^= 0x80;
//
//	sys/dev/syscons/scvgarndr.c: the text-mode renderers call
//	vga_flipattr(a, TRUE) and vga_cursorattr_adj(scp, a, TRUE), i.e. they
//	treat 0x8000 as blink; only the pixel-mode renderers pass FALSE.
//	sys/dev/fb/vga.c never clears blink-enable in ATC index 0x10.
//
// Three consequences, all verified on FreeBSD 14.4 with kern.vty=sc by reading
// the VGA text buffer at 0xB8000 back after writing each sequence:
//
//	ESC[101m          -> 0xC7   bright background
//	ESC[5m ESC[41m    -> 0xC7   explicit blink: the very same byte
//	ESC[41m           -> 0x47   dark background, no blink
//	ESC[7m ESC[91m    -> 0xC0   reverse over a bright fg: blink again
//	ESC[1m ESC[91m    -> 0x04   bold cancels bright: dark red, not bright
//	ESC[91m           -> 0x0C   bright fg on its own is fine
//
// vt(4) has none of this (vt_determine_colors turns TF_BLINK into a light
// background), which is why the same build looks right on one host and blinks
// on another. Syscons in a pixel mode is fine too, but telling text from pixel
// mode needs a console ioctl, and a dark background is a much smaller loss
// than a blinking one, so the policy is applied to syscons as a whole.

// detectFreeBSDSyscons reports whether the FreeBSD system console is driven by
// syscons. Called from the init in screenbuf.go once IsFreeBSDConsole is
// known; it must not be an init of its own, because init functions run in
// file-name order and this file sorts before screenbuf.go.
func detectFreeBSDSyscons() bool {
	if !IsFreeBSDConsole {
		return false
	}
	vty, err := unix.Sysctl("kern.vty")
	return err == nil && vty == "sc"
}

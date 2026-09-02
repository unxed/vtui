//go:build !freebsd

package vtui

// detectFreeBSDSyscons is always false off FreeBSD; see console_freebsd.go for
// what it guards.
func detectFreeBSDSyscons() bool { return false }

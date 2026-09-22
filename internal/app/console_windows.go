//go:build windows

package app

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procGetStdHandle  = kernel32.NewProc("GetStdHandle")
	procSetStdHandle  = kernel32.NewProc("SetStdHandle")
)

// AttachConsole reattaches stdout/stderr to the parent terminal. The exe
// is built with -H=windowsgui (no black console window beside the desktop
// app — CEO finding 2026-09-21), so CLI verbs launched from a terminal
// need this to print anything. A double-clicked GUI launch (no arguments)
// skips it entirely and stays quiet.
func AttachConsole() {
	if len(os.Args) <= 1 {
		return
	}
	if err := procAttachConsole.Find(); err != nil {
		return
	}
	// ATTACH_PARENT_PROCESS = (DWORD)-1
	if r, _, _ := procAttachConsole.Call(0xFFFFFFFF); r == 0 {
		return
	}
	for _, std := range []struct {
		n uintptr
		f **os.File
	}{{0xFFFFFFF5, &os.Stdout}, {0xFFFFFFF6, &os.Stderr}} { // (DWORD)-11, -12
		var h uintptr
		if r, _, _ := procGetStdHandle.Call(std.n, uintptr(unsafe.Pointer(&h))); r == 0 { // #nosec G103 -- canonical std-handle plumbing
			continue
		}
		_, _, _ = procSetStdHandle.Call(std.n, h) // #nosec G103 -- best-effort rebind; failure leaves the verb silent, not broken
		*std.f = os.NewFile(h, "console")
	}
}

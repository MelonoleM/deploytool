//go:build windows

package process

import (
	"os"
	"syscall"
	"unsafe"
)

// IsTerminal reports whether f is attached to a console. Console-mode service
// starts are only meaningful when one exists; a headless web server or daemon
// must fall back to background mode instead.
func IsTerminal(f *os.File) bool {
	var mode uint32
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleMode")
	r1, _, _ := proc.Call(f.Fd(), uintptr(unsafe.Pointer(&mode)))
	return r1 != 0
}

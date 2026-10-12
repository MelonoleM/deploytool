//go:build windows

package console

import (
	"syscall"
	"unsafe"
)

// ShowMessage displays a fatal message. On Windows it uses a native message box,
// because the desktop build has no console to write to.
func ShowMessage(msg string) {
	title, _ := syscall.UTF16PtrFromString("部署工具")
	text, _ := syscall.UTF16PtrFromString(msg)
	proc := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	proc.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10) // MB_ICONERROR
}

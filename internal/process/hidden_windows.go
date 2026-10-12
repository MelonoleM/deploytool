//go:build windows

package process

import "syscall"

// HiddenProcAttr returns process attributes that prevent a console-subsystem child
// (cmd, tasklist, taskkill, ...) from flashing a new console window when the parent
// is a GUI-subsystem process such as deploytool-desktop.exe.
func HiddenProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true}
}

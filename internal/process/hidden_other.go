//go:build !windows

package process

import "syscall"

// HiddenProcAttr is a no-op on non-Windows platforms.
func HiddenProcAttr() *syscall.SysProcAttr { return nil }

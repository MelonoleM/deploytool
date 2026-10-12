//go:build !windows

package process

import "os"

// IsTerminal reports whether f is attached to a terminal character device.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

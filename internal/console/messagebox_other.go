//go:build !windows

package console

import (
	"fmt"
	"os"
)

// ShowMessage falls back to stderr on non-Windows platforms.
func ShowMessage(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

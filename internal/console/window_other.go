//go:build !windows

package console

import "fmt"

// RunWindow is only implemented on Windows; other platforms fall back to the
// browser-based console.
func RunWindow(string) error {
	return fmt.Errorf("原生窗口仅在 Windows 上可用，请改用 desktop 命令在浏览器中打开")
}

//go:build windows

package console

import (
	"fmt"
	"syscall"

	webview2 "github.com/jchv/go-webview2"
)

// RunWindow opens a native WebView2 window (Windows 10/11) pointed at url and
// blocks until the window is closed.
func RunWindow(url string) error {
	// DPI awareness keeps window dimensions in logical pixels so the WebView2
	// content is not blur-scaled into an overflowing scrollbar on high-DPI screens.
	syscall.NewLazyDLL("user32.dll").NewProc("SetProcessDPIAware").Call()

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "部署工具",
			Width:  1200,
			Height: 780,
			Center: true,
		},
	})
	if w == nil {
		return fmt.Errorf("无法创建 WebView2 窗口：请确认系统已安装 WebView2 运行时")
	}
	defer w.Destroy()
	w.SetSize(1200, 780, webview2.HintNone)
	w.Navigate(url)
	w.Run()
	return nil
}

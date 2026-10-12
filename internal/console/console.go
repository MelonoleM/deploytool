// Package console holds the shared launcher primitives used by both the CLI
// (deploytool) and the native desktop app (deploytool-desktop): starting the local
// web server and opening either the system browser or a native window.
package console

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"deploytool/internal/webui"
)

// StartServer binds a loopback port (falling back to a random free port) and serves
// the console in the background. It returns the URL and a stop function.
func StartServer(s *webui.Server, host string, port int) (string, func() error, error) {
	if host == "" {
		host = "127.0.0.1"
	}
	ln, err := listenFree(host, port)
	if err != nil {
		return "", nil, err
	}
	server := &http.Server{Handler: s.Handler()}
	go func() {
		if e := server.Serve(ln); e != nil && e != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", e)
		}
	}()
	return "http://" + ln.Addr().String() + "/", server.Close, nil
}

func listenFree(host string, port int) (net.Listener, error) {
	if port > 0 {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp", net.JoinHostPort(host, "0"))
}

// OpenBrowser opens url in the system default browser.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

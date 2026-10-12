package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"deploytool/internal/console"
	"deploytool/internal/webui"
)

// runConsole starts the web console, opens the default browser unless noBrowser is
// set, and blocks until interrupted. It is the portable, no-runtime form of the
// desktop application: a single static binary with no WebView or browser dependency.
func runConsole(s *webui.Server, host string, port int, noBrowser bool) error {
	url, stop, err := console.StartServer(s, host, port)
	if err != nil {
		return err
	}
	defer stop()
	fmt.Printf("部署工具已启动：%s\n", url)
	if !noBrowser {
		if e := console.OpenBrowser(url); e != nil {
			fmt.Fprintf(os.Stderr, "自动打开浏览器失败，请手动访问：%s\n", url)
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig
	return nil
}

// runWindowApp starts the local console and then opens a native WebView2 window
// (Windows 10/11 only) instead of the default browser.
func runWindowApp(s *webui.Server, host string, port int) error {
	url, stop, err := console.StartServer(s, host, port)
	if err != nil {
		return err
	}
	defer stop()
	fmt.Printf("部署工具已启动：%s\n", url)
	return console.RunWindow(url)
}

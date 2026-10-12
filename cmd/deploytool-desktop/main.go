//go:build windows

// Command deploytool-desktop is the native Windows desktop build of the console.
// It is compiled with -H windowsgui so double-clicking it opens the WebView2 window
// without a console window.
package main

import (
	"flag"
	"os"

	"deploytool/internal/config"
	"deploytool/internal/console"
	"deploytool/internal/engine"
	"deploytool/internal/webui"
)

func main() {
	fs := flag.NewFlagSet("deploytool-desktop", flag.ContinueOnError)
	cfg := fs.String("config", "deploytool.yaml", "configuration path")
	host := fs.String("host", "127.0.0.1", "UI bind host")
	port := fs.Int("port", 8090, "UI port")
	if err := fs.Parse(os.Args[1:]); err != nil {
		console.ShowMessage("参数错误：" + err.Error())
		os.Exit(2)
	}

	c, err := config.Load(*cfg)
	if err != nil {
		console.ShowMessage("无法加载配置：" + err.Error() + "\n\n请把配置文件放到程序同目录，或用 --config 指定路径。")
		os.Exit(2)
	}
	o, err := engine.New(c, false)
	if err != nil {
		console.ShowMessage("初始化失败：" + err.Error())
		os.Exit(2)
	}
	initial, _ := os.ReadFile(*cfg)
	s := &webui.Server{O: o, ConfigPath: *cfg, Initial: initial, Native: true}

	url, stop, err := console.StartServer(s, *host, *port)
	if err != nil {
		console.ShowMessage("启动失败：" + err.Error())
		os.Exit(2)
	}
	defer stop()
	if err := console.RunWindow(url); err != nil {
		console.ShowMessage("启动失败：" + err.Error())
		os.Exit(2)
	}
}

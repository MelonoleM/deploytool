package main

import (
	"context"
	"deploytool/internal/artifact"
	"deploytool/internal/config"
	"deploytool/internal/engine"
	"deploytool/internal/process"
	"deploytool/internal/webui"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

const usage = `deploytool <install|uninstall|update|start|stop|status|ui|desktop|window|pack> [options]
All lifecycle commands accept --config (default deploytool.yaml) and --only svc1,svc2.
start/stop also accept a positional service name; "all" (or no name) means every service.
update requires --package; pack requires --src and --out.
Running with no arguments opens the web console in your default browser.`

// commands are validated before any config is read, so a mistyped or missing
// subcommand reports itself instead of failing later on a missing config file.
var commands = map[string]bool{
	"install": true, "uninstall": true, "update": true, "start": true,
	"stop": true, "status": true, "ui": true, "desktop": true, "window": true, "pack": true,
}

func main() {
	if len(os.Args) < 2 {
		// Double-click / no-argument launch: open the portable web console.
		runDesktopPortable()
		return
	}
	cmd := os.Args[1]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		fmt.Println(usage)
		return
	}
	if strings.HasPrefix(cmd, "-") {
		// Flags without a subcommand (for example a desktop shortcut with --config).
		runDesktopPortable()
		return
	}
	if !commands[cmd] {
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", cmd, usage)
		fmt.Fprintln(os.Stderr, "hint: the service config is a flag, for example install --config examples/example-project/deploytool.yaml")
		os.Exit(2)
	}
	run(cmd, os.Args[2:])
}

func run(cmd string, args []string) {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfg := fs.String("config", "deploytool.yaml", "configuration path")
	only := fs.String("only", "", "comma-separated service names")
	dry := fs.Bool("dry-run", false, "show actions without changing files")
	jsonOut := fs.Bool("json", false, "machine-readable status")
	purge := fs.Bool("purge-state", false, "delete state after uninstall")
	noStart := fs.Bool("no-start", false, "install without starting services")
	pkg := fs.String("package", "", "update package")
	port := fs.Int("port", 8090, "UI port")
	host := fs.String("host", "127.0.0.1", "UI bind host")
	noBrowser := fs.Bool("no-browser", false, "start the console without opening a browser")
	src := fs.String("src", "", "package source directory")
	out := fs.String("out", "", "package destination")
	fs.Parse(args)

	if cmd == "pack" {
		if *src == "" || *out == "" {
			fatal("pack requires --src and --out")
		}
		c, e := config.Load(*cfg)
		if e != nil {
			fatal(e.Error())
		}
		if e = artifact.Pack(*src, *out, c.Project.Name, c.Project.Version); e != nil {
			fatal(e.Error())
		}
		return
	}

	c, e := config.Load(*cfg)
	if e != nil {
		fatal(e.Error())
	}
	o, e := engine.New(c, *dry)
	if e != nil {
		fatal(e.Error())
	}
	// Console-mode services only run in the foreground when the operator is in an
	// interactive terminal; web/desktop launches stay non-interactive.
	if cmd != "ui" && cmd != "desktop" {
		o.SetInteractive(process.IsTerminal(os.Stdout))
	}
	initial, _ := os.ReadFile(*cfg)
	var r *engine.Report
	switch cmd {
	case "install":
		if *noStart {
			r = o.InstallWithoutStart(context.Background(), split(*only))
		} else {
			r = o.Install(context.Background(), split(*only))
		}
	case "uninstall":
		r = o.Uninstall(context.Background(), split(*only), *purge)
	case "update":
		if *pkg == "" {
			fatal("update requires --package")
		}
		r = o.Update(context.Background(), *pkg, split(*only))
	case "start":
		r = o.Start(context.Background(), targets(fs.Args(), *only))
	case "stop":
		r = o.Stop(context.Background(), targets(fs.Args(), *only))
	case "status":
		r = o.Status(context.Background(), split(*only))
	case "ui":
		warnExposed(*host)
		if err := (&webui.Server{O: o, ConfigPath: *cfg, Initial: initial}).Serve(*host, *port); err != nil {
			fatal(err.Error())
		}
	case "desktop":
		warnExposed(*host)
		if err := runConsole(&webui.Server{O: o, ConfigPath: *cfg, Initial: initial}, *host, *port, *noBrowser); err != nil {
			fatal(err.Error())
		}
	case "window":
		warnExposed(*host)
		if err := runWindowApp(&webui.Server{O: o, ConfigPath: *cfg, Initial: initial, Native: true}, *host, *port); err != nil {
			fatal(err.Error())
		}
	default:
		fatal(usage)
	}
	printReport(r, *jsonOut)
	if !r.Success {
		os.Exit(4)
	}
}

// runDesktopPortable is the double-click path. It behaves like `desktop` but keeps
// the console window open with a readable message when setup fails, because a
// double-clicked window would otherwise vanish before the error can be read.
func runDesktopPortable() {
	fs := flag.NewFlagSet("desktop", flag.ContinueOnError)
	cfg := fs.String("config", "deploytool.yaml", "configuration path")
	host := fs.String("host", "127.0.0.1", "UI bind host")
	port := fs.Int("port", 8090, "UI port")
	noBrowser := fs.Bool("no-browser", false, "do not open a browser")
	if err := fs.Parse(os.Args[1:]); err != nil {
		exitWithWait("参数错误：" + err.Error())
	}
	c, err := config.Load(*cfg)
	if err != nil {
		exitWithWait("无法加载配置：" + err.Error() + "\n请把配置文件放到程序同目录，或用 --config 指定路径。")
	}
	o, err := engine.New(c, false)
	if err != nil {
		exitWithWait("初始化失败：" + err.Error())
	}
	initial, _ := os.ReadFile(*cfg)
	warnExposed(*host)
	if err := runConsole(&webui.Server{O: o, ConfigPath: *cfg, Initial: initial}, *host, *port, *noBrowser); err != nil {
		exitWithWait("启动失败：" + err.Error())
	}
}

func warnExposed(host string) {
	if host == "0.0.0.0" {
		fmt.Fprintln(os.Stderr, "warning: UI is unauthenticated; exposing it outside localhost is unsafe")
	}
}

func exitWithWait(msg string) {
	fmt.Fprintln(os.Stderr, msg)
	fmt.Fprintln(os.Stderr, "按回车键退出…")
	var s string
	_, _ = fmt.Scanln(&s)
	os.Exit(2)
}

func split(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, ",")
}

func targets(args []string, only string) []string {
	if only != "" {
		return split(only)
	}
	if len(args) > 0 && args[0] != "all" {
		return []string{args[0]}
	}
	return nil
}

func printReport(r *engine.Report, j bool) {
	if j {
		json.NewEncoder(os.Stdout).Encode(r)
		return
	}
	for _, s := range r.Services {
		fmt.Printf("%-20s installed=%-5t running=%-5t healthy=%-5t skipped=%-5t %s\n", s.Name, s.Installed, s.Running, s.Healthy, s.Skipped, s.Reason)
	}
}

func fatal(s string) { fmt.Fprintln(os.Stderr, s); os.Exit(2) }

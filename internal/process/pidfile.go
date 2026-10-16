package process

import (
	"context"
	"deploytool/internal/logger"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func Run(command, workdir string) (int, error) {
	return RunBackground(command, workdir, "", nil, "")
}

// RunBackground starts a process without tying its output to the invoking terminal.
// When logPath is supplied both stdout and stderr are appended to that file. When
// pidPath is supplied the child process id is written there.
func RunBackground(command, workdir, logPath string, env map[string]string, pidPath string) (int, error) {
	if strings.TrimSpace(command) == "" {
		return 0, nil
	}
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.Command("cmd", "/C", command)
	} else {
		c = exec.Command("sh", "-c", command)
	}
	c.SysProcAttr = HiddenProcAttr()
	c.Dir = workdir
	if merged := mergeEnv(env); merged != nil {
		c.Env = merged
	}
	if logPath == "" {
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
	} else {
		if e := os.MkdirAll(filepath.Dir(logPath), 0755); e != nil {
			return 0, e
		}
		// Roll an oversized service log before opening a fresh one, so a
		// long-running deployment never grows a single unbounded file.
		if e := logger.RotateLogFile(logPath, logger.DefaultMaxBytes, logger.DefaultKeep); e != nil {
			return 0, e
		}
		f, e := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return 0, e
		}
		defer f.Close()
		c.Stdout, c.Stderr = f, f
	}
	if e := c.Start(); e != nil {
		return 0, e
	}
	pid := c.Process.Pid
	if pidPath != "" {
		if e := os.MkdirAll(filepath.Dir(pidPath), 0755); e != nil {
			return 0, e
		}
		if e := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0600); e != nil {
			return 0, e
		}
	}
	return pid, nil
}

// RunConsole attaches a service to the current terminal. It intentionally blocks
// until the process ends, so operators can inspect output and stop it with Ctrl+C.
func RunConsole(ctx context.Context, command, workdir string, env map[string]string) error {
	if strings.TrimSpace(command) == "" {
		return nil
	}
	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", command)
	} else {
		c = exec.CommandContext(ctx, "sh", "-c", command)
	}
	c.SysProcAttr = HiddenProcAttr()
	c.Dir, c.Stdout, c.Stderr, c.Stdin = workdir, os.Stdout, os.Stderr, os.Stdin
	if merged := mergeEnv(env); merged != nil {
		c.Env = merged
	}
	return c.Run()
}

// mergeEnv returns the parent environment with extra overlaid on top, sorted for
// determinism. A nil/empty extra returns nil so the child inherits the environment.
func mergeEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return nil
	}
	merged := map[string]string{}
	norm := func(k string) string {
		if runtime.GOOS == "windows" {
			// Windows env keys are case-insensitive; normalize so overriding PATH
			// replaces a stored "Path" instead of adding a duplicate.
			return strings.ToUpper(k)
		}
		return k
	}
	for _, kv := range os.Environ() {
		if i := strings.Index(kv, "="); i > 0 {
			merged[norm(kv[:i])] = kv[i+1:]
		}
	}
	for k, v := range extra {
		merged[norm(k)] = expandEnvValue(v)
	}
	out := make([]string, 0, len(merged))
	for k, v := range merged {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

var (
	percentEnvRE = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)
	braceEnvRE   = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

// expandEnvValue resolves %VAR% and ${VAR} references against the inherited
// process environment, so values like `bin;%PATH%` or `/opt:${PATH}` can prepend
// to an existing variable. Bare $VAR is intentionally not expanded to avoid
// clobbering literal dollar signs.
func expandEnvValue(s string) string {
	s = percentEnvRE.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(m[1 : len(m)-1])
	})
	s = braceEnvRE.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(m[2 : len(m)-1])
	})
	return s
}
func Stop(command, workdir string, pid int) error {
	if strings.TrimSpace(command) != "" {
		_, e := Run(command, workdir)
		return e
	}
	if pid <= 0 {
		return nil
	}
	p, e := os.FindProcess(pid)
	if e != nil {
		return e
	}
	if runtime.GOOS == "windows" {
		c := exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F")
		c.SysProcAttr = HiddenProcAttr()
		return c.Run()
	}
	return p.Signal(syscall.SIGTERM)
}
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	if runtime.GOOS == "windows" {
		c := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/FO", "CSV", "/NH")
		c.SysProcAttr = HiddenProcAttr()
		output, e := c.CombinedOutput()
		return e == nil && strings.Contains(string(output), fmt.Sprintf("\"%d\"", pid))
	}
	p, e := os.FindProcess(pid)
	if e != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// RemovePID deletes the pid file written by RunBackground or RunConsoleWindow. The
// file is operator-facing bookkeeping, so a stale copy after a stop would be
// misleading; an already-absent file is not an error.
func RemovePID(pidPath string) error {
	if strings.TrimSpace(pidPath) == "" {
		return nil
	}
	e := os.Remove(pidPath)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	return nil
}

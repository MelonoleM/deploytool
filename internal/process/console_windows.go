//go:build windows

package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const (
	createNewConsole         = 0x00000010
	createUnicodeEnvironment = 0x00000400
)

// RunConsoleWindow launches command in a new, visible console window and returns
// the child process id without blocking. The window belongs to the child process,
// so its standard handles are deliberately left unset: os/exec would otherwise
// force them to a pipe or NUL and leave the window blank.
func RunConsoleWindow(command, workdir string, env map[string]string, pidPath string) (int, error) {
	if strings.TrimSpace(command) == "" {
		return 0, nil
	}
	comspec := os.Getenv("COMSPEC")
	if strings.TrimSpace(comspec) == "" {
		comspec = "cmd.exe"
	}
	appName, err := syscall.UTF16PtrFromString(comspec)
	if err != nil {
		return 0, err
	}
	line, err := syscall.UTF16PtrFromString(comspec + " /C " + command)
	if err != nil {
		return 0, err
	}
	var dir *uint16
	if strings.TrimSpace(workdir) != "" {
		if dir, err = syscall.UTF16PtrFromString(workdir); err != nil {
			return 0, err
		}
	}
	envp, err := windowsEnvBlock(mergeEnv(env))
	if err != nil {
		return 0, err
	}
	si := &syscall.StartupInfo{Cb: uint32(unsafe.Sizeof(syscall.StartupInfo{}))}
	pi := &syscall.ProcessInformation{}
	if err := syscall.CreateProcess(appName, line, nil, nil, false, createNewConsole|createUnicodeEnvironment, envp, dir, si, pi); err != nil {
		return 0, err
	}
	defer syscall.CloseHandle(pi.Process)
	defer syscall.CloseHandle(pi.Thread)
	pid := int(pi.ProcessId)
	if pidPath != "" {
		if err := os.MkdirAll(filepath.Dir(pidPath), 0755); err != nil {
			return 0, err
		}
		if err := os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0600); err != nil {
			return 0, err
		}
	}
	return pid, nil
}

// windowsEnvBlock renders a sorted KEY=VALUE slice as the double-null-terminated
// UTF-16 environment block expected by CreateProcess.
func windowsEnvBlock(entries []string) (*uint16, error) {
	if entries == nil {
		return nil, nil
	}
	var block []uint16
	for _, e := range entries {
		u, err := syscall.UTF16FromString(e)
		if err != nil {
			return nil, err
		}
		block = append(block, u...)
	}
	block = append(block, 0)
	if len(block) == 0 {
		return nil, nil
	}
	return &block[0], nil
}

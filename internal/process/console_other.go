//go:build !windows

package process

// RunConsoleWindow has no visible-console concept outside Windows, so it falls
// back to the ordinary background launch and keeps console-mode services runnable.
func RunConsoleWindow(command, workdir string, env map[string]string, pidPath string) (int, error) {
	return RunBackground(command, workdir, "", env, pidPath)
}

package process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRunConsoleWaitsForSuccessfulCommand(t *testing.T) {
	command := "true"
	if runtime.GOOS == "windows" {
		command = "exit 0"
	}
	if err := RunConsole(context.Background(), command, t.TempDir(), nil); err != nil {
		t.Fatalf("console command failed: %v", err)
	}
}

func TestRemovePID(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".deploytool.pid")
	if err := RemovePID(p); err != nil {
		t.Fatalf("an absent pid file must not be an error: %v", err)
	}
	if err := os.WriteFile(p, []byte("1234"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RemovePID(p); err != nil {
		t.Fatalf("remove pid file: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("pid file still present: %v", err)
	}
}

func TestExpandEnvValue(t *testing.T) {
	t.Setenv("DEPLOY_TEST_X", "hello")
	if got := expandEnvValue("%DEPLOY_TEST_X% world"); got != "hello world" {
		t.Fatalf("percent expansion = %q", got)
	}
	if got := expandEnvValue("${DEPLOY_TEST_X} world"); got != "hello world" {
		t.Fatalf("brace expansion = %q", got)
	}
	if got := expandEnvValue("a$b"); got != "a$b" {
		t.Fatalf("bare dollar must stay literal: %q", got)
	}
}

func TestMergeEnvOverridesPathOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows only")
	}
	out := mergeEnv(map[string]string{"PATH": "aaa;" + os.Getenv("PATH")})
	var pathVal string
	n := 0
	for _, kv := range out {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			n++
			pathVal = kv
		}
	}
	if n != 1 {
		t.Fatalf("PATH appears %d times, want 1", n)
	}
	if !strings.HasPrefix(pathVal, "PATH=") || !strings.HasPrefix(pathVal[5:], "aaa;") {
		t.Fatalf("unexpected PATH entry %q", pathVal)
	}
}

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// SetServiceEnabled must round-trip a real-world document without destroying the
// anchors, aliases and comments the example project relies on.
func TestSetServiceEnabledPreservesDocument(t *testing.T) {
	example := filepath.Join("..", "..", "examples", "example-project", "deploytool.yaml")
	b, err := os.ReadFile(example)
	if err != nil {
		t.Skipf("example config not available: %v", err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "deploytool.yaml")
	if err = os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}

	if err = SetServiceEnabled(p, "java-api", false); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "# Real package example") {
		t.Fatalf("comment was dropped:\n%s", text)
	}
	if !strings.Contains(text, "*common_install") {
		t.Fatalf("alias was dropped:\n%s", text)
	}

	c, err := Load(p)
	if err != nil {
		t.Fatalf("edited config no longer parses: %v", err)
	}
	for _, s := range c.Services {
		if s.Name == "java-api" && s.IsEnabled() {
			t.Fatal("java-api should have been disabled")
		}
	}

	if err = SetServiceEnabled(p, "java-api", true); err != nil {
		t.Fatal(err)
	}
	c, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range c.Services {
		if s.Name == "java-api" && !s.IsEnabled() {
			t.Fatal("java-api should have been re-enabled")
		}
	}
}

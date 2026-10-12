package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFormRoundTripPreservesDocumentAndEdits(t *testing.T) {
	example := filepath.Join("..", "..", "examples", "example-project", "deploytool.windows.yaml")
	b, err := os.ReadFile(example)
	if err != nil {
		t.Skipf("example config not available: %v", err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "deploytool.yaml")
	if err = os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}

	f1, err := ReadForm(p)
	if err != nil {
		t.Fatal(err)
	}
	if f1.ProjectName != "all-types-live-demo" || len(f1.Services) != 7 {
		t.Fatalf("unexpected form: project=%q services=%d", f1.ProjectName, len(f1.Services))
	}
	if !strings.Contains(f1.Services[0].InstallYAML, "install:") || !strings.Contains(f1.Services[0].InstallYAML, "mkdir") {
		t.Fatalf("install block not extracted: %q", f1.Services[0].InstallYAML)
	}

	// A no-op write must round-trip to the same form and keep the install steps.
	if err = WriteForm(p, f1); err != nil {
		t.Fatal(err)
	}
	f2, err := ReadForm(p)
	if err != nil {
		t.Fatal(err)
	}
	if f2.ProjectName != f1.ProjectName || len(f2.Services) != len(f1.Services) {
		t.Fatalf("no-op write changed the form shape")
	}
	if !strings.Contains(f2.Services[0].InstallYAML, "mkdir") {
		t.Fatalf("install steps were lost: %q", f2.Services[0].InstallYAML)
	}

	// Field edits must persist and keep the document's comments.
	f1.ProjectVersion = "9.9.9"
	f1.FailFast = true
	f1.Services[0].Start = "changed-start"
	f1.Services[1].Enabled = false
	if err = WriteForm(p, f1); err != nil {
		t.Fatal(err)
	}
	f3, err := ReadForm(p)
	if err != nil {
		t.Fatal(err)
	}
	if f3.ProjectVersion != "9.9.9" || !f3.FailFast || f3.Services[0].Start != "changed-start" || f3.Services[1].Enabled {
		t.Fatalf("edits did not persist: %+v", f3)
	}
	out, _ := os.ReadFile(p)
	if !strings.Contains(string(out), "# Runnable Windows demo") {
		t.Fatalf("comment was dropped after form write")
	}
}

func TestWriteServiceOnlyChangesOneService(t *testing.T) {
	example := filepath.Join("..", "..", "examples", "example-project", "deploytool.windows.yaml")
	b, err := os.ReadFile(example)
	if err != nil {
		t.Skipf("example config not available: %v", err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "deploytool.yaml")
	if err = os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}

	before, err := ReadForm(p)
	if err != nil {
		t.Fatal(err)
	}
	svc := before.Services[1]
	svc.Workdir = "changed-workdir"
	if err = WriteService(p, svc); err != nil {
		t.Fatal(err)
	}

	after, err := ReadForm(p)
	if err != nil {
		t.Fatal(err)
	}
	if after.Services[1].Workdir != "changed-workdir" {
		t.Fatalf("target service not updated: %q", after.Services[1].Workdir)
	}
	if after.Services[0].Workdir != before.Services[0].Workdir {
		t.Fatalf("other service was touched: %q -> %q", before.Services[0].Workdir, after.Services[0].Workdir)
	}
	out, _ := os.ReadFile(p)
	if !strings.Contains(string(out), "# Runnable Windows demo") {
		t.Fatalf("comment was dropped after single-service write")
	}
}

func TestFormEnvRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "deploytool.yaml")
	yaml := "apiVersion: v1\nproject: {name: demo, version: v1}\nvars: {install_root: ./runtime}\nservices:\n  - name: web\n    type: nodejs\n    workdir: \"{{ .Vars.install_root }}/web\"\n    env:\n      NODE_ENV: production\n      PORT: \"8080\"\n    start: \"node app.js\"\n"
	if err := os.WriteFile(p, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := ReadForm(p)
	if err != nil {
		t.Fatal(err)
	}
	if f.Services[0].Env != "NODE_ENV=production\nPORT=8080\n" {
		t.Fatalf("unexpected env text %q", f.Services[0].Env)
	}
	f.Services[0].Env = "FOO=bar\nNODE_ENV=staging\n"
	if err = WriteForm(p, f); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Services[0].Env["FOO"] != "bar" || c.Services[0].Env["NODE_ENV"] != "staging" {
		t.Fatalf("env did not persist: %#v", c.Services[0].Env)
	}
}

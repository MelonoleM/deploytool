package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRendersVars(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "deploytool.yaml")
	yaml := "apiVersion: v1\nproject: {name: demo, version: v1}\nvars: {install_root: '" + filepath.ToSlash(d) + "'}\nservices:\n  - name: app\n    type: exe\n    workdir: '{{ .Vars.install_root }}/app'\n"
	if err := os.WriteFile(p, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.Services[0].Workdir == "{{ .Vars.install_root }}/app" {
		t.Fatal("template was not rendered")
	}
}

func TestValidateRejectsUnknownDependency(t *testing.T) {
	e := Validate(&Config{APIVersion: "v1", Project: Project{Name: "x"}, Services: []Service{{Name: "a", Type: "exe", Workdir: "x", DependsOn: []string{"missing"}}}})
	if e == nil {
		t.Fatal("expected dependency validation error")
	}
}

// A Windows path override must survive being substituted into a double-quoted
// YAML scalar, which is how the shipped example config references install_root.
func TestLoadEscapesWindowsPathOverrideInsideQuotedScalar(t *testing.T) {
	t.Setenv("INSTALL_ROOT", `C:\demo\runtime`)
	d := t.TempDir()
	p := filepath.Join(d, "deploytool.yaml")
	yaml := "apiVersion: v1\nproject: {name: demo, version: v1}\nvars: {install_root: ./runtime}\nservices:\n  - name: app\n    type: exe\n    workdir: \"{{ .Vars.install_root }}/app\"\n"
	if err := os.WriteFile(p, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("quoted windows path override must not break parsing: %v", err)
	}
	if c.Services[0].Workdir != `C:\demo\runtime/app` {
		t.Fatalf("unexpected workdir %q", c.Services[0].Workdir)
	}
	if c.Vars["install_root"] != `C:\demo\runtime` {
		t.Fatalf("unexpected install_root var %q", c.Vars["install_root"])
	}
}

// BaseDir must point at the config file's directory so install step sources can be
// resolved relative to the config instead of the process working directory.
func TestLoadSetsBaseDir(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "deploytool.yaml")
	yaml := "apiVersion: v1\nproject: {name: demo, version: v1}\nvars: {install_root: ./runtime}\nservices:\n  - name: app\n    type: exe\n    workdir: \"{{ .Vars.install_root }}/app\"\n"
	if err := os.WriteFile(p, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.BaseDir != dir {
		t.Fatalf("BaseDir = %q, want %q", c.BaseDir, dir)
	}
}

func TestRenderStepValues(t *testing.T) {
	s := Service{Workdir: "/srv/app", Install: Install{Artifact: "build/app.tar.gz"}}
	v, err := Render("{{ .workdir }}/{{ .install.artifact }}", s, map[string]string{})
	if err != nil || v != "/srv/app/build/app.tar.gz" {
		t.Fatalf("got %q, %v", v, err)
	}
}

func TestSetRuntimeDirRelocatesOverlay(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "deploytool.yaml")
	base := filepath.Join(d, "runtime")
	yaml := "apiVersion: v1\nproject: {name: demo, version: v1}\nvars: {install_root: '" + filepath.ToSlash(base) + "'}\nservices:\n  - name: app\n    type: exe\n    workdir: \"{{ .Vars.install_root }}/app\"\n"
	if err := os.WriteFile(p, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	if got, err := CurrentRuntimeDir(p); err != nil || !SamePath(got, base) {
		t.Fatalf("CurrentRuntimeDir = %q, %v; want %q", got, err, base)
	}

	newRoot := filepath.Join(d, "relocated")
	if err := SetRuntimeDir(p, newRoot); err != nil {
		t.Fatal(err)
	}
	overlay, err := OverlayPathFor(p)
	if err != nil {
		t.Fatal(err)
	}
	if want := OverlayPath(newRoot); overlay != want {
		t.Fatalf("overlay = %q, want %q", overlay, want)
	}

	got, err := EnsureOverlay(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != OverlayPath(newRoot) {
		t.Fatalf("EnsureOverlay = %q, want %q", got, OverlayPath(newRoot))
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("overlay was not created: %v", err)
	}
}

func TestSetRuntimeDirSkipsSidecarForDefaultRoot(t *testing.T) {
	d := t.TempDir()
	p := filepath.Join(d, "deploytool.yaml")
	base := filepath.Join(d, "runtime")
	yaml := "apiVersion: v1\nproject: {name: demo, version: v1}\nvars: {install_root: '" + filepath.ToSlash(base) + "'}\nservices:\n  - name: app\n    type: exe\n    workdir: \"{{ .Vars.install_root }}/app\"\n"
	if err := os.WriteFile(p, []byte(yaml), 0600); err != nil {
		t.Fatal(err)
	}

	sidecar := p + ".install_root"

	if err := SetRuntimeDir(p, base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("sidecar should not exist for the default root: %v", err)
	}

	other := filepath.Join(d, "elsewhere")
	if err := SetRuntimeDir(p, other); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(sidecar); err != nil || !SamePath(string(b), other) {
		t.Fatalf("sidecar = %q, %v; want %q", string(b), err, other)
	}

	if err := SetRuntimeDir(p, base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("sidecar should be removed when returning to the default root: %v", err)
	}
}

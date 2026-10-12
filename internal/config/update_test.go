package config

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.1.0", -1},
		{"1.1.0", "1.0.0", 1},
		{"v1.0.0", "1.0.0", 0},
		{"1.2", "1.10", -1},
		{"2.0", "1.9.9", 1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestMergeYAMLOfficialWinsAndPreservesUser(t *testing.T) {
	base := []byte("apiVersion: v1\nconfig_version: \"1.0.0\"\nproject: {name: demo, version: \"1.0.0\"}\nvars: {install_root: /srv/app}\nservices:\n  - name: web\n    type: nodejs\n    workdir: web\n    start: node app.js\n    enabled: true\n")
	theirs := []byte("apiVersion: v1\nconfig_version: \"1.0.0\"\nproject: {name: demo, version: \"1.0.0\"}\nvars: {install_root: /srv/app2}\nservices:\n  - name: web\n    type: nodejs\n    workdir: web\n    start: node app.js\n    enabled: false\n    env: {PORT: \"9999\"}\n")
	ours := []byte("apiVersion: v1\nconfig_version: \"1.1.0\"\nproject: {name: demo, version: \"1.1.0\"}\nvars: {install_root: /srv/app}\nservices:\n  - name: web\n    type: nodejs\n    workdir: web\n    start: node app.js --port 8080\n    enabled: true\n  - name: worker\n    type: python\n    workdir: worker\n    start: python worker.py\n")

	merged, err := MergeYAML(base, theirs, ours)
	if err != nil {
		t.Fatal(err)
	}

	var c Config
	if err := yaml.Unmarshal(merged, &c); err != nil {
		t.Fatalf("merged yaml is invalid: %v\n%s", err, merged)
	}
	if c.ConfigVersion != "1.1.0" {
		t.Fatalf("config_version = %q, want 1.1.0", c.ConfigVersion)
	}
	if c.Project.Version != "1.1.0" {
		t.Fatalf("project.version = %q, want 1.1.0", c.Project.Version)
	}
	if c.Vars["install_root"] != "/srv/app2" {
		t.Fatalf("install_root = %q, want /srv/app2", c.Vars["install_root"])
	}
	if len(c.Services) != 2 {
		t.Fatalf("services = %d, want 2", len(c.Services))
	}
	web := c.Services[0]
	if web.Start != "node app.js --port 8080" {
		t.Fatalf("web.start = %q, want official value", web.Start)
	}
	if web.IsEnabled() {
		t.Fatalf("web.enabled should be preserved as false")
	}
	if web.Env["PORT"] != "9999" {
		t.Fatalf("web.env.PORT = %q, want 9999", web.Env["PORT"])
	}
	if c.Services[1].Name != "worker" {
		t.Fatalf("second service = %q, want worker", c.Services[1].Name)
	}
}

func TestMergeYAMLExpandsAnchors(t *testing.T) {
	base := []byte("apiVersion: v1\nconfig_version: \"1.0.0\"\nproject: {name: demo, version: \"1.0.0\"}\nvars: {install_root: /srv/app}\nservices:\n  - name: a\n    type: exe\n    workdir: a\n    install: &common\n      steps:\n        - {action: mkdir, path: \"{{ .workdir }}\"}\n  - name: b\n    type: exe\n    workdir: b\n    install: *common\n")
	theirs := []byte("apiVersion: v1\nconfig_version: \"1.0.0\"\nproject: {name: demo, version: \"1.0.0\"}\nvars: {install_root: /srv/app}\nservices:\n  - name: a\n    type: exe\n    workdir: a\n    install: &common\n      steps:\n        - {action: mkdir, path: \"{{ .workdir }}\"}\n  - name: b\n    type: exe\n    workdir: b\n    install: *common\n    enabled: false\n")
	ours := []byte("apiVersion: v1\nconfig_version: \"1.1.0\"\nproject: {name: demo, version: \"1.1.0\"}\nvars: {install_root: /srv/app}\nservices:\n  - name: a\n    type: exe\n    workdir: a\n    install: &common\n      steps:\n        - {action: mkdir, path: \"{{ .workdir }}\"}\n  - name: b\n    type: exe\n    workdir: b\n    install: *common\n  - name: c\n    type: nodejs\n    workdir: c\n")

	merged, err := MergeYAML(base, theirs, ours)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(merged), "*common") {
		t.Fatalf("merged output should expand aliases:\n%s", merged)
	}
	var c Config
	if err := yaml.Unmarshal(merged, &c); err != nil {
		t.Fatalf("merged yaml invalid: %v\n%s", err, merged)
	}
	if c.ConfigVersion != "1.1.0" {
		t.Fatalf("config_version = %q", c.ConfigVersion)
	}
	if len(c.Services) != 3 || c.Services[2].Name != "c" {
		t.Fatalf("services = %+v", c.Services)
	}
	if c.Services[1].IsEnabled() {
		t.Fatal("service b should preserve enabled=false")
	}
}

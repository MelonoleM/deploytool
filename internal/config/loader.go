package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// OverlayPath returns the runtime override file for an install root. UI edits are
// written there so the base deploytool.yaml stays untouched.
func OverlayPath(installRoot string) string {
	return filepath.Join(installRoot, ".deploytool", "override.yaml")
}

// Load returns the effective config: the base file, or the runtime override when
// one exists. Relative source paths still resolve against the base config dir.
func Load(path string) (*Config, error) {
	base, err := loadBase(path)
	if err != nil {
		return nil, err
	}
	root, err := runtimeDir(path, base.Vars["install_root"])
	if err != nil {
		return nil, err
	}
	overlay := OverlayPath(root)
	if _, err := os.Stat(overlay); err != nil {
		return base, nil
	}
	oc, err := loadBase(overlay)
	if err != nil {
		return nil, fmt.Errorf("load override %q: %w", overlay, err)
	}
	oc.BaseDir = base.BaseDir
	return oc, nil
}

// NormalizeInstallRoot canonicalizes an install root to an absolute path, applying
// the same defaulting rules as loadBase so callers can compare roots reliably.
func NormalizeInstallRoot(root string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return defaultInstallRoot()
	}
	if !filepath.IsAbs(root) {
		if abs, err := filepath.Abs(root); err == nil {
			return abs
		}
	}
	return filepath.Clean(root)
}

// SamePath reports whether two install roots point at the same location, ignoring
// case on Windows where filesystem paths are case-insensitive.
func SamePath(a, b string) bool {
	a = NormalizeInstallRoot(a)
	b = NormalizeInstallRoot(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// runtimeDir returns the effective install root: the recorded runtime location, or
// the base install root when the install directory has never been changed.
func runtimeDir(basePath, fallback string) (string, error) {
	if b, err := os.ReadFile(basePath + ".install_root"); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return NormalizeInstallRoot(s), nil
		}
	}
	return NormalizeInstallRoot(fallback), nil
}

// CurrentRuntimeDir returns the currently effective install root for a base config.
func CurrentRuntimeDir(basePath string) (string, error) {
	base, err := BaseInstallRoot(basePath)
	if err != nil {
		return "", err
	}
	return runtimeDir(basePath, base)
}

// SetRuntimeDir records the effective install root after a change, so the override
// and runtime data follow it on the next load. Cleanup of the previous root is
// performed by the caller (the web server), which knows both old and new roots.
//
// When the requested root equals the base config's default, no pointer is written:
// the base value already describes the location, so the sidecar would only clutter
// the working directory. Any stale pointer from an earlier non-default choice is
// removed in that case.
func SetRuntimeDir(basePath, root string) error {
	root = NormalizeInstallRoot(root)
	base, err := BaseInstallRoot(basePath)
	if err != nil {
		return err
	}
	sidecar := basePath + ".install_root"
	if SamePath(root, base) {
		_ = os.Remove(sidecar)
		return nil
	}
	return os.WriteFile(sidecar, []byte(root), 0644)
}

// BaseInstallRoot returns the install root from the base config alone.
func BaseInstallRoot(basePath string) (string, error) {
	c, err := loadBase(basePath)
	if err != nil {
		return "", err
	}
	return c.Vars["install_root"], nil
}

// OverlayPathFor returns the runtime override path under the effective install root.
func OverlayPathFor(basePath string) (string, error) {
	base, err := BaseInstallRoot(basePath)
	if err != nil {
		return "", err
	}
	root, err := runtimeDir(basePath, base)
	if err != nil {
		return "", err
	}
	return OverlayPath(root), nil
}

// EnsureOverlay makes the runtime override file exist, seeded from the base config,
// so node-editing writes can target it without touching the base.
func EnsureOverlay(basePath string) (string, error) {
	overlay, err := OverlayPathFor(basePath)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(overlay); os.IsNotExist(err) {
		b, err := os.ReadFile(basePath)
		if err != nil {
			return "", err
		}
		if err := os.MkdirAll(filepath.Dir(overlay), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(overlay, b, 0644); err != nil {
			return "", err
		}
	}
	return overlay, nil
}

// RemoveOverlay deletes the runtime override and the recorded runtime dir,
// restoring the base config.
func RemoveOverlay(basePath string) error {
	overlay, err := OverlayPathFor(basePath)
	if err != nil {
		return err
	}
	if err := os.Remove(overlay); err != nil && !os.IsNotExist(err) {
		return err
	}
	_ = os.Remove(basePath + ".install_root")
	return nil
}
func loadBase(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %q: %w", path, err)
	}
	var raw struct {
		Vars map[string]string `yaml:"vars"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse config vars: %w", err)
	}
	if raw.Vars == nil {
		raw.Vars = map[string]string{}
	}
	for k, v := range raw.Vars {
		if e, ok := os.LookupEnv(k); ok {
			v = e
		} else if e, ok := os.LookupEnv(strings.ToUpper(k)); ok {
			// Environment variables are conventionally upper-case; retain the
			// lower-case form as an explicit override for backward compatibility.
			v = e
		}
		raw.Vars[k] = expandEnvValue(v)
	}
	if strings.TrimSpace(raw.Vars["install_root"]) == "" {
		raw.Vars["install_root"] = defaultInstallRoot()
	}
	if root := raw.Vars["install_root"]; root != "" && !filepath.IsAbs(root) {
		if abs, err := filepath.Abs(root); err == nil {
			raw.Vars["install_root"] = abs
		}
	}
	// Global rendering is intentionally limited to Vars. Service-local expressions
	// (for example {{ .workdir }}) are rendered later for each install step.
	rendered := string(b)
	for key, value := range raw.Vars {
		rendered = substituteVar(rendered, key, value)
	}
	var c Config
	if err = yaml.Unmarshal([]byte(rendered), &c); err != nil {
		return nil, fmt.Errorf("parse rendered config: %w", err)
	}
	if c.Vars == nil {
		c.Vars = map[string]string{}
	}
	for k, v := range raw.Vars {
		c.Vars[k] = v
	}
	if abs, err := filepath.Abs(path); err == nil {
		c.BaseDir = filepath.Dir(abs)
	}
	return &c, Validate(&c)
}

// substituteVar replaces a declarative variable reference in the raw config text.
// References that sit inside a double-quoted YAML scalar are escaped, because a
// value such as the Windows path C:\demo\runtime would otherwise be read as YAML
// escape sequences and fail to parse (or silently corrupt the value).
func substituteVar(text, key, value string) string {
	for _, marker := range []string{"{{ .Vars." + key + " }}", "{{.Vars." + key + "}}"} {
		text = strings.ReplaceAll(text, `"`+marker, `"`+escapeYAMLDoubleQuoted(value))
		text = strings.ReplaceAll(text, marker+`"`, escapeYAMLDoubleQuoted(value)+`"`)
		text = strings.ReplaceAll(text, marker, value)
	}
	return text
}

func escapeYAMLDoubleQuoted(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	return strings.ReplaceAll(v, `"`, `\"`)
}

var (
	percentEnvRE = regexp.MustCompile(`%([A-Za-z_][A-Za-z0-9_]*)%`)
	braceEnvRE   = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)
)

// expandEnvValue resolves %VAR% and ${VAR} references against the inherited
// process environment. Bare $VAR is not expanded to avoid clobbering literal
// dollar signs.
func expandEnvValue(s string) string {
	s = percentEnvRE.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(m[1 : len(m)-1])
	})
	s = braceEnvRE.ReplaceAllStringFunc(s, func(m string) string {
		return os.Getenv(m[2 : len(m)-1])
	})
	return s
}

// defaultInstallRoot returns the per-user install location for the current
// platform, so a config does not need a machine-specific hard-coded path.
func defaultInstallRoot() string {
	switch runtime.GOOS {
	case "windows":
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "deploytool")
		}
		if v, err := os.UserCacheDir(); err == nil {
			return filepath.Join(v, "deploytool")
		}
	case "darwin":
		if v, err := os.UserHomeDir(); err == nil {
			return filepath.Join(v, "Library", "Application Support", "deploytool")
		}
	default:
		if v := os.Getenv("XDG_DATA_HOME"); v != "" {
			return filepath.Join(v, "deploytool")
		}
		if v, err := os.UserHomeDir(); err == nil {
			return filepath.Join(v, ".local", "share", "deploytool")
		}
	}
	return "deploytool"
}

func Render(s string, svc Service, vars map[string]string) (string, error) {
	t, e := template.New("value").Option("missingkey=error").Parse(s)
	if e != nil {
		return "", e
	}
	var b bytes.Buffer
	e = t.Execute(&b, map[string]any{
		"Vars":    vars,
		"workdir": svc.Workdir,
		"env":     svc.Env,
		// Map keys retain the lower-case schema syntax documented for step values.
		"install": map[string]string{"artifact": svc.Install.Artifact},
	})
	return strings.TrimSpace(b.String()), e
}

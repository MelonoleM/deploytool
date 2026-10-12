package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// BaselinePath returns the recorded template snapshot used to detect and diff
// config updates for an install root.
func BaselinePath(installRoot string) string {
	return filepath.Join(installRoot, ".deploytool", "baseline.yaml")
}

// UpdateStatePath returns the persisted update-prompt state.
func UpdateStatePath(installRoot string) string {
	return filepath.Join(installRoot, ".deploytool", "update.json")
}

// UpdateState records which config_version the operator deferred.
type UpdateState struct {
	DismissedVersion string `json:"dismissed_version,omitempty"`
}

// ConfigVersionFromPath reads config_version from a raw config file.
func ConfigVersionFromPath(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var raw struct {
		ConfigVersion string `yaml:"config_version"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return "", err
	}
	return strings.TrimSpace(raw.ConfigVersion), nil
}

// RecordBaseline snapshots the base config the first time a deployed console is
// opened, so later template changes can be detected and diffed.
func RecordBaseline(basePath, installRoot string) error {
	dst := BaselinePath(installRoot)
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	b, err := os.ReadFile(basePath)
	if err != nil {
		return err
	}
	return writeBaseline(installRoot, b)
}

func writeBaseline(installRoot string, b []byte) error {
	dst := BaselinePath(installRoot)
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0644)
}

// BaselineVersion returns the recorded baseline's config_version (empty if none).
func BaselineVersion(installRoot string) (string, error) {
	b, err := os.ReadFile(BaselinePath(installRoot))
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var raw struct {
		ConfigVersion string `yaml:"config_version"`
	}
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return "", fmt.Errorf("parse baseline: %w", err)
	}
	return strings.TrimSpace(raw.ConfigVersion), nil
}

// ReadUpdateState loads the persisted update-prompt state.
func ReadUpdateState(installRoot string) (UpdateState, error) {
	var st UpdateState
	b, err := os.ReadFile(UpdateStatePath(installRoot))
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(b, &st); err != nil {
		return st, fmt.Errorf("read update state: %w", err)
	}
	return st, nil
}

// WriteUpdateState persists the update-prompt state.
func WriteUpdateState(installRoot string, st UpdateState) error {
	if err := os.MkdirAll(filepath.Dir(UpdateStatePath(installRoot)), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(UpdateStatePath(installRoot), b, 0600)
}

// Change describes one field-level difference between the old baseline and the
// current base config.
type Change struct {
	Path string `json:"path"`
	Kind string `json:"kind"` // added | removed | changed
	Old  string `json:"old,omitempty"`
	New  string `json:"new,omitempty"`
}

// DiffUpdate computes the template changes between the recorded baseline and the
// current base config.
func DiffUpdate(basePath, installRoot string) ([]Change, error) {
	oldB, err := os.ReadFile(BaselinePath(installRoot))
	if err != nil {
		return nil, err
	}
	newB, err := os.ReadFile(basePath)
	if err != nil {
		return nil, err
	}
	oldDoc, err := parseDoc(oldB)
	if err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}
	newDoc, err := parseDoc(newB)
	if err != nil {
		return nil, fmt.Errorf("parse base config: %w", err)
	}
	var changes []Change
	diffAt(oldDoc, newDoc, "", &changes)
	return changes, nil
}

// ApplyUpdate performs a three-way merge of the base config's official changes
// into the user's override: base=baseline, theirs=override, ours=current base,
// with official values winning conflicts. The previous override is backed up and
// the baseline advances to the current base.
func ApplyUpdate(basePath, installRoot string) error {
	baseBytes, err := os.ReadFile(basePath)
	if err != nil {
		return err
	}
	overlayPath := OverlayPath(installRoot)
	overlayBytes, err := os.ReadFile(overlayPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if os.IsNotExist(err) || strings.TrimSpace(string(overlayBytes)) == "" {
		return writeBaseline(installRoot, baseBytes)
	}

	baselineBytes, err := os.ReadFile(BaselinePath(installRoot))
	if err != nil {
		return err
	}
	merged, err := MergeYAML(baselineBytes, overlayBytes, baseBytes)
	if err != nil {
		return err
	}
	if err := backupFile(overlayPath); err != nil {
		return err
	}
	if err := os.WriteFile(overlayPath, merged, 0644); err != nil {
		return err
	}
	if err := writeBaseline(installRoot, baseBytes); err != nil {
		_ = os.WriteFile(overlayPath, overlayBytes, 0644)
		return err
	}
	// Keep the override locatable even if the new template changed its default
	// install directory.
	_ = SetRuntimeDir(basePath, installRoot)
	_ = WriteUpdateState(installRoot, UpdateState{})
	return nil
}

// MergeYAML merges "theirs" onto "ours" using "base" as the common ancestor,
// preferring "ours" (the official template) when both sides changed a node.
func MergeYAML(baseBytes, theirsBytes, oursBytes []byte) ([]byte, error) {
	baseDoc, err := parseDoc(baseBytes)
	if err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}
	theirsDoc, err := parseDoc(theirsBytes)
	if err != nil {
		return nil, fmt.Errorf("parse override: %w", err)
	}
	oursDoc, err := parseDoc(oursBytes)
	if err != nil {
		return nil, fmt.Errorf("parse base config: %w", err)
	}
	merged := mergeMapping(baseDoc, theirsDoc, oursDoc, "")
	preserveInstallRoot(merged, theirsDoc)
	return yaml.Marshal(merged)
}

func parseDoc(b []byte) (*yaml.Node, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 {
		return nil, fmt.Errorf("empty config document")
	}
	return root.Content[0], nil
}

func backupFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dst := path + ".bak-" + time.Now().Format("20060102-150405")
	return os.WriteFile(dst, b, 0644)
}

// preserveInstallRoot keeps the operator's chosen install directory, which is a
// deployment-specific runtime location and must not be relocated by a template
// update even though other conflicting fields resolve in favor of the official
// template.
func preserveInstallRoot(merged, theirs *yaml.Node) {
	tv := mappingValue(theirs, "vars")
	if tv == nil {
		return
	}
	ir := mappingValue(tv, "install_root")
	if ir == nil {
		return
	}
	vars := ensureMapping(merged, "vars")
	setMapping(vars, "install_root", cloneNode(ir))
}

func resolveAlias(n *yaml.Node) *yaml.Node {
	if n != nil && n.Kind == yaml.AliasNode && n.Alias != nil {
		return n.Alias
	}
	return n
}

func cloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.AliasNode {
		return cloneNode(n.Alias)
	}
	out := &yaml.Node{
		Kind:        n.Kind,
		Style:       n.Style,
		Tag:         n.Tag,
		Value:       n.Value,
		HeadComment: n.HeadComment,
		LineComment: n.LineComment,
		FootComment: n.FootComment,
	}
	for _, c := range n.Content {
		out.Content = append(out.Content, cloneNode(c))
	}
	return out
}

func nodeEqual(a, b *yaml.Node) bool {
	a, b = resolveAlias(a), resolveAlias(b)
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case yaml.ScalarNode:
		return a.Value == b.Value && a.Tag == b.Tag
	case yaml.MappingNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := 0; i+1 < len(a.Content); i += 2 {
			if !nodeEqual(a.Content[i+1], mappingValue(b, a.Content[i].Value)) {
				return false
			}
		}
		return true
	case yaml.SequenceNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := range a.Content {
			if !nodeEqual(a.Content[i], b.Content[i]) {
				return false
			}
		}
		return true
	default:
		return a.Value == b.Value
	}
}

func orderedKeys(ms ...*yaml.Node) []string {
	var keys []string
	seen := map[string]bool{}
	for _, m := range ms {
		if m == nil || m.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i+1 < len(m.Content); i += 2 {
			k := m.Content[i].Value
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	return keys
}

func mergeMapping(base, theirs, ours *yaml.Node, path string) *yaml.Node {
	out := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range orderedKeys(base, theirs, ours) {
		v := mergeAt(mappingValue(base, k), mappingValue(theirs, k), mappingValue(ours, k), joinPath(path, k), k)
		if v == nil {
			continue
		}
		out.Content = append(out.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k, Tag: "!!str"}, v)
	}
	return out
}

func mergeAt(base, theirs, ours *yaml.Node, path, key string) *yaml.Node {
	if key == "services" {
		return mergeServices(base, theirs, ours, path)
	}
	b, t, o := resolveAlias(base), resolveAlias(theirs), resolveAlias(ours)
	if (b == nil || b.Kind == yaml.MappingNode) && (t == nil || t.Kind == yaml.MappingNode) && (o == nil || o.Kind == yaml.MappingNode) {
		return mergeMapping(b, t, o, path)
	}
	officialChanged := !nodeEqual(b, o)
	userChanged := !nodeEqual(b, t)
	if officialChanged {
		return cloneNode(ours)
	}
	if userChanged {
		return cloneNode(theirs)
	}
	return cloneNode(base)
}

func mergeServices(base, theirs, ours *yaml.Node, path string) *yaml.Node {
	out := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, name := range unionNames(seqNames(base), seqNames(theirs), seqNames(ours)) {
		b := findService(base, name)
		t := findService(theirs, name)
		o := findService(ours, name)
		switch {
		case o == nil && b == nil:
			out.Content = append(out.Content, cloneNode(t))
		case o == nil:
			// The official template removed this service.
			continue
		case b == nil:
			out.Content = append(out.Content, cloneNode(o))
		default:
			out.Content = append(out.Content, mergeMapping(b, t, o, joinPath(path, name)))
		}
	}
	return out
}

func unionNames(sets ...[]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range sets {
		for _, n := range s {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	return out
}

func seqNames(seq *yaml.Node) []string {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	var names []string
	for _, item := range seq.Content {
		if n := mappingValue(item, "name"); n != nil {
			names = append(names, n.Value)
		}
	}
	return names
}

func findService(seq *yaml.Node, name string) *yaml.Node {
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range seq.Content {
		if n := mappingValue(item, "name"); n != nil && n.Value == name {
			return item
		}
	}
	return nil
}

func joinPath(base, seg string) string {
	if base == "" {
		return seg
	}
	return base + "." + seg
}

func isServicesPath(path string) bool {
	return path == "services" || strings.HasSuffix(path, ".services")
}

func diffAt(base, ours *yaml.Node, path string, out *[]Change) {
	b, o := resolveAlias(base), resolveAlias(ours)
	if nodeEqual(b, o) {
		return
	}
	if b != nil && o != nil && b.Kind == yaml.MappingNode && o.Kind == yaml.MappingNode {
		for _, k := range orderedKeys(b, o) {
			diffAt(mappingValue(b, k), mappingValue(o, k), joinPath(path, k), out)
		}
		return
	}
	if isServicesPath(path) {
		for _, name := range unionNames(seqNames(b), seqNames(o)) {
			diffAt(findService(b, name), findService(o, name), joinPath(path, name), out)
		}
		return
	}
	kind := "changed"
	if base == nil {
		kind = "added"
	} else if ours == nil {
		kind = "removed"
	}
	*out = append(*out, Change{Path: path, Kind: kind, Old: renderNode(base), New: renderNode(ours)})
}

func renderNode(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	b, err := yaml.Marshal(n)
	if err != nil {
		return n.Value
	}
	return strings.TrimSpace(string(b))
}

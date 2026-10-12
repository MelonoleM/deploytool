package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Form is the form-editable view of a configuration. Every field that has a natural
// control is exposed here; only install.steps is intentionally left as raw YAML.
type Form struct {
	ProjectName       string        `json:"project_name"`
	ProjectVersion    string        `json:"project_version"`
	InstallRoot       string        `json:"install_root"`
	FailFast          bool          `json:"fail_fast"`
	RollbackOnFailure bool          `json:"rollback_on_failure"`
	WelcomeTitle      string        `json:"welcome_title"`
	WelcomeDesc       string        `json:"welcome_description"`
	WelcomeGuide      string        `json:"welcome_guide"`
	WelcomeFeatures   []string      `json:"welcome_features"`
	Services          []FormService `json:"services"`
}

type FormService struct {
	Name           string   `json:"name"`
	Type           string   `json:"type"`
	Enabled        bool     `json:"enabled"`
	Workdir        string   `json:"workdir"`
	DependsOn      []string `json:"depends_on"`
	Start          string   `json:"start"`
	StartMode      string   `json:"start_mode"`
	Stop           string   `json:"stop"`
	HealthType     string   `json:"health_type"`
	HealthURL      string   `json:"health_url"`
	HealthAddress  string   `json:"health_address"`
	HealthInterval string   `json:"health_interval"`
	HealthTimeout  string   `json:"health_timeout"`
	HealthRetries  int      `json:"health_retries"`
	Env            string   `json:"env"`
	InstallYAML    string   `json:"install_yaml"`
}

// ReadForm loads the config and projects it into the form shape, extracting each
// service's install block as raw YAML text.
func ReadForm(path string) (Form, error) {
	c, err := Load(path)
	if err != nil {
		return Form{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Form{}, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return Form{}, err
	}
	f := Form{
		ProjectName:       c.Project.Name,
		ProjectVersion:    c.Project.Version,
		InstallRoot:       c.Vars["install_root"],
		FailFast:          c.FailFast,
		RollbackOnFailure: c.UpdatePolicy.RollbackOnFailure,
	}
	w := c.Welcome.Defaults()
	f.WelcomeTitle = w.Title
	f.WelcomeDesc = w.Description
	f.WelcomeGuide = w.Guide
	f.WelcomeFeatures = w.Features
	servicesNode := mappingValue(root.Content[0], "services")
	for _, svc := range c.Services {
		fs := FormService{
			Name:           svc.Name,
			Type:           svc.Type,
			Enabled:        svc.IsEnabled(),
			Workdir:        relativeWorkdir(c.Vars["install_root"], svc.Workdir),
			DependsOn:      svc.DependsOn,
			Start:          svc.Start,
			StartMode:      svc.StartMode,
			Stop:           svc.Stop,
			HealthType:     svc.Healthcheck.Type,
			HealthURL:      svc.Healthcheck.URL,
			HealthAddress:  svc.Healthcheck.Address,
			HealthInterval: durStr(svc.Healthcheck.Interval),
			HealthTimeout:  durStr(svc.Healthcheck.Timeout),
			HealthRetries:  svc.Healthcheck.Retries,
			Env:            envToString(svc.Env),
		}
		if fs.StartMode == "" {
			fs.StartMode = "background"
		}
		if node := findServiceNode(servicesNode, svc.Name); node != nil {
			if inst := mappingValue(node, "install"); inst != nil {
				fs.InstallYAML = renderMapping("install", inst)
			}
		}
		f.Services = append(f.Services, fs)
	}
	return f, nil
}

// WriteForm applies the form values back to the YAML document. It edits the parsed
// node tree rather than re-marshalling structs, preserving comments, anchors, flow
// style and key order.
func WriteForm(path string, f Form) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if len(root.Content) == 0 {
		return fmt.Errorf("empty config document")
	}
	doc := root.Content[0]

	project := ensureMapping(doc, "project")
	setString(project, "name", f.ProjectName)
	setString(project, "version", f.ProjectVersion)

	vars := ensureMapping(doc, "vars")
	setString(vars, "install_root", f.InstallRoot)

	setBoolIfTrue(doc, "fail_fast", f.FailFast)
	up := ensureMapping(doc, "update_policy")
	setBoolIfTrue(up, "rollback_on_failure", f.RollbackOnFailure)

	welcome := ensureMapping(doc, "welcome")
	setString(welcome, "title", f.WelcomeTitle)
	setString(welcome, "description", f.WelcomeDesc)
	setString(welcome, "guide", f.WelcomeGuide)
	setSequenceIfAny(welcome, "features", f.WelcomeFeatures)

	services := ensureSequence(doc, "services")
	for _, fs := range f.Services {
		svc := findServiceNode(services, fs.Name)
		if svc == nil {
			continue
		}
		if err := applyService(svc, fs); err != nil {
			return err
		}
	}

	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

// WriteService applies a single service's form values without touching any other
// part of the document. It powers the per-service config dialog.
func WriteService(path string, fs FormService) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if len(root.Content) == 0 {
		return fmt.Errorf("empty config document")
	}
	services := ensureSequence(root.Content[0], "services")
	svc := findServiceNode(services, fs.Name)
	if svc == nil {
		return fmt.Errorf("service %q not found", fs.Name)
	}
	if err := applyService(svc, fs); err != nil {
		return err
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0644)
}

func applyService(svc *yaml.Node, fs FormService) error {
	setString(svc, "name", fs.Name)
	setString(svc, "type", fs.Type)
	if fs.Enabled {
		removeKey(svc, "enabled")
	} else {
		setBool(svc, "enabled", false)
	}
	setString(svc, "workdir", fs.Workdir)
	setSequenceIfAny(svc, "depends_on", fs.DependsOn)
	setStringIfAny(svc, "start", fs.Start)
	setStringIfAny(svc, "start_mode", fs.StartMode)
	setStringIfAny(svc, "stop", fs.Stop)
	setHealthcheck(svc, fs)
	setStringMap(svc, "env", parseEnv(fs.Env))
	if strings.TrimSpace(fs.InstallYAML) != "" {
		inst, err := parseMappingValue(fs.InstallYAML, "install")
		if err != nil {
			return fmt.Errorf("service %s install: %w", fs.Name, err)
		}
		setMapping(svc, "install", inst)
	}
	return nil
}

func setHealthcheck(svc *yaml.Node, s FormService) {
	if s.HealthType == "" {
		removeKey(svc, "healthcheck")
		return
	}
	hc := ensureMapping(svc, "healthcheck")
	setString(hc, "type", s.HealthType)
	setStringIfAny(hc, "url", s.HealthURL)
	setStringIfAny(hc, "address", s.HealthAddress)
	if s.HealthInterval != "" {
		setScalar(hc, "interval", s.HealthInterval, "")
	} else {
		removeKey(hc, "interval")
	}
	if s.HealthTimeout != "" {
		setScalar(hc, "timeout", s.HealthTimeout, "")
	} else {
		removeKey(hc, "timeout")
	}
	if s.HealthRetries > 0 {
		setScalar(hc, "retries", strconv.Itoa(s.HealthRetries), "!!int")
	} else {
		removeKey(hc, "retries")
	}
}

// relativeWorkdir returns the portion of workdir beneath installRoot, so the form
// edits a portable sub-directory instead of a resolved absolute path. Custom
// absolute locations are returned unchanged.
func relativeWorkdir(installRoot, workdir string) string {
	if workdir == "" {
		return workdir
	}
	root := filepath.ToSlash(filepath.Clean(installRoot))
	wd := filepath.ToSlash(filepath.Clean(workdir))
	if strings.HasPrefix(wd, root+"/") {
		return strings.TrimPrefix(wd, root+"/")
	}
	return workdir
}
func durStr(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.String()
}

func ensureMapping(m *yaml.Node, key string) *yaml.Node {
	if v := mappingValue(m, key); v != nil && v.Kind == yaml.MappingNode {
		return v
	}
	v := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"}, v)
	return v
}

func ensureSequence(m *yaml.Node, key string) *yaml.Node {
	if v := mappingValue(m, key); v != nil && v.Kind == yaml.SequenceNode {
		return v
	}
	v := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"}, v)
	return v
}

func setScalar(m *yaml.Node, key, value, tag string) {
	if v := mappingValue(m, key); v != nil && v.Kind == yaml.ScalarNode {
		v.Value = value
		if tag != "" {
			v.Tag = tag
		}
		return
	}
	if tag == "" {
		tag = "!!str"
	}
	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: value, Tag: tag},
	)
}

func setString(m *yaml.Node, key, value string) { setScalar(m, key, value, "!!str") }
func setBool(m *yaml.Node, key string, value bool) {
	setScalar(m, key, strconv.FormatBool(value), "!!bool")
}
func setBoolIfTrue(m *yaml.Node, key string, v bool) {
	if v {
		setBool(m, key, true)
	} else {
		removeKey(m, key)
	}
}

func setStringIfAny(m *yaml.Node, key, value string) {
	if value != "" {
		setString(m, key, value)
	} else {
		removeKey(m, key)
	}
}

func setSequenceIfAny(m *yaml.Node, key string, items []string) {
	if len(items) == 0 {
		removeKey(m, key)
		return
	}
	seq := ensureSequence(m, key)
	seq.Content = nil
	for _, it := range items {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: it, Tag: "!!str"})
	}
}

func setStringMap(m *yaml.Node, key string, values map[string]string) {
	if len(values) == 0 {
		removeKey(m, key)
		return
	}
	mp := ensureMapping(m, key)
	mp.Content = nil
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		mp.Content = append(mp.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: k, Tag: "!!str"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: values[k], Tag: "!!str"},
		)
	}
}

// envToString renders a service's environment map as one KEY=VALUE line per entry.
func envToString(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(env[k])
		b.WriteString("\n")
	}
	return b.String()
}

func parseEnv(s string) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		i := strings.Index(line, "=")
		if i <= 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.TrimSpace(line[i+1:])
		if k != "" {
			out[k] = v
		}
	}
	return out
}

func setMapping(m *yaml.Node, key string, val *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"}, val)
}

func removeKey(m *yaml.Node, key string) {
	if m == nil || m.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

func findServiceNode(services *yaml.Node, name string) *yaml.Node {
	if services == nil || services.Kind != yaml.SequenceNode {
		return nil
	}
	for _, item := range services.Content {
		if item.Kind == yaml.MappingNode && scalar(mappingValue(item, "name")) == name {
			return item
		}
	}
	return nil
}

func parseMappingValue(text, key string) (*yaml.Node, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(text), &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected a mapping")
	}
	v := mappingValue(root.Content[0], key)
	if v == nil {
		return nil, fmt.Errorf("key %q not found", key)
	}
	return v, nil
}

func renderMapping(key string, val *yaml.Node) string {
	w := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Value: key, Tag: "!!str"},
		val,
	}}
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	_ = enc.Encode(w)
	_ = enc.Close()
	return buf.String()
}

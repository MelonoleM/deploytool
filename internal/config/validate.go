package config

import (
	"fmt"
	"strings"
)

var types = map[string]bool{"python": true, "java": true, "nodejs": true, "go": true, "nginx": true, "exe": true}

func Validate(c *Config) error {
	if c.APIVersion != "v1" {
		return fmt.Errorf("apiVersion must be v1")
	}
	if strings.TrimSpace(c.Project.Name) == "" {
		return fmt.Errorf("project.name is required")
	}
	names := map[string]bool{}
	for _, s := range c.Services {
		if s.Name == "" {
			return fmt.Errorf("service.name is required")
		}
		if names[s.Name] {
			return fmt.Errorf("duplicate service %q", s.Name)
		}
		names[s.Name] = true
		if !types[s.Type] {
			return fmt.Errorf("service %q has unsupported type %q", s.Name, s.Type)
		}
		if s.Workdir == "" {
			return fmt.Errorf("service %q workdir is required", s.Name)
		}
		if s.StartMode != "" && s.StartMode != "background" && s.StartMode != "console" {
			return fmt.Errorf("service %q start_mode must be background or console", s.Name)
		}
	}
	for _, s := range c.Services {
		for _, d := range s.DependsOn {
			if !names[d] {
				return fmt.Errorf("service %q depends on unknown service %q", s.Name, d)
			}
		}
	}
	return nil
}

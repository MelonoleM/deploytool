package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// SetServiceEnabled flips the `enabled` key of one service in the YAML document.
// It edits the parsed node tree rather than re-marshalling from structs, which
// preserves comments, anchors, aliases and key order.
func SetServiceEnabled(path, name string, enabled bool) error {
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
	services := mappingValue(doc, "services")
	if services == nil || services.Kind != yaml.SequenceNode {
		return fmt.Errorf("services list not found")
	}
	for _, item := range services.Content {
		if item.Kind != yaml.MappingNode {
			continue
		}
		if scalar(mappingValue(item, "name")) != name {
			continue
		}
		if v := mappingValue(item, "enabled"); v != nil {
			v.Value = strconv.FormatBool(enabled)
			v.Tag = "!!bool"
		} else {
			item.Content = append(item.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "enabled"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(enabled)},
			)
		}
		out, err := yaml.Marshal(&root)
		if err != nil {
			return err
		}
		return os.WriteFile(path, out, 0644)
	}
	return fmt.Errorf("service %q not found", name)
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalar(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	return n.Value
}

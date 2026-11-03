package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	SkillSourceNamespace = "namespace"
	SkillSourceHub       = "hub"
	SkillSourceAll       = "all"
)

// NormalizeSkillSource resolves the built-in default for profiles without a preference.
func NormalizeSkillSource(value string) (string, error) {
	switch value = strings.ToLower(strings.TrimSpace(value)); value {
	case "":
		return SkillSourceAll, nil
	case SkillSourceNamespace, SkillSourceHub, SkillSourceAll:
		return value, nil
	default:
		return "", fmt.Errorf("invalid source %q (expect namespace, hub or all)", value)
	}
}

// UnmarshalYAML reads older development profiles but exposes and saves only source.
func (c *Config) UnmarshalYAML(node *yaml.Node) error {
	type plainConfig Config
	decoded := plainConfig(*c)
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*c = Config(decoded)
	var legacy *yaml.Node
	for i := 0; i+1 < len(node.Content); i += 2 {
		switch node.Content[i].Value {
		case "source":
			// An explicit canonical field, even empty, takes precedence over the legacy field.
			return nil
		case "skillSource":
			legacy = node.Content[i+1]
		}
	}
	if legacy != nil {
		return legacy.Decode(&c.Source)
	}
	return nil
}

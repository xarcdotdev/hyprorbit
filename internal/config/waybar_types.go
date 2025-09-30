package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// StringList allows YAML values to be provided as either a single string or a list of strings.
type StringList []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *StringList) UnmarshalYAML(value *yaml.Node) error {
	if value.Tag == "!!null" {
		*s = nil
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		var v string
		if err := value.Decode(&v); err != nil {
			return err
		}
		if strings.TrimSpace(v) == "" {
			*s = nil
			return nil
		}
		*s = []string{v}
		return nil
	case yaml.SequenceNode:
		var out []string
		if err := value.Decode(&out); err != nil {
			return err
		}
		*s = out
		return nil
	default:
		return fmt.Errorf("expected string or list, got kind %v", value.Kind)
	}
}

// ClassValue captures rule outputs that can be either a string or list of strings.
type ClassValue []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (c *ClassValue) UnmarshalYAML(value *yaml.Node) error {
	if value.Tag == "!!null" {
		*c = nil
		return nil
	}
	switch value.Kind {
	case yaml.ScalarNode:
		var v string
		if err := value.Decode(&v); err != nil {
			return err
		}
		v = strings.TrimSpace(v)
		if v == "" {
			*c = nil
			return nil
		}
		*c = []string{v}
		return nil
	case yaml.SequenceNode:
		var out []string
		if err := value.Decode(&out); err != nil {
			return err
		}
		*c = out
		return nil
	default:
		return fmt.Errorf("expected string or list, got kind %v", value.Kind)
	}
}

// WaybarConfig captures Waybar-specific tuning knobs.
type WaybarConfig struct {
	ModuleWatch WaybarModuleWatchConfig `yaml:"module_watch"`
	Extras      map[string]any          `yaml:",inline"`
}

// WaybarModuleWatchConfig describes how to format module watch output for Waybar.
type WaybarModuleWatchConfig struct {
	Text       StringList        `yaml:"text"`
	Tooltip    StringList        `yaml:"tooltip"`
	Alt        StringList        `yaml:"alt"`
	Percentage *WaybarPercentage `yaml:"percentage"`
	Class      WaybarClassConfig `yaml:"class"`
	Extras     map[string]any    `yaml:",inline"`
}

// WaybarPercentage tunes the percentage field calculation.
type WaybarPercentage struct {
	Source string         `yaml:"source"`
	Max    int            `yaml:"max"`
	Extras map[string]any `yaml:",inline"`
}

// WaybarClassConfig configures CSS class generation.
type WaybarClassConfig struct {
	Sources StringList        `yaml:"sources"`
	Rules   []WaybarClassRule `yaml:"rules"`
	Extras  map[string]any    `yaml:",inline"`
}

// WaybarClassRule describes a conditional class override.
type WaybarClassRule struct {
	Field  string         `yaml:"field"`
	Equals string         `yaml:"equals"`
	Match  string         `yaml:"match"`
	Value  ClassValue     `yaml:"value"`
	Append *bool          `yaml:"append"`
	Extras map[string]any `yaml:",inline"`
}

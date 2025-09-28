package config

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
)

var (
	orbitNamePattern  = regexp.MustCompile(`^[A-Za-z0-9]+$`)
	moduleNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
)

// Config models the user configuration file.
type Config struct {
	Orbits   []Orbit           `yaml:"orbits"`
	Modules  map[string]Module `yaml:"modules"`
	Defaults ModuleDefaults    `yaml:"defaults"`
	Extras   map[string]any    `yaml:",inline"`
}

// Orbit describes a single orbit/context.
type Orbit struct {
	Name   string         `yaml:"name"`
	Label  string         `yaml:"label"`
	Color  string         `yaml:"color"`
	Extras map[string]any `yaml:",inline"`
}

// Module captures focus and spawn behavior for a module workspace.
type Module struct {
	Hotkey string         `yaml:"hotkey"`
	Focus  ModuleFocus    `yaml:"focus"`
	Seed   []SeedEntry    `yaml:"seed"`
	Extras map[string]any `yaml:",inline"`
}

// ModuleFocus guides module focus-or-launch behavior.
type ModuleFocus struct {
	Match         string         `yaml:"match"`
	Cmd           []string       `yaml:"cmd"`
	WorkspaceType string         `yaml:"workspace_type"`
	Extras        map[string]any `yaml:",inline"`
}

// SeedEntry mirrors focus overrides used when seeding a module.
type SeedEntry struct {
	Match  string         `yaml:"match"`
	Cmd    []string       `yaml:"cmd"`
	Extras map[string]any `yaml:",inline"`
}

// ModuleDefaults carries global toggles for module focus actions.
type ModuleDefaults struct {
	Float  *bool          `yaml:"float"`
	Move   *bool          `yaml:"move"`
	Extras map[string]any `yaml:",inline"`
}

// Validate verifies structure and naming constraints.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config: nil")
	}

	var errs []error

	if len(c.Orbits) == 0 {
		errs = append(errs, fmt.Errorf("config: at least one orbit required"))
	}

	seenOrbit := make(map[string]struct{}, len(c.Orbits))
	for i, orbit := range c.Orbits {
		if orbit.Name == "" {
			errs = append(errs, fmt.Errorf("config: orbit[%d] missing name", i))
			continue
		}
		if !orbitNamePattern.MatchString(orbit.Name) {
			errs = append(errs, fmt.Errorf("config: orbit[%d] name %q must match %s", i, orbit.Name, orbitNamePattern.String()))
		}
		if _, ok := seenOrbit[orbit.Name]; ok {
			errs = append(errs, fmt.Errorf("config: orbit name %q duplicated", orbit.Name))
		}
		seenOrbit[orbit.Name] = struct{}{}
	}

	if len(c.Modules) == 0 {
		errs = append(errs, fmt.Errorf("config: at least one module required"))
	}

	moduleNames := make([]string, 0, len(c.Modules))
	for name := range c.Modules {
		moduleNames = append(moduleNames, name)
	}
	sort.Strings(moduleNames)
	for _, name := range moduleNames {
		if name == "" {
			errs = append(errs, fmt.Errorf("config: module name cannot be empty"))
			continue
		}
		if !moduleNamePattern.MatchString(name) {
			errs = append(errs, fmt.Errorf("config: module %q must match %s", name, moduleNamePattern.String()))
		}
	}

	if len(errs) == 0 {
		return nil
	}

	if len(errs) == 1 {
		return errs[0]
	}

	msg := "config validation failed:"
	for _, err := range errs {
		msg += "\n - " + err.Error()
	}
	return errors.New(msg)
}

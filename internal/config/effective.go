package config

import (
	"fmt"
	"sort"
	"strings"
)

// EffectiveConfig is the validated, runtime-ready configuration.
type EffectiveConfig struct {
	Orbits   []OrbitRecord
	Modules  map[string]ModuleRecord
	Defaults ModuleSettings
	Source   string
	Warnings []string
}

// OrbitRecord represents a normalized orbit entry.
type OrbitRecord struct {
	Name  string
	Label string
	Color string
}

// ModuleRecord represents a module and its behaviors.
type ModuleRecord struct {
	Name   string
	Hotkey string
	Focus  ModuleFocusSpec
	Seed   []SeedSpec
}

// ModuleFocusSpec captures matcher and launch behavior.
type ModuleFocusSpec struct {
	Matcher       Matcher
	Cmd           []string
	WorkspaceType string
}

// SeedSpec defines individual seed steps.
type SeedSpec struct {
	Matcher Matcher
	Cmd     []string
}

// ModuleSettings contains resolved defaults for module operations.
type ModuleSettings struct {
	Float bool
	Move  bool
}

// Matcher instructs how to filter Hyprland clients.
type Matcher struct {
	Field string
	Expr  string
	Raw   string
}

const (
	defaultMatcherField = "class"
)

// BuildEffective constructs the runtime config, returning warnings for unknown keys.
func BuildEffective(source string, cfg *Config) (*EffectiveConfig, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	warnings := collectWarnings(cfg)

	orbits := make([]OrbitRecord, 0, len(cfg.Orbits))
	for _, o := range cfg.Orbits {
		orbits = append(orbits, OrbitRecord{
			Name:  o.Name,
			Label: o.Label,
			Color: o.Color,
		})
	}

	modules := make(map[string]ModuleRecord, len(cfg.Modules))
	names := make([]string, 0, len(cfg.Modules))
	for name := range cfg.Modules {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		mod := cfg.Modules[name]
		focusMatcher, err := parseMatcher(mod.Focus.Match)
		if err != nil {
			return nil, fmt.Errorf("module %q focus matcher: %w", name, err)
		}

		focus := ModuleFocusSpec{
			Matcher:       focusMatcher,
			Cmd:           append([]string(nil), mod.Focus.Cmd...),
			WorkspaceType: mod.Focus.WorkspaceType,
		}

		seeds := make([]SeedSpec, 0, len(mod.Seed))
		for i, seed := range mod.Seed {
			matcher, err := parseMatcher(seed.Match)
			if err != nil {
				return nil, fmt.Errorf("module %q seed[%d] matcher: %w", name, i, err)
			}
			seeds = append(seeds, SeedSpec{
				Matcher: matcher,
				Cmd:     append([]string(nil), seed.Cmd...),
			})
		}

		modules[name] = ModuleRecord{
			Name:   name,
			Hotkey: mod.Hotkey,
			Focus:  focus,
			Seed:   seeds,
		}
	}

	defaults := ModuleSettings{
		Float: false,
		Move:  true,
	}

	if cfg.Defaults.Float != nil {
		defaults.Float = *cfg.Defaults.Float
	}
	if cfg.Defaults.Move != nil {
		defaults.Move = *cfg.Defaults.Move
	}

	return &EffectiveConfig{
		Orbits:   orbits,
		Modules:  modules,
		Defaults: defaults,
		Source:   source,
		Warnings: warnings,
	}, nil
}

func parseMatcher(input string) (Matcher, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Matcher{}, nil
	}
	field := defaultMatcherField
	expr := input

	if idx := strings.IndexRune(input, '='); idx > 0 {
		field = strings.TrimSpace(input[:idx])
		expr = strings.TrimSpace(input[idx+1:])
		if field == "" {
			return Matcher{}, fmt.Errorf("empty field in matcher %q", input)
		}
		if expr == "" {
			return Matcher{}, fmt.Errorf("empty expression in matcher %q", input)
		}
	}

	return Matcher{Field: field, Expr: expr, Raw: input}, nil
}

func collectWarnings(cfg *Config) []string {
	var warnings []string

	if len(cfg.Extras) > 0 {
		warnings = append(warnings, sortedKeysMessage("config", cfg.Extras))
	}

	for i := range cfg.Orbits {
		if len(cfg.Orbits[i].Extras) > 0 {
			warnings = append(warnings, sortedKeysMessage(fmt.Sprintf("orbit[%d]", i), cfg.Orbits[i].Extras))
		}
	}

	moduleNames := make([]string, 0, len(cfg.Modules))
	for name := range cfg.Modules {
		moduleNames = append(moduleNames, name)
	}
	sort.Strings(moduleNames)

	for _, name := range moduleNames {
		mod := cfg.Modules[name]
		if len(mod.Extras) > 0 {
			warnings = append(warnings, sortedKeysMessage("module "+name, mod.Extras))
		}
		if len(mod.Focus.Extras) > 0 {
			warnings = append(warnings, sortedKeysMessage("module "+name+" focus", mod.Focus.Extras))
		}
		for idx, seed := range mod.Seed {
			if len(seed.Extras) > 0 {
				warnings = append(warnings, sortedKeysMessage(fmt.Sprintf("module %s seed[%d]", name, idx), seed.Extras))
			}
		}
	}

	if len(cfg.Defaults.Extras) > 0 {
		warnings = append(warnings, sortedKeysMessage("defaults", cfg.Defaults.Extras))
	}

	return warnings
}

func sortedKeysMessage(scope string, extras map[string]any) string {
	keys := make([]string, 0, len(extras))
	for key := range extras {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return fmt.Sprintf("%s: unknown keys %s", scope, strings.Join(keys, ", "))
}

package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// EffectiveConfig is the validated, runtime-ready configuration.
type EffectiveConfig struct {
	Orbits   []OrbitRecord
	Modules  map[string]ModuleRecord
	Defaults ModuleSettings
	Orbit    OrbitSettings
	Source   string
	Warnings []string
	Waybar   WaybarSettings
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

// OrbitSettings contains resolved orbit behaviour toggles.
type OrbitSettings struct {
	SwitchPreference OrbitSwitchPreference
}

// WaybarSettings captures configuration specific to Waybar integrations.
type WaybarSettings struct {
	ModuleWatch WaybarModuleWatchSettings
}

// WaybarModuleWatchSettings describes how to project status snapshots into Waybar JSON.
type WaybarModuleWatchSettings struct {
	Text       []WaybarValueSource
	Tooltip    []WaybarValueSource
	Alt        []WaybarValueSource
	Percentage *WaybarPercentageSetting
	Class      WaybarClassSettings
}

// WaybarValueSource enumerates known data sources for Waybar strings.
type WaybarValueSource string

const (
	WaybarValueModule      WaybarValueSource = "module"
	WaybarValueWorkspace   WaybarValueSource = "workspace"
	WaybarValueOrbit       WaybarValueSource = "orbit"
	WaybarValueOrbitLabel  WaybarValueSource = "orbit_label"
	WaybarValueOrbitColor  WaybarValueSource = "orbit_color"
	WaybarValueModuleOrbit WaybarValueSource = "module_orbit"
	WaybarValueWindows     WaybarValueSource = "windows"
)

// WaybarPercentageSetting configures the optional percentage field.
type WaybarPercentageSetting struct {
	Source WaybarMetricSource
	Max    int
}

// WaybarMetricSource enumerates numeric sources.
type WaybarMetricSource string

const (
	WaybarMetricWindows WaybarMetricSource = "windows"
)

// WaybarClassSettings configures CSS class composition.
type WaybarClassSettings struct {
	Sources []WaybarClassSource
	Rules   []WaybarClassRuleSetting
}

// WaybarClassSource enumerates class components.
type WaybarClassSource string

const (
	WaybarClassModule      WaybarClassSource = "module"
	WaybarClassOrbit       WaybarClassSource = "orbit"
	WaybarClassModuleOrbit WaybarClassSource = "module_orbit"
	WaybarClassWorkspace   WaybarClassSource = "workspace"
	WaybarClassWindows     WaybarClassSource = "windows"
)

// WaybarClassRuleSetting stores a compiled class rule.
type WaybarClassRuleSetting struct {
	Field  WaybarValueSource
	Equals string
	Regex  *regexp.Regexp
	Value  []string
	Append bool
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

	waybar, err := buildWaybarSettings(cfg)
	if err != nil {
		return nil, err
	}

	pref, err := ParseOrbitSwitchPreference(cfg.Orbit.SwitchPreference)
	if err != nil {
		return nil, err
	}

	return &EffectiveConfig{
		Orbits:   orbits,
		Modules:  modules,
		Defaults: defaults,
		Orbit:    OrbitSettings{SwitchPreference: pref},
		Source:   source,
		Warnings: warnings,
		Waybar:   waybar,
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

func buildWaybarSettings(cfg *Config) (WaybarSettings, error) {
	mw := cfg.Waybar.ModuleWatch

	text, err := parseWaybarValueList("text", mw.Text, defaultWaybarText)
	if err != nil {
		return WaybarSettings{}, err
	}

	tooltip, err := parseWaybarValueList("tooltip", mw.Tooltip, defaultWaybarTooltip)
	if err != nil {
		return WaybarSettings{}, err
	}

	alt, err := parseWaybarValueList("alt", mw.Alt, defaultWaybarAlt)
	if err != nil {
		return WaybarSettings{}, err
	}

	classSources, err := parseWaybarClassSources(mw.Class.Sources, defaultWaybarClassSources)
	if err != nil {
		return WaybarSettings{}, err
	}

	rules, err := parseWaybarClassRules(mw.Class.Rules)
	if err != nil {
		return WaybarSettings{}, err
	}

	var percentage *WaybarPercentageSetting
	if mw.Percentage != nil {
		p, err := parseWaybarPercentage(*mw.Percentage)
		if err != nil {
			return WaybarSettings{}, err
		}
		percentage = &p
	}

	return WaybarSettings{
		ModuleWatch: WaybarModuleWatchSettings{
			Text:       text,
			Tooltip:    tooltip,
			Alt:        alt,
			Percentage: percentage,
			Class: WaybarClassSettings{
				Sources: classSources,
				Rules:   rules,
			},
		},
	}, nil
}

var (
	defaultWaybarText         = []WaybarValueSource{WaybarValueModule, WaybarValueWorkspace}
	defaultWaybarTooltip      = []WaybarValueSource{WaybarValueOrbitLabel, WaybarValueOrbit, WaybarValueWorkspace}
	defaultWaybarAlt          = []WaybarValueSource{WaybarValueWorkspace}
	defaultWaybarClassSources = []WaybarClassSource{WaybarClassModule, WaybarClassOrbit}
)

func parseWaybarValueList(name string, input StringList, defaults []WaybarValueSource) ([]WaybarValueSource, error) {
	if len(input) == 0 {
		return append([]WaybarValueSource(nil), defaults...), nil
	}
	result := make([]WaybarValueSource, 0, len(input))
	for idx, raw := range input {
		src, err := parseWaybarValueSource(raw)
		if err != nil {
			return nil, fmt.Errorf("waybar module_watch %s[%d]: %w", name, idx, err)
		}
		result = append(result, src)
	}
	return result, nil
}

func parseWaybarValueSource(raw string) (WaybarValueSource, error) {
	switch strings.TrimSpace(raw) {
	case "", "module":
		if strings.TrimSpace(raw) == "" {
			return "", fmt.Errorf("value cannot be empty")
		}
		return WaybarValueModule, nil
	case "workspace":
		return WaybarValueWorkspace, nil
	case "orbit":
		return WaybarValueOrbit, nil
	case "orbit_label":
		return WaybarValueOrbitLabel, nil
	case "orbit_color":
		return WaybarValueOrbitColor, nil
	case "module_orbit":
		return WaybarValueModuleOrbit, nil
	case "windows":
		return WaybarValueWindows, nil
	default:
		return "", fmt.Errorf("unsupported value source %q", raw)
	}
}

func parseWaybarClassSources(input StringList, defaults []WaybarClassSource) ([]WaybarClassSource, error) {
	if len(input) == 0 {
		return append([]WaybarClassSource(nil), defaults...), nil
	}
	result := make([]WaybarClassSource, 0, len(input))
	for idx, raw := range input {
		src, err := parseWaybarClassSource(raw)
		if err != nil {
			return nil, fmt.Errorf("waybar module_watch class.sources[%d]: %w", idx, err)
		}
		result = append(result, src)
	}
	return result, nil
}

func parseWaybarClassSource(raw string) (WaybarClassSource, error) {
	switch strings.TrimSpace(raw) {
	case "module":
		return WaybarClassModule, nil
	case "orbit":
		return WaybarClassOrbit, nil
	case "module_orbit":
		return WaybarClassModuleOrbit, nil
	case "workspace":
		return WaybarClassWorkspace, nil
	case "windows":
		return WaybarClassWindows, nil
	case "":
		return "", fmt.Errorf("value cannot be empty")
	default:
		return "", fmt.Errorf("unsupported class source %q", raw)
	}
}

func parseWaybarClassRules(rules []WaybarClassRule) ([]WaybarClassRuleSetting, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	result := make([]WaybarClassRuleSetting, 0, len(rules))
	for idx, rule := range rules {
		field := strings.TrimSpace(rule.Field)
		if field == "" {
			return nil, fmt.Errorf("waybar module_watch class.rules[%d]: field is required", idx)
		}
		source, err := parseWaybarValueSource(field)
		if err != nil {
			return nil, fmt.Errorf("waybar module_watch class.rules[%d]: %w", idx, err)
		}
		equals := strings.TrimSpace(rule.Equals)
		match := strings.TrimSpace(rule.Match)
		if equals == "" && match == "" {
			return nil, fmt.Errorf("waybar module_watch class.rules[%d]: specify equals or match", idx)
		}
		if equals != "" && match != "" {
			return nil, fmt.Errorf("waybar module_watch class.rules[%d]: equals and match are mutually exclusive", idx)
		}
		var compiled *regexp.Regexp
		if match != "" {
			var err error
			compiled, err = regexp.Compile(match)
			if err != nil {
				return nil, fmt.Errorf("waybar module_watch class.rules[%d]: compile match: %w", idx, err)
			}
		}
		values := make([]string, 0, len(rule.Value))
		for _, rawValue := range rule.Value {
			sanitised := strings.TrimSpace(rawValue)
			if sanitised == "" {
				continue
			}
			values = append(values, sanitised)
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("waybar module_watch class.rules[%d]: value must not be empty", idx)
		}
		appendMode := rule.Append != nil && *rule.Append
		result = append(result, WaybarClassRuleSetting{
			Field:  source,
			Equals: equals,
			Regex:  compiled,
			Value:  values,
			Append: appendMode,
		})
	}
	return result, nil
}

func parseWaybarPercentage(raw WaybarPercentage) (WaybarPercentageSetting, error) {
	source := strings.TrimSpace(raw.Source)
	if source == "" {
		return WaybarPercentageSetting{}, fmt.Errorf("waybar module_watch percentage.source is required")
	}
	var metric WaybarMetricSource
	switch source {
	case "windows":
		metric = WaybarMetricWindows
	default:
		return WaybarPercentageSetting{}, fmt.Errorf("waybar module_watch percentage.source %q unsupported", raw.Source)
	}
	if raw.Max < 0 {
		return WaybarPercentageSetting{}, fmt.Errorf("waybar module_watch percentage.max must be non-negative")
	}
	return WaybarPercentageSetting{Source: metric, Max: raw.Max}, nil
}

func collectWarnings(cfg *Config) []string {
	var warnings []string

	if len(cfg.Extras) > 0 {
		warnings = append(warnings, sortedKeysMessage("config", cfg.Extras))
	}

	if len(cfg.Orbit.Extras) > 0 {
		warnings = append(warnings, sortedKeysMessage("orbit", cfg.Orbit.Extras))
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

	if len(cfg.Waybar.Extras) > 0 {
		warnings = append(warnings, sortedKeysMessage("waybar", cfg.Waybar.Extras))
	}

	if len(cfg.Waybar.ModuleWatch.Extras) > 0 {
		warnings = append(warnings, sortedKeysMessage("waybar module_watch", cfg.Waybar.ModuleWatch.Extras))
	}

	if len(cfg.Waybar.ModuleWatch.Class.Extras) > 0 {
		warnings = append(warnings, sortedKeysMessage("waybar module_watch class", cfg.Waybar.ModuleWatch.Class.Extras))
	}

	if cfg.Waybar.ModuleWatch.Percentage != nil && len(cfg.Waybar.ModuleWatch.Percentage.Extras) > 0 {
		warnings = append(warnings, sortedKeysMessage("waybar module_watch percentage", cfg.Waybar.ModuleWatch.Percentage.Extras))
	}

	for idx, rule := range cfg.Waybar.ModuleWatch.Class.Rules {
		if len(rule.Extras) > 0 {
			warnings = append(warnings, sortedKeysMessage(fmt.Sprintf("waybar module_watch class.rules[%d]", idx), rule.Extras))
		}
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

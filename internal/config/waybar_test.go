package config

import (
	"reflect"
	"testing"
)

func TestBuildEffectiveWaybarDefaults(t *testing.T) {
	raw := &Config{
		Orbits: []Orbit{{Name: "alpha"}},
		Modules: map[string]Module{
			"dev": {Focus: ModuleFocus{}},
		},
	}
	eff, err := BuildEffective("<defaults>", raw)
	if err != nil {
		t.Fatalf("BuildEffective: %v", err)
	}

	got := eff.Waybar.ModuleWatch
	if !reflect.DeepEqual(got.Text, defaultWaybarText) {
		t.Fatalf("expected default text %v, got %v", defaultWaybarText, got.Text)
	}
	if !reflect.DeepEqual(got.Tooltip, defaultWaybarTooltip) {
		t.Fatalf("expected default tooltip %v, got %v", defaultWaybarTooltip, got.Tooltip)
	}
	if !reflect.DeepEqual(got.Alt, defaultWaybarAlt) {
		t.Fatalf("expected default alt %v, got %v", defaultWaybarAlt, got.Alt)
	}
	if got.Percentage != nil {
		t.Fatalf("expected nil percentage, got %+v", got.Percentage)
	}
	if !reflect.DeepEqual(got.Class.Sources, defaultWaybarClassSources) {
		t.Fatalf("expected default class sources %v, got %v", defaultWaybarClassSources, got.Class.Sources)
	}
	if len(got.Class.Rules) != 0 {
		t.Fatalf("expected no class rules, got %d", len(got.Class.Rules))
	}
}

func TestBuildEffectiveWaybarCustom(t *testing.T) {
	raw := &Config{
		Orbits: []Orbit{{Name: "alpha"}},
		Modules: map[string]Module{
			"dev": {Focus: ModuleFocus{}},
		},
		Waybar: WaybarConfig{
			ModuleWatch: WaybarModuleWatchConfig{
				Text:    StringList{"workspace", "module"},
				Tooltip: StringList{"orbit"},
				Alt:     StringList{"module"},
				Percentage: &WaybarPercentage{
					Source: "windows",
					Max:    10,
				},
				Class: WaybarClassConfig{
					Sources: StringList{"module_orbit", "windows"},
					Rules: []WaybarClassRule{
						{Field: "module", Equals: "dev", Value: ClassValue{"focused"}},
						{Field: "windows", Match: "^0$", Value: ClassValue{"empty"}, Append: boolPtr(true)},
					},
				},
			},
		},
	}
	eff, err := BuildEffective("<custom>", raw)
	if err != nil {
		t.Fatalf("BuildEffective: %v", err)
	}

	mw := eff.Waybar.ModuleWatch
	wantText := []WaybarValueSource{WaybarValueWorkspace, WaybarValueModule}
	if !reflect.DeepEqual(mw.Text, wantText) {
		t.Fatalf("unexpected text sources: %v", mw.Text)
	}
	wantTooltip := []WaybarValueSource{WaybarValueOrbit}
	if !reflect.DeepEqual(mw.Tooltip, wantTooltip) {
		t.Fatalf("unexpected tooltip sources: %v", mw.Tooltip)
	}
	wantAlt := []WaybarValueSource{WaybarValueModule}
	if !reflect.DeepEqual(mw.Alt, wantAlt) {
		t.Fatalf("unexpected alt sources: %v", mw.Alt)
	}
	if mw.Percentage == nil {
		t.Fatalf("expected percentage setting")
	}
	if mw.Percentage.Source != WaybarMetricWindows || mw.Percentage.Max != 10 {
		t.Fatalf("unexpected percentage %+v", mw.Percentage)
	}

	wantClassSources := []WaybarClassSource{WaybarClassModuleOrbit, WaybarClassWindows}
	if !reflect.DeepEqual(mw.Class.Sources, wantClassSources) {
		t.Fatalf("unexpected class sources: %v", mw.Class.Sources)
	}
	if len(mw.Class.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(mw.Class.Rules))
	}
	first := mw.Class.Rules[0]
	if first.Field != WaybarValueModule || first.Equals != "dev" || first.Regex != nil || first.Append {
		t.Fatalf("unexpected first rule %+v", first)
	}
	if !reflect.DeepEqual(first.Value, []string{"focused"}) {
		t.Fatalf("unexpected first rule value: %v", first.Value)
	}

	second := mw.Class.Rules[1]
	if second.Field != WaybarValueWindows || second.Equals != "" || second.Regex == nil || !second.Append {
		t.Fatalf("unexpected second rule %+v", second)
	}
	if !reflect.DeepEqual(second.Value, []string{"empty"}) {
		t.Fatalf("unexpected second rule value: %v", second.Value)
	}
}

func boolPtr(v bool) *bool { return &v }

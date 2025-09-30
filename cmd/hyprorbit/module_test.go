package main

import (
	"context"
	"encoding/json"
	"testing"

	"hyprorbit/internal/app/service"
	"hyprorbit/internal/config"
	"hyprorbit/internal/orbit"
)

func TestModuleWatchFormatterGeneralDefaults(t *testing.T) {
	formatter, err := newModuleWatchFormatter(context.Background(), moduleWatchFormatterOptions{})
	if err != nil {
		t.Fatalf("formatter: %v", err)
	}

	snapshot := service.StatusSnapshot{
		Workspace: "dev-alpha",
		Module:    "dev",
		Orbit: &orbit.Record{
			Name:  "alpha",
			Label: "Alpha Orbit",
			Color: "#ff0000",
		},
	}

	data, err := formatter.Format(snapshot)
	if err != nil {
		t.Fatalf("format: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if got := payload["text"]; got != "dev" {
		t.Fatalf("expected text 'dev', got %v", got)
	}
	if got := payload["workspace"]; got != "dev-alpha" {
		t.Fatalf("expected workspace 'dev-alpha', got %v", got)
	}
	if got := payload["orbit"]; got != "alpha" {
		t.Fatalf("expected orbit 'alpha', got %v", got)
	}
	if got := payload["color"]; got != "#ff0000" {
		t.Fatalf("expected color '#ff0000', got %v", got)
	}
	if got := payload["class"]; got != "dev alpha" {
		t.Fatalf("expected class 'dev alpha', got %v", got)
	}
	if got := payload["tooltip"]; got != "Alpha Orbit" {
		t.Fatalf("expected tooltip 'Alpha Orbit', got %v", got)
	}
}

func TestModuleWatchFormatterWaybarDefaults(t *testing.T) {
	raw := &config.Config{
		Orbits: []config.Orbit{{Name: "alpha", Label: "Alpha Orbit"}},
		Modules: map[string]config.Module{
			"dev": {Focus: config.ModuleFocus{}},
		},
	}
	effective, err := config.BuildEffective("<test>", raw)
	if err != nil {
		t.Fatalf("build effective: %v", err)
	}

	formatter, err := newModuleWatchFormatter(context.Background(), moduleWatchFormatterOptions{
		Waybar: true,
		Config: effective,
	})
	if err != nil {
		t.Fatalf("formatter: %v", err)
	}

	snapshot := service.StatusSnapshot{
		Workspace: "dev-alpha",
		Module:    "dev",
		Orbit: &orbit.Record{
			Name:  "alpha",
			Label: "Alpha Orbit",
			Color: "#ff0000",
		},
		Windows: 3,
	}

	data, err := formatter.Format(snapshot)
	if err != nil {
		t.Fatalf("format: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if got := payload["text"]; got != "dev" {
		t.Fatalf("expected text 'dev', got %v", got)
	}
	if got := payload["alt"]; got != "dev-alpha" {
		t.Fatalf("expected alt 'dev-alpha', got %v", got)
	}
	if got := payload["tooltip"]; got != "Alpha Orbit" {
		t.Fatalf("expected tooltip 'Alpha Orbit', got %v", got)
	}

	classRaw, ok := payload["class"].([]any)
	if !ok {
		t.Fatalf("expected class to be []any, got %T", payload["class"])
	}
	if len(classRaw) != 2 {
		t.Fatalf("expected 2 classes, got %d", len(classRaw))
	}
	if classRaw[0] != "dev" || classRaw[1] != "alpha" {
		t.Fatalf("unexpected class entries: %v", classRaw)
	}

	if _, ok := payload["percentage"]; ok {
		t.Fatalf("did not expect percentage field by default")
	}
}

func TestModuleWatchFormatterWaybarCustomAlt(t *testing.T) {
	raw := &config.Config{
		Orbits: []config.Orbit{{Name: "alpha", Label: "Alpha Orbit"}},
		Modules: map[string]config.Module{
			"dev": {Focus: config.ModuleFocus{}},
		},
		Waybar: config.WaybarConfig{
			ModuleWatch: config.WaybarModuleWatchConfig{
				Alt: config.StringList{"orbit_label", "workspace"},
			},
		},
	}
	effective, err := config.BuildEffective("<custom>", raw)
	if err != nil {
		t.Fatalf("build effective: %v", err)
	}

	formatter, err := newModuleWatchFormatter(context.Background(), moduleWatchFormatterOptions{
		Waybar: true,
		Config: effective,
	})
	if err != nil {
		t.Fatalf("formatter: %v", err)
	}

	snapshot := service.StatusSnapshot{
		Workspace: "dev-alpha",
		Module:    "dev",
		Orbit: &orbit.Record{
			Name:  "alpha",
			Label: "Alpha Orbit",
		},
	}

	data, err := formatter.Format(snapshot)
	if err != nil {
		t.Fatalf("format: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if got := payload["alt"]; got != "Alpha Orbit" {
		t.Fatalf("expected alt 'Alpha Orbit', got %v", got)
	}
}

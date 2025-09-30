package main

import (
	"encoding/json"
	"testing"

	"hyprorbits/internal/app/service"
	"hyprorbits/internal/orbit"
)

func TestMarshalWaybarSnapshot(t *testing.T) {
	snapshot := service.StatusSnapshot{
		Workspace: "dev-alpha",
		Module:    "dev",
		Orbit: &orbit.Record{
			Name:  "alpha",
			Label: "Alpha Orbit",
			Color: "#ff0000",
		},
	}

	data, err := marshalWaybarSnapshot(snapshot)
	if err != nil {
		t.Fatalf("marshalWaybarSnapshot returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload["text"] != "dev" {
		t.Fatalf("expected text 'dev', got %v", payload["text"])
	}
	if payload["workspace"] != "dev-alpha" {
		t.Fatalf("expected workspace 'dev-alpha', got %v", payload["workspace"])
	}
	if payload["orbit"] != "alpha" {
		t.Fatalf("expected orbit 'alpha', got %v", payload["orbit"])
	}
	if payload["color"] != "#ff0000" {
		t.Fatalf("expected color '#ff0000', got %v", payload["color"])
	}
	if payload["class"] != "dev alpha" {
		t.Fatalf("expected class 'dev alpha', got %v", payload["class"])
	}
	if payload["tooltip"] != "Alpha Orbit" {
		t.Fatalf("expected tooltip 'Alpha Orbit', got %v", payload["tooltip"])
	}
}

func TestMarshalWaybarSnapshotFallbacks(t *testing.T) {
	snapshot := service.StatusSnapshot{Workspace: "orphan"}

	data, err := marshalWaybarSnapshot(snapshot)
	if err != nil {
		t.Fatalf("marshalWaybarSnapshot returned error: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}

	if payload["text"] != "orphan" {
		t.Fatalf("expected fallback text 'orphan', got %v", payload["text"])
	}
	if payload["tooltip"] != "orphan" {
		t.Fatalf("expected tooltip 'orphan', got %v", payload["tooltip"])
	}
	if _, ok := payload["orbit"]; ok {
		t.Fatalf("did not expect orbit field, got %v", payload["orbit"])
	}
}

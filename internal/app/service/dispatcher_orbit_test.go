package service

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"hyprorbit/internal/config"
	"hyprorbit/internal/orbit"
)

func TestFilteredOrbitSequenceAll(t *testing.T) {
	d := &Dispatcher{state: &DaemonState{orbitWindowCounts: map[string]int{"alpha": 0}}}
	seq := []string{"alpha"}
	got := d.filteredOrbitSequence(seq, "alpha", config.OrbitCycleModeAll)
	if !reflect.DeepEqual(got, seq) {
		t.Fatalf("expected sequence unchanged, got %v", got)
	}
}

func TestFilteredOrbitSequenceNotEmpty(t *testing.T) {
	d := &Dispatcher{state: &DaemonState{orbitWindowCounts: map[string]int{"alpha": 1, "beta": 0, "gamma": 2}}}
	seq := []string{"alpha", "beta", "gamma"}
	got := d.filteredOrbitSequence(seq, "alpha", config.OrbitCycleModeNotEmpty)
	want := []string{"alpha", "beta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected sequence: got %v want %v", got, want)
	}
}

func TestFilteredOrbitSequenceSkipsExtraEmpties(t *testing.T) {
	d := &Dispatcher{state: &DaemonState{orbitWindowCounts: map[string]int{"alpha": 1, "beta": 0, "gamma": 0, "delta": 2}}}
	seq := []string{"alpha", "beta", "gamma", "delta"}
	got := d.filteredOrbitSequence(seq, "alpha", config.OrbitCycleModeNotEmpty)
	want := []string{"alpha", "beta", "delta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered sequence: got %v want %v", got, want)
	}
}

func TestFilteredOrbitSequenceNoWindowsFallsBack(t *testing.T) {
	d := &Dispatcher{state: &DaemonState{orbitWindowCounts: map[string]int{"alpha": 0, "beta": 0}}}
	seq := []string{"alpha", "beta"}
	got := d.filteredOrbitSequence(seq, "alpha", config.OrbitCycleModeNotEmpty)
	if !reflect.DeepEqual(got, seq) {
		t.Fatalf("expected fallback to original sequence, got %v", got)
	}
}

func TestFilteredOrbitSequenceKeepsEarlierEmpty(t *testing.T) {
	d := &Dispatcher{state: &DaemonState{orbitWindowCounts: map[string]int{"alpha": 0, "beta": 2, "gamma": 1, "delta": 0}}}
	seq := []string{"alpha", "beta", "gamma", "delta"}
	got := d.filteredOrbitSequence(seq, "gamma", config.OrbitCycleModeNotEmpty)
	want := []string{"alpha", "beta", "gamma", "delta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected earlier empty orbit retained, got %v", got)
	}
}

func TestFilteredOrbitSequenceMatchesExample(t *testing.T) {
	d := &Dispatcher{state: &DaemonState{orbitWindowCounts: map[string]int{"orbit1": 1, "orbit2": 0, "orbit3": 2, "orbit4": 0, "orbit5": 0, "orbit6": 0, "orbit7": 0}}}
	seq := []string{"orbit1", "orbit2", "orbit3", "orbit4", "orbit5", "orbit6", "orbit7"}
	got := d.filteredOrbitSequence(seq, "orbit3", config.OrbitCycleModeNotEmpty)
	want := []string{"orbit1", "orbit2", "orbit3", "orbit4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered sequence when current occupied: got %v want %v", got, want)
	}

	got = d.filteredOrbitSequence(seq, "orbit4", config.OrbitCycleModeNotEmpty)
	want = []string{"orbit1", "orbit2", "orbit3", "orbit4"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered sequence when current empty: got %v want %v", got, want)
	}
}

func TestSelectOrbitNameSelectors(t *testing.T) {
	ctx := context.Background()
	tracker := &fakeOrbitTracker{sequence: []string{"alpha", "beta", "gamma"}, current: "alpha"}
	cfg := &config.EffectiveConfig{
		Orbits: []config.OrbitRecord{{Name: "alpha"}, {Name: "beta"}, {Name: "gamma"}},
		Orbit:  config.OrbitSettings{CycleMode: config.OrbitCycleModeAll},
	}
	orbitSvc, err := orbit.NewServiceWithDependencies(orbit.Dependencies{Tracker: tracker, Config: cfg})
	if err != nil {
		t.Fatalf("new orbit service: %v", err)
	}
	state := &DaemonState{config: cfg, orbitSvc: orbitSvc, orbitWindowCounts: map[string]int{"alpha": 1, "beta": 0, "gamma": 0}}
	d := &Dispatcher{state: state}

	name, err := d.selectOrbitName(ctx, orbitSvc, "next")
	if err != nil {
		t.Fatalf("select next orbit: %v", err)
	}
	if name != "beta" {
		t.Fatalf("expected next orbit beta, got %q", name)
	}

	tracker.current = "gamma"
	name, err = d.selectOrbitName(ctx, orbitSvc, "prev")
	if err != nil {
		t.Fatalf("select prev orbit: %v", err)
	}
	if name != "beta" {
		t.Fatalf("expected prev orbit beta, got %q", name)
	}

	name, err = d.selectOrbitName(ctx, orbitSvc, "index:2")
	if err != nil {
		t.Fatalf("select index orbit: %v", err)
	}
	if name != "beta" {
		t.Fatalf("expected index orbit beta, got %q", name)
	}

	name, err = d.selectOrbitName(ctx, orbitSvc, "regex:^g")
	if err != nil {
		t.Fatalf("select regex orbit: %v", err)
	}
	if name != "gamma" {
		t.Fatalf("expected regex orbit gamma, got %q", name)
	}

	name, err = d.selectOrbitName(ctx, orbitSvc, "beta")
	if err != nil {
		t.Fatalf("select explicit orbit: %v", err)
	}
	if name != "beta" {
		t.Fatalf("expected explicit orbit beta, got %q", name)
	}

	if _, err := d.selectOrbitName(ctx, orbitSvc, "regex:^z"); err == nil {
		t.Fatalf("expected regex miss to error")
	}
}

type fakeOrbitTracker struct {
	sequence []string
	current  string
}

func (f *fakeOrbitTracker) Current(context.Context) (string, error) {
	return f.current, nil
}

func (f *fakeOrbitTracker) Set(_ context.Context, name string) error {
	for _, candidate := range f.sequence {
		if candidate == name {
			f.current = name
			return nil
		}
	}
	return fmt.Errorf("unknown orbit %q", name)
}

func (f *fakeOrbitTracker) Sequence(context.Context) ([]string, error) {
	return append([]string(nil), f.sequence...), nil
}

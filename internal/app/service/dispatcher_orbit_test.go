package service

import (
	"reflect"
	"testing"

	"hyprorbit/internal/config"
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

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
	want := []string{"orbit1", "orbit2", "orbit3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered sequence when current occupied: got %v want %v", got, want)
	}

	got = d.filteredOrbitSequence(seq, "orbit4", config.OrbitCycleModeNotEmpty)
	want = []string{"orbit1", "orbit2", "orbit3", "orbit4", "orbit5"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected filtered sequence when current empty: got %v want %v", got, want)
	}
}

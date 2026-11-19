package daemon

import (
	"context"
	"reflect"
	"testing"

	"hyprorbit/internal/config"
	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/runtime"
)

type hotplugHyprStub struct {
	resetHyprStub
	dispatches []string
}

func (h *hotplugHyprStub) Dispatch(_ context.Context, args ...string) error {
	h.dispatches = append(h.dispatches, "dispatch:"+args[0]+":"+args[1])
	h.events = append(h.events, "dispatch:"+args[0]+":"+args[1])
	return nil
}

func hotplugStub(newMonitorWorkspace string) *hotplugHyprStub {
	h := &hotplugHyprStub{}
	h.monitors = []hyprctl.Monitor{
		{Name: "eDP-1", Focused: true, ActiveWorkspace: hyprctl.Workspace{Name: "alpha-code"}},
		{Name: "DP-9", ActiveWorkspace: hyprctl.Workspace{Name: newMonitorWorkspace}},
	}
	h.query = func(context.Context, int) ([]hyprctl.Workspace, error) {
		return []hyprctl.Workspace{{Name: "alpha-code"}, {Name: newMonitorWorkspace}}, nil
	}
	return h
}

func TestAlignAddedMonitorMovesNumberedWorkspace(t *testing.T) {
	h := hotplugStub("2")
	d := resetDispatcher(t, h)

	if err := d.alignAddedMonitor(context.Background(), "DP-9"); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, ev := range h.events {
		if ev != "monitors" && ev != "workspaces" {
			got = append(got, ev)
		}
	}
	want := []string{"dispatch:focusmonitor:DP-9", "switch:alpha-web", "dispatch:focusmonitor:eDP-1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events=%v want=%v", got, want)
	}
}

func TestAlignAddedMonitorSkipsOrbitWorkspace(t *testing.T) {
	h := hotplugStub("alpha-web")
	d := resetDispatcher(t, h)
	if err := d.alignAddedMonitor(context.Background(), "DP-9"); err != nil {
		t.Fatal(err)
	}
	if len(h.dispatches) != 0 {
		t.Fatalf("unexpected dispatches %v", h.dispatches)
	}
}

// hyprSim models the Hyprland behaviour that matters for alignment: switching
// to a workspace owned by another monitor focuses that monitor instead of
// moving the workspace.
type hyprSim struct {
	runtime.HyprctlClient
	order   []string
	focused string
	visible map[string]string // monitor -> visible workspace
	owner   map[string]string // workspace -> monitor
}

func newHyprSim(focused string, visible map[string]string, order ...string) *hyprSim {
	s := &hyprSim{order: order, focused: focused, visible: visible, owner: map[string]string{}}
	for mon, ws := range visible {
		s.owner[ws] = mon
	}
	return s
}

func (s *hyprSim) Monitors(context.Context) ([]hyprctl.Monitor, error) {
	out := make([]hyprctl.Monitor, 0, len(s.order))
	for _, name := range s.order {
		out = append(out, hyprctl.Monitor{Name: name, Focused: name == s.focused, ActiveWorkspace: hyprctl.Workspace{Name: s.visible[name]}})
	}
	return out, nil
}
func (s *hyprSim) Workspaces(context.Context) ([]hyprctl.Workspace, error) {
	out := make([]hyprctl.Workspace, 0, len(s.owner))
	for ws, mon := range s.owner {
		out = append(out, hyprctl.Workspace{Name: ws, Monitor: mon})
	}
	return out, nil
}
func (s *hyprSim) ActiveWorkspace(context.Context) (*hyprctl.Workspace, error) {
	return &hyprctl.Workspace{Name: s.visible[s.focused], Monitor: s.focused}, nil
}
func (s *hyprSim) Dispatch(_ context.Context, args ...string) error {
	if args[0] == "focusmonitor" {
		s.focused = args[1]
	}
	return nil
}
func (s *hyprSim) SwitchWorkspace(_ context.Context, name string) error {
	if mon, ok := s.owner[name]; ok && mon != s.focused {
		s.focused = mon
		s.visible[mon] = name
		return nil
	}
	s.owner[name] = s.focused
	s.visible[s.focused] = name
	return nil
}
func (s *hyprSim) DecodeClients(_ context.Context, out any) error {
	*out.(*[]hyprctl.ClientInfo) = nil
	return nil
}
func (s *hyprSim) InvalidateClients() {}

func TestAlignAllMonitorsAvoidsWorkspacesOwnedElsewhere(t *testing.T) {
	sim := newHyprSim("DP-10", map[string]string{"eDP-1": "alpha-web", "DP-9": "13", "DP-10": "alpha-code"}, "eDP-1", "DP-9", "DP-10")
	d := resetDispatcher(t, sim, "note")
	d.state.config.Orbit.SwitchPreference = config.OrbitSwitchPreferenceSameModuleFirst
	d.state.recordActiveModule("code", "alpha", "DP-9")

	if err := d.alignMonitorsToOrbit(context.Background(), "alpha", false, ""); err != nil {
		t.Fatal(err)
	}
	for mon, ws := range sim.visible {
		if !d.isOrbitWorkspace(ws) {
			t.Fatalf("monitor %s left on %q (visible=%v)", mon, ws, sim.visible)
		}
	}
	if sim.visible["DP-10"] != "alpha-code" {
		t.Fatalf("DP-10 should keep alpha-code, visible=%v", sim.visible)
	}
}

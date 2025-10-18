package daemon

import (
	"context"
	"testing"

	"hyprorbit/internal/config"
	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/module"
	"hyprorbit/internal/orbit"
)

// stubHyprClient satisfies runtime.HyprctlClient for targeted workspace tests.
type stubHyprClient struct {
	workspaces []hyprctl.Workspace
}

func (s *stubHyprClient) Dispatch(context.Context, ...string) error { return nil }
func (s *stubHyprClient) Clients(context.Context) ([]byte, error)   { return nil, nil }
func (s *stubHyprClient) DecodeClients(context.Context, any) error  { return nil }
func (s *stubHyprClient) InvalidateClients()                        {}
func (s *stubHyprClient) Workspaces(context.Context) ([]hyprctl.Workspace, error) {
	return s.workspaces, nil
}
func (s *stubHyprClient) ActiveWorkspace(context.Context) (*hyprctl.Workspace, error) {
	return nil, nil
}
func (s *stubHyprClient) ActiveWindow(context.Context) (*hyprctl.Window, error) { return nil, nil }
func (s *stubHyprClient) Monitors(context.Context) ([]hyprctl.Monitor, error)   { return nil, nil }
func (s *stubHyprClient) Batch(context.Context, ...[]string) ([]hyprctl.BatchResult, error) {
	return nil, nil
}
func (s *stubHyprClient) BatchDispatch(context.Context, ...[]string) ([]hyprctl.BatchResult, error) {
	return nil, nil
}
func (s *stubHyprClient) SwitchWorkspace(context.Context, string) error { return nil }
func (s *stubHyprClient) FocusWindow(context.Context, string) error     { return nil }
func (s *stubHyprClient) MoveToWorkspaceFollow(context.Context, string, string) error {
	return nil
}
func (s *stubHyprClient) MoveToWorkspaceSilent(context.Context, string, string) error {
	return nil
}

type stubOrbitAccessor struct{}

func (stubOrbitAccessor) Current(context.Context) (string, error) { return "alpha", nil }
func (stubOrbitAccessor) Record(_ context.Context, name string) (*orbit.Record, error) {
	return &orbit.Record{Name: name}, nil
}

func TestRefreshOrbitWindowCounts(t *testing.T) {
	cfg := &config.EffectiveConfig{
		Orbits: []config.OrbitRecord{{Name: "alpha"}, {Name: "beta"}},
		Modules: map[string]config.ModuleRecord{
			"dev": {Name: "dev"},
		},
	}

	hypr := &stubHyprClient{
		workspaces: []hyprctl.Workspace{
			{Name: "dev-alpha", Windows: 2},
			{Name: "dev-beta", Windows: 0},
			{Name: "scratch-beta", Windows: 1},
		},
	}

	modSvc, err := module.NewServiceWithDependencies(module.Dependencies{
		Config:  cfg,
		Orbit:   stubOrbitAccessor{},
		Hyprctl: hypr,
	})
	if err != nil {
		t.Fatalf("new module service: %v", err)
	}

	state := &DaemonState{
		moduleSvc:         modSvc,
		orbitWindowCounts: make(map[string]int),
		logger:            func(string, ...any) {},
	}
	state.mu.Lock()
	state.config = cfg
	state.mu.Unlock()

	state.refreshOrbitWindowCounts(context.Background())

	want := map[string]int{"alpha": 2, "beta": 1}
	got := state.orbitWindowCountsSnapshot()
	if len(got) != len(want) {
		t.Fatalf("window count size mismatch: got %v want %v", got, want)
	}
	for orbit, expected := range want {
		if got[orbit] != expected {
			t.Fatalf("orbit %s window count: got %d want %d", orbit, got[orbit], expected)
		}
	}

	if other := state.OrbitWindowCount("gamma"); other != 0 {
		t.Fatalf("expected gamma to have 0 windows, got %d", other)
	}
}

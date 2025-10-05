package module

import (
	"context"
	"fmt"
	"testing"

	"hyprorbit/internal/config"
	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/runtime"
)

type stubOrbitAccessor struct {
	current string
	records map[string]*orbit.Record
}

func (s *stubOrbitAccessor) Current(context.Context) (string, error) {
	return s.current, nil
}

func (s *stubOrbitAccessor) Record(_ context.Context, name string) (*orbit.Record, error) {
	record, ok := s.records[name]
	if !ok {
		return nil, fmt.Errorf("orbit %s not found", name)
	}
	return record, nil
}

type moveCall struct {
	address   string
	workspace string
}

type stubHyprctl struct {
	clients         []hyprctl.ClientInfo
	activeWorkspace hyprctl.Workspace
	focusCalls      []string
	moveCalls       []moveCall
	dispatchCalls   [][]string
	switchCalls     []string
}

func (s *stubHyprctl) Dispatch(_ context.Context, args ...string) error {
	s.dispatchCalls = append(s.dispatchCalls, append([]string(nil), args...))
	return nil
}

func (s *stubHyprctl) Clients(context.Context) ([]byte, error) {
	return nil, nil
}

func (s *stubHyprctl) DecodeClients(_ context.Context, out any) error {
	ptr, ok := out.(*[]hyprctl.ClientInfo)
	if !ok {
		return fmt.Errorf("unexpected type %T", out)
	}
	*ptr = append([]hyprctl.ClientInfo(nil), s.clients...)
	return nil
}

func (s *stubHyprctl) InvalidateClients() {}

func (s *stubHyprctl) Workspaces(context.Context) ([]hyprctl.Workspace, error) {
	return nil, nil
}

func (s *stubHyprctl) ActiveWorkspace(context.Context) (*hyprctl.Workspace, error) {
	return &s.activeWorkspace, nil
}

func (s *stubHyprctl) ActiveWindow(context.Context) (*hyprctl.Window, error) { return nil, nil }

func (s *stubHyprctl) Monitors(context.Context) ([]hyprctl.Monitor, error) { return nil, nil }

func (s *stubHyprctl) Batch(context.Context, ...[]string) ([]hyprctl.BatchResult, error) {
	return nil, nil
}

func (s *stubHyprctl) BatchDispatch(context.Context, ...[]string) ([]hyprctl.BatchResult, error) {
	return nil, nil
}

func (s *stubHyprctl) SwitchWorkspace(_ context.Context, workspace string) error {
	s.switchCalls = append(s.switchCalls, workspace)
	s.activeWorkspace.Name = workspace
	return nil
}

func (s *stubHyprctl) FocusWindow(_ context.Context, address string) error {
	s.focusCalls = append(s.focusCalls, address)
	return nil
}

func (s *stubHyprctl) MoveToWorkspace(_ context.Context, address, workspace string) error {
	s.moveCalls = append(s.moveCalls, moveCall{address: address, workspace: workspace})
	return nil
}

var _ OrbitAccessor = (*stubOrbitAccessor)(nil)
var _ runtime.HyprctlClient = (*stubHyprctl)(nil)

func newTestService(t *testing.T, cfg *config.EffectiveConfig, hypr *stubHyprctl) *Service {
	t.Helper()

	orbitName := "alpha"
	orbitStub := &stubOrbitAccessor{
		current: orbitName,
		records: map[string]*orbit.Record{
			orbitName: {Name: orbitName},
		},
	}

	svc, err := NewServiceWithDependencies(Dependencies{
		Config:  cfg,
		Orbit:   orbitStub,
		Hyprctl: hypr,
	})
	if err != nil {
		t.Fatalf("NewServiceWithDependencies: %v", err)
	}
	return svc
}

func TestFocusFirstMatchWinsStopsAfterSuccess(t *testing.T) {
	t.Parallel()

	workspace := "mod-alpha"
	hypr := &stubHyprctl{
		clients: []hyprctl.ClientInfo{
			{Address: "0x01", Class: "One", Workspace: hyprctl.WorkspaceHandle{Name: workspace}},
			{Address: "0x02", Class: "Two", Workspace: hyprctl.WorkspaceHandle{Name: "other-alpha"}},
		},
		activeWorkspace: hyprctl.Workspace{Name: workspace},
	}

	cfg := &config.EffectiveConfig{
		Orbits:   []config.OrbitRecord{{Name: "alpha"}},
		Defaults: config.ModuleSettings{Move: true},
		Modules: map[string]config.ModuleRecord{
			"mod": {
				Name: "mod",
				Focus: config.ModuleFocusSpec{
					Logic: config.ModuleFocusLogicFirstMatchWins,
					Rules: []config.ModuleFocusRuleSpec{
						{Matcher: config.Matcher{Field: "class", Expr: "^One$", Raw: "class:^One$"}},
						{Matcher: config.Matcher{Field: "class", Expr: "^Two$", Raw: "class:^Two$"}, Cmd: []string{"two"}},
					},
				},
			},
		},
	}

	svc := newTestService(t, cfg, hypr)

	res, err := svc.Focus(context.Background(), "mod", FocusOptions{})
	if err != nil {
		t.Fatalf("Focus returned error: %v", err)
	}
	if res.Action != "focused" {
		t.Fatalf("expected action focused, got %q", res.Action)
	}
	if len(hypr.focusCalls) != 1 {
		t.Fatalf("expected 1 focus call, got %d", len(hypr.focusCalls))
	}
	if len(hypr.moveCalls) != 0 {
		t.Fatalf("expected no move calls, got %d", len(hypr.moveCalls))
	}
}

func TestFocusTryAllContinuesAfterFirstSuccess(t *testing.T) {
	t.Parallel()

	workspace := "mod-alpha"
	hypr := &stubHyprctl{
		clients: []hyprctl.ClientInfo{
			{Address: "0x01", Class: "One", Workspace: hyprctl.WorkspaceHandle{Name: workspace}},
			{Address: "0x02", Class: "Two", Workspace: hyprctl.WorkspaceHandle{Name: "other-alpha"}},
		},
		activeWorkspace: hyprctl.Workspace{Name: workspace},
	}

	cfg := &config.EffectiveConfig{
		Orbits:   []config.OrbitRecord{{Name: "alpha"}},
		Defaults: config.ModuleSettings{Move: true},
		Modules: map[string]config.ModuleRecord{
			"mod": {
				Name: "mod",
				Focus: config.ModuleFocusSpec{
					Logic: config.ModuleFocusLogicTryAll,
					Rules: []config.ModuleFocusRuleSpec{
						{Matcher: config.Matcher{Field: "class", Expr: "^One$", Raw: "class:^One$"}},
						{Matcher: config.Matcher{Field: "class", Expr: "^Two$", Raw: "class:^Two$"}},
					},
				},
			},
		},
	}

	svc := newTestService(t, cfg, hypr)

	res, err := svc.Focus(context.Background(), "mod", FocusOptions{})
	if err != nil {
		t.Fatalf("Focus returned error: %v", err)
	}
	if res.Action != "focused" {
		t.Fatalf("expected action focused, got %q", res.Action)
	}
	if len(hypr.focusCalls) != 1 {
		t.Fatalf("expected 1 focus call, got %d", len(hypr.focusCalls))
	}
	if len(hypr.moveCalls) != 1 {
		t.Fatalf("expected 1 move call, got %d", len(hypr.moveCalls))
	}
	if hypr.moveCalls[0].workspace != workspace {
		t.Fatalf("expected move to %s, got %s", workspace, hypr.moveCalls[0].workspace)
	}
}

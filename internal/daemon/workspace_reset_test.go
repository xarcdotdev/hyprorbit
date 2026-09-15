package daemon

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"hyprorbit/internal/config"
	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/module"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/runtime"
)

// Any unapproved dispatch/config/focus call panics through the nil interface.
type resetHyprStub struct {
	runtime.HyprctlClient
	query              func(context.Context, int) ([]hyprctl.Workspace, error)
	calls              int
	monitors           []hyprctl.Monitor
	monitorErr         error
	events             []string
	clients            []hyprctl.ClientInfo
	moves              []string
	moveErr, switchErr error
	moveErrAt          int
	invalidations      int
}

func (h *resetHyprStub) Workspaces(ctx context.Context) ([]hyprctl.Workspace, error) {
	h.calls++
	h.events = append(h.events, "workspaces")
	return h.query(ctx, h.calls)
}
func (h *resetHyprStub) Monitors(context.Context) ([]hyprctl.Monitor, error) {
	h.events = append(h.events, "monitors")
	return h.monitors, h.monitorErr
}
func (h *resetHyprStub) ActiveWorkspace(context.Context) (*hyprctl.Workspace, error) {
	return &hyprctl.Workspace{Name: "alpha-code", Monitor: "DP-1"}, nil
}
func (h *resetHyprStub) SwitchWorkspace(_ context.Context, name string) error {
	h.events = append(h.events, "switch:"+name)
	return h.switchErr
}
func (h *resetHyprStub) DecodeClients(_ context.Context, out any) error {
	*out.(*[]hyprctl.ClientInfo) = h.clients
	return nil
}
func (h *resetHyprStub) MoveToWorkspaceSilent(_ context.Context, address, target string) error {
	h.moves = append(h.moves, address+":"+target)
	if h.moveErrAt == 0 || len(h.moves) == h.moveErrAt {
		return h.moveErr
	}
	return nil
}
func (h *resetHyprStub) InvalidateClients() { h.invalidations++ }

func resetDispatcher(t *testing.T, h *resetHyprStub) *Dispatcher {
	t.Helper()
	cfg := &config.EffectiveConfig{Modules: map[string]config.ModuleRecord{"code": {}, "web": {}}, Orbits: []config.OrbitRecord{{Name: "alpha"}}}
	os, err := orbit.NewServiceWithDependencies(orbit.Dependencies{Config: cfg, Tracker: &fakeOrbitTracker{current: "alpha", sequence: []string{"alpha"}}})
	if err != nil {
		t.Fatal(err)
	}
	ms, err := module.NewServiceWithDependencies(module.Dependencies{Config: cfg, Orbit: os, Hyprctl: h})
	if err != nil {
		t.Fatal(err)
	}
	return &Dispatcher{state: &DaemonState{config: cfg, orbitSvc: os, moduleSvc: ms, hyprctl: h}}
}

func TestWorkspaceResetPlanAndEvacuation(t *testing.T) {
	h := &resetHyprStub{query: func(context.Context, int) ([]hyprctl.Workspace, error) {
		return []hyprctl.Workspace{{Name: "alpha-code"}, {Name: " alpha-web "}, {Name: "special:scratch"}, {Name: "special"}, {Name: " "}, {Name: " beta-code "}, {Name: "alpha-1"}}, nil
	}}
	d := resetDispatcher(t, h)
	plan, err := d.buildWorkspaceResetPlan(context.Background(), h, d.state.moduleSvc, "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.targets, []string{"beta-code", "alpha-1"}) || plan.primaryWorkspace != "alpha-code" {
		t.Fatalf("plan=%+v", plan)
	}
	for i, name := range []string{"alpha-code", "alpha-web", "beta-code", "alpha-1", "special:scratch", " "} {
		h.clients = append(h.clients, hyprctl.ClientInfo{Address: []string{"0x1", "0x2", "0x3", "0x4", "0x5", "0x6"}[i], Workspace: hyprctl.WorkspaceHandle{Name: name}})
	}
	if err := d.moveClientsToPrimaryWorkspace(context.Background(), h, plan); err != nil {
		t.Fatal(err)
	}
	if want := []string{"0x2:alpha-code", "0x3:alpha-code", "0x4:alpha-code"}; !reflect.DeepEqual(h.moves, want) {
		t.Fatalf("moves=%v want=%v", h.moves, want)
	}
}

func TestWorkspaceResetAlignmentAndPartialFailure(t *testing.T) {
	for _, mode := range []string{"success", "zero targets", "alignment failure", "move failure"} {
		t.Run(mode, func(t *testing.T) {
			sentinel := errors.New("mutation failed")
			h := &resetHyprStub{monitors: []hyprctl.Monitor{{Name: "DP-1", Focused: true}}}
			h.query = func(_ context.Context, n int) ([]hyprctl.Workspace, error) {
				if n == 1 && mode != "zero targets" {
					return []hyprctl.Workspace{{Name: "alpha-1"}, {Name: "alpha-2"}, {Name: "alpha-code"}}, nil
				}
				if mode == "alignment failure" || mode == "move failure" {
					return []hyprctl.Workspace{{Name: "alpha-2"}}, nil
				}
				return nil, nil
			}
			if mode == "alignment failure" {
				h.switchErr = sentinel
			}
			if mode == "move failure" {
				h.moveErr = sentinel
				h.moveErrAt = 2
				h.clients = []hyprctl.ClientInfo{
					{Address: "0x1", Workspace: hyprctl.WorkspaceHandle{Name: "alpha-1"}},
					{Address: "0x2", Workspace: hyprctl.WorkspaceHandle{Name: "alpha-2"}},
				}
			}
			d := resetDispatcher(t, h)
			for _, m := range []string{"1", "2", "99"} {
				d.state.RegisterTempModule("alpha", m)
			}
			err := d.resetWorkspaces(context.Background())
			failed := strings.Contains(mode, "failure")
			if failed {
				if !errors.Is(err, sentinel) {
					t.Fatalf("error=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(h.events) < 3 || h.events[0] != "workspaces" || h.events[1] != "monitors" || h.events[2] != "switch:alpha-code" {
				t.Fatalf("order=%v", h.events)
			}
			if h.invalidations != 1 {
				t.Fatalf("invalidations=%d", h.invalidations)
			}
			if mode == "move failure" && len(h.moves) != 2 {
				t.Fatalf("expected success before failed move, got %v", h.moves)
			}
			want := []string{"99"}
			if failed {
				want = []string{"2", "99"}
			}
			if mode == "zero targets" {
				want = []string{"1", "2", "99"}
			}
			if got := d.state.TempModuleNames("alpha"); !reflect.DeepEqual(got, want) {
				t.Fatalf("registrations=%v want=%v", got, want)
			}
		})
	}
}

func TestWorkspaceRemovalImmediateAndDelayed(t *testing.T) {
	for _, delay := range []bool{false, true} {
		t.Run(map[bool]string{false: "immediate", true: "delayed"}[delay], func(t *testing.T) {
			h := &resetHyprStub{query: func(_ context.Context, n int) ([]hyprctl.Workspace, error) {
				if delay && n == 1 {
					return []hyprctl.Workspace{{Name: " alpha-1 "}}, nil
				}
				return []hyprctl.Workspace{{Name: "alpha-10"}, {Name: "new"}}, nil
			}}
			d := &Dispatcher{state: &DaemonState{}}
			d.state.RegisterTempModule("alpha", "1")
			if err := d.verifyWorkspaceRemoval(context.Background(), h, []string{" alpha-1 "}); err != nil {
				t.Fatal(err)
			}
			want := 1
			if delay {
				want = 2
			}
			if h.calls != want || len(d.state.TempModuleNames("alpha")) != 0 {
				t.Fatalf("calls=%d registrations=%v", h.calls, d.state.TempModuleNames("alpha"))
			}
		})
	}
}

func TestWorkspaceRemovalRetainedDiagnostics(t *testing.T) {
	h := &resetHyprStub{query: func(context.Context, int) ([]hyprctl.Workspace, error) {
		return []hyprctl.Workspace{{Name: "alpha-2"}, {Name: "alpha-3", Windows: 4}, {Name: "alpha-4"}, {Name: "unrelated"}}, nil
	}, monitors: []hyprctl.Monitor{{Name: "DP-1", Focused: true, ActiveWorkspace: hyprctl.Workspace{Name: "safe"}}, {Name: "DP-2", ActiveWorkspace: hyprctl.Workspace{Name: " alpha-2 "}}}}
	d := &Dispatcher{state: &DaemonState{}}
	for _, m := range []string{"1", "2", "3", "4"} {
		d.state.RegisterTempModule("alpha", m)
	}
	start := time.Now()
	err := d.verifyWorkspaceRemoval(context.Background(), h, []string{"alpha-1", "alpha-2", "alpha-3", "alpha-4"})
	if err == nil {
		t.Fatal("expected timeout")
	}
	for _, text := range []string{"timed out", "\"alpha-2\" (0 windows; visible on DP-2", "\"alpha-3\" (4 windows", "\"alpha-4\" (0 windows", "may be persistent or otherwise retained"} {
		if !strings.Contains(err.Error(), text) {
			t.Fatalf("missing %q in %v", text, err)
		}
	}
	if strings.Contains(err.Error(), "unrelated") || strings.Contains(err.Error(), "alpha-1") {
		t.Fatalf("unexpected targets: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond || elapsed > 2*time.Second {
		t.Fatalf("unbounded wait: %v", elapsed)
	}
	if got := d.state.TempModuleNames("alpha"); !reflect.DeepEqual(got, []string{"2", "3", "4"}) {
		t.Fatalf("registrations=%v", got)
	}
}

func TestWorkspaceRemovalMonitorError(t *testing.T) {
	sentinel := errors.New("monitor query failed")
	h := &resetHyprStub{monitorErr: sentinel}
	err := workspaceRemovalTimeout(context.Background(), h, []hyprctl.Workspace{{Name: "alpha-1", Windows: 2}})
	if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), "\"alpha-1\" (2 windows") {
		t.Fatalf("error=%v", err)
	}
}

func TestWorkspaceResetFailedMutationQueryError(t *testing.T) {
	mutationErr := errors.New("mutation failed")
	queryErr := errors.New("query failed")
	h := &resetHyprStub{query: func(context.Context, int) ([]hyprctl.Workspace, error) { return nil, queryErr }}
	d := &Dispatcher{state: &DaemonState{}}
	d.state.RegisterTempModule("alpha", "1")
	err := d.reconcileFailedWorkspaceReset(context.Background(), h, []string{"alpha-1"}, mutationErr)
	if !errors.Is(err, mutationErr) || !errors.Is(err, queryErr) || len(d.state.TempModuleNames("alpha")) != 1 {
		t.Fatalf("error=%v registrations=%v", err, d.state.TempModuleNames("alpha"))
	}
}

func TestWorkspaceRemovalErrorsAndCancellation(t *testing.T) {
	sentinel := errors.New("query failed")
	for _, mode := range []string{"query", "partial query", "cancel", "deadline", "blocking query"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			h := &resetHyprStub{query: func(ctx context.Context, n int) ([]hyprctl.Workspace, error) {
				if mode == "blocking query" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				if mode == "query" || (mode == "partial query" && n > 1) {
					return nil, sentinel
				}
				if mode == "cancel" {
					cancel()
				}
				return []hyprctl.Workspace{{Name: "alpha-2"}}, nil
			}}
			if mode == "deadline" {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, 20*time.Millisecond)
				defer stop()
			}
			d := &Dispatcher{state: &DaemonState{}}
			d.state.RegisterTempModule("alpha", "1")
			d.state.RegisterTempModule("alpha", "2")
			start := time.Now()
			err := d.verifyWorkspaceRemoval(ctx, h, []string{"alpha-1", "alpha-2"})
			want := sentinel
			if mode == "cancel" {
				want = context.Canceled
			}
			if mode == "deadline" || mode == "blocking query" {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) {
				t.Fatalf("error=%v want=%v", err, want)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatal("unbounded wait")
			}
			if mode == "partial query" && !reflect.DeepEqual(d.state.TempModuleNames("alpha"), []string{"2"}) {
				t.Fatal("partial absence not reconciled")
			}
		})
	}
}

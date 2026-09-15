package workspace

import (
	"context"
	"errors"
	"testing"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/runtime"
)

// Unimplemented methods panic, ensuring cleanup never dispatches or changes focus.
type cleanupHypr struct {
	runtime.HyprctlClient
	workspaces []hyprctl.Workspace
	err        error
	calls      int
}

func (h *cleanupHypr) Workspaces(context.Context) ([]hyprctl.Workspace, error) {
	h.calls++
	return h.workspaces, h.err
}

type cleanupState struct {
	registered bool
	workspace  string
	logs       int
}

func (s *cleanupState) RegisterTempModule(string, string) {}
func (s *cleanupState) IsTemporaryWorkspace(string) bool  { return s.registered }
func (s *cleanupState) TempModuleWorkspace(string, string) (string, bool) {
	return s.workspace, s.registered
}
func (s *cleanupState) UnregisterTempModule(string, string) { s.registered = false }
func (s *cleanupState) Logf(string, ...any)                 { s.logs++ }

func TestCleanupTemporaryObservation(t *testing.T) {
	for _, tc := range []struct {
		name, target, registeredWorkspace string
		registered                        bool
		workspaces                        []hyprctl.Workspace
		err                               error
		want                              bool
		calls                             int
	}{
		{name: "absent", target: " alpha-1 ", registeredWorkspace: "alpha-1", registered: true, workspaces: []hyprctl.Workspace{{Name: "alpha-10"}}, want: true, calls: 1},
		{name: "empty present", target: "alpha-1", registeredWorkspace: "alpha-1", registered: true, workspaces: []hyprctl.Workspace{{Name: " alpha-1 "}}, calls: 1},
		{name: "occupied", target: "alpha-1", registeredWorkspace: "alpha-1", registered: true, workspaces: []hyprctl.Workspace{{Name: "alpha-1", Windows: 2}}, calls: 1},
		{name: "query error", target: "alpha-1", registeredWorkspace: "alpha-1", registered: true, err: errors.New("query failed"), calls: 1},
		{name: "unregistered", target: "alpha-1"},
		{name: "blank", target: " ", registered: true},
		{name: "invalid", target: "invalid", registered: true},
		{name: "registration mismatch", target: "alpha-1", registeredWorkspace: "alpha-2", registered: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &cleanupHypr{workspaces: tc.workspaces, err: tc.err}
			s := &cleanupState{registered: tc.registered, workspace: tc.registeredWorkspace}
			if got := CleanupTemporary(context.Background(), h, s, tc.target); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
			if h.calls != tc.calls || s.registered != (tc.registered && !tc.want) {
				t.Fatalf("calls=%d registered=%v", h.calls, s.registered)
			}
			if tc.err != nil && s.logs == 0 {
				t.Fatal("query error not logged")
			}
		})
	}
	if CleanupTemporary(context.Background(), nil, &cleanupState{registered: true}, "alpha-1") {
		t.Fatal("nil client cleaned registration")
	}
}

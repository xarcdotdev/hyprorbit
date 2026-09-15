package module

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"hyprorbit/internal/config"
	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/runtime"
)

type monitorBatchStub struct {
	runtime.HyprctlClient
	commands [][]string
	err      error
}

func (h *monitorBatchStub) BatchDispatch(_ context.Context, commands ...[]string) ([]hyprctl.BatchResult, error) {
	h.commands = commands
	return nil, h.err
}

func TestFocusMonitorJumpBareMonitor(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "error"}[fail], func(t *testing.T) {
			sentinel := errors.New("batch failed")
			h := &monitorBatchStub{}
			if fail {
				h.err = sentinel
			}
			s, err := NewServiceWithDependencies(Dependencies{Config: &config.EffectiveConfig{Modules: map[string]config.ModuleRecord{"code": {}}}, Orbit: &stubOrbitAccessor{current: "alpha", records: map[string]*orbit.Record{"alpha": {Name: "alpha"}}}, Hyprctl: h})
			if err != nil {
				t.Fatal(err)
			}
			result, err := s.FocusMonitorJump(context.Background(), " DP-2 ", " code ")
			if fail {
				if !errors.Is(err, sentinel) || result != nil {
					t.Fatalf("result=%v error=%v", result, err)
				}
			} else if err != nil || result.Workspace != "alpha-code" {
				t.Fatalf("result=%v error=%v", result, err)
			}
			want := [][]string{{"focusmonitor", "DP-2"}, {"workspace", "name:alpha-code"}}
			if !reflect.DeepEqual(h.commands, want) {
				t.Fatalf("commands=%v want %v", h.commands, want)
			}
		})
	}
}

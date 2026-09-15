package workspace

import (
	"context"
	"fmt"
	"strings"

	"hyprorbit/internal/module"
	"hyprorbit/internal/runtime"
	"hyprorbit/internal/util"
)

const maxTemporaryWorkspace = 99

// TemporaryStateProvider abstracts temporary workspace state management.
type TemporaryStateProvider interface {
	RegisterTempModule(orbitName, moduleName string)
	IsTemporaryWorkspace(workspace string) bool
	TempModuleWorkspace(orbitName, moduleName string) (string, bool)
	UnregisterTempModule(orbitName, moduleName string)
	Logf(format string, args ...any)
}

// TemporaryCreateResult captures the outcome of temporary workspace creation.
type TemporaryCreateResult struct {
	Workspace string
	Orbit     string
	Module    string
}

// CreateTemporary creates a new temporary workspace for the active orbit.
// Returns to origin workspace if specified and different from created workspace.
func CreateTemporary(ctx context.Context, hypr runtime.HyprctlClient, state TemporaryStateProvider, orbitName, origin string) (*TemporaryCreateResult, error) {
	if util.IsEmptyOrWhitespace(orbitName) {
		return nil, fmt.Errorf("active orbit not available")
	}
	orbitName = strings.TrimSpace(orbitName)

	workspaces, err := hypr.Workspaces(ctx)
	if err != nil {
		return nil, err
	}

	existing := make(map[string]struct{}, len(workspaces))
	for _, ws := range workspaces {
		name := strings.TrimSpace(ws.Name)
		if name == "" {
			continue
		}
		existing[name] = struct{}{}
	}

	target, err := generateTemporaryName(orbitName, existing)
	if err != nil {
		return nil, err
	}

	if err := hypr.SwitchWorkspace(ctx, target); err != nil {
		return nil, err
	}

	origin = strings.TrimSpace(origin)
	if origin != "" && origin != target {
		if err := hypr.SwitchWorkspace(ctx, origin); err != nil {
			return nil, err
		}
	}

	moduleName, _, err := module.ParseWorkspaceName(target)
	if err == nil {
		state.RegisterTempModule(orbitName, moduleName)
	}

	return &TemporaryCreateResult{
		Workspace: target,
		Orbit:     orbitName,
		Module:    moduleName,
	}, nil
}

// generateTemporaryName finds the first available temporary workspace slot.
func generateTemporaryName(orbitName string, existing map[string]struct{}) (string, error) {
	var target string
	for i := 1; i <= maxTemporaryWorkspace; i++ {
		candidate := fmt.Sprintf("%s-%d", orbitName, i)
		if _, ok := existing[candidate]; ok {
			continue
		}
		target = candidate
		break
	}

	if target == "" {
		return "", fmt.Errorf("no temporary workspace slots available")
	}

	return target, nil
}

// CleanupTemporary forgets a registered temporary workspace only after observing
// that Hyprland has removed it. Present workspaces, even empty ones, are retained.
func CleanupTemporary(ctx context.Context, hypr runtime.HyprctlClient, state TemporaryStateProvider, targetWorkspace string) bool {
	targetWorkspace = strings.TrimSpace(targetWorkspace)
	if targetWorkspace == "" || hypr == nil {
		return false
	}
	if !state.IsTemporaryWorkspace(targetWorkspace) {
		return false
	}

	moduleName, orbitName, err := module.ParseWorkspaceName(targetWorkspace)
	if err != nil {
		return false
	}

	if regWorkspace, ok := state.TempModuleWorkspace(orbitName, moduleName); !ok || regWorkspace != targetWorkspace {
		return false
	}

	workspaces, err := hypr.Workspaces(ctx)
	if err != nil {
		state.Logf("temp workspace cleanup %s: %v", targetWorkspace, err)
		return false
	}
	for _, ws := range workspaces {
		if strings.TrimSpace(ws.Name) == targetWorkspace {
			return false
		}
	}
	state.UnregisterTempModule(orbitName, moduleName)
	state.Logf("temporary workspace %s disappeared; removed registration", targetWorkspace)
	return true
}

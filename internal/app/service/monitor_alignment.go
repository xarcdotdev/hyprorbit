package service

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/module"
	"hyprorbit/internal/runtime"
)

// alignMonitorsToOrbit ensures all monitors (or only the focused one) display a primary module workspace for the given orbit.
// When preferredWorkspace is provided, the focused monitor finishes on that workspace instead of the orbit's primary module.
func (d *Dispatcher) alignMonitorsToOrbit(ctx context.Context, orbitName string, onlyFocusedMonitor bool, preferredWorkspace string) error {
	if d == nil || d.state == nil {
		return nil
	}
	hypr := d.state.HyprctlClient()
	if hypr == nil {
		return nil
	}
	monitors, err := hypr.Monitors(ctx)
	if err != nil {
		return fmt.Errorf("align monitors: failed to list monitors: %w", err)
	}

	d.logMonitorSnapshot(monitors, onlyFocusedMonitor)

	if len(monitors) == 0 {
		d.debugf("alignMonitorsToOrbit: no monitors reported, skipping alignment")
		return nil
	}

	focusedIdx := d.findFocusedMonitor(monitors)
	preferredWorkspace = strings.TrimSpace(preferredWorkspace)
	usedWorkspaces := make(map[string]struct{})

	if len(monitors) <= 1 || onlyFocusedMonitor {
		workspace, err := d.alignSingleMonitor(ctx, hypr, monitors[focusedIdx], preferredWorkspace, usedWorkspaces)
		if workspace != "" {
			usedWorkspaces[workspace] = struct{}{}
		}
		return err
	}

	return d.alignAllMonitors(ctx, hypr, monitors, focusedIdx, preferredWorkspace, orbitName, usedWorkspaces)
}

func (d *Dispatcher) logMonitorSnapshot(monitors []hyprctl.Monitor, onlyFocusedMonitor bool) {
	summaries := make([]string, len(monitors))
	for i, mon := range monitors {
		summaries[i] = fmt.Sprintf("%s(focused=%v, workspace=%q)",
			mon.Name, mon.Focused, strings.TrimSpace(mon.ActiveWorkspace.Name))
	}
	d.debugf("alignMonitorsToOrbit: monitor snapshot (count=%d, focusedOnly=%v): %s",
		len(monitors), onlyFocusedMonitor, strings.Join(summaries, ", "))
}

func (d *Dispatcher) findFocusedMonitor(monitors []hyprctl.Monitor) int {
	idx := slices.IndexFunc(monitors, func(m hyprctl.Monitor) bool { return m.Focused })
	if idx == -1 {
		idx = 0
		d.debugf("alignMonitorsToOrbit: no focused monitor, defaulting to %q", monitors[0].Name)
	}
	return idx
}

func (d *Dispatcher) alignSingleMonitor(ctx context.Context, hypr runtime.HyprctlClient, monitor hyprctl.Monitor, preferredWorkspace string, used map[string]struct{}) (string, error) {
	d.debugf("alignMonitorsToOrbit: aligning focused monitor only")

	currentWorkspace := strings.TrimSpace(monitor.ActiveWorkspace.Name)
	exclude := cloneWorkspaceSet(used)

	if preferredWorkspace != "" {
		if currentWorkspace == preferredWorkspace {
			d.debugf("alignMonitorsToOrbit: monitor snapshot already on preferred workspace %q", preferredWorkspace)
			return preferredWorkspace, nil
		}
		if active, err := hypr.ActiveWorkspace(ctx); err == nil && active != nil {
			if strings.TrimSpace(active.Name) == preferredWorkspace {
				d.debugf("alignMonitorsToOrbit: active workspace already %q", preferredWorkspace)
				return preferredWorkspace, nil
			}
		}
		if err := hypr.SwitchWorkspace(ctx, preferredWorkspace); err != nil {
			return "", fmt.Errorf("align monitors: switch to %q: %w", preferredWorkspace, err)
		}
		d.state.recordWorkspaceActivation(preferredWorkspace)
		d.debugf("alignMonitorsToOrbit: aligned to preferred workspace %q", preferredWorkspace)
		return preferredWorkspace, nil
	}

	workspaceName, err := d.jumpToPrimaryModuleWorkspaceFiltered(ctx, exclude)
	if err != nil {
		return "", fmt.Errorf("align monitors: %w", err)
	}
	d.debugf("alignMonitorsToOrbit: aligned to workspace %q", workspaceName)
	return workspaceName, nil
}

func (d *Dispatcher) alignAllMonitors(ctx context.Context, hypr runtime.HyprctlClient, monitors []hyprctl.Monitor, focusedIdx int, preferredWorkspace, orbitName string, used map[string]struct{}) error {
	// Process non-focused monitors first, then focused last
	ordered := append(append([]hyprctl.Monitor{}, monitors[:focusedIdx]...), monitors[focusedIdx+1:]...)
	ordered = append(ordered, monitors[focusedIdx])

	d.logMonitorOrder(ordered)

	if used == nil {
		used = make(map[string]struct{})
	}
	preferredWorkspace = strings.TrimSpace(preferredWorkspace)

	for idx, mon := range ordered {
		isLast := idx == len(ordered)-1
		exclude := cloneWorkspaceSet(used)
		if preferredWorkspace != "" && !isLast {
			exclude[preferredWorkspace] = struct{}{}
		}
		workspaceName, err := d.alignMonitor(ctx, hypr, mon, isLast, preferredWorkspace, orbitName, exclude)
		if err != nil {
			return err
		}
		if strings.TrimSpace(workspaceName) != "" {
			used[strings.TrimSpace(workspaceName)] = struct{}{}
		}
	}
	return nil
}

func (d *Dispatcher) logMonitorOrder(monitors []hyprctl.Monitor) {
	names := make([]string, len(monitors))
	for i, mon := range monitors {
		names[i] = mon.Name
	}
	d.debugf("alignMonitorsToOrbit: monitor focus order=%s", strings.Join(names, " -> "))
}

func (d *Dispatcher) alignMonitor(ctx context.Context, hypr runtime.HyprctlClient, mon hyprctl.Monitor, isLast bool, preferredWorkspace, orbitName string, exclude map[string]struct{}) (string, error) {
	d.debugf("alignMonitorsToOrbit: focusing monitor %q for orbit %q", mon.Name, orbitName)

	if err := hypr.Dispatch(ctx, "focusmonitor", mon.Name); err != nil {
		return "", fmt.Errorf("align monitors: failed to focus monitor %q: %w", mon.Name, err)
	}

	if preferredWorkspace != "" && isLast {
		if err := d.switchToPreferred(ctx, hypr, mon.Name, preferredWorkspace); err != nil {
			return "", err
		}
		return preferredWorkspace, nil
	}

	workspaceName, err := d.jumpToPrimaryModuleWorkspaceFiltered(ctx, exclude)
	if err != nil {
		return "", fmt.Errorf("align monitors: %w", err)
	}
	d.debugf("alignMonitorsToOrbit: monitor %q now on workspace %q", mon.Name, workspaceName)
	return workspaceName, nil
}

func (d *Dispatcher) switchToPreferred(ctx context.Context, hypr runtime.HyprctlClient, monitorName, preferredWorkspace string) error {
	if ws := d.currentWorkspaceForMonitor(ctx, hypr, monitorName); ws == preferredWorkspace {
		d.debugf("alignMonitorsToOrbit: monitor %q already on preferred workspace %q", monitorName, preferredWorkspace)
		return nil
	}

	if err := hypr.SwitchWorkspace(ctx, preferredWorkspace); err != nil {
		return fmt.Errorf("align monitors: switch to %q: %w", preferredWorkspace, err)
	}
	d.state.recordWorkspaceActivation(preferredWorkspace)
	d.debugf("alignMonitorsToOrbit: monitor %q now on preferred workspace %q", monitorName, preferredWorkspace)
	return nil
}

func cloneWorkspaceSet(src map[string]struct{}) map[string]struct{} {
	dst := make(map[string]struct{}, len(src))
	for ws := range src {
		dst[ws] = struct{}{}
	}
	return dst
}

func (d *Dispatcher) currentWorkspaceForMonitor(ctx context.Context, hypr runtime.HyprctlClient, monitorName string) string {
	if hypr == nil || monitorName == "" {
		return ""
	}
	if active, err := hypr.ActiveWorkspace(ctx); err == nil && active != nil {
		if strings.TrimSpace(active.Monitor) == monitorName {
			return strings.TrimSpace(active.Name)
		}
	}
	monitors, err := hypr.Monitors(ctx)
	if err != nil {
		d.debugf("alignMonitorsToOrbit: refresh monitors for %q failed: %v", monitorName, err)
		return ""
	}
	for _, mon := range monitors {
		if mon.Name == monitorName {
			return strings.TrimSpace(mon.ActiveWorkspace.Name)
		}
	}
	return ""
}

// getActiveOrbitName retrieves the active orbit name, returning empty string on error.
func (d *Dispatcher) getActiveOrbitName(ctx context.Context, modSvc *module.Service) string {
	activeOrbit, err := modSvc.ActiveOrbit(ctx)
	if err != nil || activeOrbit == nil {
		return ""
	}
	return strings.TrimSpace(activeOrbit.Name)
}

// getCurrentModuleAndMonitor extracts the current module name and monitor from the active workspace.
func (d *Dispatcher) getCurrentModuleAndMonitor(ctx context.Context, hypr runtime.HyprctlClient) (moduleName, monitorName string) {
	ws, err := hypr.ActiveWorkspace(ctx)
	if err != nil || ws == nil {
		return "", ""
	}
	monitorName = strings.TrimSpace(ws.Monitor)
	if name := strings.TrimSpace(ws.Name); name != "" {
		if mod, _, err := module.ParseWorkspaceName(name); err == nil {
			moduleName = mod
		}
	}
	return moduleName, monitorName
}

// buildPrimaryWorkspaceCandidates returns an ordered list of candidate module names based on preferences.
func (d *Dispatcher) buildPrimaryWorkspaceCandidates(currentModule, lastActive string, preferLastActive bool) []string {
	candidates := make([]string, 0, 2)
	if preferLastActive {
		candidates = append(candidates, lastActive, currentModule)
	} else {
		candidates = append(candidates, currentModule, lastActive)
	}
	return candidates
}

// Selects and jumps to the primary module workspace for the active orbit and focused monitor.
// It chooses between the currently focused module or the last active module for this orbit, based on config, then returns the workspace name.
func (d *Dispatcher) jumpToPrimaryModuleWorkspaceFiltered(ctx context.Context, exclude map[string]struct{}) (string, error) {
	d.debugf("jumpToPrimaryModuleWorkspace: selecting primary workspace")
	modSvc, err := d.requireModuleService()
	if err != nil {
		return "", err
	}
	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return "", err
	}

	orbitName := d.getActiveOrbitName(ctx, modSvc)
	d.debugf("jumpToPrimaryModuleWorkspace: active orbit=%q", orbitName)

	currentModule, currentMonitor := d.getCurrentModuleAndMonitor(ctx, hypr)
	d.debugf("jumpToPrimaryModuleWorkspace: current module=%q monitor=%q", currentModule, currentMonitor)

	var lastActive string
	if orbitName != "" {
		lastActive = strings.TrimSpace(d.state.LastActiveModule(orbitName, currentMonitor))
		d.debugf("jumpToPrimaryModuleWorkspace: last active module=%q", lastActive)
	}

	exclude = cloneWorkspaceSet(exclude)

	preferLastActive := d.state.PreferLastActiveFirst()
	candidates := d.buildPrimaryWorkspaceCandidates(currentModule, lastActive, preferLastActive)
	d.debugf("jumpToPrimaryModuleWorkspace: candidates (ordered): %v", candidates)

	seen := make(map[string]struct{}, len(candidates))
	for i, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if _, ok := modSvc.Module(candidate); !ok {
			d.debugf("jumpToPrimaryModuleWorkspace: candidate[%d] %q not configured, skipping", i, candidate)
			continue
		}
		if exclude != nil {
			wsName := module.WorkspaceName(candidate, orbitName)
			if _, skip := exclude[wsName]; skip {
				d.debugf("jumpToPrimaryModuleWorkspace: candidate[%d] %q excluded (workspace %q)", i, candidate, wsName)
				continue
			}
		}
		d.debugf("jumpToPrimaryModuleWorkspace: jumping to candidate[%d] %q", i, candidate)
		res, err := modSvc.Jump(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("failed to jump to module %q: %w", candidate, err)
		}
		d.recordModuleResult(res)
		d.debugf("jumpToPrimaryModuleWorkspace: jumped to workspace=%q", res.Workspace)
		if exclude != nil {
			exclude[strings.TrimSpace(res.Workspace)] = struct{}{}
		}
		return strings.TrimSpace(res.Workspace), nil
	}

	d.debugf("jumpToPrimaryModuleWorkspace: no valid candidates, falling back to jumpToFirstModuleWorkspace")
	return d.jumpToFirstModuleWorkspaceWithExclusions(ctx, modSvc, exclude)
}

// jumpToFirstModuleWorkspace jumps to the first configured module workspace for the active orbit.
// This provides deterministic workspace selection, ignoring user preferences and history.
// It skips modules that are already in use by other monitors to avoid conflicts.
func (d *Dispatcher) jumpToFirstModuleWorkspaceWithExclusions(ctx context.Context, modSvc *module.Service, exclude map[string]struct{}) (string, error) {
	moduleNames := modSvc.ModuleNames()
	d.debugf("jumpToFirstModuleWorkspace: modules=%v", moduleNames)
	if len(moduleNames) == 0 {
		return "", fmt.Errorf("no modules configured")
	}

	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return "", err
	}

	orbitName := d.getActiveOrbitName(ctx, modSvc)
	d.debugf("jumpToFirstModuleWorkspace: active orbit=%q", orbitName)

	exclude = cloneWorkspaceSet(exclude)

	// Get current workspaces to identify which modules are already in use
	workspaces, err := hypr.Workspaces(ctx)
	if err != nil {
		d.debugf("jumpToFirstModuleWorkspace: failed to list workspaces, proceeding without filtering: %v", err)
		// Fall back to jumping to first module without checking
		for _, moduleName := range moduleNames {
			workspaceName := module.WorkspaceName(moduleName, orbitName)
			if exclude != nil {
				if _, skip := exclude[workspaceName]; skip {
					d.debugf("jumpToFirstModuleWorkspace: skipping %q (workspace %q excluded)", moduleName, workspaceName)
					continue
				}
			}
			result, err := modSvc.Jump(ctx, moduleName)
			if err != nil {
				return "", fmt.Errorf("failed to jump to first module %q: %w", moduleName, err)
			}
			d.recordModuleResult(result)
			d.debugf("jumpToFirstModuleWorkspace: jumped to workspace=%q", result.Workspace)
			return strings.TrimSpace(result.Workspace), nil
		}
		return "", fmt.Errorf("failed to jump to module: no available workspace")
	}

	// Build set of active workspace names
	activeWorkspaces := make(map[string]struct{}, len(workspaces))
	for _, ws := range workspaces {
		activeWorkspaces[strings.TrimSpace(ws.Name)] = struct{}{}
	}
	d.debugf("jumpToFirstModuleWorkspace: active workspaces: %v", activeWorkspaces)

	// Find first module that is not already active
	for _, moduleName := range moduleNames {
		workspaceName := module.WorkspaceName(moduleName, orbitName)
		if exclude != nil {
			if _, skip := exclude[workspaceName]; skip {
				d.debugf("jumpToFirstModuleWorkspace: skipping %q (workspace %q excluded)", moduleName, workspaceName)
				continue
			}
		}
		if _, inUse := activeWorkspaces[workspaceName]; inUse {
			d.debugf("jumpToFirstModuleWorkspace: skipping %q (workspace %q already in use)", moduleName, workspaceName)
			continue
		}

		d.debugf("jumpToFirstModuleWorkspace: attempting to jump to module %q", moduleName)
		result, err := modSvc.Jump(ctx, moduleName)
		if err != nil {
			return "", fmt.Errorf("failed to jump to module %q: %w", moduleName, err)
		}
		d.recordModuleResult(result)
		d.debugf("jumpToFirstModuleWorkspace: jumped to workspace=%q", result.Workspace)
		return strings.TrimSpace(result.Workspace), nil
	}

	// All modules are in use, fall back to first module
	d.debugf("jumpToFirstModuleWorkspace: all modules in use, falling back to first module")
	for _, moduleName := range moduleNames {
		workspaceName := module.WorkspaceName(moduleName, orbitName)
		if exclude != nil {
			if _, skip := exclude[workspaceName]; skip {
				d.debugf("jumpToFirstModuleWorkspace: cannot fall back to %q (workspace %q excluded)", moduleName, workspaceName)
				continue
			}
		}
		result, err := modSvc.Jump(ctx, moduleName)
		if err != nil {
			return "", fmt.Errorf("failed to jump to first module %q: %w", moduleName, err)
		}
		d.recordModuleResult(result)
		d.debugf("jumpToFirstModuleWorkspace: jumped to workspace=%q", result.Workspace)
		return strings.TrimSpace(result.Workspace), nil
	}
	return "", fmt.Errorf("failed to jump to module: no eligible workspace")
}

package daemon

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/module"
	"hyprorbit/internal/runtime"
	"hyprorbit/internal/window"
	"hyprorbit/internal/workspace"
)

// workspaceResetPlan describes the safe and target workspaces involved in a reset.
type workspaceResetPlan struct {
	orbitName        string
	primaryWorkspace string
	safeWorkspaces   map[string]struct{}
	targets          []string
}

// buildWorkspaceResetPlan collects the data required to safely reset workspaces.
func (d *Dispatcher) buildWorkspaceResetPlan(ctx context.Context, hypr runtime.HyprctlClient, modSvc *module.Service, orbitName string) (*workspaceResetPlan, error) {
	workspaces, err := hypr.Workspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace reset: failed to list workspaces: %w", err)
	}

	allWorkspaceNames := make([]string, 0, len(workspaces))
	for _, ws := range workspaces {
		allWorkspaceNames = append(allWorkspaceNames, ws.Name)
	}
	d.debugf("resetWorkspaces: current workspaces: %v", allWorkspaceNames)

	moduleNames := modSvc.ModuleNames()
	d.debugf("resetWorkspaces: configured modules: %v", moduleNames)
	if len(moduleNames) == 0 {
		return nil, fmt.Errorf("workspace reset: no modules configured")
	}

	primaryWorkspace := module.WorkspaceName(moduleNames[0], orbitName)
	d.debugf("resetWorkspaces: primaryWorkspace=%q", primaryWorkspace)

	safeSet := make(map[string]struct{}, len(moduleNames)+1)
	if primary := strings.TrimSpace(primaryWorkspace); primary != "" {
		safeSet[primary] = struct{}{}
	}
	for _, moduleName := range moduleNames {
		workspaceName := module.WorkspaceName(moduleName, orbitName)
		safeSet[workspaceName] = struct{}{}
	}

	safeList := make([]string, 0, len(safeSet))
	for ws := range safeSet {
		safeList = append(safeList, ws)
	}
	sort.Strings(safeList)
	d.infof("[hyprorbit] workspace reset: safe workspaces: %s", strings.Join(safeList, ", "))

	targets := make([]string, 0, len(workspaces))
	for _, ws := range workspaces {
		name := strings.TrimSpace(ws.Name)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "special") {
			continue
		}
		if _, keep := safeSet[name]; keep {
			continue
		}
		targets = append(targets, name)
	}

	if len(targets) == 0 {
		d.infof("[hyprorbit] workspace reset: no workspaces to remove")
	} else {
		d.infof("[hyprorbit] workspace reset: waiting for removal after evacuation: %s", strings.Join(targets, ", "))
	}

	return &workspaceResetPlan{
		orbitName:        orbitName,
		primaryWorkspace: primaryWorkspace,
		safeWorkspaces:   safeSet,
		targets:          targets,
	}, nil
}

// moveClientsToPrimaryWorkspace evacuates windows so empty workspaces can disappear.
func (d *Dispatcher) moveClientsToPrimaryWorkspace(ctx context.Context, hypr runtime.HyprctlClient, plan *workspaceResetPlan) error {
	d.debugf("resetWorkspaces: moving all windows to safe workspace %q before waiting for workspace removal", plan.primaryWorkspace)

	clients, err := window.DecodeClients(ctx, hypr)
	if err != nil {
		return fmt.Errorf("workspace reset: failed to get clients: %w", err)
	}

	clientsToMove := plan.collectClientsToMove(clients)
	if len(clientsToMove) == 0 {
		d.debugf("resetWorkspaces: no windows need to be moved")
		return nil
	}

	d.debugf("resetWorkspaces: moving %d windows to safe workspace", len(clientsToMove))
	if err := workspace.MoveClients(ctx, hypr, clientsToMove, plan.primaryWorkspace, false); err != nil {
		return fmt.Errorf("workspace reset: failed to move windows to safe workspace: %w", err)
	}

	d.infof("[hyprorbit] workspace reset: moved %d windows to %q", len(clientsToMove), plan.primaryWorkspace)
	return nil
}

const workspaceRemovalWait = time.Second
const workspaceRemovalPoll = 50 * time.Millisecond

// observeWorkspaceRemoval only reconciles names in the captured reset plan.
// Registrations are removed immediately on observed absence, including partial failures.
func (d *Dispatcher) observeWorkspaceRemoval(ctx context.Context, hypr runtime.HyprctlClient, targets []string) ([]hyprctl.Workspace, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	workspaces, err := hypr.Workspaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("workspace reset: failed to verify workspace removal: %w", err)
	}
	existing := make(map[string]hyprctl.Workspace, len(workspaces))
	for _, ws := range workspaces {
		ws.Name = strings.TrimSpace(ws.Name)
		existing[ws.Name] = ws
	}
	var remaining []hyprctl.Workspace
	for _, target := range targets {
		name := strings.TrimSpace(target)
		if ws, ok := existing[name]; ok {
			remaining = append(remaining, ws)
			continue
		}
		if moduleName, orbitName, err := module.ParseWorkspaceName(name); err == nil && d.state != nil {
			d.state.UnregisterTempModule(orbitName, moduleName)
		}
	}
	return remaining, nil
}

// reconcileFailedWorkspaceReset takes one read-only snapshot after a mutation
// failure. Unobserved targets retain their registrations; cancellation is respected.
func (d *Dispatcher) reconcileFailedWorkspaceReset(ctx context.Context, hypr runtime.HyprctlClient, targets []string, cause error) error {
	if len(targets) == 0 {
		return cause
	}
	queryCtx, cancel := context.WithTimeout(ctx, workspaceRemovalWait)
	defer cancel()
	_, err := d.observeWorkspaceRemoval(queryCtx, hypr, targets)
	return errors.Join(cause, err)
}

// verifyWorkspaceRemoval waits for normal empty-workspace disappearance without
// changing focus, persistence, or compositor configuration.
func (d *Dispatcher) verifyWorkspaceRemoval(ctx context.Context, hypr runtime.HyprctlClient, targets []string) error {
	if len(targets) == 0 {
		return ctx.Err()
	}
	waitCtx, cancel := context.WithTimeout(ctx, workspaceRemovalWait)
	defer cancel()
	remaining, err := d.observeWorkspaceRemoval(waitCtx, hypr, targets)
	if err != nil {
		return err
	}
	ticker := time.NewTicker(workspaceRemovalPoll)
	defer ticker.Stop()
	for len(remaining) > 0 {
		select {
		case <-waitCtx.Done():
			if err := ctx.Err(); err != nil {
				return err
			}
			return workspaceRemovalTimeout(ctx, hypr, remaining)
		case <-ticker.C:
			if waitCtx.Err() != nil {
				continue
			}
			names := make([]string, len(remaining))
			for i, ws := range remaining {
				names[i] = ws.Name
			}
			next, err := d.observeWorkspaceRemoval(waitCtx, hypr, names)
			if err != nil {
				if ctx.Err() == nil && waitCtx.Err() != nil {
					return errors.Join(workspaceRemovalTimeout(ctx, hypr, remaining), err)
				}
				return err
			}
			remaining = next
		}
	}
	return ctx.Err()
}

func workspaceRemovalTimeout(ctx context.Context, hypr runtime.HyprctlClient, remaining []hyprctl.Workspace) error {
	// Bound diagnostics too; visibility on any monitor can retain an empty workspace.
	diagnosticCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	monitors, err := hypr.Monitors(diagnosticCtx)
	visible := make(map[string][]string)
	for _, monitor := range monitors {
		name := strings.TrimSpace(monitor.ActiveWorkspace.Name)
		visible[name] = append(visible[name], monitor.Name)
	}
	details := make([]string, 0, len(remaining))
	for _, ws := range remaining {
		detail := fmt.Sprintf("%q (%d windows", ws.Name, ws.Windows)
		if names := visible[ws.Name]; len(names) > 0 {
			detail += "; visible on " + strings.Join(names, ", ")
		}
		if ws.Windows == 0 {
			detail += "; empty, may be persistent or otherwise retained"
		}
		details = append(details, detail+")")
	}
	sort.Strings(details)
	message := fmt.Sprintf("workspace reset: timed out waiting for workspace removal; remaining: %s", strings.Join(details, ", "))
	if err != nil {
		return fmt.Errorf("%s; failed to query monitor visibility: %w", message, err)
	}
	return fmt.Errorf("%s", message)
}

// collectClientsToMove filters the list of Hyprland clients that should be evacuated.
func (plan *workspaceResetPlan) collectClientsToMove(clients []hyprctl.ClientInfo) []hyprctl.ClientInfo {
	result := make([]hyprctl.ClientInfo, 0, len(clients))
	for _, client := range clients {
		sanitized := window.SanitizeClient(client)
		wsName := sanitized.WorkspaceName()
		if wsName == "" || strings.HasPrefix(wsName, "special") {
			continue
		}
		// We intentionally migrate all non-special windows to guarantee none
		// remain on the workspaces awaiting removal.
		result = append(result, sanitized)
	}
	return result
}

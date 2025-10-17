package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

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
		d.infof("[hyprorbit] workspace reset: no workspaces to kill")
	} else {
		d.infof("[hyprorbit] workspace reset: workspaces scheduled for kill: %s", strings.Join(targets, ", "))
		d.debugf("workspace reset: workspaces scheduled for kill: %s", strings.Join(targets, ", "))
	}

	return &workspaceResetPlan{
		orbitName:        orbitName,
		primaryWorkspace: primaryWorkspace,
		safeWorkspaces:   safeSet,
		targets:          targets,
	}, nil
}

// moveClientsToPrimaryWorkspace evacuates windows to the primary workspace prior to deletion.
func (d *Dispatcher) moveClientsToPrimaryWorkspace(ctx context.Context, hypr runtime.HyprctlClient, plan *workspaceResetPlan) error {
	d.debugf("resetWorkspaces: moving all windows to safe workspace %q before killing workspaces", plan.primaryWorkspace)

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

// executeWorkspaceKillCommands sends kill requests for all target workspaces.
func (d *Dispatcher) executeWorkspaceKillCommands(ctx context.Context, hypr runtime.HyprctlClient, targets []string) error {
	commands := make([][]string, 0, len(targets))
	for _, name := range targets {
		nameArg := "name:" + name
		if strings.ContainsAny(name, " \t\";") {
			nameArg = fmt.Sprintf("name:%q", name)
		}
		cmd := []string{"dispatch", "killworkspace", nameArg}
		commands = append(commands, cmd)
		d.debugf("workspace reset: queued %v", cmd)
	}

	if len(commands) == 0 {
		return nil
	}

	if _, err := hypr.Batch(ctx, commands...); err != nil {
		return fmt.Errorf("workspace reset: failed to kill %d workspace(s): %w", len(commands), err)
	}

	d.debugf("resetWorkspaces: successfully killed %d workspace(s)", len(commands))
	return nil
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
		// remain on the soon-to-be-destroyed workspaces.
		result = append(result, sanitized)
	}
	return result
}

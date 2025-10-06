package workspace

import (
	"context"
	"fmt"
	"strings"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/runtime"
)

// EnsureExists ensures a workspace exists by switching to it, then optionally switching back to origin.
func EnsureExists(ctx context.Context, hypr runtime.HyprctlClient, target, origin string) error {
	if target == "" {
		return fmt.Errorf("workspace: target name missing")
	}
	if err := hypr.SwitchWorkspace(ctx, target); err != nil {
		return err
	}
	if origin != "" && origin != target {
		if err := hypr.SwitchWorkspace(ctx, origin); err != nil {
			return err
		}
	}
	return nil
}

// MoveClients moves multiple clients to a target workspace.
// If follow is true and there's only one client, it follows the window to the destination.
// If there are multiple clients, all but the last are moved silently, and the last follows if follow is true.
func MoveClients(ctx context.Context, hypr runtime.HyprctlClient, clients []hyprctl.ClientInfo, target string, follow bool) error {
	if target == "" {
		return nil
	}
	var firstErr error
	var validClients []hyprctl.ClientInfo
	for _, client := range clients {
		if client.Address == "" {
			continue
		}
		name := strings.TrimSpace(client.Workspace.Name)
		if name == "" || name == target {
			continue
		}
		if strings.HasPrefix(name, "special") {
			continue
		}
		validClients = append(validClients, client)
	}

	for idx, client := range validClients {
		isLast := idx == len(validClients)-1
		var err error
		if isLast && follow {
			err = hypr.MoveToWorkspaceFollow(ctx, client.Address, target)
		} else {
			err = hypr.MoveToWorkspaceSilent(ctx, client.Address, target)
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// WindowCount returns the number of windows in a workspace.
func WindowCount(ctx context.Context, hypr runtime.HyprctlClient, workspace string) (int, error) {
	workspaces, err := hypr.Workspaces(ctx)
	if err != nil {
		return 0, err
	}
	for _, ws := range workspaces {
		if strings.TrimSpace(ws.Name) == workspace {
			return ws.Windows, nil
		}
	}
	return 0, nil
}

// ActiveName returns the name of the active workspace.
func ActiveName(ctx context.Context, hypr runtime.HyprctlClient) (string, error) {
	ws, err := hypr.ActiveWorkspace(ctx)
	if err != nil {
		return "", err
	}
	if ws == nil {
		return "", fmt.Errorf("active workspace not available")
	}
	name := strings.TrimSpace(ws.Name)
	if name == "" {
		return "", fmt.Errorf("active workspace name unavailable")
	}
	return name, nil
}

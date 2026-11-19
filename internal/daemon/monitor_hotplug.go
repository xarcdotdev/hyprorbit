package daemon

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/module"
	"hyprorbit/internal/runtime"
)

const (
	monitorAddedTimeout = 3 * time.Second
	monitorAddedSettle  = time.Second
	monitorAddedPoll    = 50 * time.Millisecond
)

// handleMonitorAdded is the daemon's hotplug hook. Errors are logged, not returned.
func (d *Dispatcher) handleMonitorAdded(ctx context.Context, monitorName string) {
	ctx, cancel := context.WithTimeout(ctx, monitorAddedTimeout)
	defer cancel()
	if err := d.alignAddedMonitor(ctx, monitorName); err != nil {
		d.infof("[hyprorbit] monitor %q added: %v", monitorName, err)
	}
}

// alignAddedMonitor moves a newly connected monitor off Hyprland's numbered
// default workspace onto an unused module workspace of the active orbit, then
// restores focus to the previously focused monitor.
func (d *Dispatcher) alignAddedMonitor(ctx context.Context, monitorName string) error {
	d.alignMu.Lock()
	defer d.alignMu.Unlock()

	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return err
	}
	modSvc, err := d.requireModuleService()
	if err != nil {
		return err
	}
	orbitName := d.getActiveOrbitName(ctx, modSvc)
	if orbitName == "" {
		return fmt.Errorf("active orbit not available")
	}

	monitors, err := waitForMonitorWorkspace(ctx, hypr, monitorName)
	if err != nil {
		return err
	}

	var current, previousFocus string
	exclude := make(map[string]struct{}, len(monitors))
	for _, mon := range monitors {
		ws := strings.TrimSpace(mon.ActiveWorkspace.Name)
		if mon.Name == monitorName {
			current = ws
			continue
		}
		if mon.Focused {
			previousFocus = mon.Name
		}
		if ws != "" {
			exclude[ws] = struct{}{}
		}
	}

	if d.isOrbitWorkspace(current) {
		d.debugf("alignAddedMonitor: %q already on orbit workspace %q", monitorName, current)
		return nil
	}

	d.debugf("alignAddedMonitor: %q on %q, aligning to orbit %q (exclude=%v)", monitorName, current, orbitName, exclude)
	if err := hypr.Dispatch(ctx, "focusmonitor", monitorName); err != nil {
		return fmt.Errorf("focus monitor: %w", err)
	}
	for ws := range d.workspacesOnOtherMonitors(ctx, hypr, monitorName) {
		exclude[ws] = struct{}{}
	}
	workspaceName, alignErr := d.jumpToFirstModuleWorkspaceWithExclusions(ctx, modSvc, exclude)
	if previousFocus != "" {
		if err := hypr.Dispatch(ctx, "focusmonitor", previousFocus); err != nil {
			alignErr = errors.Join(alignErr, fmt.Errorf("restore focus to %q: %w", previousFocus, err))
		}
	}
	if alignErr != nil {
		return alignErr
	}
	d.state.InvalidateClients()
	d.infof("[hyprorbit] monitor %q added: switched %q -> %q", monitorName, current, workspaceName)
	return nil
}

// isOrbitWorkspace reports whether name is an <orbit>-<module> workspace of a configured orbit.
func (d *Dispatcher) isOrbitWorkspace(name string) bool {
	_, orbitName, err := module.ParseWorkspaceName(name)
	if err != nil {
		return false
	}
	cfg := d.state.Config()
	if cfg == nil {
		return false
	}
	for _, o := range cfg.Orbits {
		if o.Name == orbitName {
			return true
		}
	}
	return false
}

// waitForMonitorWorkspace polls until the monitor is listed with an active workspace.
func waitForMonitorWorkspace(ctx context.Context, hypr runtime.HyprctlClient, monitorName string) ([]hyprctl.Monitor, error) {
	deadline := time.Now().Add(monitorAddedSettle)
	for {
		monitors, err := hypr.Monitors(ctx)
		if err != nil {
			return nil, fmt.Errorf("list monitors: %w", err)
		}
		for _, mon := range monitors {
			if mon.Name == monitorName && strings.TrimSpace(mon.ActiveWorkspace.Name) != "" {
				return monitors, nil
			}
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("monitor not reported with an active workspace")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(monitorAddedPoll):
		}
	}
}

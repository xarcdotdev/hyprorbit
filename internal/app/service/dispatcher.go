package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/ipc"
	"hyprorbit/internal/module"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/regex"
	"hyprorbit/internal/runtime"
	"hyprorbit/internal/util"
	"hyprorbit/internal/window"
)

// Dispatcher routes IPC requests to domain handlers.
type Dispatcher struct {
	state *DaemonState
}

type windowMoveResult struct {
	Window    string `json:"window"`
	Workspace string `json:"workspace"`
	Module    string `json:"module,omitempty"`
	Orbit     string `json:"orbit,omitempty"`
	Created   bool   `json:"created,omitempty"`
	Temporary bool   `json:"temporary,omitempty"`
	Focused   bool   `json:"focused"`
}

type moduleTarget struct {
	Module    string
	Workspace string
	Orbit     string
	Created   bool
	Temporary bool
}

type orbitProvider interface {
	ActiveOrbit(context.Context) (*orbit.Record, error)
}

// StreamHandler streams data back to a client over an established IPC connection.
type StreamHandler func(ctx context.Context, conn net.Conn) error

// NewDispatcher constructs a dispatcher bound to the daemon state.
func NewDispatcher(state *DaemonState) *Dispatcher {
	return &Dispatcher{state: state}
}

// Handle executes the request, returning a response suitable for IPC clients and an optional stream handler.
func (d *Dispatcher) Handle(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	if req.Version != ipc.Version {
		resp := ipc.NewResponse(false)
		resp.Error = fmt.Sprintf("unsupported protocol version %d", req.Version)
		resp.ExitCode = 1
		return resp, nil, nil
	}

	switch req.Command {
	case "orbit":
		resp, stream := d.handleOrbit(ctx, req)
		return resp, stream, nil
	case "module":
		return d.handleModule(ctx, req)
	case "window":
		return d.handleWindow(ctx, req)
	case "daemon":
		resp, stream := d.handleDaemon(ctx, req)
		return resp, stream, nil
	default:
		resp := ipc.NewResponse(false)
		resp.Error = fmt.Sprintf("unknown command %q", req.Command)
		resp.ExitCode = 2
		return resp, nil, nil
	}
}

func (d *Dispatcher) handleDaemon(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler) {
	resp := ipc.NewResponse(false)
	switch req.Action {
	case "status":
		resp.Success = true
		return resp, nil
	case "reload":
		if err := d.state.Reload(ctx); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		resp.Success = true
		d.publishSnapshot()
		return resp, nil
	default:
		resp.Error = fmt.Sprintf("unknown daemon action %q", req.Action)
		resp.ExitCode = 2
		return resp, nil
	}
}

func (d *Dispatcher) handleOrbit(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler) {
	resp := ipc.NewResponse(false)
	svc := d.state.OrbitService()
	if svc == nil {
		resp.Error = "orbit service unavailable"
		resp.ExitCode = 1
		return resp, nil
	}

	switch req.Action {
	case "get":
		name, err := svc.Current(ctx)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		record, err := svc.Record(ctx, name)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		if err := assignData(&resp, record); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		return resp, nil
	case "next":
		return d.handleOrbitStep(ctx, svc, 1)
	case "prev":
		return d.handleOrbitStep(ctx, svc, -1)
	case "list":
		summaries, err := d.state.OrbitSummaries(ctx)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		if err := assignData(&resp, summaries); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		return resp, nil
	case "set":
		if len(req.Args) != 1 {
			resp.Error = "orbit set requires exactly one argument"
			resp.ExitCode = 2
			return resp, nil
		}
		target := req.Args[0]
		record, err := svc.Record(ctx, target)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 2
			return resp, nil
		}
		if err := svc.Set(ctx, target); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		primaryWorkspace, err := d.jumpToActiveModuleWorkspace(ctx)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		if err := d.alignMonitorsToOrbit(ctx, target, primaryWorkspace); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		d.state.InvalidateClients()
		if err := assignData(&resp, record); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil
		}
		d.publishSnapshot()
		return resp, nil
	default:
		resp.Error = fmt.Sprintf("unknown orbit action %q", req.Action)
		resp.ExitCode = 2
		return resp, nil
	}
}

func (d *Dispatcher) handleOrbitStep(ctx context.Context, svc *orbit.Service, delta int) (ipc.Response, StreamHandler) {
	resp := ipc.NewResponse(false)
	seq, err := svc.Sequence(ctx)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil
	}
	if len(seq) == 0 {
		resp.Error = "orbit: no orbits configured"
		resp.ExitCode = 1
		return resp, nil
	}
	current, err := svc.Current(ctx)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil
	}
	idx := util.IndexOf(seq, current)
	if idx == -1 {
		resp.Error = fmt.Sprintf("orbit: current orbit %q not found", current)
		resp.ExitCode = 1
		return resp, nil
	}
	var nextIdx int
	if delta > 0 {
		nextIdx = (idx + 1) % len(seq)
	} else {
		nextIdx = idx - 1
		if nextIdx < 0 {
			nextIdx = len(seq) - 1
		}
	}
	name := seq[nextIdx]
	if err := svc.Set(ctx, name); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil
	}
	primaryWorkspace, err := d.jumpToActiveModuleWorkspace(ctx)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil
	}
	if err := d.alignMonitorsToOrbit(ctx, name, primaryWorkspace); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil
	}
	d.state.InvalidateClients()
	record, err := svc.Record(ctx, name)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil
	}
	if err := assignData(&resp, record); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil
	}
	d.publishSnapshot()
	return resp, nil
}

func (d *Dispatcher) handleModule(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	resp := ipc.NewResponse(false)
	svc := d.state.ModuleService()
	if svc == nil {
		resp.Error = "module service unavailable"
		resp.ExitCode = 1
		return resp, nil, nil
	}

	switch req.Action {
	case "list":
		filter, err := moduleListFilterFromFlags(req.Flags)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 2
			return resp, nil, nil
		}
		summaries, err := svc.WorkspaceSummaries(ctx)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		filtered := filterWorkspaceSummaries(summaries, filter)
		if err := assignData(&resp, filtered); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		return resp, nil, nil
	case "status-stream":
		resp.Success = true
		resp.Streaming = true
		return resp, d.streamModuleStatus(), nil
	case "workspace-reset":
		if err := d.resetWorkspaces(ctx); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		resp.Success = true
		d.publishSnapshot()
		return resp, nil, nil
	case "workspace-align":
		if err := d.alignWorkspace(ctx); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		resp.Success = true
		d.publishSnapshot()
		return resp, nil, nil
	case "get":
		return d.handleModuleGet(ctx), nil, nil
	case "jump-next":
		return d.handleModuleStep(ctx, svc, 1)
	case "jump-prev":
		return d.handleModuleStep(ctx, svc, -1)
	case "jump-create":
		return d.handleModuleCreate(ctx, svc)
	}

	if len(req.Args) == 0 {
		resp.Error = "module command requires a module name"
		resp.ExitCode = 2
		return resp, nil, nil
	}
	moduleName := req.Args[0]

	if req.Action == "jump" {
		hypr := d.state.HyprctlClient()
		originWorkspace := ""
		originWasTemp := false
		if hypr != nil {
			if ws, err := hypr.ActiveWorkspace(ctx); err == nil && ws != nil {
				originWorkspace = strings.TrimSpace(ws.Name)
				originWasTemp = d.state.IsTemporaryWorkspace(originWorkspace)
			}
		}
		var result *module.Result
		var err error
		if _, ok := svc.Module(moduleName); ok {
			result, err = svc.Jump(ctx, moduleName)
			if err != nil {
				resp.Error = err.Error()
				resp.ExitCode = 1
				return resp, nil, nil
			}
		} else {
			if hypr == nil {
				resp.Error = "hyprctl client unavailable"
				resp.ExitCode = 1
				return resp, nil, nil
			}
			record, err := svc.ActiveOrbit(ctx)
			if err != nil {
				resp.Error = err.Error()
				resp.ExitCode = 1
				return resp, nil, nil
			}
			if record == nil {
				resp.Error = "active orbit not available"
				resp.ExitCode = 1
				return resp, nil, nil
			}
			workspace := module.WorkspaceName(moduleName, record.Name)
			if err := hypr.SwitchWorkspace(ctx, workspace); err != nil {
				resp.Error = err.Error()
				resp.ExitCode = 1
				return resp, nil, nil
			}
			d.state.registerTempModule(record.Name, moduleName)
			result = &module.Result{Action: "jumped", Workspace: workspace, Orbit: record.Name}
		}

		if originWasTemp && originWorkspace != "" && result != nil && strings.TrimSpace(result.Workspace) != originWorkspace {
			d.cleanupTemporaryWorkspace(ctx, hypr, originWorkspace)
		}

		if err := assignData(&resp, result); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		d.recordModuleResult(result)
		d.publishSnapshot()
		return resp, nil, nil
	}
	if _, ok := svc.Module(moduleName); !ok {
		available := strings.Join(svc.ModuleNames(), ", ")
		resp.Error = fmt.Sprintf("module %q not configured (available: %s)", moduleName, available)
		resp.ExitCode = 2
		return resp, nil, nil
	}

	switch req.Action {
	case "focus":
		opts, err := focusOptionsFromFlags(req.Flags)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 2
			return resp, nil, nil
		}
		result, err := svc.Focus(ctx, moduleName, opts)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		if err := assignData(&resp, result); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		d.recordModuleResult(result)
		d.publishSnapshot()
		return resp, nil, nil
	case "seed":
		results, err := svc.Seed(ctx, moduleName)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		if results == nil {
			results = []*module.Result{}
		}
		if err := assignData(&resp, results); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		return resp, nil, nil
	default:
		resp.Error = fmt.Sprintf("unknown module action %q", req.Action)
		resp.ExitCode = 2
		return resp, nil, nil
	}
}

func (d *Dispatcher) handleModuleStep(ctx context.Context, svc *module.Service, delta int) (ipc.Response, StreamHandler, error) {
	resp := ipc.NewResponse(false)

	if delta == 0 {
		resp.Error = "module step: delta cannot be zero"
		resp.ExitCode = 2
		return resp, nil, nil
	}

	hypr := d.state.HyprctlClient()
	if hypr == nil {
		resp.Error = "hyprctl client unavailable"
		resp.ExitCode = 1
		return resp, nil, nil
	}

	ws, err := hypr.ActiveWorkspace(ctx)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil, nil
	}
	if ws == nil {
		resp.Error = "active workspace not available"
		resp.ExitCode = 1
		return resp, nil, nil
	}

	name := strings.TrimSpace(ws.Name)
	if name == "" {
		resp.Error = "active workspace name is empty"
		resp.ExitCode = 1
		return resp, nil, nil
	}

	moduleName, orbitName, err := module.ParseWorkspaceName(name)
	if err != nil {
		resp.Error = fmt.Sprintf("active workspace %q is not a module workspace", name)
		resp.ExitCode = 1
		return resp, nil, nil
	}
	originTemp := d.state.IsTemporaryWorkspace(name)
	if _, ok := svc.Module(moduleName); !ok {
		d.state.registerTempModule(orbitName, moduleName)
	}

	names := svc.ModuleNames()
	tempNames := d.state.TempModuleNames(orbitName)
	if len(tempNames) > 0 {
		names = util.MergeStrings(names, tempNames)
	}
	if len(names) == 0 {
		resp.Error = "no modules configured"
		resp.ExitCode = 1
		return resp, nil, nil
	}

	idx := util.IndexOf(names, moduleName)
	if idx == -1 {
		if delta > 0 {
			idx = 0
		} else {
			idx = len(names) - 1
		}
	} else {
		next := idx + delta
		next = (next%len(names) + len(names)) % len(names)
		idx = next
	}

	target := names[idx]
	var result *module.Result
	if _, ok := svc.Module(target); ok {
		result, err = svc.Jump(ctx, target)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
	} else if workspace, ok := d.state.tempModuleWorkspace(orbitName, target); ok {
		if err := hypr.SwitchWorkspace(ctx, workspace); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		result = &module.Result{Action: "jumped", Workspace: workspace, Orbit: orbitName}
	} else {
		workspace := module.WorkspaceName(target, orbitName)
		if err := hypr.SwitchWorkspace(ctx, workspace); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		result = &module.Result{Action: "jumped", Workspace: workspace, Orbit: orbitName}
	}

	if err := assignData(&resp, result); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil, nil
	}

	d.recordModuleResult(result)
	if originTemp && strings.TrimSpace(result.Workspace) != name {
		d.cleanupTemporaryWorkspace(ctx, hypr, name)
	}
	d.publishSnapshot()
	return resp, nil, nil
}

func (d *Dispatcher) handleModuleCreate(ctx context.Context, svc *module.Service) (ipc.Response, StreamHandler, error) {
	resp := ipc.NewResponse(false)

	hypr := d.state.HyprctlClient()
	if hypr == nil {
		resp.Error = "hyprctl client unavailable"
		resp.ExitCode = 1
		return resp, nil, nil
	}

	result, err := d.createModuleWorkspace(ctx, svc, hypr, "")
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil, nil
	}

	if err := assignData(&resp, result); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil, nil
	}

	d.recordModuleResult(result)
	d.publishSnapshot()
	return resp, nil, nil
}

func (d *Dispatcher) createModuleWorkspace(ctx context.Context, svc *module.Service, hypr runtime.HyprctlClient, origin string) (*module.Result, error) {
	if hypr == nil {
		return nil, fmt.Errorf("hyprctl client unavailable")
	}
	record, err := svc.ActiveOrbit(ctx)
	if err != nil {
		return nil, err
	}
	if record == nil || strings.TrimSpace(record.Name) == "" {
		return nil, fmt.Errorf("active orbit not available")
	}
	orbitName := strings.TrimSpace(record.Name)

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

	const maxTemporaryWorkspace = 99
	var target string
	for i := 1; i <= maxTemporaryWorkspace; i++ {
		candidate := fmt.Sprintf("%d-%s", i, orbitName)
		if _, ok := existing[candidate]; ok {
			continue
		}
		target = candidate
		break
	}

	if target == "" {
		return nil, fmt.Errorf("no temporary workspace slots available")
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
		d.state.registerTempModule(orbitName, moduleName)
	}

	return &module.Result{Action: "created", Workspace: target, Orbit: orbitName}, nil
}

func (d *Dispatcher) handleWindow(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	resp := ipc.NewResponse(false)
	switch req.Action {
	case "move":
		return d.handleWindowMove(ctx, req)
	default:
		resp.Error = fmt.Sprintf("unknown window action %q", req.Action)
		resp.ExitCode = 2
		return resp, nil, nil
	}
}

func (d *Dispatcher) handleWindowMove(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	resp := ipc.NewResponse(false)

	if len(req.Args) != 2 {
		resp.Error = "window move requires a window reference and target"
		resp.ExitCode = 2
		return resp, nil, nil
	}

	windowRef := strings.TrimSpace(req.Args[0])
	targetRef := strings.TrimSpace(req.Args[1])

	silent := false
	if req.Flags != nil {
		if raw, ok := req.Flags["silent"]; ok {
			val, err := util.ToBool(raw)
			if err != nil {
				resp.Error = fmt.Sprintf("window move silent flag: %v", err)
				resp.ExitCode = 2
				return resp, nil, nil
			}
			silent = val
		}
	}

	hypr := d.state.HyprctlClient()
	if hypr == nil {
		resp.Error = "hyprctl client unavailable"
		resp.ExitCode = 1
		return resp, nil, nil
	}

	modSvc := d.state.ModuleService()
	var orbitProvider orbitProvider
	if modSvc != nil {
		orbitProvider = modSvc
	}

	clients, err := d.resolveWindowSelection(ctx, hypr, orbitProvider, windowRef)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil, nil
	}
	if len(clients) == 0 {
		resp.Error = fmt.Sprintf("window selector %q matched no windows", windowRef)
		resp.ExitCode = 1
		return resp, nil, nil
	}

	if !strings.HasPrefix(strings.ToLower(targetRef), "module:") {
		resp.Error = fmt.Sprintf("window move: unsupported target %q", targetRef)
		resp.ExitCode = 2
		return resp, nil, nil
	}

	if modSvc == nil {
		resp.Error = "module service unavailable"
		resp.ExitCode = 1
		return resp, nil, nil
	}

	results := make([]windowMoveResult, 0, len(clients))
	for idx, client := range clients {
		focus := !silent && idx == len(clients)-1
		res, err := d.moveClientToModule(ctx, modSvc, hypr, client, targetRef, focus)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp, nil, nil
		}
		results = append(results, res)
	}

	var payload any
	if len(results) == 1 {
		payload = results[0]
	} else {
		payload = results
	}

	if err := assignData(&resp, payload); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp, nil, nil
	}

	d.publishSnapshot()
	return resp, nil, nil
}

func (d *Dispatcher) resolveWindowSelection(ctx context.Context, hypr runtime.HyprctlClient, orbit orbitProvider, ref string) ([]hyprctl.ClientInfo, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("window reference cannot be empty")
	}
	lower := strings.ToLower(ref)
	switch {
	case lower == "current":
		win, err := hypr.ActiveWindow(ctx)
		if err != nil {
			return nil, err
		}
		if win == nil {
			return nil, nil
		}
		client := clientInfoFromWindow(win)
		if client.Address == "" {
			return nil, nil
		}
		return []hyprctl.ClientInfo{client}, nil
	case lower == "workspace":
		workspaceName, err := activeWorkspaceName(ctx, hypr)
		if err != nil {
			return nil, err
		}
		clients, err := decodeClients(ctx, hypr)
		if err != nil {
			return nil, err
		}
		return window.FilterByScope(clients, window.ScopeWorkspace, workspaceName, ""), nil
	default:
		reference, isRegex, err := window.ParseReference(ref)
		if err != nil {
			switch {
			case errors.Is(err, regex.ErrEmptyPattern):
				return nil, fmt.Errorf("window regex reference requires a pattern")
			case errors.Is(err, regex.ErrEmptyQualifier):
				return nil, fmt.Errorf("window regex reference requires a field before the pattern")
			case errors.Is(err, regex.ErrEmptySelector):
				return nil, fmt.Errorf("window regex reference requires a pattern")
			default:
				return nil, err
			}
		}
		if !isRegex {
			return nil, fmt.Errorf("window reference %q not supported", ref)
		}
		selector := reference.Selector
		if selector.Pattern == "" {
			return nil, fmt.Errorf("window regex reference requires a pattern")
		}
		re, err := regexp.Compile(selector.Pattern)
		if err != nil {
			return nil, fmt.Errorf("window regex %q invalid: %w", selector.Pattern, err)
		}

		clients, err := decodeClients(ctx, hypr)
		if err != nil {
			return nil, err
		}

		var workspaceName string
		if reference.Scope == window.ScopeWorkspace || reference.Scope == window.ScopeOrbit {
			workspaceName, err = activeWorkspaceName(ctx, hypr)
			if err != nil {
				return nil, err
			}
		}

		var orbitName string
		if reference.Scope == window.ScopeOrbit {
			orbitName, err = activeOrbitName(ctx, orbit)
			if err != nil {
				return nil, err
			}
		}

		scoped := window.FilterByScope(clients, reference.Scope, workspaceName, orbitName)
		if len(scoped) == 0 {
			return scoped, nil
		}

		matched := make([]hyprctl.ClientInfo, 0, len(scoped))
		for _, client := range scoped {
			if window.MatchClient(re, selector, client) {
				matched = append(matched, client)
			}
		}
		return matched, nil
	}
	return nil, fmt.Errorf("window reference %q not supported", ref)
}

func activeWorkspaceName(ctx context.Context, hypr runtime.HyprctlClient) (string, error) {
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

func activeOrbitName(ctx context.Context, provider orbitProvider) (string, error) {
	if provider == nil {
		return "", fmt.Errorf("active orbit not available")
	}
	record, err := provider.ActiveOrbit(ctx)
	if err != nil {
		return "", err
	}
	if record == nil {
		return "", fmt.Errorf("active orbit not available")
	}
	name := strings.TrimSpace(record.Name)
	if name == "" {
		return "", fmt.Errorf("active orbit not available")
	}
	return name, nil
}

func decodeClients(ctx context.Context, hypr runtime.HyprctlClient) ([]hyprctl.ClientInfo, error) {
	var clients []hyprctl.ClientInfo
	if err := hypr.DecodeClients(ctx, &clients); err != nil {
		return nil, err
	}
	return clients, nil
}

func describeClient(client hyprctl.ClientInfo) string {
	title := strings.TrimSpace(client.Title)
	class := strings.TrimSpace(client.Class)
	if title != "" && class != "" {
		return fmt.Sprintf("%s (%s)", title, class)
	}
	if title != "" {
		return title
	}
	if class != "" {
		return class
	}
	if addr := strings.TrimSpace(client.Address); addr != "" {
		return addr
	}
	return "window"
}

func clientInfoFromWindow(win *hyprctl.Window) hyprctl.ClientInfo {
	if win == nil {
		return hyprctl.ClientInfo{}
	}
	info := hyprctl.ClientInfo{
		Address:      win.Address,
		Class:        win.Class,
		Title:        win.Title,
		InitialClass: win.InitialClass,
		InitialTitle: win.InitialTitle,
		Floating:     bool(win.Floating),
		Tags:         win.Tags,
		Workspace: hyprctl.WorkspaceHandle{
			Name: win.Workspace.Name,
		},
	}
	return window.SanitizeClient(info)
}
func (d *Dispatcher) handleModuleGet(ctx context.Context) ipc.Response {
	resp := ipc.NewResponse(false)
	svc := d.state.ModuleService()
	if svc == nil {
		resp.Error = "module service unavailable"
		resp.ExitCode = 1
		return resp
	}

	hyprClient := d.state.HyprctlClient()
	activeGetter, ok := hyprClient.(interface {
		ActiveWorkspace(context.Context) (*hyprctl.Workspace, error)
	})
	if !ok || activeGetter == nil {
		resp.Error = "hyprctl client does not expose active workspace"
		resp.ExitCode = 1
		return resp
	}

	ws, err := activeGetter.ActiveWorkspace(ctx)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp
	}
	if ws == nil || ws.Name == "" {
		resp.Error = "active workspace not available"
		resp.ExitCode = 1
		return resp
	}

	moduleName, orbitName, err := module.ParseWorkspaceName(ws.Name)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 2
		return resp
	}

	status, err := svc.Status(ctx, moduleName, orbitName)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 2
		return resp
	}

	if err := assignData(&resp, status); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp
	}
	return resp
}

func (d *Dispatcher) publishSnapshot() {
	if d == nil || d.state == nil {
		return
	}
	if err := d.state.PublishSnapshot(context.Background()); err != nil {
		d.state.Logf("snapshot publish: %v", err)
	}
}

func (d *Dispatcher) streamModuleStatus() StreamHandler {
	return func(ctx context.Context, conn net.Conn) error {
		if d == nil || d.state == nil {
			return fmt.Errorf("dispatcher unavailable")
		}

		streamCtx := ctx
		if streamCtx == nil {
			streamCtx = context.Background()
		}

		streamCtx, cancel := context.WithCancel(streamCtx)
		defer cancel()

		ch, unsubscribe := d.state.SubscribeSnapshots(streamCtx)
		defer unsubscribe()

		if err := d.state.PublishSnapshot(context.Background()); err != nil {
			d.state.Logf("snapshot publish: %v", err)
		}

		encoder := json.NewEncoder(conn)

		for {
			select {
			case <-streamCtx.Done():
				return streamCtx.Err()
			case snapshot, ok := <-ch:
				if !ok {
					return nil
				}
				if err := encoder.Encode(snapshot); err != nil {
					return err
				}
			}
		}
	}
}

func moduleListFilterFromFlags(flags map[string]any) (string, error) {
	if len(flags) == 0 {
		return "all", nil
	}
	raw, ok := flags["filter"]
	if !ok {
		return "all", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("module list filter must be a string")
	}
	switch value {
	case "all", "active", "inactive":
		return value, nil
	default:
		return "", fmt.Errorf("module list filter %q not supported", value)
	}
}

func filterWorkspaceSummaries(summaries []module.WorkspaceSummary, filter string) []module.WorkspaceSummary {
	if filter == "all" {
		return summaries
	}
	filtered := make([]module.WorkspaceSummary, 0, len(summaries))
	for _, summary := range summaries {
		switch filter {
		case "active":
			if (summary.Configured && summary.Exists) || (summary.Temporary && summary.Exists) {
				filtered = append(filtered, summary)
			}
		case "inactive":
			if summary.Configured && !summary.Exists {
				filtered = append(filtered, summary)
			}
		}
	}
	return filtered
}

func (d *Dispatcher) resetWorkspaces(ctx context.Context) error {
	hypr := d.state.HyprctlClient()
	if hypr == nil {
		return fmt.Errorf("hyprctl unavailable")
	}
	workspaces, err := hypr.Workspaces(ctx)
	if err != nil {
		return err
	}
	if len(workspaces) == 0 {
		return nil
	}
	commands := make([][]string, 0, len(workspaces))
	for _, ws := range workspaces {
		name := strings.TrimSpace(ws.Name)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "special") {
			continue
		}
		commands = append(commands, []string{"dispatch", "killworkspace", "name:" + name})
	}
	if _, err := hypr.Batch(ctx, commands...); err != nil {
		return fmt.Errorf("workspace reset: %w", err)
	}
	d.state.InvalidateClients()
	d.state.clearTempModules()
	return nil
}

func (d *Dispatcher) alignMonitorsToOrbit(ctx context.Context, orbitName, primaryWorkspace string) error {
	if d == nil || d.state == nil {
		return nil
	}
	hypr := d.state.HyprctlClient()
	if hypr == nil {
		return nil
	}
	monitors, err := hypr.Monitors(ctx)
	if err != nil {
		return err
	}
	if len(monitors) == 0 {
		return nil
	}
	modSvc := d.state.ModuleService()
	if modSvc == nil {
		return nil
	}

	var focusedMonitor string
	for _, mon := range monitors {
		if mon.Focused {
			focusedMonitor = mon.Name
			break
		}
	}

	for _, mon := range monitors {
		current := strings.TrimSpace(mon.ActiveWorkspace.Name)
		if current == "" {
			continue
		}
		if primaryWorkspace != "" && current == primaryWorkspace {
			continue
		}
		moduleName, _, err := module.ParseWorkspaceName(current)
		if err != nil {
			continue
		}
		moduleName = strings.TrimSpace(moduleName)
		if moduleName == "" {
			continue
		}
		if _, ok := modSvc.Module(moduleName); !ok {
			continue
		}
		target := module.WorkspaceName(moduleName, orbitName)
		if target == current {
			continue
		}
		if err := hypr.Dispatch(ctx, "focusmonitor", mon.Name); err != nil {
			return err
		}
		res, err := modSvc.Jump(ctx, moduleName)
		if err != nil {
			return err
		}
		d.recordModuleResult(res)
	}

	if focusedMonitor != "" {
		if err := hypr.Dispatch(ctx, "focusmonitor", focusedMonitor); err != nil {
			return err
		}
	}

	return nil
}

func (d *Dispatcher) jumpToActiveModuleWorkspace(ctx context.Context) (string, error) {
	modSvc := d.state.ModuleService()
	if modSvc == nil {
		return "", nil
	}
	hypr := d.state.HyprctlClient()
	if hypr == nil {
		return "", nil
	}
	activeOrbit, err := modSvc.ActiveOrbit(ctx)
	if err != nil {
		return "", err
	}
	var orbitName string
	if activeOrbit != nil {
		orbitName = strings.TrimSpace(activeOrbit.Name)
	}

	ws, err := hypr.ActiveWorkspace(ctx)
	if err != nil {
		return "", err
	}

	var currentModule string
	if ws != nil {
		name := strings.TrimSpace(ws.Name)
		if name != "" {
			if moduleName, _, err := module.ParseWorkspaceName(name); err == nil {
				currentModule = moduleName
			}
		}
	}

	var lastActive string
	if orbitName != "" {
		lastActive = strings.TrimSpace(d.state.LastActiveModule(orbitName))
	}

	preferLastActive := d.state.PreferLastActiveFirst()

	candidates := make([]string, 0, 2)
	if preferLastActive {
		candidates = append(candidates, lastActive, currentModule)
	} else {
		candidates = append(candidates, currentModule, lastActive)
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if _, ok := modSvc.Module(candidate); !ok {
			continue
		}
		res, err := modSvc.Jump(ctx, candidate)
		if err != nil {
			return "", err
		}
		d.recordModuleResult(res)
		return strings.TrimSpace(res.Workspace), nil
	}

	return "", nil
}

func (d *Dispatcher) alignWorkspace(ctx context.Context) error {
	hypr := d.state.HyprctlClient()
	if hypr == nil {
		return fmt.Errorf("hyprctl unavailable")
	}
	modSvc := d.state.ModuleService()
	if modSvc == nil {
		return fmt.Errorf("module service unavailable")
	}
	names := modSvc.ModuleNames()
	if len(names) == 0 {
		return fmt.Errorf("no modules configured")
	}
	record, err := modSvc.ActiveOrbit(ctx)
	if err != nil {
		return err
	}
	workspace := module.WorkspaceName(names[0], record.Name)

	// Move current active workspace windows into the target before switching.
	ws, err := hypr.ActiveWorkspace(ctx)
	if err == nil && ws != nil {
		if err := d.ensureWorkspaceExists(ctx, hypr, workspace, ws.Name); err != nil {
			return err
		}

		clients := d.collectClients(ctx)
		moveErr := d.moveClientsToWorkspace(ctx, hypr, clients, workspace)
		if moveErr != nil {
			return moveErr
		}
	}
	res, err := modSvc.Jump(ctx, names[0])
	if err != nil {
		return err
	}
	d.recordModuleResult(res)
	return nil
}

func (d *Dispatcher) moveClientToModule(ctx context.Context, svc *module.Service, hypr runtime.HyprctlClient, client hyprctl.ClientInfo, targetRef string, focus bool) (windowMoveResult, error) {
	var result windowMoveResult
	client = window.SanitizeClient(client)
	if client.Address == "" {
		return result, fmt.Errorf("window not available")
	}
	origin := client.Workspace.Name
	originTemp := d.state.IsTemporaryWorkspace(origin)
	target, err := d.resolveModuleTarget(ctx, svc, hypr, origin, targetRef)
	if err != nil {
		return result, err
	}

	if err := d.moveClientsToWorkspace(ctx, hypr, []hyprctl.ClientInfo{client}, target.Workspace); err != nil {
		return result, err
	}

	if focus && strings.TrimSpace(target.Workspace) != "" {
		if err := hypr.SwitchWorkspace(ctx, target.Workspace); err != nil {
			return result, err
		}
	}

	d.state.recordWorkspaceActivation(target.Workspace)

	result.Window = describeClient(client)
	result.Workspace = target.Workspace
	result.Module = target.Module
	result.Orbit = target.Orbit
	result.Created = target.Created
	result.Temporary = target.Temporary
	result.Focused = focus
	if result.Module == "" {
		if moduleName, _, err := module.ParseWorkspaceName(target.Workspace); err == nil {
			result.Module = moduleName
		}
	}
	if originTemp && origin != "" && origin != target.Workspace {
		d.cleanupTemporaryWorkspace(ctx, hypr, origin)
	}
	return result, nil
}

func (d *Dispatcher) resolveModuleTarget(ctx context.Context, svc *module.Service, hypr runtime.HyprctlClient, origin, ref string) (moduleTarget, error) {
	var target moduleTarget
	if svc == nil {
		return target, fmt.Errorf("module service unavailable")
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref)), "module:") {
		return target, fmt.Errorf("window move: unsupported module target %q", ref)
	}
	spec := strings.TrimSpace(ref[len("module:"):])
	if spec == "" {
		return target, fmt.Errorf("window move: module target missing")
	}

	origin = strings.TrimSpace(origin)

	if strings.EqualFold(spec, "create") {
		res, err := d.createModuleWorkspace(ctx, svc, hypr, origin)
		if err != nil {
			return target, err
		}
		target.Workspace = res.Workspace
		target.Orbit = res.Orbit
		target.Created = true
		if moduleName, orbit, err := module.ParseWorkspaceName(res.Workspace); err == nil {
			target.Module = moduleName
			target.Temporary = true
			d.state.registerTempModule(orbit, moduleName)
		}
		return target, nil
	}

	record, err := svc.ActiveOrbit(ctx)
	if err != nil {
		return target, err
	}
	if record == nil || strings.TrimSpace(record.Name) == "" {
		return target, fmt.Errorf("active orbit not available")
	}
	orbitName := strings.TrimSpace(record.Name)

	names := svc.ModuleNames()
	tempNames := d.state.TempModuleNames(orbitName)
	if len(tempNames) > 0 {
		names = util.MergeStrings(names, tempNames)
	}
	if len(names) == 0 {
		return target, fmt.Errorf("no modules configured")
	}
	current := d.currentModuleForOrbit(ctx, hypr, orbitName)
	moduleName, err := d.selectModuleName(names, current, spec)
	if err != nil {
		return target, err
	}
	if _, ok := svc.Module(moduleName); !ok {
		d.state.registerTempModule(orbitName, moduleName)
		target.Temporary = true
	}

	workspace := module.WorkspaceName(moduleName, orbitName)
	if err := d.ensureWorkspaceExists(ctx, hypr, workspace, origin); err != nil {
		return target, err
	}

	target.Module = moduleName
	target.Workspace = workspace
	target.Orbit = orbitName
	return target, nil
}

func (d *Dispatcher) selectModuleName(names []string, current, spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", fmt.Errorf("module target missing")
	}
	lower := strings.ToLower(spec)
	switch lower {
	case "next":
		if len(names) == 0 {
			return "", fmt.Errorf("no modules configured")
		}
		idx := util.IndexOf(names, current)
		if idx == -1 {
			idx = 0
		} else {
			idx = (idx + 1) % len(names)
		}
		return names[idx], nil
	case "prev":
		if len(names) == 0 {
			return "", fmt.Errorf("no modules configured")
		}
		idx := util.IndexOf(names, current)
		if idx == -1 {
			idx = len(names) - 1
		} else {
			idx = idx - 1
			if idx < 0 {
				idx = len(names) - 1
			}
		}
		return names[idx], nil
	}

	if strings.HasPrefix(lower, "index:") {
		value := strings.TrimSpace(spec[len("index:"):])
		idx, err := strconv.Atoi(value)
		if err != nil {
			return "", fmt.Errorf("module index %q invalid: %w", value, err)
		}
		if idx <= 0 {
			return "", fmt.Errorf("module index must be >= 1")
		}
		idx--
		if idx < 0 || idx >= len(names) {
			return "", fmt.Errorf("module index %d out of range", idx+1)
		}
		return names[idx], nil
	}

	if strings.HasPrefix(lower, "regex:") {
		pattern := spec[len("regex:"):]
		re, err := regexp.Compile(pattern)
		if err != nil {
			return "", fmt.Errorf("module regex %q invalid: %w", pattern, err)
		}
		for _, name := range names {
			if re.MatchString(name) {
				return name, nil
			}
		}
		return "", fmt.Errorf("module regex %q matched no modules", pattern)
	}

	for _, name := range names {
		if name == spec {
			return name, nil
		}
	}

	return "", fmt.Errorf("module %q not configured", spec)
}

func (d *Dispatcher) currentModuleForOrbit(ctx context.Context, hypr runtime.HyprctlClient, orbitName string) string {
	orbitName = strings.TrimSpace(orbitName)
	if hypr != nil {
		if ws, err := hypr.ActiveWorkspace(ctx); err == nil && ws != nil {
			name := strings.TrimSpace(ws.Name)
			if name != "" {
				if moduleName, orbit, err := module.ParseWorkspaceName(name); err == nil && orbit == orbitName {
					return moduleName
				}
			}
		}
	}
	if d.state == nil {
		return ""
	}
	return strings.TrimSpace(d.state.LastActiveModule(orbitName))
}

func (d *Dispatcher) recordModuleResult(result *module.Result) {
	if d == nil || d.state == nil || result == nil {
		return
	}
	if result.Workspace == "" {
		return
	}
	d.state.recordWorkspaceActivation(result.Workspace)
}

func (d *Dispatcher) ensureWorkspaceExists(ctx context.Context, hypr runtime.HyprctlClient, target, origin string) error {
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

func (d *Dispatcher) moveClientsToWorkspace(ctx context.Context, hypr runtime.HyprctlClient, clients []hyprctl.ClientInfo, target string) error {
	if target == "" {
		return nil
	}
	var firstErr error
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
		if err := hypr.MoveToWorkspace(ctx, client.Address, target); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (d *Dispatcher) collectClients(ctx context.Context) []hyprctl.ClientInfo {
	hypr := d.state.HyprctlClient()
	if hypr == nil {
		return nil
	}
	var clients []hyprctl.ClientInfo
	if err := hypr.DecodeClients(ctx, &clients); err != nil {
		return nil
	}
	return clients
}

func focusOptionsFromFlags(flags map[string]any) (module.FocusOptions, error) {
	var opts module.FocusOptions
	if len(flags) == 0 {
		return opts, nil
	}
	if matcher, ok := flags["matcher"]; ok {
		str, ok := matcher.(string)
		if !ok {
			return opts, fmt.Errorf("module focus matcher must be a string")
		}
		opts.MatcherOverride = str
	}
	if cmd, ok := flags["cmd"]; ok {
		switch v := cmd.(type) {
		case []any:
			for _, raw := range v {
				str, ok := raw.(string)
				if !ok {
					return opts, fmt.Errorf("module focus cmd entries must be strings")
				}
				opts.CmdOverride = append(opts.CmdOverride, str)
			}
		case []string:
			opts.CmdOverride = append(opts.CmdOverride, v...)
		default:
			return opts, fmt.Errorf("module focus cmd must be an array of strings")
		}
	}
	if force, ok := flags["force_float"]; ok {
		b, err := util.ToBool(force)
		if err != nil {
			return opts, fmt.Errorf("module focus force_float must be boolean")
		}
		opts.ForceFloat = b
	}
	if noMove, ok := flags["no_move"]; ok {
		b, err := util.ToBool(noMove)
		if err != nil {
			return opts, fmt.Errorf("module focus no_move must be boolean")
		}
		opts.NoMove = b
	}
	return opts, nil
}

func assignData(resp *ipc.Response, value any) error {
	if value == nil {
		resp.Success = true
		resp.Data = nil
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	resp.Data = data
	resp.Success = true
	return nil
}

func (d *Dispatcher) cleanupTemporaryWorkspace(ctx context.Context, hypr runtime.HyprctlClient, workspace string) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" || hypr == nil {
		return
	}
	if !d.state.IsTemporaryWorkspace(workspace) {
		return
	}
	moduleName, orbitName, err := module.ParseWorkspaceName(workspace)
	if err != nil {
		return
	}
	if regWorkspace, ok := d.state.tempModuleWorkspace(orbitName, moduleName); !ok || regWorkspace != workspace {
		return
	}

	if ws, err := hypr.ActiveWorkspace(ctx); err == nil && ws != nil {
		if strings.TrimSpace(ws.Name) == workspace {
			return
		}
	}

	count, err := d.workspaceWindowCount(ctx, hypr, workspace)
	if err != nil {
		d.state.Logf("temp workspace cleanup %s: %v", workspace, err)
		return
	}
	if count > 0 {
		return
	}

	if err := hypr.Dispatch(ctx, "dispatch", "killworkspace", "name:"+workspace); err != nil {
		d.state.Logf("temp workspace kill %s: %v", workspace, err)
		return
	}
	d.state.unregisterTempModule(orbitName, moduleName)
	d.state.InvalidateClients()
	d.state.Logf("removed temporary workspace %s", workspace)
}

func (d *Dispatcher) workspaceWindowCount(ctx context.Context, hypr runtime.HyprctlClient, workspace string) (int, error) {
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

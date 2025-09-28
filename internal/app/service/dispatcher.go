package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"hypr-orbits/internal/hyprctl"
	"hypr-orbits/internal/ipc"
	"hypr-orbits/internal/module"
	"hypr-orbits/internal/orbit"
	"hypr-orbits/internal/runtime"
)

// Dispatcher routes IPC requests to domain handlers.
type Dispatcher struct {
	state *DaemonState
}

// NewDispatcher constructs a dispatcher bound to the daemon state.
func NewDispatcher(state *DaemonState) *Dispatcher {
	return &Dispatcher{state: state}
}

// Handle executes the request, returning a response suitable for IPC clients.
func (d *Dispatcher) Handle(ctx context.Context, req ipc.Request) (ipc.Response, error) {
	if req.Version != ipc.Version {
		resp := ipc.NewResponse(false)
		resp.Error = fmt.Sprintf("unsupported protocol version %d", req.Version)
		resp.ExitCode = 1
		return resp, nil
	}

	switch req.Command {
	case "orbit":
		return d.handleOrbit(ctx, req), nil
	case "module":
		return d.handleModule(ctx, req), nil
	case "daemon":
		return d.handleDaemon(ctx, req), nil
	default:
		resp := ipc.NewResponse(false)
		resp.Error = fmt.Sprintf("unknown command %q", req.Command)
		resp.ExitCode = 2
		return resp, nil
	}
}

func (d *Dispatcher) handleDaemon(ctx context.Context, req ipc.Request) ipc.Response {
	resp := ipc.NewResponse(false)
	switch req.Action {
	case "status":
		resp.Success = true
		return resp
	case "reload":
		if err := d.state.Reload(ctx); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		resp.Success = true
		return resp
	default:
		resp.Error = fmt.Sprintf("unknown daemon action %q", req.Action)
		resp.ExitCode = 2
		return resp
	}
}

func (d *Dispatcher) handleOrbit(ctx context.Context, req ipc.Request) ipc.Response {
	resp := ipc.NewResponse(false)
	svc := d.state.OrbitService()
	if svc == nil {
		resp.Error = "orbit service unavailable"
		resp.ExitCode = 1
		return resp
	}

	switch req.Action {
	case "get":
		name, err := svc.Current(ctx)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		record, err := svc.Record(ctx, name)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		if err := assignData(&resp, record); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		return resp
	case "next":
		return d.handleOrbitStep(ctx, svc, 1)
	case "prev":
		return d.handleOrbitStep(ctx, svc, -1)
	case "set":
		if len(req.Args) != 1 {
			resp.Error = "orbit set requires exactly one argument"
			resp.ExitCode = 2
			return resp
		}
		target := req.Args[0]
		record, err := svc.Record(ctx, target)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 2
			return resp
		}
		if err := svc.Set(ctx, target); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		d.state.InvalidateClients()
		if err := assignData(&resp, record); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		return resp
	default:
		resp.Error = fmt.Sprintf("unknown orbit action %q", req.Action)
		resp.ExitCode = 2
		return resp
	}
}

func (d *Dispatcher) handleOrbitStep(ctx context.Context, svc *orbit.Service, delta int) ipc.Response {
	resp := ipc.NewResponse(false)
	seq, err := svc.Sequence(ctx)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp
	}
	if len(seq) == 0 {
		resp.Error = "orbit: no orbits configured"
		resp.ExitCode = 1
		return resp
	}
	current, err := svc.Current(ctx)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp
	}
	idx := indexOf(seq, current)
	if idx == -1 {
		resp.Error = fmt.Sprintf("orbit: current orbit %q not found", current)
		resp.ExitCode = 1
		return resp
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
		return resp
	}
	d.state.InvalidateClients()
	record, err := svc.Record(ctx, name)
	if err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp
	}
	if err := assignData(&resp, record); err != nil {
		resp.Error = err.Error()
		resp.ExitCode = 1
		return resp
	}
	return resp
}

func (d *Dispatcher) handleModule(ctx context.Context, req ipc.Request) ipc.Response {
	resp := ipc.NewResponse(false)
	svc := d.state.ModuleService()
	if svc == nil {
		resp.Error = "module service unavailable"
		resp.ExitCode = 1
		return resp
	}

	switch req.Action {
	case "list":
		filter, err := listFilterFromFlags(req.Flags)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 2
			return resp
		}
		summaries, err := svc.WorkspaceSummaries(ctx)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		filtered := filterWorkspaceSummaries(summaries, filter)
		if err := assignData(&resp, filtered); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		return resp
	case "workspace-reset":
		if err := d.resetWorkspaces(ctx); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		resp.Success = true
		return resp
	case "workspace-align":
		if err := d.alignWorkspace(ctx); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		resp.Success = true
		return resp
	case "get":
		return d.handleModuleGet(ctx)
	}

	if len(req.Args) == 0 {
		resp.Error = "module command requires a module name"
		resp.ExitCode = 2
		return resp
	}
	moduleName := req.Args[0]
	if _, ok := svc.Module(moduleName); !ok {
		available := strings.Join(svc.ModuleNames(), ", ")
		resp.Error = fmt.Sprintf("module %q not configured (available: %s)", moduleName, available)
		resp.ExitCode = 2
		return resp
	}

	switch req.Action {
	case "focus":
		opts, err := focusOptionsFromFlags(req.Flags)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 2
			return resp
		}
		result, err := svc.Focus(ctx, moduleName, opts)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		if err := assignData(&resp, result); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		return resp
	case "jump":
		result, err := svc.Jump(ctx, moduleName)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		if err := assignData(&resp, result); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		return resp
	case "seed":
		results, err := svc.Seed(ctx, moduleName)
		if err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		if results == nil {
			results = []*module.Result{}
		}
		if err := assignData(&resp, results); err != nil {
			resp.Error = err.Error()
			resp.ExitCode = 1
			return resp
		}
		return resp
	default:
		resp.Error = fmt.Sprintf("unknown module action %q", req.Action)
		resp.ExitCode = 2
		return resp
	}
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

func listFilterFromFlags(flags map[string]any) (string, error) {
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
			if summary.Configured && summary.Exists {
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
	return nil
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
		moveErr := d.moveClientsToWorkspace(ctx, hypr, clients, ws.Name, workspace)
		if moveErr != nil {
			return moveErr
		}
	}
	return hypr.SwitchWorkspace(ctx, workspace)
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

func (d *Dispatcher) moveClientsToWorkspace(ctx context.Context, hypr runtime.HyprctlClient, clients []hyprctl.ClientInfo, origin, target string) error {
	if origin == "" || target == "" {
		return nil
	}
	var firstErr error
	for _, client := range clients {
		if client.Address == "" {
			continue
		}
		if client.Workspace.Name != origin {
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
		b, err := toBool(force)
		if err != nil {
			return opts, fmt.Errorf("module focus force_float must be boolean")
		}
		opts.ForceFloat = b
	}
	if noMove, ok := flags["no_move"]; ok {
		b, err := toBool(noMove)
		if err != nil {
			return opts, fmt.Errorf("module focus no_move must be boolean")
		}
		opts.NoMove = b
	}
	return opts, nil
}

func toBool(v any) (bool, error) {
	switch b := v.(type) {
	case bool:
		return b, nil
	case string:
		if b == "true" {
			return true, nil
		}
		if b == "false" {
			return false, nil
		}
		return false, fmt.Errorf("invalid boolean string %q", b)
	default:
		return false, fmt.Errorf("invalid boolean type %T", v)
	}
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

func indexOf(values []string, needle string) int {
	for i, v := range values {
		if v == needle {
			return i
		}
	}
	return -1
}

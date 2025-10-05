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
	"hyprorbit/internal/workspace"
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


// StreamHandler streams data back to a client over an established IPC connection.
type StreamHandler func(ctx context.Context, conn net.Conn) error

// NewDispatcher constructs a dispatcher bound to the daemon state.
func NewDispatcher(state *DaemonState) *Dispatcher {
	return &Dispatcher{state: state}
}

// errorResponse creates a failed response with the given error message and exit code.
func errorResponse(msg string, exitCode int) ipc.Response {
	resp := ipc.NewResponse(false)
	resp.Error = msg
	resp.ExitCode = exitCode
	return resp
}

// requireOrbitService returns the orbit service or an error if unavailable.
func (d *Dispatcher) requireOrbitService() (*orbit.Service, error) {
	svc := d.state.OrbitService()
	if svc == nil {
		return nil, fmt.Errorf("orbit service unavailable")
	}
	return svc, nil
}

// requireModuleService returns the module service or an error if unavailable.
func (d *Dispatcher) requireModuleService() (*module.Service, error) {
	svc := d.state.ModuleService()
	if svc == nil {
		return nil, fmt.Errorf("module service unavailable")
	}
	return svc, nil
}

// requireHyprctlClient returns the hyprctl client or an error if unavailable.
func (d *Dispatcher) requireHyprctlClient() (runtime.HyprctlClient, error) {
	hypr := d.state.HyprctlClient()
	if hypr == nil {
		return nil, fmt.Errorf("hyprctl client unavailable")
	}
	return hypr, nil
}

// successResponse creates a successful response, optionally assigns data, and publishes a snapshot.
func (d *Dispatcher) successResponse(data any) (ipc.Response, StreamHandler, error) {
	resp := ipc.NewResponse(false)
	if data != nil {
		if err := ipc.AssignData(&resp, data); err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
	}
	d.publishSnapshot()
	return resp, nil, nil
}

// successResponseWithModuleResult records module result and returns success response with data.
func (d *Dispatcher) successResponseWithModuleResult(result *module.Result) (ipc.Response, StreamHandler, error) {
	resp := ipc.NewResponse(false)
	if err := ipc.AssignData(&resp, result); err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}
	d.recordModuleResult(result)
	d.publishSnapshot()
	return resp, nil, nil
}

// Handle executes the request, returning a response suitable for IPC clients and an optional stream handler.
func (d *Dispatcher) Handle(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	if req.Version != ipc.Version {
		return errorResponse(fmt.Sprintf("unsupported protocol version %d", req.Version), 1), nil, nil
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
		return errorResponse(fmt.Sprintf("unknown command %q", req.Command), 2), nil, nil
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
			return errorResponse(err.Error(), 1), nil
		}
		resp.Success = true
		d.publishSnapshot()
		return resp, nil
	default:
		return errorResponse(fmt.Sprintf("unknown daemon action %q", req.Action), 2), nil
	}
}

func (d *Dispatcher) handleOrbit(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler) {
	resp := ipc.NewResponse(false)
	svc, err := d.requireOrbitService()
	if err != nil {
		return errorResponse(err.Error(), 1), nil
	}

	switch req.Action {
	case "get":
		name, err := svc.Current(ctx)
		if err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		record, err := svc.Record(ctx, name)
		if err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		if err := ipc.AssignData(&resp, record); err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		return resp, nil
	case "next":
		return d.handleOrbitStep(ctx, svc, 1)
	case "prev":
		return d.handleOrbitStep(ctx, svc, -1)
	case "list":
		summaries, err := d.state.OrbitSummaries(ctx)
		if err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		if err := ipc.AssignData(&resp, summaries); err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		return resp, nil
	case "set":
		if len(req.Args) != 1 {
			return errorResponse("orbit set requires exactly one argument", 2), nil
		}
		target := req.Args[0]
		record, err := svc.Record(ctx, target)
		if err != nil {
			return errorResponse(err.Error(), 2), nil
		}
		if err := svc.Set(ctx, target); err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		primaryWorkspace, err := d.jumpToActiveModuleWorkspace(ctx)
		if err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		if err := d.alignMonitorsToOrbit(ctx, target, primaryWorkspace); err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		d.state.InvalidateClients()
		if err := ipc.AssignData(&resp, record); err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		d.publishSnapshot()
		return resp, nil
	default:
		return errorResponse(fmt.Sprintf("unknown orbit action %q", req.Action), 2), nil
	}
}

func (d *Dispatcher) handleOrbitStep(ctx context.Context, svc *orbit.Service, delta int) (ipc.Response, StreamHandler) {
	resp := ipc.NewResponse(false)
	seq, err := svc.Sequence(ctx)
	if err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	if len(seq) == 0 {
		return errorResponse("orbit: no orbits configured", 1), nil
	}
	current, err := svc.Current(ctx)
	if err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	idx := util.IndexOf(seq, current)
	if idx == -1 {
		return errorResponse(fmt.Sprintf("orbit: current orbit %q not found", current), 1), nil
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
		return errorResponse(err.Error(), 1), nil
	}
	primaryWorkspace, err := d.jumpToActiveModuleWorkspace(ctx)
	if err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	if err := d.alignMonitorsToOrbit(ctx, name, primaryWorkspace); err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	d.state.InvalidateClients()
	record, err := svc.Record(ctx, name)
	if err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	if err := ipc.AssignData(&resp, record); err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	d.publishSnapshot()
	return resp, nil
}

func (d *Dispatcher) handleModule(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	resp := ipc.NewResponse(false)
	svc, err := d.requireModuleService()
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	switch req.Action {
	case "list":
		filter, err := moduleListFilterFromFlags(req.Flags)
		if err != nil {
			return errorResponse(err.Error(), 2), nil, nil
		}
		summaries, err := svc.WorkspaceSummaries(ctx)
		if err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		filtered := filterWorkspaceSummaries(summaries, filter)
		if err := ipc.AssignData(&resp, filtered); err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		return resp, nil, nil
	case "status-stream":
		resp.Success = true
		resp.Streaming = true
		return resp, d.streamModuleStatus(), nil
	case "workspace-reset":
		if err := d.resetWorkspaces(ctx); err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		return d.successResponse(nil)
	case "workspace-align":
		if err := d.alignWorkspace(ctx); err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		return d.successResponse(nil)
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
		return errorResponse("module command requires a module name", 2), nil, nil
	}
	moduleName := req.Args[0]

	if req.Action == "jump" {
		return d.handleModuleJump(ctx, svc, moduleName)
	}

	if _, ok := svc.Module(moduleName); !ok {
		available := strings.Join(svc.ModuleNames(), ", ")
		return errorResponse(fmt.Sprintf("module %q not configured (available: %s)", moduleName, available), 2), nil, nil
	}

	switch req.Action {
	case "focus":
		opts, err := focusOptionsFromFlags(req.Flags)
		if err != nil {
			return errorResponse(err.Error(), 2), nil, nil
		}
		result, err := svc.Focus(ctx, moduleName, opts)
		if err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		return d.successResponseWithModuleResult(result)
	case "seed":
		results, err := svc.Seed(ctx, moduleName)
		if err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		if results == nil {
			results = []*module.Result{}
		}
		return d.successResponse(results)
	default:
		return errorResponse(fmt.Sprintf("unknown module action %q", req.Action), 2), nil, nil
	}
}

func (d *Dispatcher) handleModuleJump(ctx context.Context, svc *module.Service, moduleName string) (ipc.Response, StreamHandler, error) {
	hypr := d.state.HyprctlClient()
	originWorkspace := ""
	originWasTemp := false
	if hypr != nil {
		if name, err := workspace.ActiveName(ctx, hypr); err == nil {
			originWorkspace = name
			originWasTemp = d.state.IsTemporaryWorkspace(originWorkspace)
		}
	}
	var result *module.Result
	var err error
	if _, ok := svc.Module(moduleName); ok {
		result, err = svc.Jump(ctx, moduleName)
		if err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
	} else {
		if hypr == nil {
			return errorResponse("hyprctl client unavailable", 1), nil, nil
		}
		record, err := svc.ActiveOrbit(ctx)
		if err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		if record == nil {
			return errorResponse("active orbit not available", 1), nil, nil
		}
		workspace := module.WorkspaceName(moduleName, record.Name)
		if err := hypr.SwitchWorkspace(ctx, workspace); err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		d.state.registerTempModule(record.Name, moduleName)
		result = &module.Result{Action: "jumped", Workspace: workspace, Orbit: record.Name}
	}

	if originWasTemp && originWorkspace != "" && result != nil && strings.TrimSpace(result.Workspace) != originWorkspace {
		d.cleanupTemporaryWorkspace(ctx, hypr, originWorkspace)
	}

	return d.successResponseWithModuleResult(result)
}

func (d *Dispatcher) handleModuleStep(ctx context.Context, svc *module.Service, delta int) (ipc.Response, StreamHandler, error) {
	resp := ipc.NewResponse(false)

	if delta == 0 {
		return errorResponse("module step: delta cannot be zero", 2), nil, nil
	}

	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	name, err := workspace.ActiveName(ctx, hypr)
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	moduleName, orbitName, err := module.ParseWorkspaceName(name)
	if err != nil {
		return errorResponse(fmt.Sprintf("active workspace %q is not a module workspace", name), 1), nil, nil
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
		return errorResponse("no modules configured", 1), nil, nil
	}

	target := util.CyclicIndex(names, moduleName, delta)
	var result *module.Result
	if _, ok := svc.Module(target); ok {
		result, err = svc.Jump(ctx, target)
		if err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
	} else if workspace, ok := d.state.tempModuleWorkspace(orbitName, target); ok {
		if err := hypr.SwitchWorkspace(ctx, workspace); err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		result = &module.Result{Action: "jumped", Workspace: workspace, Orbit: orbitName}
	} else {
		workspace := module.WorkspaceName(target, orbitName)
		if err := hypr.SwitchWorkspace(ctx, workspace); err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		result = &module.Result{Action: "jumped", Workspace: workspace, Orbit: orbitName}
	}

	if err := ipc.AssignData(&resp, result); err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	d.recordModuleResult(result)
	if originTemp && strings.TrimSpace(result.Workspace) != name {
		d.cleanupTemporaryWorkspace(ctx, hypr, name)
	}
	d.publishSnapshot()
	return resp, nil, nil
}

func (d *Dispatcher) handleModuleCreate(ctx context.Context, svc *module.Service) (ipc.Response, StreamHandler, error) {
	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	result, err := d.createModuleWorkspace(ctx, svc, hypr, "")
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	return d.successResponseWithModuleResult(result)
}

// *********************************************
// Helper functions
// *********************************************

func (d *Dispatcher) createModuleWorkspace(ctx context.Context, svc *module.Service, hypr runtime.HyprctlClient, origin string) (*module.Result, error) {
	record, err := svc.ActiveOrbit(ctx)
	if err != nil {
		return nil, err
	}
	if record == nil || util.IsEmptyOrWhitespace(record.Name) {
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
	switch req.Action {
	case "move":
		return d.handleWindowMove(ctx, req)
	default:
		return errorResponse(fmt.Sprintf("unknown window action %q", req.Action), 2), nil, nil
	}
}

func (d *Dispatcher) handleWindowMove(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	if len(req.Args) != 2 {
		return errorResponse("window move requires a window reference and target", 2), nil, nil
	}

	windowRef := strings.TrimSpace(req.Args[0])
	targetRef := strings.TrimSpace(req.Args[1])

	silent, err := parseSilentFlag(req.Flags)
	if err != nil {
		return errorResponse(err.Error(), 2), nil, nil
	}

	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	modSvc, err := d.requireModuleService()
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	clients, err := d.resolveWindowsForMove(ctx, hypr, modSvc, windowRef)
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	if err := validateMoveTarget(targetRef); err != nil {
		return errorResponse(err.Error(), 2), nil, nil
	}

	results, err := d.moveClientsToTarget(ctx, modSvc, hypr, clients, targetRef, silent)
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	return d.successResponse(formatMoveResults(results))
}

// resolveWindowsForMove resolves window references and validates the selection.
func (d *Dispatcher) resolveWindowsForMove(ctx context.Context, hypr runtime.HyprctlClient, modSvc *module.Service, windowRef string) ([]hyprctl.ClientInfo, error) {
	var orbitProvider orbit.Provider
	if modSvc != nil {
		orbitProvider = modSvc
	}

	clients, err := d.resolveWindowSelection(ctx, hypr, orbitProvider, windowRef)
	if err != nil {
		return nil, err
	}
	if len(clients) == 0 {
		return nil, fmt.Errorf("window selector %q matched no windows", windowRef)
	}
	return clients, nil
}

// validateMoveTarget checks if the target reference is valid.
func validateMoveTarget(targetRef string) error {
	if !strings.HasPrefix(strings.ToLower(targetRef), "module:") {
		return fmt.Errorf("window move: unsupported target %q", targetRef)
	}
	return nil
}

// moveClientsToTarget moves all clients to the target, focusing the last one if not silent.
func (d *Dispatcher) moveClientsToTarget(ctx context.Context, svc *module.Service, hypr runtime.HyprctlClient, clients []hyprctl.ClientInfo, targetRef string, silent bool) ([]windowMoveResult, error) {
	results := make([]windowMoveResult, 0, len(clients))
	for idx, client := range clients {
		focus := !silent && idx == len(clients)-1
		res, err := d.moveClientToModule(ctx, svc, hypr, client, targetRef, focus)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

// formatMoveResults formats the move results as a single object or array.
func formatMoveResults(results []windowMoveResult) any {
	if len(results) == 1 {
		return results[0]
	}
	return results
}

func (d *Dispatcher) resolveWindowSelection(ctx context.Context, hypr runtime.HyprctlClient, orbitProvider orbit.Provider, ref string) ([]hyprctl.ClientInfo, error) {
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
		client := window.ClientInfoFromWindow(win)
		if client.Address == "" {
			return nil, nil
		}
		return []hyprctl.ClientInfo{client}, nil
	case lower == "workspace":
		workspaceName, err := workspace.ActiveName(ctx, hypr)
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
			workspaceName, err = workspace.ActiveName(ctx, hypr)
			if err != nil {
				return nil, err
			}
		}

		var orbitName string
		if reference.Scope == window.ScopeOrbit {
			orbitName, err = orbit.ActiveName(ctx, orbitProvider)
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

func decodeClients(ctx context.Context, hypr runtime.HyprctlClient) ([]hyprctl.ClientInfo, error) {
	var clients []hyprctl.ClientInfo
	if err := hypr.DecodeClients(ctx, &clients); err != nil {
		return nil, err
	}
	return clients, nil
}

func (d *Dispatcher) handleModuleGet(ctx context.Context) ipc.Response {
	resp := ipc.NewResponse(false)
	svc, err := d.requireModuleService()
	if err != nil {
		return errorResponse(err.Error(), 1)
	}

	hyprClient := d.state.HyprctlClient()
	activeGetter, ok := hyprClient.(interface {
		ActiveWorkspace(context.Context) (*hyprctl.Workspace, error)
	})
	if !ok || activeGetter == nil {
		return errorResponse("hyprctl client does not expose active workspace", 1)
	}

	ws, err := activeGetter.ActiveWorkspace(ctx)
	if err != nil {
		return errorResponse(err.Error(), 1)
	}
	if ws == nil || util.IsEmptyOrWhitespace(ws.Name) {
		return errorResponse("active workspace not available", 1)
	}

	moduleName, orbitName, err := module.ParseWorkspaceName(ws.Name)
	if err != nil {
		return errorResponse(err.Error(), 2)
	}

	status, err := svc.Status(ctx, moduleName, orbitName)
	if err != nil {
		return errorResponse(err.Error(), 2)
	}

	if err := ipc.AssignData(&resp, status); err != nil {
		return errorResponse(err.Error(), 1)
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
	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return err
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

	var currentModule string
	if name, err := workspace.ActiveName(ctx, hypr); err == nil {
		if moduleName, _, err := module.ParseWorkspaceName(name); err == nil {
			currentModule = moduleName
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
	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return err
	}
	modSvc, err := d.requireModuleService()
	if err != nil {
		return err
	}
	names := modSvc.ModuleNames()
	if len(names) == 0 {
		return fmt.Errorf("no modules configured")
	}
	record, err := modSvc.ActiveOrbit(ctx)
	if err != nil {
		return err
	}
	targetWorkspace := module.WorkspaceName(names[0], record.Name)

	// Move current active workspace windows into the target before switching.
	ws, err := hypr.ActiveWorkspace(ctx)
	if err == nil && ws != nil {
		if err := workspace.EnsureExists(ctx, hypr, targetWorkspace, ws.Name); err != nil {
			return err
		}

		clients := d.collectClients(ctx)
		moveErr := workspace.MoveClients(ctx, hypr, clients, targetWorkspace)
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

	if err := workspace.MoveClients(ctx, hypr, []hyprctl.ClientInfo{client}, target.Workspace); err != nil {
		return result, err
	}

	if focus && strings.TrimSpace(target.Workspace) != "" {
		if err := hypr.SwitchWorkspace(ctx, target.Workspace); err != nil {
			return result, err
		}
	}

	d.state.recordWorkspaceActivation(target.Workspace)

	result.Window = window.DescribeClient(client)
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
	if record == nil || util.IsEmptyOrWhitespace(record.Name) {
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

	targetWorkspace := module.WorkspaceName(moduleName, orbitName)
	if err := workspace.EnsureExists(ctx, hypr, targetWorkspace, origin); err != nil {
		return target, err
	}

	target.Module = moduleName
	target.Workspace = targetWorkspace
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
		return util.CyclicNext(names, current), nil
	case "prev":
		if len(names) == 0 {
			return "", fmt.Errorf("no modules configured")
		}
		return util.CyclicPrev(names, current), nil
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
		if name, err := workspace.ActiveName(ctx, hypr); err == nil {
			if moduleName, orbit, err := module.ParseWorkspaceName(name); err == nil && orbit == orbitName {
				return moduleName
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

func (d *Dispatcher) cleanupTemporaryWorkspace(ctx context.Context, hypr runtime.HyprctlClient, targetWorkspace string) {
	targetWorkspace = strings.TrimSpace(targetWorkspace)
	if targetWorkspace == "" || hypr == nil {
		return
	}
	if !d.state.IsTemporaryWorkspace(targetWorkspace) {
		return
	}
	moduleName, orbitName, err := module.ParseWorkspaceName(targetWorkspace)
	if err != nil {
		return
	}
	if regWorkspace, ok := d.state.tempModuleWorkspace(orbitName, moduleName); !ok || regWorkspace != targetWorkspace {
		return
	}

	if wsName, err := workspace.ActiveName(ctx, hypr); err == nil {
		if wsName == targetWorkspace {
			return
		}
	}

	count, err := workspace.WindowCount(ctx, hypr, targetWorkspace)
	if err != nil {
		d.state.Logf("temp workspace cleanup %s: %v", targetWorkspace, err)
		return
	}
	if count > 0 {
		return
	}

	if err := hypr.Dispatch(ctx, "dispatch", "killworkspace", "name:"+targetWorkspace); err != nil {
		d.state.Logf("temp workspace kill %s: %v", targetWorkspace, err)
		return
	}
	d.state.unregisterTempModule(orbitName, moduleName)
	d.state.InvalidateClients()
	d.state.Logf("removed temporary workspace %s", targetWorkspace)
}

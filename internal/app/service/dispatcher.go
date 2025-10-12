package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"hyprorbit/internal/config"
	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/ipc"
	"hyprorbit/internal/module"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/runtime"
	"hyprorbit/internal/util"
	"hyprorbit/internal/window"
	"hyprorbit/internal/workspace"
)

// Dispatcher routes IPC requests to domain handlers.
type Dispatcher struct {
	state  *DaemonState
	logger *log.Logger
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

type windowMoveListEntry struct {
	Address   string `json:"address"`
	Class     string `json:"class,omitempty"`
	Title     string `json:"title,omitempty"`
	Workspace string `json:"workspace,omitempty"`
	Module    string `json:"module,omitempty"`
	Orbit     string `json:"orbit,omitempty"`
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
func NewDispatcher(state *DaemonState, logger *log.Logger) *Dispatcher {
	return &Dispatcher{
		state:  state,
		logger: logger,
	}
}

// debugf logs a debug message if debug logging is enabled.
func (d *Dispatcher) debugf(format string, args ...any) {
	if d != nil && d.logger != nil {
		d.logger.Printf(format, args...)
	}
}

// infof logs to stdout and optionally to debug log if enabled.
func (d *Dispatcher) infof(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stdout, "%s\n", msg)
	if d != nil && d.logger != nil {
		d.logger.Print(msg)
	}
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
	orbitSvc := d.state.OrbitService()
	if orbitSvc == nil {
		return nil, fmt.Errorf("orbit service unavailable")
	}
	return orbitSvc, nil
}

// allModuleNamesForOrbit returns all module names (configured + temporary) for the given orbit.
func (d *Dispatcher) allModuleNamesForOrbit(modSvc *module.Service, orbitName string) []string {
	names := modSvc.ModuleNames()
	tempNames := d.state.TempModuleNames(orbitName)
	if len(tempNames) > 0 {
		names = util.MergeStrings(names, tempNames)
	}
	return names
}

// requireModuleService returns the module service or an error if unavailable.
func (d *Dispatcher) requireModuleService() (*module.Service, error) {
	modSvc := d.state.ModuleService()
	if modSvc == nil {
		return nil, fmt.Errorf("module service unavailable")
	}
	return modSvc, nil
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
	d.debugf("Handle: command=%q action=%q args=%v", req.Command, req.Action, req.Args)

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

// handleDaemon processes daemon control commands like status checks and config reloads.
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

// handleOrbit processes orbit switching and query commands.
func (d *Dispatcher) handleOrbit(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler) {
	d.debugf("handleOrbit: action=%q args=%v", req.Action, req.Args)
	resp := ipc.NewResponse(false)
	orbitSvc, err := d.requireOrbitService()
	if err != nil {
		return errorResponse(err.Error(), 1), nil
	}

	switch req.Action {
	case "get":
		name, err := orbitSvc.Current(ctx)
		if err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		record, err := orbitSvc.Record(ctx, name)
		if err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		if err := ipc.AssignData(&resp, record); err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		return resp, nil
	case "next":
		return d.handleOrbitStep(ctx, orbitSvc, 1)
	case "prev":
		return d.handleOrbitStep(ctx, orbitSvc, -1)
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
		//TODO shouldnt there be an handleOrbitSet as well?
		// What about invalidateClients() vs publishSnapshot?
		if len(req.Args) != 1 {
			return errorResponse("orbit set requires exactly one argument", 2), nil
		}
		target := req.Args[0]
		d.debugf("handleOrbit set: target=%q", target)
		record, err := orbitSvc.Record(ctx, target)
		if err != nil {
			return errorResponse(err.Error(), 2), nil
		}
		if err := orbitSvc.Set(ctx, target); err != nil {
			return errorResponse(err.Error(), 1), nil
		}
		if err := d.alignMonitorsToOrbit(ctx, target, false); err != nil {
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

// handleOrbitStep cycles through the orbit sequence by the given delta (1 for next, -1 for prev).
func (d *Dispatcher) handleOrbitStep(ctx context.Context, orbitSvc *orbit.Service, delta int) (ipc.Response, StreamHandler) {
	d.debugf("handleOrbitStep: delta=%d", delta)
	resp := ipc.NewResponse(false)
	seq, err := orbitSvc.Sequence(ctx)
	if err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	if len(seq) == 0 {
		return errorResponse("orbit: no orbits configured", 1), nil
	}
	current, err := orbitSvc.Current(ctx)
	if err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	cycleMode := config.OrbitCycleModeAll
	if d.state != nil {
		if cfg := d.state.Config(); cfg != nil {
			cycleMode = cfg.Orbit.CycleMode
		}
	}
	sequence := seq
	if d.state != nil {
		sequence = d.filteredOrbitSequence(seq, current, cycleMode)
	}
	idx := util.IndexOf(sequence, current)
	if idx == -1 {
		idx = util.IndexOf(seq, current)
		if idx == -1 {
			return errorResponse(fmt.Sprintf("orbit: current orbit %q not found", current), 1), nil
		}
		sequence = seq
	}
	var nextIdx int
	if delta > 0 {
		nextIdx = (idx + 1) % len(sequence)
	} else {
		nextIdx = idx - 1
		if nextIdx < 0 {
			nextIdx = len(sequence) - 1
		}
	}
	name := sequence[nextIdx]
	d.debugf("handleOrbitStep: current=%q sequence=%v filtered=%v next=%q (index %d)", current, seq, sequence, name, nextIdx)
	if err := orbitSvc.Set(ctx, name); err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	if err := d.alignMonitorsToOrbit(ctx, name, false); err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	d.state.InvalidateClients()
	record, err := orbitSvc.Record(ctx, name)
	if err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	if err := ipc.AssignData(&resp, record); err != nil {
		return errorResponse(err.Error(), 1), nil
	}
	// Snapshot refresh triggers window count cache updates used for orbit cycling.
	d.publishSnapshot()
	return resp, nil
}

func (d *Dispatcher) filteredOrbitSequence(seq []string, current string, mode config.OrbitCycleMode) []string {
	if mode != config.OrbitCycleModeNotEmpty || len(seq) <= 1 || d == nil || d.state == nil {
		return seq
	}

	hasWindows := false
	for _, name := range seq {
		if d.state.OrbitWindowCount(name) > 0 {
			hasWindows = true
			break
		}
	}
	if !hasWindows {
		return seq
	}

	idx := util.IndexOf(seq, current)
	if idx == -1 {
		idx = 0
	}

	includeEmpty := d.state.OrbitWindowCount(current) > 0

	nextEmpty := ""
	if includeEmpty {
		for i := idx + 1; i < len(seq); i++ {
			if d.state.OrbitWindowCount(seq[i]) == 0 {
				nextEmpty = seq[i]
				break
			}
		}
	}

	filtered := make([]string, 0, len(seq))
	for i, name := range seq {
		count := d.state.OrbitWindowCount(name)
		switch {
		case name == current:
			filtered = append(filtered, name)
		case count > 0:
			filtered = append(filtered, name)
		case i <= idx:
			filtered = append(filtered, name)
		case nextEmpty != "" && name == nextEmpty:
			filtered = append(filtered, name)
		}
	}

	if len(filtered) <= 1 {
		return seq
	}

	return filtered
}

// handleModule processes module commands including list, jump, focus, and seed operations.
func (d *Dispatcher) handleModule(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	d.debugf("handleModule: action=%q args=%v", req.Action, req.Args)
	resp := ipc.NewResponse(false)
	modSvc, err := d.requireModuleService()
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	switch req.Action {
	case "list":
		filter, err := moduleListFilterFromFlags(req.Flags)
		if err != nil {
			return errorResponse(err.Error(), 2), nil, nil
		}
		summaries, err := modSvc.WorkspaceSummaries(ctx)
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
	case "get":
		return d.handleModuleGet(ctx), nil, nil
	case "jump-next":
		return d.handleModuleStep(ctx, modSvc, 1)
	case "jump-prev":
		return d.handleModuleStep(ctx, modSvc, -1)
	case "jump-create":
		return d.handleModuleCreate(ctx, modSvc)
	}

	if len(req.Args) == 0 {
		return errorResponse("module command requires a module name", 2), nil, nil
	}
	moduleName := req.Args[0]

	if req.Action == "jump" {
		return d.handleModuleJump(ctx, modSvc, moduleName)
	}

	if _, ok := modSvc.Module(moduleName); !ok {
		available := strings.Join(modSvc.ModuleNames(), ", ")
		return errorResponse(fmt.Sprintf("module %q not configured (available: %s)", moduleName, available), 2), nil, nil
	}

	switch req.Action {
	case "focus":
		opts, err := focusOptionsFromFlags(req.Flags)
		if err != nil {
			return errorResponse(err.Error(), 2), nil, nil
		}
		result, err := modSvc.Focus(ctx, moduleName, opts)
		if err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		return d.successResponseWithModuleResult(result)
	case "seed":
		results, err := modSvc.Seed(ctx, moduleName)
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

// handleModuleJump switches to the specified module workspace, creating temporary workspaces as needed.
func (d *Dispatcher) handleModuleJump(ctx context.Context, modSvc *module.Service, moduleName string) (ipc.Response, StreamHandler, error) {
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
	if _, ok := modSvc.Module(moduleName); ok {
		result, err = modSvc.Jump(ctx, moduleName)
		if err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
	} else {
		if hypr == nil {
			return errorResponse("hyprctl client unavailable", 1), nil, nil
		}
		orbitName := d.getActiveOrbitName(ctx, modSvc)
		if orbitName == "" {
			return errorResponse("active orbit not available", 1), nil, nil
		}
		workspace := module.WorkspaceName(moduleName, orbitName)
		if err := hypr.SwitchWorkspace(ctx, workspace); err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
		d.state.RegisterTempModule(orbitName, moduleName)
		result = &module.Result{Action: "jumped", Workspace: workspace, Orbit: orbitName}
	}

	if originWasTemp && originWorkspace != "" && result != nil && strings.TrimSpace(result.Workspace) != originWorkspace {
		d.cleanupTemporaryWorkspace(ctx, hypr, originWorkspace)
	}

	return d.successResponseWithModuleResult(result)
}

// handleModuleStep cycles through modules relative to the active workspace by the given delta.
func (d *Dispatcher) handleModuleStep(ctx context.Context, modSvc *module.Service, delta int) (ipc.Response, StreamHandler, error) {
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
	if _, ok := modSvc.Module(moduleName); !ok {
		d.state.RegisterTempModule(orbitName, moduleName)
	}

	names := d.allModuleNamesForOrbit(modSvc, orbitName)
	if len(names) == 0 {
		return errorResponse("no modules configured", 1), nil, nil
	}

	target := util.CyclicIndex(names, moduleName, delta)
	var result *module.Result
	if _, ok := modSvc.Module(target); ok {
		result, err = modSvc.Jump(ctx, target)
		if err != nil {
			return errorResponse(err.Error(), 1), nil, nil
		}
	} else if workspace, ok := d.state.TempModuleWorkspace(orbitName, target); ok {
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

// handleModuleCreate creates a new temporary module workspace and switches to it.
func (d *Dispatcher) handleModuleCreate(ctx context.Context, modSvc *module.Service) (ipc.Response, StreamHandler, error) {
	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	result, err := d.createModuleWorkspace(ctx, modSvc, hypr, "")
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	return d.successResponseWithModuleResult(result)
}

// *********************************************
// Helper functions
// *********************************************

// createModuleWorkspace creates a new temporary module workspace for the active orbit.
func (d *Dispatcher) createModuleWorkspace(ctx context.Context, modSvc *module.Service, hypr runtime.HyprctlClient, origin string) (*module.Result, error) {
	orbitName := d.getActiveOrbitName(ctx, modSvc)
	if orbitName == "" {
		return nil, fmt.Errorf("active orbit not available")
	}

	result, err := workspace.CreateTemporary(ctx, hypr, d.state, orbitName, origin)
	if err != nil {
		return nil, err
	}

	return &module.Result{Action: "created", Workspace: result.Workspace, Orbit: result.Orbit}, nil
}

// handleWindow processes window management commands like move and list.
func (d *Dispatcher) handleWindow(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	d.debugf("handleWindow: action=%q args=%v", req.Action, req.Args)
	switch req.Action {
	case "move":
		return d.handleWindowMove(ctx, req)
	case "list":
		return d.handleWindowList(ctx, req)
	default:
		return errorResponse(fmt.Sprintf("unknown window action %q", req.Action), 2), nil, nil
	}
}

// handleWindowList returns a list of all windows with their module and orbit assignments.
func (d *Dispatcher) handleWindowList(ctx context.Context, req ipc.Request) (ipc.Response, StreamHandler, error) {
	if len(req.Args) > 0 {
		return errorResponse("window list does not accept arguments", 2), nil, nil
	}

	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	clients, err := window.DecodeClients(ctx, hypr)
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	summaries := make([]windowMoveListEntry, 0, len(clients))
	for _, client := range clients {
		sanitized := window.SanitizeClient(client)
		workspaceName := sanitized.WorkspaceName()
		moduleName := ""
		orbitName := ""
		if workspaceName != "" {
			if m, o, err := module.ParseWorkspaceName(workspaceName); err == nil {
				moduleName = m
				orbitName = o
			}
		}

		summary := windowMoveListEntry{
			Address:   sanitized.Address,
			Class:     sanitized.Class,
			Title:     sanitized.Title,
			Workspace: workspaceName,
		}
		if moduleName != "" {
			summary.Module = moduleName
		}
		if orbitName != "" {
			summary.Orbit = orbitName
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		a := summaries[i]
		b := summaries[j]
		if a.Workspace != b.Workspace {
			return a.Workspace < b.Workspace
		}
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		if a.Orbit != b.Orbit {
			return a.Orbit < b.Orbit
		}
		return a.Address < b.Address
	})

	if len(summaries) == 0 {
		summaries = []windowMoveListEntry{}
	}

	resp := ipc.NewResponse(false)
	if err := ipc.AssignData(&resp, summaries); err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}
	return resp, nil, nil
}

// handleWindowMove relocates one or more windows to a target module workspace.
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

	global, err := parseGlobalFlag(req.Flags)
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

	clients, err := d.resolveWindowsForMove(ctx, hypr, modSvc, windowRef, global)
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	if err := validateMoveTarget(targetRef); err != nil {
		return errorResponse(err.Error(), 2), nil, nil
	}
	// NOTE: When orbit-level move targets (e.g. orbit:NAME) arrive, ensure snapshot refresh keeps window counts aligned.

	results, err := d.moveClientsToTarget(ctx, modSvc, hypr, clients, targetRef, silent)
	if err != nil {
		return errorResponse(err.Error(), 1), nil, nil
	}

	return d.successResponse(formatMoveResults(results))
}

// resolveWindowsForMove resolves window references and validates the selection.
func (d *Dispatcher) resolveWindowsForMove(ctx context.Context, hypr runtime.HyprctlClient, modSvc *module.Service, windowRef string, global bool) ([]hyprctl.ClientInfo, error) {
	var orbitProvider orbit.Provider
	if modSvc != nil {
		orbitProvider = modSvc
	}

	clients, err := window.ResolveSelection(ctx, hypr, orbitProvider, windowRef, global)
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
func (d *Dispatcher) moveClientsToTarget(ctx context.Context, modSvc *module.Service, hypr runtime.HyprctlClient, clients []hyprctl.ClientInfo, targetRef string, follow bool) ([]windowMoveResult, error) {
	results := make([]windowMoveResult, 0, len(clients))
	for idx, client := range clients {
		follow := !follow && idx == len(clients)-1
		res, err := d.moveClientToModule(ctx, modSvc, hypr, client, targetRef, follow)
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

// handleModuleGet returns the module status for the currently active workspace.
func (d *Dispatcher) handleModuleGet(ctx context.Context) ipc.Response {
	resp := ipc.NewResponse(false)
	modSvc, err := d.requireModuleService()
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

	status, err := modSvc.Status(ctx, moduleName, orbitName)
	if err != nil {
		return errorResponse(err.Error(), 2)
	}

	if err := ipc.AssignData(&resp, status); err != nil {
		return errorResponse(err.Error(), 1)
	}
	return resp
}

// publishSnapshot broadcasts the current daemon state to all subscribed clients.
func (d *Dispatcher) publishSnapshot() {
	if d == nil || d.state == nil {
		return
	}
	if err := d.state.PublishSnapshot(context.Background()); err != nil {
		d.state.Logf("snapshot publish: %v", err)
	}
}

// streamModuleStatus returns a handler that streams state snapshots to a connected client.
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

// filterWorkspaceSummaries filters workspace summaries by the given filter (all, active, inactive).
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

// resetWorkspaces destroys all workspaces except configured modules and the primary workspace.
func (d *Dispatcher) resetWorkspaces(ctx context.Context) error {
	d.debugf("resetWorkspaces: starting workspace reset")
	hypr, err := d.requireHyprctlClient()
	if err != nil {
		return fmt.Errorf("workspace reset: %w", err)
	}
	modSvc, err := d.requireModuleService()
	if err != nil {
		return fmt.Errorf("workspace reset: %w", err)
	}
	orbitName := d.getActiveOrbitName(ctx, modSvc)
	if orbitName == "" {
		return fmt.Errorf("workspace reset: active orbit not available")
	}
	d.debugf("resetWorkspaces: active orbit=%q", orbitName)

	primaryWorkspace, err := d.jumpToFirstModuleWorkspace(ctx, modSvc)
	if err != nil {
		return fmt.Errorf("workspace reset: %w", err)
	}
	d.debugf("resetWorkspaces: primaryWorkspace=%q", primaryWorkspace)

	if err := d.alignMonitorsToOrbit(ctx, orbitName, false); err != nil {
		return fmt.Errorf("workspace reset: %w", err)
	}
	workspaces, err := hypr.Workspaces(ctx)
	if err != nil {
		return fmt.Errorf("workspace reset: failed to list workspaces: %w", err)
	}

	// Log all current workspaces
	allWorkspaceNames := make([]string, 0, len(workspaces))
	for _, ws := range workspaces {
		allWorkspaceNames = append(allWorkspaceNames, ws.Name)
	}
	d.debugf("resetWorkspaces: current workspaces: %v", allWorkspaceNames)

	moduleNames := modSvc.ModuleNames()
	d.debugf("resetWorkspaces: configured modules: %v", moduleNames)
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
		return nil
	}
	d.infof("[hyprorbit] workspace reset: workspaces scheduled for kill: %s", strings.Join(targets, ", "))
	d.debugf("workspace reset: workspaces scheduled for kill: %s", strings.Join(targets, ", "))

	// Move all windows from all workspaces (across all orbits) to the primary safe workspace
	d.debugf("resetWorkspaces: moving all windows to safe workspace %q before killing workspaces", primaryWorkspace)
	clients, err := window.DecodeClients(ctx, hypr)
	if err != nil {
		return fmt.Errorf("workspace reset: failed to get clients: %w", err)
	}

	clientsToMove := make([]hyprctl.ClientInfo, 0, len(clients))
	for _, client := range clients {
		sanitized := window.SanitizeClient(client)
		wsName := sanitized.WorkspaceName()
		if wsName == "" || strings.HasPrefix(wsName, "special") {
			continue
		}
		// Only move windows from workspaces that will be killed
		// if _, isSafe := safeSet[wsName]; !isSafe {
		clientsToMove = append(clientsToMove, sanitized)
		// }
	}

	if len(clientsToMove) > 0 {
		d.debugf("resetWorkspaces: moving %d windows to safe workspace", len(clientsToMove))
		if err := workspace.MoveClients(ctx, hypr, clientsToMove, primaryWorkspace, false); err != nil {
			return fmt.Errorf("workspace reset: failed to move windows to safe workspace: %w", err)
		}
		d.infof("[hyprorbit] workspace reset: moved %d windows to %q", len(clientsToMove), primaryWorkspace)
	} else {
		d.debugf("resetWorkspaces: no windows need to be moved")
	}

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
	d.state.InvalidateClients()
	d.state.clearTempModules()
	d.debugf("resetWorkspaces: workspace reset complete")
	return nil
}

// alignMonitorsToOrbit ensures all monitors (or only the focused one) display a primary module workspace for the given orbit.
func (d *Dispatcher) alignMonitorsToOrbit(ctx context.Context, orbitName string, onlyFocusedMonitor bool) error {
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

	monitorSummaries := make([]string, 0, len(monitors))
	for _, mon := range monitors {
		monitorSummaries = append(monitorSummaries, fmt.Sprintf("%s(focused=%v, workspace=%q)", mon.Name, mon.Focused, strings.TrimSpace(mon.ActiveWorkspace.Name)))
	}
	d.debugf("alignMonitorsToOrbit: monitor snapshot (count=%d, focusedOnly=%v): %s", len(monitors), onlyFocusedMonitor, strings.Join(monitorSummaries, ", "))

	if len(monitors) == 0 {
		d.debugf("alignMonitorsToOrbit: no monitors reported, skipping alignment")
		return nil
	}

	if len(monitors) <= 1 || onlyFocusedMonitor {
		d.debugf("alignMonitorsToOrbit: aligning focused monitor only (monitors=%d flag=%v)", len(monitors), onlyFocusedMonitor)
		if workspaceName, err := d.jumpToPrimaryModuleWorkspace(ctx); err != nil {
			return fmt.Errorf("align monitors: %w", err)
		} else {
			d.debugf("alignMonitorsToOrbit: focused monitor aligned to workspace %q", workspaceName)
		}
		return nil
	}

	// Identify focused monitor
	focusedIdx := slices.IndexFunc(monitors, func(m hyprctl.Monitor) bool { return m.Focused })
	if focusedIdx == -1 {
		focusedIdx = 0
		d.debugf("alignMonitorsToOrbit: focused monitor not reported, defaulting to index 0 (%q)", monitors[0].Name)
	}

	// Create list of monitors without focused monitor and add it as last entry
	ordered := make([]hyprctl.Monitor, 0, len(monitors))
	for idx, mon := range monitors {
		if idx == focusedIdx {
			continue
		}
		ordered = append(ordered, mon)
	}
	ordered = append(ordered, monitors[focusedIdx])
	orderedNames := make([]string, 0, len(ordered))
	for _, mon := range ordered {
		orderedNames = append(orderedNames, mon.Name)
	}
	d.debugf("alignMonitorsToOrbit: monitor focus order=%s", strings.Join(orderedNames, " -> "))
	for _, mon := range ordered {
		d.debugf("alignMonitorsToOrbit: focusing monitor %q for orbit %q", mon.Name, orbitName)
		if err := hypr.Dispatch(ctx, "focusmonitor", mon.Name); err != nil {
			return fmt.Errorf("align monitors: failed to focus monitor %q: %w", mon.Name, err)
		}
		if workspaceName, err := d.jumpToPrimaryModuleWorkspace(ctx); err != nil {
			return fmt.Errorf("align monitors: %w", err)
		} else {
			d.debugf("alignMonitorsToOrbit: monitor %q now on workspace %q", mon.Name, workspaceName)
		}
	}
	return nil
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
func (d *Dispatcher) jumpToPrimaryModuleWorkspace(ctx context.Context) (string, error) {
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
		d.debugf("jumpToPrimaryModuleWorkspace: jumping to candidate[%d] %q", i, candidate)
		res, err := modSvc.Jump(ctx, candidate)
		if err != nil {
			return "", fmt.Errorf("failed to jump to module %q: %w", candidate, err)
		}
		d.recordModuleResult(res)
		d.debugf("jumpToPrimaryModuleWorkspace: jumped to workspace=%q", res.Workspace)
		return strings.TrimSpace(res.Workspace), nil
	}

	d.debugf("jumpToPrimaryModuleWorkspace: no valid candidates, falling back to jumpToFirstModuleWorkspace")
	return d.jumpToFirstModuleWorkspace(ctx, modSvc)
}

// jumpToFirstModuleWorkspace jumps to the first configured module workspace for the active orbit.
// This provides deterministic workspace selection, ignoring user preferences and history.
// It skips modules that are already in use by other monitors to avoid conflicts.
func (d *Dispatcher) jumpToFirstModuleWorkspace(ctx context.Context, modSvc *module.Service) (string, error) {
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

	// Get current workspaces to identify which modules are already in use
	workspaces, err := hypr.Workspaces(ctx)
	if err != nil {
		d.debugf("jumpToFirstModuleWorkspace: failed to list workspaces, proceeding without filtering: %v", err)
		// Fall back to jumping to first module without checking
		result, err := modSvc.Jump(ctx, moduleNames[0])
		if err != nil {
			return "", fmt.Errorf("failed to jump to first module %q: %w", moduleNames[0], err)
		}
		d.recordModuleResult(result)
		d.debugf("jumpToFirstModuleWorkspace: jumped to workspace=%q", result.Workspace)
		return strings.TrimSpace(result.Workspace), nil
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
	result, err := modSvc.Jump(ctx, moduleNames[0])
	if err != nil {
		return "", fmt.Errorf("failed to jump to first module %q: %w", moduleNames[0], err)
	}
	d.recordModuleResult(result)
	d.debugf("jumpToFirstModuleWorkspace: jumped to workspace=%q", result.Workspace)
	return strings.TrimSpace(result.Workspace), nil
}

// moveClientToModule relocates a single window to the specified module target, optionally following focus.
func (d *Dispatcher) moveClientToModule(ctx context.Context, modSvc *module.Service, hypr runtime.HyprctlClient, client hyprctl.ClientInfo, targetRef string, follow bool) (windowMoveResult, error) {
	var result windowMoveResult
	client = window.SanitizeClient(client)
	if client.Address == "" {
		return result, fmt.Errorf("window not available")
	}
	origin := client.Workspace.Name
	originTemp := d.state.IsTemporaryWorkspace(origin)
	target, err := d.resolveModuleTarget(ctx, modSvc, hypr, origin, targetRef)
	if err != nil {
		return result, err
	}

	if err := workspace.MoveClients(ctx, hypr, []hyprctl.ClientInfo{client}, target.Workspace, follow); err != nil {
		return result, err
	}

	d.state.recordWorkspaceActivation(target.Workspace)

	result.Window = window.DescribeClient(client)
	result.Workspace = target.Workspace
	result.Module = target.Module
	result.Orbit = target.Orbit
	result.Created = target.Created
	result.Temporary = target.Temporary
	result.Focused = follow
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

// resolveModuleTarget parses a module target reference and returns the resolved target workspace details.
func (d *Dispatcher) resolveModuleTarget(ctx context.Context, modSvc *module.Service, hypr runtime.HyprctlClient, origin, ref string) (moduleTarget, error) {
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
		res, err := d.createModuleWorkspace(ctx, modSvc, hypr, origin)
		if err != nil {
			return target, err
		}
		target.Workspace = res.Workspace
		target.Orbit = res.Orbit
		target.Created = true
		if moduleName, orbit, err := module.ParseWorkspaceName(res.Workspace); err == nil {
			target.Module = moduleName
			target.Temporary = true
			d.state.RegisterTempModule(orbit, moduleName)
		}
		return target, nil
	}

	orbitName := d.getActiveOrbitName(ctx, modSvc)
	if orbitName == "" {
		return target, fmt.Errorf("active orbit not available")
	}

	names := d.allModuleNamesForOrbit(modSvc, orbitName)
	if len(names) == 0 {
		return target, fmt.Errorf("no modules configured")
	}
	current := d.currentModuleForOrbit(ctx, hypr, orbitName)
	moduleName, err := d.selectModuleName(names, current, spec)
	if err != nil {
		return target, err
	}
	if _, ok := modSvc.Module(moduleName); !ok {
		d.state.RegisterTempModule(orbitName, moduleName)
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

// selectModuleName resolves a module selector (next, prev, index:N, regex:PATTERN, or name) to a module name.
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

// currentModuleForOrbit returns the module name from the active workspace or last active module for the given orbit.
func (d *Dispatcher) currentModuleForOrbit(ctx context.Context, hypr runtime.HyprctlClient, orbitName string) string {
	orbitName = strings.TrimSpace(orbitName)
	currentModule, monitorName := d.getCurrentModuleAndMonitor(ctx, hypr)

	// If we're currently in the target orbit, return the current module
	if currentModule != "" {
		if ws, err := hypr.ActiveWorkspace(ctx); err == nil && ws != nil {
			if _, orbit, err := module.ParseWorkspaceName(strings.TrimSpace(ws.Name)); err == nil && orbit == orbitName {
				return currentModule
			}
		}
	}

	if d.state == nil {
		return ""
	}
	return strings.TrimSpace(d.state.LastActiveModule(orbitName, monitorName))
}

// recordModuleResult tracks workspace activation from a module operation result.
func (d *Dispatcher) recordModuleResult(result *module.Result) {
	if d == nil || d.state == nil || result == nil {
		return
	}
	if result.Workspace == "" {
		return
	}
	d.state.recordWorkspaceActivation(result.Workspace)
}

// collectClients retrieves all Hyprland windows from the hyprctl client.
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

// cleanupTemporaryWorkspace destroys the specified temporary workspace if it is empty.
func (d *Dispatcher) cleanupTemporaryWorkspace(ctx context.Context, hypr runtime.HyprctlClient, targetWorkspace string) {
	if workspace.CleanupTemporary(ctx, hypr, d.state, targetWorkspace) {
		d.state.InvalidateClients()
	}
}

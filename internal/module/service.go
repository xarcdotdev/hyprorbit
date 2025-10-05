package module

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"

	"hyprorbit/internal/config"
	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/runtime"
)

// OrbitAccessor captures the orbit functionality required by the module service.
type OrbitAccessor interface {
	Current(ctx context.Context) (string, error)
	Record(ctx context.Context, name string) (*orbit.Record, error)
}

// Dependencies wires the collaborators required to build a module service instance.
type Dependencies struct {
	Config  *config.EffectiveConfig
	Orbit   OrbitAccessor
	Hyprctl runtime.HyprctlClient
}

// Service exposes module-focused orchestration helpers backed by shared dependencies.
type Service struct {
	cfg      *config.EffectiveConfig
	orbitSvc OrbitAccessor
	hyprctl  runtime.HyprctlClient

	clientsOnce sync.Once
	clientCache []hyprctl.ClientInfo
	clientErr   error
}

// WorkspaceSummary describes the relationship between configured modules and Hyprland workspaces.
type WorkspaceSummary struct {
	Name            string `json:"name"`
	Module          string `json:"module,omitempty"`
	Orbit           string `json:"orbit,omitempty"`
	Configured      bool   `json:"configured"`
	Exists          bool   `json:"exists"`
	Special         bool   `json:"special,omitempty"`
	Temporary       bool   `json:"temporary,omitempty"`
	Windows         int    `json:"windows,omitempty"`
	Monitor         string `json:"monitor,omitempty"`
	HasFullscreen   bool   `json:"has_fullscreen,omitempty"`
	LastWindow      string `json:"last_window,omitempty"`
	LastWindowTitle string `json:"last_window_title,omitempty"`
}

// NewService wires module-specific helpers using runtime stored in context.
func NewService(ctx context.Context) (*Service, error) {
	rt, err := runtime.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := rt.Config(ctx)
	if err != nil {
		return nil, err
	}
	orbitSvc, err := orbit.NewService(ctx)
	if err != nil {
		return nil, err
	}
	deps := Dependencies{
		Config:  cfg,
		Orbit:   orbitSvc,
		Hyprctl: rt.Dependencies().HyprctlClient,
	}
	return NewServiceWithDependencies(deps)
}

// NewServiceWithDependencies constructs a module service from explicit collaborators.
func NewServiceWithDependencies(deps Dependencies) (*Service, error) {
	if deps.Config == nil {
		return nil, fmt.Errorf("module: config dependency is required")
	}
	if deps.Orbit == nil {
		return nil, fmt.Errorf("module: orbit dependency is required")
	}
	if deps.Hyprctl == nil {
		return nil, fmt.Errorf("module: hyprctl dependency is required")
	}
	return &Service{
		cfg:      deps.Config,
		orbitSvc: deps.Orbit,
		hyprctl:  deps.Hyprctl,
	}, nil
}

// Module retrieves a module definition by name if present.
func (s *Service) Module(name string) (config.ModuleRecord, bool) {
	if s.cfg == nil {
		return config.ModuleRecord{}, false
	}
	mod, ok := s.cfg.Modules[name]
	return mod, ok
}

// ModuleNames returns the configured module identifiers in sorted order.
func (s *Service) ModuleNames() []string {
	if s.cfg == nil {
		return nil
	}
	names := make([]string, 0, len(s.cfg.Modules))
	for name := range s.cfg.Modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Status returns metadata for the specified module within the given orbit.
func (s *Service) Status(ctx context.Context, moduleName, orbitName string) (*Status, error) {
	record, err := s.orbitSvc.Record(ctx, orbitName)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, fmt.Errorf("orbit %q not defined", orbitName)
	}
	workspace := WorkspaceName(moduleName, orbitName)
	return &Status{
		Module:    moduleName,
		Workspace: workspace,
		Orbit:     *record,
	}, nil
}

// ActiveOrbit resolves the currently active orbit metadata.
func (s *Service) ActiveOrbit(ctx context.Context) (*orbit.Record, error) {
	name, err := s.orbitSvc.Current(ctx)
	if err != nil {
		return nil, err
	}
	return s.orbitSvc.Record(ctx, name)
}

// Focus performs focus-or-launch for a module within the active orbit.
func (s *Service) Focus(ctx context.Context, moduleName string, opts FocusOptions) (*Result, error) {
	mod, orbitRecord, workspace, err := s.workspace(ctx, moduleName)
	if err != nil {
		return nil, err
	}

	matcher := mod.Focus.Matcher
	if opts.MatcherOverride != "" {
		override, err := ParseMatcher(opts.MatcherOverride)
		if err != nil {
			return nil, err
		}
		matcher = override
	}

	compiled, err := compileMatcher(matcher)
	if err != nil {
		return nil, fmt.Errorf("module %s matcher: %w", moduleName, err)
	}

	allowMove := s.cfg.Defaults.Move
	if opts.NoMove {
		allowMove = false
	}

	shouldFloat := s.cfg.Defaults.Float
	if opts.ForceFloat {
		shouldFloat = true
	}

	spawnCmd := mod.Focus.Cmd
	if len(opts.CmdOverride) > 0 {
		spawnCmd = opts.CmdOverride
	}

	clients, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}

	workspaceClients, orbitClients := bucketClients(clients, matcher, compiled, workspace, orbitRecord.Name)

	if len(workspaceClients) > 0 {
		client := workspaceClients[0]
		cmds := [][]string{{"workspace", "name:" + workspace}}
		if shouldFloat && !client.Floating {
			cmds = append(cmds, []string{"togglefloating", "address:" + client.Address})
		}
		cmds = append(cmds, []string{"focuswindow", "address:" + client.Address})
		if err := dispatchSequence(ctx, s.hyprctl, cmds...); err != nil {
			return nil, err
		}
		return &Result{Action: "focused", Workspace: workspace}, nil
	}

	if allowMove && len(orbitClients) > 0 {
		client := orbitClients[0]
		cmds := [][]string{
			{"movetoworkspace", "name:" + workspace + ",address:" + client.Address},
			{"workspace", "name:" + workspace},
		}
		if shouldFloat && !client.Floating {
			cmds = append(cmds, []string{"togglefloating", "address:" + client.Address})
		}
		cmds = append(cmds, []string{"focuswindow", "address:" + client.Address})
		if err := dispatchSequence(ctx, s.hyprctl, cmds...); err != nil {
			return nil, err
		}
		return &Result{Action: "moved", Workspace: workspace}, nil
	}

	if len(spawnCmd) == 0 {
		return nil, fmt.Errorf("module %s: no matching clients and no command to spawn", moduleName)
	}

	if err := s.hyprctl.SwitchWorkspace(ctx, workspace); err != nil {
		return nil, err
	}
	if err := spawnProcess(ctx, spawnCmd); err != nil {
		return nil, err
	}
	return &Result{Action: "spawned", Workspace: workspace}, nil
}

// Jump switches focus to the module workspace inside the active orbit.
func (s *Service) Jump(ctx context.Context, moduleName string) (*Result, error) {
	_, orbitRecord, workspace, err := s.workspace(ctx, moduleName)
	if err != nil {
		return nil, err
	}
	if err := s.hyprctl.SwitchWorkspace(ctx, workspace); err != nil {
		return nil, err
	}
	return &Result{Action: "jumped", Workspace: workspace, Orbit: orbitRecord.Name}, nil
}

// Seed bootstraps a module workspace using its configured seed steps.
func (s *Service) Seed(ctx context.Context, moduleName string) ([]*Result, error) {
	mod, _, workspace, err := s.workspace(ctx, moduleName)
	if err != nil {
		return nil, err
	}
	clients, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	if hasWorkspaceClients(clients, workspace) {
		return []*Result{{Action: "seed-skip", Workspace: workspace}}, nil
	}
	results := make([]*Result, 0, len(mod.Seed))
	for _, seed := range mod.Seed {
		opts := FocusOptions{
			MatcherOverride: matcherToString(seed.Matcher),
			CmdOverride:     seed.Cmd,
			NoMove:          true,
		}
		res, err := s.Focus(ctx, moduleName, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	if len(results) == 0 {
		return []*Result{{Action: "seed-empty", Workspace: workspace}}, nil
	}
	return results, nil
}

// WorkspaceSummaries returns configured and active workspace metadata.
func (s *Service) WorkspaceSummaries(ctx context.Context) ([]WorkspaceSummary, error) {
	if s.cfg == nil {
		return nil, fmt.Errorf("module: config unavailable")
	}
	if s.hyprctl == nil {
		return nil, fmt.Errorf("module: hyprctl dependency is required")
	}

	workspaces, err := s.hyprctl.Workspaces(ctx)
	if err != nil {
		return nil, err
	}

	existing := make(map[string]hyprctl.Workspace, len(workspaces))
	for _, ws := range workspaces {
		existing[ws.Name] = ws
	}

	summaries := make([]WorkspaceSummary, 0, len(s.cfg.Modules)*len(s.cfg.Orbits)+len(existing))
	seen := make(map[string]struct{}, len(existing))

	for moduleName := range s.cfg.Modules {
		for _, orbitRecord := range s.cfg.Orbits {
			name := WorkspaceName(moduleName, orbitRecord.Name)
			ws, ok := existing[name]
			summary := WorkspaceSummary{
				Name:       name,
				Module:     moduleName,
				Orbit:      orbitRecord.Name,
				Configured: true,
				Exists:     ok,
			}
			if ok {
				summary.Windows = ws.Windows
				summary.Monitor = ws.Monitor
				summary.HasFullscreen = ws.HasFullscreen
				summary.LastWindow = ws.LastWindow
				summary.LastWindowTitle = ws.LastWindowTitle
				seen[name] = struct{}{}
			}
			summaries = append(summaries, summary)
		}
	}

	for name, ws := range existing {
		if _, ok := seen[name]; ok {
			continue
		}
		summary := WorkspaceSummary{
			Name:            name,
			Configured:      false,
			Exists:          true,
			Windows:         ws.Windows,
			Monitor:         ws.Monitor,
			HasFullscreen:   ws.HasFullscreen,
			LastWindow:      ws.LastWindow,
			LastWindowTitle: ws.LastWindowTitle,
		}
		if moduleName, orbitName, err := ParseWorkspaceName(name); err == nil {
			summary.Module = moduleName
			summary.Orbit = orbitName
			if _, ok := s.Module(moduleName); !ok {
				summary.Temporary = true
				summary.Special = false
			}
		}
		if !summary.Temporary {
			summary.Special = true
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})

	return summaries, nil
}

func (s *Service) workspace(ctx context.Context, moduleName string) (config.ModuleRecord, *orbit.Record, string, error) {
	mod, ok := s.Module(moduleName)
	if !ok {
		return config.ModuleRecord{}, nil, "", fmt.Errorf("module %q not configured", moduleName)
	}
	orbitRecord, err := s.ActiveOrbit(ctx)
	if err != nil {
		return config.ModuleRecord{}, nil, "", err
	}
	ws := WorkspaceName(moduleName, orbitRecord.Name)
	return mod, orbitRecord, ws, nil
}

func (s *Service) clients(ctx context.Context) ([]hyprctl.ClientInfo, error) {
	s.clientsOnce.Do(func() {
		var out []hyprctl.ClientInfo
		err := s.hyprctl.DecodeClients(ctx, &out)
		if err != nil {
			s.clientErr = err
			return
		}
		s.clientCache = out
	})
	return s.clientCache, s.clientErr
}

// WorkspaceName composes the workspace identifier for a module within an orbit.
func WorkspaceName(moduleName, orbitName string) string {
	return fmt.Sprintf("%s-%s", moduleName, orbitName)
}

func spawnProcess(ctx context.Context, command []string) error {
	cmd := exec.CommandContext(ctx, command[0], command[1:]...) // #nosec G204 - command defined by config/user
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("spawn: %w", err)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}

func dispatchSequence(ctx context.Context, client runtime.HyprctlClient, commands ...[]string) error {
	filtered := make([][]string, 0, len(commands))
	for _, cmd := range commands {
		if len(cmd) == 0 {
			continue
		}
		filtered = append(filtered, cmd)
	}
	if len(filtered) == 0 {
		return nil
	}
	_, err := client.BatchDispatch(ctx, filtered...)
	return err
}

func bucketClients(clients []hyprctl.ClientInfo, matcher config.Matcher, compiled *regexp.Regexp, workspace string, orbitName string) ([]hyprctl.ClientInfo, []hyprctl.ClientInfo) {
	workspaceMatches := make([]hyprctl.ClientInfo, 0)
	orbitMatches := make([]hyprctl.ClientInfo, 0)
	suffix := "-" + orbitName
	for _, client := range clients {
		value := client.FieldValue(matcher.Field)
		if !matches(compiled, matcher.Expr, value) {
			continue
		}
		ws := client.WorkspaceName()
		if ws == workspace {
			workspaceMatches = append(workspaceMatches, client)
			continue
		}
		if strings.HasSuffix(ws, suffix) {
			orbitMatches = append(orbitMatches, client)
		}
	}
	return workspaceMatches, orbitMatches
}

func hasWorkspaceClients(clients []hyprctl.ClientInfo, workspace string) bool {
	for _, client := range clients {
		if client.WorkspaceName() == workspace {
			return true
		}
	}
	return false
}

// ResetClientCache clears the memoized hyprctl client listing.
func (s *Service) ResetClientCache() {
	s.clientsOnce = sync.Once{}
	s.clientCache = nil
	s.clientErr = nil
}

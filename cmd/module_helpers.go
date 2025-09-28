package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"

	"hypr-orbits/internal/config"
	"hypr-orbits/internal/runtime"
)

type moduleService struct {
	runtime  *runtime.Runtime
	cfg      *config.EffectiveConfig
	orbitSvc *orbitService

	clientsOnce sync.Once
	clientCache []hyprClient
	clientErr   error
}

func newModuleService(ctx context.Context) (*moduleService, error) {
	rt, err := runtime.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := rt.Config(ctx)
	if err != nil {
		return nil, err
	}
	orbitSvc, err := newOrbitService(ctx)
	if err != nil {
		return nil, err
	}
	return &moduleService{runtime: rt, cfg: cfg, orbitSvc: orbitSvc}, nil
}

func (s *moduleService) moduleRecord(name string) (config.ModuleRecord, bool) {
	mod, ok := s.cfg.Modules[name]
	return mod, ok
}

func (s *moduleService) moduleNames() []string {
	names := make([]string, 0, len(s.cfg.Modules))
	for name := range s.cfg.Modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (s *moduleService) activeOrbit(ctx context.Context) (*orbitRecord, error) {
	name, err := s.orbitSvc.currentOrbit(ctx)
	if err != nil {
		return nil, err
	}
	return s.orbitSvc.orbitRecord(ctx, name)
}

func (s *moduleService) workspace(ctx context.Context, moduleName string) (config.ModuleRecord, *orbitRecord, string, error) {
	mod, ok := s.moduleRecord(moduleName)
	if !ok {
		return config.ModuleRecord{}, nil, "", fmt.Errorf("module %q not configured", moduleName)
	}
	orbit, err := s.activeOrbit(ctx)
	if err != nil {
		return config.ModuleRecord{}, nil, "", err
	}
	ws := composeWorkspaceName(moduleName, orbit.Name)
	return mod, orbit, ws, nil
}

func (s *moduleService) clients(ctx context.Context) ([]hyprClient, error) {
	s.clientsOnce.Do(func() {
		var out []hyprClient
		err := s.runtime.Dependencies().HyprctlClient.DecodeClients(ctx, &out)
		if err != nil {
			s.clientErr = err
			return
		}
		s.clientCache = out
	})
	return s.clientCache, s.clientErr
}

func (s *moduleService) hyprctl() runtime.HyprctlClient {
	return s.runtime.Dependencies().HyprctlClient
}

func (s *moduleService) defaults() config.ModuleSettings {
	return s.cfg.Defaults
}

func (s *moduleService) focusModule(ctx context.Context, moduleName string, opts focusOptions) (*moduleResult, error) {
	mod, orbit, workspace, err := s.workspace(ctx, moduleName)
	if err != nil {
		return nil, err
	}

	matcher := mod.Focus.Matcher
	if opts.MatcherOverride != "" {
		override, err := parseMatcherString(opts.MatcherOverride)
		if err != nil {
			return nil, err
		}
		matcher = override
	}

	compiled, err := compileMatcher(matcher)
	if err != nil {
		return nil, fmt.Errorf("module %s matcher: %w", moduleName, err)
	}

	allowMove := s.defaults().Move
	if opts.NoMove {
		allowMove = false
	}

	shouldFloat := s.defaults().Float
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

	workspaceClients, orbitClients := bucketClients(clients, matcher, compiled, workspace, orbit.Name)

	if len(workspaceClients) > 0 {
		client := workspaceClients[0]
		if err := s.hyprctl().Dispatch(ctx, "workspace", "name:"+workspace); err != nil {
			return nil, err
		}
		if shouldFloat && !client.Floating {
			_ = s.hyprctl().Dispatch(ctx, "togglefloating", "address:"+client.Address)
		}
		if err := s.hyprctl().Dispatch(ctx, "focuswindow", "address:"+client.Address); err != nil {
			return nil, err
		}
		return &moduleResult{Action: "focused", Workspace: workspace}, nil
	}

	if allowMove && len(orbitClients) > 0 {
		client := orbitClients[0]
		if err := s.hyprctl().Dispatch(ctx, "movetoworkspace", "name:"+workspace, "address:"+client.Address); err != nil {
			return nil, err
		}
		if err := s.hyprctl().Dispatch(ctx, "workspace", "name:"+workspace); err != nil {
			return nil, err
		}
		if shouldFloat && !client.Floating {
			_ = s.hyprctl().Dispatch(ctx, "togglefloating", "address:"+client.Address)
		}
		if err := s.hyprctl().Dispatch(ctx, "focuswindow", "address:"+client.Address); err != nil {
			return nil, err
		}
		return &moduleResult{Action: "moved", Workspace: workspace}, nil
	}

	if len(spawnCmd) == 0 {
		return nil, fmt.Errorf("module %s: no matching clients and no command to spawn", moduleName)
	}

	if err := s.hyprctl().Dispatch(ctx, "workspace", "name:"+workspace); err != nil {
		return nil, err
	}
	if err := spawnProcess(ctx, spawnCmd); err != nil {
		return nil, err
	}
	return &moduleResult{Action: "spawned", Workspace: workspace}, nil
}

func (s *moduleService) jumpModule(ctx context.Context, moduleName string) (*moduleResult, error) {
	_, orbit, workspace, err := s.workspace(ctx, moduleName)
	if err != nil {
		return nil, err
	}
	if err := s.hyprctl().Dispatch(ctx, "workspace", "name:"+workspace); err != nil {
		return nil, err
	}
	return &moduleResult{Action: "jumped", Workspace: workspace, Orbit: orbit.Name}, nil
}

func (s *moduleService) seedModule(ctx context.Context, moduleName string) ([]*moduleResult, error) {
	mod, _, workspace, err := s.workspace(ctx, moduleName)
	if err != nil {
		return nil, err
	}
	clients, err := s.clients(ctx)
	if err != nil {
		return nil, err
	}
	if hasWorkspaceClients(clients, workspace) {
		return []*moduleResult{{Action: "seed-skip", Workspace: workspace}}, nil
	}
	results := make([]*moduleResult, 0, len(mod.Seed))
	for _, seed := range mod.Seed {
		opts := focusOptions{
			MatcherOverride: matcherToString(seed.Matcher),
			CmdOverride:     seed.Cmd,
			NoMove:          true,
		}
		res, err := s.focusModule(ctx, moduleName, opts)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	if len(results) == 0 {
		// No seed steps defined; nothing to do.
		return []*moduleResult{{Action: "seed-empty", Workspace: workspace}}, nil
	}
	return results, nil
}

type moduleResult struct {
	Action    string
	Workspace string
	Orbit     string
}

type focusOptions struct {
	MatcherOverride string
	CmdOverride     []string
	ForceFloat      bool
	NoMove          bool
}

type hyprClient struct {
	Address       string `json:"address"`
	Class         string `json:"class"`
	Title         string `json:"title"`
	InitialClass  string `json:"initialClass"`
	InitialTitle  string `json:"initialTitle"`
	Floating      bool   `json:"floating"`
	WorkspaceInfo struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"workspace"`
}

func (c hyprClient) workspaceName() string {
	if c.WorkspaceInfo.Name != "" {
		return c.WorkspaceInfo.Name
	}
	if c.WorkspaceInfo.ID != 0 {
		return fmt.Sprintf("%d", c.WorkspaceInfo.ID)
	}
	return ""
}

func (c hyprClient) fieldValue(field string) string {
	switch strings.ToLower(field) {
	case "class":
		return c.Class
	case "title":
		return c.Title
	case "initialclass":
		return c.InitialClass
	case "initialtitle":
		return c.InitialTitle
	default:
		return ""
	}
}

func bucketClients(clients []hyprClient, matcher config.Matcher, compiled *regexp.Regexp, workspace string, orbitName string) ([]hyprClient, []hyprClient) {
	workspaceMatches := make([]hyprClient, 0)
	orbitMatches := make([]hyprClient, 0)
	suffix := "-" + orbitName
	for _, client := range clients {
		value := client.fieldValue(matcher.Field)
		if !matches(compiled, matcher.Expr, value) {
			continue
		}
		ws := client.workspaceName()
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

func hasWorkspaceClients(clients []hyprClient, workspace string) bool {
	for _, client := range clients {
		if client.workspaceName() == workspace {
			return true
		}
	}
	return false
}

func matches(re *regexp.Regexp, expr string, value string) bool {
	if expr == "" {
		return true
	}
	if re == nil {
		return false
	}
	return re.MatchString(value)
}

func compileMatcher(m config.Matcher) (*regexp.Regexp, error) {
	if m.Expr == "" {
		return nil, nil
	}
	return regexp.Compile(m.Expr)
}

func parseMatcherString(input string) (config.Matcher, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return config.Matcher{}, fmt.Errorf("matcher override cannot be empty")
	}
	field := "class"
	expr := input
	if idx := strings.IndexRune(input, '='); idx > 0 {
		field = strings.TrimSpace(input[:idx])
		expr = strings.TrimSpace(input[idx+1:])
		if field == "" || expr == "" {
			return config.Matcher{}, fmt.Errorf("invalid matcher %q", input)
		}
	}
	return config.Matcher{Field: field, Expr: expr, Raw: input}, nil
}

func matcherToString(m config.Matcher) string {
	if m.Raw != "" {
		return m.Raw
	}
	if m.Expr == "" {
		return ""
	}
	return fmt.Sprintf("%s=%s", m.Field, m.Expr)
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

func composeWorkspaceName(moduleName, orbitName string) string {
	return fmt.Sprintf("%s-%s", moduleName, orbitName)
}

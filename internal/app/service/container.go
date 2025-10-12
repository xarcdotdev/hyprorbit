package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"hyprorbit/internal/config"
	"hyprorbit/internal/debug"
	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/hyprctl/events"
	"hyprorbit/internal/module"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/runtime"
	"hyprorbit/internal/state"
)

const snapshotQueryTimeout = 1200 * time.Millisecond

type orbitActivityRecord struct {
	LastModule string
	Monitors   map[string]string
}

// DaemonState aggregates long-lived daemon dependencies for hyprorbitd.
type DaemonState struct {
	opts       Options
	loaderOpts config.LoaderOptions

	mu        sync.RWMutex
	config    *config.EffectiveConfig
	orbitMgr  *state.Manager
	orbitSvc  *orbit.Service
	moduleSvc *module.Service
	hyprctl   runtime.HyprctlClient

	orbitActivityMu sync.RWMutex
	orbitActivity   map[string]orbitActivityRecord

	orbitWindowCountsMu sync.RWMutex
	orbitWindowCounts   map[string]int

	tempModulesMu sync.RWMutex
	tempModules   map[string][]string

	broadcaster *StatusBroadcaster

	eventsSub    *events.Subscriber
	eventsCancel context.CancelFunc

	wg        sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once

	logger func(string, ...any)
}

// NewDaemonState loads configuration and assembles domain services for the daemon.
func NewDaemonState(ctx context.Context, opts Options) (*DaemonState, error) {
	loaderOpts := config.LoaderOptions{OverridePath: opts.ConfigPath}

	cfg, err := config.NewLoader(loaderOpts).Load(ctx)
	if err != nil {
		return nil, err
	}

	orbitMgr, err := state.NewManager(state.Options{Orbits: cfg.Orbits})
	if err != nil {
		return nil, err
	}

	hyprClient := hyprctl.NewClient(hyprctl.Options{
		CacheTTL:     opts.EffectiveCacheTTL(),
		DisableCache: opts.DisableCache,
	})

	// Setup debug logging for hyprctl
	hyprctlLogger, err := debug.NewLogger("hyprctl", &cfg.Debug)
	if err != nil {
		return nil, fmt.Errorf("failed to setup hyprctl debug logging: %w", err)
	}
	if hyprctlLogger != nil {
		hyprClient.SetLogger(hyprctlLogger)
	}

	orbitSvc, err := orbit.NewServiceWithDependencies(orbit.Dependencies{
		Tracker: orbitMgr,
		Config:  cfg,
	})
	if err != nil {
		return nil, err
	}

	moduleSvc, err := module.NewServiceWithDependencies(module.Dependencies{
		Config:  cfg,
		Orbit:   orbitSvc,
		Hyprctl: hyprClient,
	})
	if err != nil {
		return nil, err
	}

	state := &DaemonState{
		opts:              opts,
		loaderOpts:        loaderOpts,
		config:            cfg,
		orbitMgr:          orbitMgr,
		orbitSvc:          orbitSvc,
		moduleSvc:         moduleSvc,
		hyprctl:           hyprClient,
		orbitActivity:     make(map[string]orbitActivityRecord, len(cfg.Orbits)),
		orbitWindowCounts: make(map[string]int, len(cfg.Orbits)),
		tempModules:       make(map[string][]string),
		broadcaster:       NewStatusBroadcaster(0),
		logger:            func(string, ...any) {},
	}

	state.refreshOrbitActivity(cfg.Orbits)

	return state, nil
}

// Start activates background event consumers for the daemon.
func (s *DaemonState) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	var startErr error
	s.startOnce.Do(func() {
		sub, err := events.NewSubscriber(events.Options{
			Logf: func(format string, args ...any) { s.logf(format, args...) },
		})
		if err != nil {
			startErr = err
			return
		}

		subCtx, cancel := context.WithCancel(ctx)

		s.mu.Lock()
		s.eventsSub = sub
		s.eventsCancel = cancel
		s.mu.Unlock()

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.consumeHyprEvents(subCtx, sub)
		}()

		sub.Start(subCtx)

		if err := s.PublishSnapshot(context.Background()); err != nil {
			s.logf("initial snapshot publish: %v", err)
		}
	})
	return startErr
}

// Stop stops background consumers started via Start.
func (s *DaemonState) Stop() {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		cancel := s.eventsCancel
		s.eventsCancel = nil
		s.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		s.wg.Wait()
	})
}

// Config returns the effective configuration snapshot.
func (s *DaemonState) Config() *config.EffectiveConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// OrbitService exposes the cached orbit service.
func (s *DaemonState) OrbitService() *orbit.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.orbitSvc
}

// ModuleService exposes the cached module service.
func (s *DaemonState) ModuleService() *module.Service {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.moduleSvc
}

// HyprctlClient exposes the underlying hyprctl client.
func (s *DaemonState) HyprctlClient() runtime.HyprctlClient {
	return s.hyprctl
}

// Broadcaster exposes the status broadcaster.
func (s *DaemonState) Broadcaster() *StatusBroadcaster {
	s.mu.RLock()
	broadcaster := s.broadcaster
	s.mu.RUnlock()
	if broadcaster != nil {
		return broadcaster
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.broadcaster == nil {
		s.broadcaster = NewStatusBroadcaster(0)
	}
	return s.broadcaster
}

// SubscribeSnapshots registers a new snapshot subscriber bound to ctx.
func (s *DaemonState) SubscribeSnapshots(ctx context.Context) (<-chan StatusSnapshot, func()) {
	return s.Broadcaster().Subscribe(ctx)
}

// InvalidateClients clears the cached hyprctl client listing.
func (s *DaemonState) InvalidateClients() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.moduleSvc.ResetClientCache()
	if s.hyprctl != nil {
		s.hyprctl.InvalidateClients()
	}
}

// Reload refreshes configuration and domain services without restarting the daemon.
func (s *DaemonState) Reload(ctx context.Context) error {
	loader := config.NewLoader(s.loaderOpts)
	cfg, err := loader.Load(ctx)
	if err != nil {
		return err
	}

	orbitMgr, err := state.NewManager(state.Options{Orbits: cfg.Orbits})
	if err != nil {
		return err
	}

	orbitSvc, err := orbit.NewServiceWithDependencies(orbit.Dependencies{
		Tracker: orbitMgr,
		Config:  cfg,
	})
	if err != nil {
		return err
	}

	moduleSvc, err := module.NewServiceWithDependencies(module.Dependencies{
		Config:  cfg,
		Orbit:   orbitSvc,
		Hyprctl: s.hyprctl,
	})
	if err != nil {
		return err
	}

	s.refreshOrbitActivity(cfg.Orbits)
	s.orbitWindowCountsMu.Lock()
	s.orbitWindowCounts = make(map[string]int, len(cfg.Orbits))
	s.orbitWindowCountsMu.Unlock()

	s.mu.Lock()
	s.config = cfg
	s.orbitMgr = orbitMgr
	s.orbitSvc = orbitSvc
	s.moduleSvc = moduleSvc
	s.mu.Unlock()

	if s.hyprctl != nil {
		s.hyprctl.InvalidateClients()
	}

	if err := s.PublishSnapshot(context.Background()); err != nil {
		s.logf("snapshot publish after reload: %v", err)
	}
	return nil
}

// PublishSnapshot computes and broadcasts the current status snapshot.
func (s *DaemonState) PublishSnapshot(ctx context.Context) error {
	broadcaster := s.Broadcaster()
	if broadcaster == nil {
		return fmt.Errorf("broadcaster unavailable")
	}

	snapshot, err := s.buildSnapshot(ctx)
	if err != nil {
		return err
	}
	if snapshot == nil {
		return nil
	}

	if s.needsOrbitWindowCounts() {
		s.refreshOrbitWindowCounts(ctx)
	}

	snapshot.Generated = time.Now().UTC()
	broadcaster.Publish(*snapshot)
	return nil
}

func (s *DaemonState) buildSnapshot(ctx context.Context) (*StatusSnapshot, error) {
	hypr := s.HyprctlClient()
	if hypr == nil {
		return nil, fmt.Errorf("hyprctl unavailable")
	}

	getter, ok := hypr.(interface {
		ActiveWorkspace(context.Context) (*hyprctl.Workspace, error)
	})
	if !ok || getter == nil {
		return nil, fmt.Errorf("hyprctl client does not expose active workspace")
	}

	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	baseCtx, cancel := context.WithTimeout(baseCtx, snapshotQueryTimeout)
	defer cancel()

	ws, err := getter.ActiveWorkspace(baseCtx)
	if err != nil {
		return nil, err
	}

	snapshot := &StatusSnapshot{}
	if ws != nil {
		snapshot.Workspace = strings.TrimSpace(ws.Name)
		snapshot.Windows = ws.Windows
		snapshot.Monitor = ws.Monitor
	}

	if snapshot.Workspace == "" {
		return snapshot, nil
	}

	moduleName, orbitName, err := module.ParseWorkspaceName(snapshot.Workspace)
	if err != nil {
		s.logf("snapshot parse workspace %q: %v", snapshot.Workspace, err)
		return snapshot, nil
	}

	svc := s.ModuleService()
	if svc == nil {
		return snapshot, fmt.Errorf("module service unavailable")
	}

	status, err := svc.Status(baseCtx, moduleName, orbitName)
	if err != nil {
		s.logf("snapshot status %s/%s: %v", moduleName, orbitName, err)
		return snapshot, nil
	}

	snapshot.Module = status.Module
	snapshot.Orbit = &status.Orbit
	if snapshot.Orbit != nil {
		s.recordActiveModule(snapshot.Module, snapshot.Orbit.Name, snapshot.Monitor)
	}
	return snapshot, nil
}

func (s *DaemonState) recordActiveModule(moduleName, orbitName, monitorName string) {
	moduleName = strings.TrimSpace(moduleName)
	orbitName = strings.TrimSpace(orbitName)
	monitorName = strings.TrimSpace(monitorName)
	if moduleName == "" || orbitName == "" {
		return
	}
	s.orbitActivityMu.Lock()
	defer s.orbitActivityMu.Unlock()
	if s.orbitActivity == nil {
		s.orbitActivity = make(map[string]orbitActivityRecord)
	}
	record := s.orbitActivity[orbitName]
	record.LastModule = moduleName
	if monitorName != "" {
		if record.Monitors == nil {
			record.Monitors = make(map[string]string)
		}
		record.Monitors[monitorName] = moduleName
	}
	s.orbitActivity[orbitName] = record
}

func (s *DaemonState) recordWorkspaceActivation(workspace string) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return
	}
	moduleName, orbitName, err := module.ParseWorkspaceName(workspace)
	if err != nil {
		return
	}
	monitorName := s.workspaceMonitor(workspace)
	s.recordActiveModule(moduleName, orbitName, monitorName)
}

func (s *DaemonState) workspaceMonitor(workspace string) string {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return ""
	}
	h := s.HyprctlClient()
	if h == nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), snapshotQueryTimeout)
	defer cancel()
	workspaces, err := h.Workspaces(ctx)
	if err != nil {
		return ""
	}
	for _, ws := range workspaces {
		if strings.TrimSpace(ws.Name) == workspace {
			return strings.TrimSpace(ws.Monitor)
		}
	}
	return ""
}

// RegisterTempModule registers a temporary module for an orbit.
func (s *DaemonState) RegisterTempModule(orbitName, moduleName string) {
	orbitName = strings.TrimSpace(orbitName)
	moduleName = strings.TrimSpace(moduleName)
	if orbitName == "" || moduleName == "" {
		return
	}
	s.tempModulesMu.Lock()
	defer s.tempModulesMu.Unlock()
	if s.tempModules == nil {
		s.tempModules = make(map[string][]string)
	}
	list := s.tempModules[orbitName]
	for _, existing := range list {
		if existing == moduleName {
			return
		}
	}
	list = append(list, moduleName)
	sort.Strings(list)
	s.tempModules[orbitName] = list
}

// UnregisterTempModule removes a temporary module from an orbit.
func (s *DaemonState) UnregisterTempModule(orbitName, moduleName string) {
	orbitName = strings.TrimSpace(orbitName)
	moduleName = strings.TrimSpace(moduleName)
	if orbitName == "" || moduleName == "" {
		return
	}
	s.tempModulesMu.Lock()
	defer s.tempModulesMu.Unlock()
	if len(s.tempModules) == 0 {
		return
	}
	list := s.tempModules[orbitName]
	if len(list) == 0 {
		return
	}
	out := list[:0]
	removed := false
	for _, v := range list {
		if v == moduleName {
			removed = true
			continue
		}
		out = append(out, v)
	}
	if !removed {
		return
	}
	if len(out) == 0 {
		delete(s.tempModules, orbitName)
	} else {
		s.tempModules[orbitName] = out
	}
}

func (s *DaemonState) TempModuleNames(orbitName string) []string {
	orbitName = strings.TrimSpace(orbitName)
	if orbitName == "" {
		return nil
	}
	s.tempModulesMu.RLock()
	defer s.tempModulesMu.RUnlock()
	if len(s.tempModules) == 0 {
		return nil
	}
	list := s.tempModules[orbitName]
	if len(list) == 0 {
		return nil
	}
	return append([]string(nil), list...)
}

// TempModuleWorkspace returns the workspace for a temporary module if it exists.
func (s *DaemonState) TempModuleWorkspace(orbitName, moduleName string) (string, bool) {
	orbitName = strings.TrimSpace(orbitName)
	moduleName = strings.TrimSpace(moduleName)
	if orbitName == "" || moduleName == "" {
		return "", false
	}
	s.tempModulesMu.RLock()
	defer s.tempModulesMu.RUnlock()
	if len(s.tempModules) == 0 {
		return "", false
	}
	list := s.tempModules[orbitName]
	for _, existing := range list {
		if existing == moduleName {
			return module.WorkspaceName(moduleName, orbitName), true
		}
	}
	return "", false
}

func (s *DaemonState) clearTempModules() {
	s.tempModulesMu.Lock()
	defer s.tempModulesMu.Unlock()
	s.tempModules = make(map[string][]string)
}

// IsTemporaryWorkspace checks if a workspace is a temporary workspace.
func (s *DaemonState) IsTemporaryWorkspace(workspace string) bool {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return false
	}
	moduleName, orbitName, err := module.ParseWorkspaceName(workspace)
	if err != nil {
		return false
	}
	_, ok := s.TempModuleWorkspace(orbitName, moduleName)
	return ok
}

// TODO: Refactor?
func (s *DaemonState) orbitActivitySnapshot() map[string]orbitActivityRecord {
	s.orbitActivityMu.RLock()
	defer s.orbitActivityMu.RUnlock()
	if len(s.orbitActivity) == 0 {
		return map[string]orbitActivityRecord{}
	}
	snapshot := make(map[string]orbitActivityRecord, len(s.orbitActivity))
	for name, record := range s.orbitActivity {
		clone := orbitActivityRecord{LastModule: record.LastModule}
		if len(record.Monitors) != 0 {
			clone.Monitors = make(map[string]string, len(record.Monitors))
			for monitorName, moduleName := range record.Monitors {
				clone.Monitors[monitorName] = moduleName
			}
		}
		snapshot[name] = clone
	}
	return snapshot
}

func (s *DaemonState) refreshOrbitActivity(orbits []config.OrbitRecord) {
	s.orbitActivityMu.Lock()
	defer s.orbitActivityMu.Unlock()
	if len(orbits) == 0 {
		s.orbitActivity = make(map[string]orbitActivityRecord)
		return
	}
	current := s.orbitActivity
	next := make(map[string]orbitActivityRecord, len(orbits))
	for _, orbitRecord := range orbits {
		name := strings.TrimSpace(orbitRecord.Name)
		if name == "" {
			continue
		}
		if current != nil {
			if record, ok := current[name]; ok {
				clone := orbitActivityRecord{LastModule: record.LastModule}
				if len(record.Monitors) != 0 {
					clone.Monitors = make(map[string]string, len(record.Monitors))
					for monitorName, moduleName := range record.Monitors {
						clone.Monitors[monitorName] = moduleName
					}
				}
				next[name] = clone
				continue
			}
		}
		next[name] = orbitActivityRecord{}
	}
	s.orbitActivity = next
}

func (s *DaemonState) refreshOrbitWindowCounts(ctx context.Context) {
	if s == nil {
		return
	}
	svc := s.ModuleService()
	if svc == nil {
		return
	}

	baseCtx := ctx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	baseCtx, cancel := context.WithTimeout(baseCtx, snapshotQueryTimeout)
	defer cancel()

	summaries, err := svc.WorkspaceSummaries(baseCtx)
	if err != nil {
		s.logf("refresh orbit window counts: %v", err)
		return
	}

	counts := make(map[string]int, len(summaries))
	for _, summary := range summaries {
		orbitName := strings.TrimSpace(summary.Orbit)
		if orbitName == "" {
			continue
		}
		counts[orbitName] += summary.Windows
	}

	if cfg := s.Config(); cfg != nil {
		for _, orbitRecord := range cfg.Orbits {
			name := strings.TrimSpace(orbitRecord.Name)
			if name == "" {
				continue
			}
			if _, ok := counts[name]; !ok {
				counts[name] = 0
			}
		}
	}

	s.orbitWindowCountsMu.Lock()
	s.orbitWindowCounts = counts
	s.orbitWindowCountsMu.Unlock()
}

func (s *DaemonState) orbitWindowCountsSnapshot() map[string]int {
	s.orbitWindowCountsMu.RLock()
	defer s.orbitWindowCountsMu.RUnlock()
	if len(s.orbitWindowCounts) == 0 {
		return map[string]int{}
	}
	clone := make(map[string]int, len(s.orbitWindowCounts))
	for name, count := range s.orbitWindowCounts {
		clone[name] = count
	}
	return clone
}

func (s *DaemonState) needsOrbitWindowCounts() bool {
	cfg := s.Config()
	if cfg == nil {
		return false
	}
	return cfg.Orbit.CycleMode == config.OrbitCycleModeNotEmpty
}

// LastActiveModule returns the most recent module observed within the orbit.
// When a monitor name is provided, it returns the last module seen on that monitor,
// falling back to the orbit-wide record when no monitor-specific entry exists.
func (s *DaemonState) LastActiveModule(orbitName, monitorName string) string {
	orbitName = strings.TrimSpace(orbitName)
	monitorName = strings.TrimSpace(monitorName)
	if orbitName == "" {
		return ""
	}
	s.orbitActivityMu.RLock()
	defer s.orbitActivityMu.RUnlock()
	record, ok := s.orbitActivity[orbitName]
	if !ok {
		return ""
	}
	if monitorName != "" && len(record.Monitors) != 0 {
		if moduleName := strings.TrimSpace(record.Monitors[monitorName]); moduleName != "" {
			return moduleName
		}
	}
	return strings.TrimSpace(record.LastModule)
}

// PreferLastActiveFirst reports whether orbit switching should favour the last active module.
func (s *DaemonState) PreferLastActiveFirst() bool {
	cfg := s.Config()
	if cfg == nil {
		return true
	}
	return cfg.Orbit.SwitchPreference != config.OrbitSwitchPreferenceSameModuleFirst
}

func (s *DaemonState) OrbitSummaries(ctx context.Context) ([]orbit.Summary, error) {
	svc := s.OrbitService()
	if svc == nil {
		return nil, fmt.Errorf("orbit service unavailable")
	}
	seq, err := svc.Sequence(ctx)
	if err != nil {
		return nil, err
	}
	current, err := svc.Current(ctx)
	if err != nil {
		return nil, err
	}
	activity := s.orbitActivitySnapshot()
	summaries := make([]orbit.Summary, 0, len(seq))
	for _, name := range seq {
		status := "inactive"
		if name == current {
			status = "active"
		}
		summary := orbit.Summary{Name: name, Status: status}
		summary.Windows = s.OrbitWindowCount(name)
		if record, ok := activity[name]; ok {
			if moduleName := strings.TrimSpace(record.LastModule); moduleName != "" {
				summary.ActiveModule = moduleName
			}
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
}

// OrbitWindowCount returns the cached number of windows assigned to the orbit.
func (s *DaemonState) OrbitWindowCount(orbitName string) int {
	orbitName = strings.TrimSpace(orbitName)
	if orbitName == "" {
		return 0
	}
	s.orbitWindowCountsMu.RLock()
	defer s.orbitWindowCountsMu.RUnlock()
	if s.orbitWindowCounts == nil {
		return 0
	}
	return s.orbitWindowCounts[orbitName]
}

func (s *DaemonState) consumeHyprEvents(ctx context.Context, sub *events.Subscriber) {
	eventsCh := sub.Events()
	errsCh := sub.Errors()

	for {
		if eventsCh == nil && errsCh == nil {
			return
		}

		select {
		case <-ctx.Done():
			return
		case ev, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				continue
			}
			if shouldPublishSnapshot(ev.Type) {
				if err := s.PublishSnapshot(context.Background()); err != nil {
					s.logf("snapshot publish: %v", err)
				}
			}
		case err, ok := <-errsCh:
			if !ok {
				errsCh = nil
				continue
			}
			s.logf("hyprland events: %v", err)
		}
	}
}

func shouldPublishSnapshot(t events.EventType) bool {
	switch t {
	case events.TypeWorkspace, events.TypeWorkspaceV2, events.TypeActiveWorkspace, events.TypeActiveWindow, events.TypeFocusedMonitor:
		return true
	default:
		return false
	}
}

// Logf records diagnostic messages if a logger is available.
func (s *DaemonState) Logf(format string, args ...any) {
	s.logf(format, args...)
}

func (s *DaemonState) logf(format string, args ...any) {
	if s == nil || s.logger == nil {
		return
	}
	s.logger(format, args...)
}

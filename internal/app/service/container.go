package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"hyprorbit/internal/config"
	"hyprorbit/internal/hyprctl"
	"hyprorbit/internal/hyprctl/events"
	"hyprorbit/internal/module"
	"hyprorbit/internal/orbit"
	"hyprorbit/internal/runtime"
	"hyprorbit/internal/state"
)

const snapshotQueryTimeout = 1200 * time.Millisecond

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
	orbitActivity   map[string]string

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

	return &DaemonState{
		opts:          opts,
		loaderOpts:    loaderOpts,
		config:        cfg,
		orbitMgr:      orbitMgr,
		orbitSvc:      orbitSvc,
		moduleSvc:     moduleSvc,
		hyprctl:       hyprClient,
		orbitActivity: make(map[string]string, len(cfg.Orbits)),
		broadcaster:   NewStatusBroadcaster(0),
		logger:        func(string, ...any) {},
	}, nil
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
		s.recordActiveModule(snapshot.Module, snapshot.Orbit.Name)
	}
	return snapshot, nil
}

func (s *DaemonState) recordActiveModule(moduleName, orbitName string) {
	moduleName = strings.TrimSpace(moduleName)
	orbitName = strings.TrimSpace(orbitName)
	if moduleName == "" || orbitName == "" {
		return
	}
	s.orbitActivityMu.Lock()
	if s.orbitActivity == nil {
		s.orbitActivity = make(map[string]string)
	}
	s.orbitActivity[orbitName] = moduleName
	s.orbitActivityMu.Unlock()
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
	s.recordActiveModule(moduleName, orbitName)
}

func (s *DaemonState) orbitActivitySnapshot() map[string]string {
	s.orbitActivityMu.RLock()
	defer s.orbitActivityMu.RUnlock()
	if len(s.orbitActivity) == 0 {
		return map[string]string{}
	}
	snapshot := make(map[string]string, len(s.orbitActivity))
	for name, moduleName := range s.orbitActivity {
		snapshot[name] = moduleName
	}
	return snapshot
}

func (s *DaemonState) refreshOrbitActivity(orbits []config.OrbitRecord) {
	s.orbitActivityMu.Lock()
	defer s.orbitActivityMu.Unlock()
	if len(orbits) == 0 {
		s.orbitActivity = make(map[string]string)
		return
	}
	current := s.orbitActivity
	next := make(map[string]string, len(orbits))
	for _, orbitRecord := range orbits {
		if current != nil {
			if moduleName := current[orbitRecord.Name]; moduleName != "" {
				next[orbitRecord.Name] = moduleName
			}
		}
	}
	s.orbitActivity = next
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
		if moduleName := activity[name]; moduleName != "" {
			summary.ActiveModule = moduleName
		}
		summaries = append(summaries, summary)
	}
	return summaries, nil
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

package service

import (
	"context"
	"sync"

	"hyprorbits/internal/config"
	"hyprorbits/internal/hyprctl"
	"hyprorbits/internal/module"
	"hyprorbits/internal/orbit"
	"hyprorbits/internal/runtime"
	"hyprorbits/internal/state"
)

// DaemonState aggregates long-lived daemon dependencies for hyprorbitsd.
type DaemonState struct {
	opts       Options
	loaderOpts config.LoaderOptions

	mu        sync.RWMutex
	config    *config.EffectiveConfig
	orbitMgr  *state.Manager
	orbitSvc  *orbit.Service
	moduleSvc *module.Service
	hyprctl   runtime.HyprctlClient
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
		opts:       opts,
		loaderOpts: loaderOpts,
		config:     cfg,
		orbitMgr:   orbitMgr,
		orbitSvc:   orbitSvc,
		moduleSvc:  moduleSvc,
		hyprctl:    hyprClient,
	}, nil
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

	s.mu.Lock()
	s.config = cfg
	s.orbitMgr = orbitMgr
	s.orbitSvc = orbitSvc
	s.moduleSvc = moduleSvc
	s.mu.Unlock()

	if s.hyprctl != nil {
		s.hyprctl.InvalidateClients()
	}
	return nil
}

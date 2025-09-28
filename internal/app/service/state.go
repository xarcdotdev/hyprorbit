package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"hypr-orbits/internal/config"
	"hypr-orbits/internal/hyprctl"
	"hypr-orbits/internal/module"
	"hypr-orbits/internal/orbit"
	"hypr-orbits/internal/runtime"
	"hypr-orbits/internal/state"
)

// DaemonState aggregates long-lived daemon dependencies for hypr-orbitsd.
type DaemonState struct {
	opts   Options
	loader *config.Loader

	mu        sync.RWMutex
	config    *config.EffectiveConfig
	orbitMgr  *state.Manager
	orbitSvc  *orbit.Service
	moduleSvc *module.Service
	hyprctl   *cachedHyprctl
}

// NewDaemonState loads configuration and assembles domain services for the daemon.
func NewDaemonState(ctx context.Context, opts Options) (*DaemonState, error) {
	loader := config.NewLoader(config.LoaderOptions{OverridePath: opts.ConfigPath})

	cfg, err := loader.Load(ctx)
	if err != nil {
		return nil, err
	}

	orbitMgr, err := state.NewManager(state.Options{Orbits: cfg.Orbits})
	if err != nil {
		return nil, err
	}

	hyp := newCachedHyprctl(hyprctl.Options{}, opts.EffectiveCacheTTL(), opts.DisableCache)

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
		Hyprctl: hyp,
	})
	if err != nil {
		return nil, err
	}

	return &DaemonState{
		opts:      opts,
		loader:    loader,
		config:    cfg,
		orbitMgr:  orbitMgr,
		orbitSvc:  orbitSvc,
		moduleSvc: moduleSvc,
		hyprctl:   hyp,
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

// InvalidateClients clears the cached hyprctl client listing.
func (s *DaemonState) InvalidateClients() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.moduleSvc.ResetClientCache()
	s.hyprctl.Invalidate()
}

// cachedHyprctl implements runtime.HyprctlClient with TTL-bound caching.
type cachedHyprctl struct {
	opts         hyprctl.Options
	ttl          time.Duration
	disableCache bool

	cacheMu   sync.Mutex
	cachedRaw []byte
	cachedAt  time.Time

	dispatchOnce sync.Once
	dispatch     *hyprctl.Client
}

func newCachedHyprctl(opts hyprctl.Options, ttl time.Duration, disable bool) *cachedHyprctl {
	if opts.Timeout <= 0 {
		opts.Timeout = 500 * time.Millisecond
	}
	return &cachedHyprctl{opts: opts, ttl: ttl, disableCache: disable}
}

// Dispatch proxies hyprctl dispatch requests without caching.
func (c *cachedHyprctl) Dispatch(ctx context.Context, args ...string) error {
	client := c.dispatchClient()
	return client.Dispatch(ctx, args...)
}

// Clients returns the hyprctl clients JSON, honoring the configured TTL.
func (c *cachedHyprctl) Clients(ctx context.Context) ([]byte, error) {
	if c.disableCache || c.ttl == 0 {
		return c.fetchClients(ctx)
	}

	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()

	if len(c.cachedRaw) > 0 && time.Since(c.cachedAt) < c.ttl {
		return append([]byte(nil), c.cachedRaw...), nil
	}

	data, err := c.fetchClients(ctx)
	if err != nil {
		return nil, err
	}

	c.cachedRaw = append(c.cachedRaw[:0], data...)
	c.cachedAt = time.Now()
	return append([]byte(nil), c.cachedRaw...), nil
}

// DecodeClients deserializes the cached clients payload into the provided sink.
func (c *cachedHyprctl) DecodeClients(ctx context.Context, out any) error {
	data, err := c.Clients(ctx)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("hyprctl: decode clients: %w", err)
	}
	return nil
}

// Invalidate clears any cached clients response.
func (c *cachedHyprctl) Invalidate() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.cachedRaw = nil
	c.cachedAt = time.Time{}
}

func (c *cachedHyprctl) dispatchClient() *hyprctl.Client {
	c.dispatchOnce.Do(func() {
		c.dispatch = hyprctl.NewClient(c.opts)
	})
	return c.dispatch
}

func (c *cachedHyprctl) fetchClients(ctx context.Context) ([]byte, error) {
	client := hyprctl.NewClient(c.opts)
	data, err := client.Clients(ctx)
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), data...), nil
}

var _ runtime.HyprctlClient = (*cachedHyprctl)(nil)

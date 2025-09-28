package orbit

import (
	"context"
	"fmt"

	"hyprorbits/internal/config"
	"hyprorbits/internal/runtime"
)

// Dependencies bundles the collaborators required by the orbit service.
type Dependencies struct {
	Tracker runtime.OrbitTracker
	Config  *config.EffectiveConfig
}

// Service wraps runtime dependencies required by orbit commands.
type Service struct {
	tracker runtime.OrbitTracker
	config  *config.EffectiveConfig
}

// NewService creates a new orbit service from the given context.
func NewService(ctx context.Context) (*Service, error) {
	rt, err := runtime.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := rt.Config(ctx)
	if err != nil {
		return nil, err
	}
	deps := Dependencies{
		Tracker: rt.Dependencies().OrbitTracker,
		Config:  cfg,
	}
	return NewServiceWithDependencies(deps)
}

// NewServiceWithDependencies wires an orbit service using explicit collaborators.
func NewServiceWithDependencies(deps Dependencies) (*Service, error) {
	if deps.Tracker == nil {
		return nil, fmt.Errorf("orbit: tracker dependency is required")
	}
	if deps.Config == nil {
		return nil, fmt.Errorf("orbit: config dependency is required")
	}
	return &Service{tracker: deps.Tracker, config: deps.Config}, nil
}

// Current returns the name of the currently active orbit.
func (s *Service) Current(ctx context.Context) (string, error) {
	return s.tracker.Current(ctx)
}

// Set changes the active orbit to the specified name.
func (s *Service) Set(ctx context.Context, name string) error {
	return s.tracker.Set(ctx, name)
}

// Sequence returns the ordered list of orbit names.
func (s *Service) Sequence(ctx context.Context) ([]string, error) {
	return s.tracker.Sequence(ctx)
}

// Record returns the orbit record for the given name.
func (s *Service) Record(ctx context.Context, name string) (*Record, error) {
	if s.config == nil {
		return nil, fmt.Errorf("orbit: configuration not loaded")
	}
	for _, o := range s.config.Orbits {
		if o.Name == name {
			return &Record{Name: o.Name, Label: o.Label, Color: o.Color}, nil
		}
	}
	return nil, fmt.Errorf("orbit %q not defined", name)
}

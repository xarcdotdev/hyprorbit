package orbit

import (
	"context"
	"fmt"

	"hypr-orbits/internal/runtime"
)

// Service wraps runtime dependencies required by orbit commands.
type Service struct {
	runtime *runtime.Runtime
}

// NewService creates a new orbit service from the given context.
func NewService(ctx context.Context) (*Service, error) {
	rt, err := runtime.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	return &Service{runtime: rt}, nil
}

// Current returns the name of the currently active orbit.
func (s *Service) Current(ctx context.Context) (string, error) {
	return s.runtime.Dependencies().OrbitTracker.Current(ctx)
}

// Set changes the active orbit to the specified name.
func (s *Service) Set(ctx context.Context, name string) error {
	return s.runtime.Dependencies().OrbitTracker.Set(ctx, name)
}

// Sequence returns the ordered list of orbit names.
func (s *Service) Sequence(ctx context.Context) ([]string, error) {
	return s.runtime.Dependencies().OrbitTracker.Sequence(ctx)
}

// Record returns the orbit record for the given name.
func (s *Service) Record(ctx context.Context, name string) (*Record, error) {
	cfg, err := s.runtime.Config(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range cfg.Orbits {
		if o.Name == name {
			return &Record{Name: o.Name, Label: o.Label, Color: o.Color}, nil
		}
	}
	return nil, fmt.Errorf("orbit %q not defined", name)
}
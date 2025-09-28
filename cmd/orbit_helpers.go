package cmd

import (
	"context"
	"fmt"

	"hypr-orbits/internal/runtime"
)

type orbitService struct {
	runtime *runtime.Runtime
}

func newOrbitService(ctx context.Context) (*orbitService, error) {
	rt, err := runtime.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	return &orbitService{runtime: rt}, nil
}

func (s *orbitService) currentOrbit(ctx context.Context) (string, error) {
	return s.runtime.Dependencies().OrbitTracker.Current(ctx)
}

func (s *orbitService) setOrbit(ctx context.Context, name string) error {
	return s.runtime.Dependencies().OrbitTracker.Set(ctx, name)
}

func (s *orbitService) sequence(ctx context.Context) ([]string, error) {
	return s.runtime.Dependencies().OrbitTracker.Sequence(ctx)
}

func (s *orbitService) orbitRecord(ctx context.Context, name string) (*orbitRecord, error) {
	cfg, err := s.runtime.Config(ctx)
	if err != nil {
		return nil, err
	}
	for _, orbit := range cfg.Orbits {
		if orbit.Name == name {
			return &orbitRecord{Name: orbit.Name, Label: orbit.Label, Color: orbit.Color}, nil
		}
	}
	return nil, fmt.Errorf("orbit %q not defined", name)
}

type orbitRecord struct {
	Name  string
	Label string
	Color string
}

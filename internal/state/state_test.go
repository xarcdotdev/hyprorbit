package state

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"hyprorbits/internal/config"
)

func TestManagerConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	override := filepath.Join(t.TempDir(), "state")
	mgr, err := NewManager(Options{
		OverridePath: override,
		Orbits: []config.OrbitRecord{
			{Name: "alpha"},
			{Name: "beta"},
		},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if _, err := mgr.Current(ctx); err != nil {
		t.Fatalf("initial Current: %v", err)
	}

	var wg sync.WaitGroup

	// Writers toggle between the configured orbits repeatedly.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			names := []string{"alpha", "beta"}
			for j := 0; j < 500; j++ {
				name := names[j%len(names)]
				if err := mgr.Set(ctx, name); err != nil {
					t.Errorf("Set(%q): %v", name, err)
					return
				}
			}
		}()
	}

	// Readers ensure the visible value is always one of the known orbits.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				current, err := mgr.Current(ctx)
				if err != nil {
					t.Errorf("Current(): %v", err)
					return
				}
				if current != "alpha" && current != "beta" {
					t.Errorf("Current() returned unexpected orbit %q", current)
					return
				}
			}
		}()
	}

	wg.Wait()
}

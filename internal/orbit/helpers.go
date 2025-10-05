package orbit

import (
	"context"
	"fmt"
	"strings"
)

// Provider defines the interface for accessing orbit information.
type Provider interface {
	ActiveOrbit(context.Context) (*Record, error)
}

// ActiveName returns the name of the active orbit.
func ActiveName(ctx context.Context, provider Provider) (string, error) {
	if provider == nil {
		return "", fmt.Errorf("active orbit not available")
	}
	record, err := provider.ActiveOrbit(ctx)
	if err != nil {
		return "", err
	}
	if record == nil {
		return "", fmt.Errorf("active orbit not available")
	}
	name := strings.TrimSpace(record.Name)
	if name == "" {
		return "", fmt.Errorf("active orbit not available")
	}
	return name, nil
}

package state

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"hypr-orbits/internal/config"
)

// Options configures the orbit state manager.
type Options struct {
	// OverridePath allows tests to control the state file location.
	OverridePath string
	// Orbits is the ordered list of configured orbit records.
	Orbits []config.OrbitRecord
}

// Manager provides serialized access to the orbit state file.
type Manager struct {
	opts Options

	path string

	once    sync.Once
	loadErr error

	mu      sync.Mutex
	current string

	// cachedNames retains the sequence for repeated access without recomputing.
	cachedNames []string
}

// NewManager constructs a Manager with resolved file paths.
func NewManager(opts Options) (*Manager, error) {
	if len(opts.Orbits) == 0 {
		return nil, fmt.Errorf("state: at least one orbit required to initialize manager")
	}

	path, err := resolvePath(opts.OverridePath)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(opts.Orbits))
	for _, orbit := range opts.Orbits {
		names = append(names, orbit.Name)
	}

	return &Manager{
		opts:        opts,
		path:        path,
		cachedNames: names,
	}, nil
}

// Current returns the active orbit name, loading it from disk at most once per process invocation.
func (m *Manager) Current(ctx context.Context) (string, error) {
	_ = ctx // reserved for future cancellation hooks when adding async caching.

	m.once.Do(func() {
		m.current, m.loadErr = m.load()
	})
	if m.loadErr != nil {
		return "", m.loadErr
	}
	return m.current, nil
}

// Set persists the active orbit name.
func (m *Manager) Set(ctx context.Context, name string) error {
	_ = ctx
	if !m.isValidOrbit(name) {
		return fmt.Errorf("state: unknown orbit %q", name)
	}

	if err := m.write(name); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.current = name
	return nil
}

// Sequence returns the configured orbit names in order.
func (m *Manager) Sequence(ctx context.Context) ([]string, error) {
	_ = ctx
	return append([]string(nil), m.cachedNames...), nil
}

func (m *Manager) load() (string, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return m.resetToDefault()
		}
		return "", fmt.Errorf("state: read %s: %w", m.path, err)
	}

	name := strings.TrimSpace(string(data))
	if name == "" || !m.isValidOrbit(name) {
		return m.resetToDefault()
	}

	return name, nil
}

func (m *Manager) resetToDefault() (string, error) {
	fallback := m.cachedNames[0]
	if err := m.write(fallback); err != nil {
		return "", err
	}
	return fallback, nil
}

func (m *Manager) write(name string) error {
	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("state: create dir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, "orbit-*.tmp")
	if err != nil {
		return fmt.Errorf("state: create temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName)
	}()

	if _, err := tmp.WriteString(name + "\n"); err != nil {
		return fmt.Errorf("state: write temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("state: sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("state: close temp file: %w", err)
	}

	if err := os.Rename(tmpName, m.path); err != nil {
		return fmt.Errorf("state: atomic rename: %w", err)
	}

	if err := os.Chmod(m.path, 0o644); err != nil && !errors.Is(err, fs.ErrPermission) {
		return fmt.Errorf("state: chmod %s: %w", m.path, err)
	}

	return nil
}

func (m *Manager) isValidOrbit(name string) bool {
	for _, candidate := range m.cachedNames {
		if candidate == name {
			return true
		}
	}
	return false
}

func resolvePath(override string) (string, error) {
	if override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("state: resolve override: %w", err)
		}
		return abs, nil
	}

	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("state: resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".local", "state")
	}

	return filepath.Join(base, "hypr-orbits", "orbit"), nil
}

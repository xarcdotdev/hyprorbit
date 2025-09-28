package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// LoaderOptions controls how configuration is located and parsed.
type LoaderOptions struct {
	OverridePath string
	FS           fs.FS
}

// Loader implements runtime.ConfigProvider.
type Loader struct {
	opts LoaderOptions

	once sync.Once
	cfg  *EffectiveConfig
	err  error
}

// NewLoader returns a new Loader.
func NewLoader(opts LoaderOptions) *Loader {
	return &Loader{opts: opts}
}

// Load parses the configuration, caching the result per-process.
func (l *Loader) Load(ctx context.Context) (*EffectiveConfig, error) {
	l.once.Do(func() {
		l.cfg, l.err = l.loadOnce(ctx)
	})
	return l.cfg, l.err
}

func (l *Loader) loadOnce(ctx context.Context) (*EffectiveConfig, error) {
	path, err := l.resolvePath()
	if err != nil {
		return nil, err
	}

	var data []byte

	if path == "" {
		// No file available; fall back to defaults.
		defaults := DefaultConfig()
		return BuildEffective("<defaults>", defaults)
	}

	data, err = l.readFile(path)
	if err != nil {
		return nil, err
	}

	var doc Config
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	return BuildEffective(path, &doc)
}

func (l *Loader) resolvePath() (string, error) {
	if l.opts.OverridePath != "" {
		abs, err := filepath.Abs(l.opts.OverridePath)
		if err != nil {
			return "", fmt.Errorf("config: resolve override: %w", err)
		}
		if _, err := os.Stat(abs); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("config: override %s not found", abs)
			}
			return "", fmt.Errorf("config: override %s: %w", abs, err)
		}
		return abs, nil
	}

	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}

	candidate := filepath.Join(base, "hyprorbits", "config.yaml")
	if _, err := os.Stat(candidate); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Intentionally return empty path to indicate defaults.
			return "", nil
		}
		return "", fmt.Errorf("config: stat %s: %w", candidate, err)
	}
	return candidate, nil
}

func (l *Loader) readFile(path string) ([]byte, error) {
	if l.opts.FS != nil {
		data, err := fs.ReadFile(l.opts.FS, path)
		if err == nil {
			return data, nil
		}
		// If the configured FS cannot read the absolute path, fall back to os.
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	return data, nil
}

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
	OverridePath       string
	WaybarOverridePath string
	FS                 fs.FS
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
	configPath, configDir, err := l.resolveConfigPath()
	if err != nil {
		return nil, err
	}

	source := "<defaults>"
	var doc *Config

	if configPath == "" {
		doc = DefaultConfig()
	} else {
		data, err := l.readFile(configPath)
		if err != nil {
			return nil, err
		}

		var parsed Config
		if err := yaml.Unmarshal(data, &parsed); err != nil {
			return nil, fmt.Errorf("config: parse %s: %w", configPath, err)
		}
		doc = &parsed
		source = configPath
	}

	if err := l.applyWaybarConfig(doc, configDir); err != nil {
		return nil, err
	}

	return BuildEffective(source, doc)
}

func (l *Loader) resolveConfigPath() (path string, dir string, err error) {
	if l.opts.OverridePath != "" {
		abs, err := filepath.Abs(l.opts.OverridePath)
		if err != nil {
			return "", "", fmt.Errorf("config: resolve override: %w", err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", "", fmt.Errorf("config: override %s not found", abs)
			}
			return "", "", fmt.Errorf("config: override %s: %w", abs, err)
		}
		if info.IsDir() {
			return "", "", fmt.Errorf("config: override %s is a directory", abs)
		}
		return abs, filepath.Dir(abs), nil
	}

	baseDir, err := l.defaultConfigDir()
	if err != nil {
		return "", "", err
	}

	configPath := filepath.Join(baseDir, "config.yaml")
	info, err := os.Stat(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", baseDir, nil
		}
		return "", "", fmt.Errorf("config: stat %s: %w", configPath, err)
	}
	if info.IsDir() {
		return "", "", fmt.Errorf("config: %s is a directory", configPath)
	}
	return configPath, baseDir, nil
}

func (l *Loader) defaultConfigDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: resolve home dir: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "hyprorbit"), nil
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

const waybarConfigFilename = "waybar.yaml"

func (l *Loader) applyWaybarConfig(doc *Config, configDir string) error {
	if doc == nil {
		return fmt.Errorf("config: internal error: nil config supplied")
	}

	path, err := l.resolveWaybarPath(configDir)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}

	data, err := l.readFile(path)
	if err != nil {
		return err
	}

	var waybarCfg WaybarConfig
	if err := yaml.Unmarshal(data, &waybarCfg); err != nil {
		return fmt.Errorf("config: parse %s: %w", path, err)
	}

	doc.Waybar = waybarCfg
	return nil
}

func (l *Loader) resolveWaybarPath(configDir string) (string, error) {
	if l.opts.WaybarOverridePath != "" {
		abs, err := filepath.Abs(l.opts.WaybarOverridePath)
		if err != nil {
			return "", fmt.Errorf("config: resolve waybar override: %w", err)
		}
		info, err := os.Stat(abs)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", fmt.Errorf("config: waybar override %s not found", abs)
			}
			return "", fmt.Errorf("config: waybar override %s: %w", abs, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("config: waybar override %s is a directory", abs)
		}
		return abs, nil
	}

	if configDir == "" {
		var err error
		configDir, err = l.defaultConfigDir()
		if err != nil {
			return "", err
		}
	}

	candidate := filepath.Join(configDir, waybarConfigFilename)
	info, err := os.Stat(candidate)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("config: stat %s: %w", candidate, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("config: %s is a directory", candidate)
	}
	return candidate, nil
}

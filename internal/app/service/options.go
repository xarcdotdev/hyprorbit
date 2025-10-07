package service

import (
	"fmt"
	"strings"
	"time"

	"hyprorbit/internal/module"
	"hyprorbit/internal/util"
)

const (
	defaultLogLevel  = "info"
	defaultLogFormat = "auto"
	defaultCacheTTL  = 150 * time.Millisecond
)

// Options captures configuration for the daemon server lifecycle.
type Options struct {
	ConfigPath   string
	SocketPath   string
	LogLevel     string
	LogFormat    string
	CacheTTL     time.Duration
	DisableCache bool
}

// normalize applies defaults and basic sanitation to the provided options.
func (o Options) normalize() Options {
	if strings.TrimSpace(o.LogLevel) == "" {
		o.LogLevel = defaultLogLevel
	}
	if strings.TrimSpace(o.LogFormat) == "" {
		o.LogFormat = defaultLogFormat
	}
	if o.DisableCache {
		o.CacheTTL = 0
	} else if o.CacheTTL <= 0 {
		o.CacheTTL = defaultCacheTTL
	}
	return o
}

// Validate ensures the option set is internally consistent.
func (o Options) Validate() error {
	if o.CacheTTL < 0 {
		return fmt.Errorf("service: cache TTL cannot be negative")
	}
	return nil
}

// EffectiveCacheTTL returns the cache duration after defaults and flags are applied.
func (o Options) EffectiveCacheTTL() time.Duration {
	if o.DisableCache {
		return 0
	}
	return o.CacheTTL
}

// moduleListFilterFromFlags extracts the filter value from request flags.
func moduleListFilterFromFlags(flags map[string]any) (string, error) {
	if len(flags) == 0 {
		return "all", nil
	}
	raw, ok := flags["filter"]
	if !ok {
		return "all", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("module list filter must be a string")
	}
	switch value {
	case "all", "active", "inactive":
		return value, nil
	default:
		return "", fmt.Errorf("module list filter %q not supported", value)
	}
}

// focusOptionsFromFlags extracts module focus options from request flags.
func focusOptionsFromFlags(flags map[string]any) (module.FocusOptions, error) {
	var opts module.FocusOptions
	if len(flags) == 0 {
		return opts, nil
	}
	if matcher, ok := flags["matcher"]; ok {
		str, ok := matcher.(string)
		if !ok {
			return opts, fmt.Errorf("module focus matcher must be a string")
		}
		opts.MatcherOverride = str
	}
	if cmd, ok := flags["cmd"]; ok {
		switch v := cmd.(type) {
		case []any:
			for _, raw := range v {
				str, ok := raw.(string)
				if !ok {
					return opts, fmt.Errorf("module focus cmd entries must be strings")
				}
				opts.CmdOverride = append(opts.CmdOverride, str)
			}
		case []string:
			opts.CmdOverride = append(opts.CmdOverride, v...)
		default:
			return opts, fmt.Errorf("module focus cmd must be an array of strings")
		}
	}
	if force, ok := flags["force_float"]; ok {
		b, err := util.ToBool(force)
		if err != nil {
			return opts, fmt.Errorf("module focus force_float must be boolean")
		}
		opts.ForceFloat = b
	}
	if noMove, ok := flags["no_move"]; ok {
		b, err := util.ToBool(noMove)
		if err != nil {
			return opts, fmt.Errorf("module focus no_move must be boolean")
		}
		opts.NoMove = b
	}
	if global, ok := flags["global"]; ok {
		b, err := util.ToBool(global)
		if err != nil {
			return opts, fmt.Errorf("module focus global must be boolean")
		}
		opts.Global = b
	}
	return opts, nil
}

// parseSilentFlag extracts the silent flag from request flags.
func parseSilentFlag(flags map[string]any) (bool, error) {
	if flags == nil {
		return false, nil
	}
	raw, ok := flags["silent"]
	if !ok {
		return false, nil
	}
	val, err := util.ToBool(raw)
	if err != nil {
		return false, fmt.Errorf("window move silent flag: %w", err)
	}
	return val, nil
}

// parseGlobalFlag extracts the global flag from request flags.
func parseGlobalFlag(flags map[string]any) (bool, error) {
	if flags == nil {
		return false, nil
	}
	raw, ok := flags["global"]
	if !ok {
		return false, nil
	}
	val, err := util.ToBool(raw)
	if err != nil {
		return false, fmt.Errorf("window move global flag: %w", err)
	}
	return val, nil
}

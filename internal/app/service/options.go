package service

import (
	"fmt"
	"strings"
	"time"
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

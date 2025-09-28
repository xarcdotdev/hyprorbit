package runtime

import (
	"context"
	"errors"
	"fmt"
	"os"

	"hypr-orbits/internal/config"
	"hypr-orbits/internal/hyprctl"
	"hypr-orbits/internal/state"
)

// Options carries process-wide configuration for a single CLI invocation.
type Options struct {
	ConfigPath string
	Verbose    bool
}

// Runtime holds shared dependencies for command execution.
type Runtime struct {
	options      Options
	dependencies Container
	config       *config.EffectiveConfig
}

// Container bundles interfaces required by command handlers.
type Container struct {
	ConfigProvider ConfigProvider
	OrbitTracker   OrbitTracker
	HyprctlClient  HyprctlClient
}

// ConfigProvider yields the effective configuration for the current run.
type ConfigProvider interface {
	Load(ctx context.Context) (*config.EffectiveConfig, error)
}

// OrbitTracker manages the active orbit state.
type OrbitTracker interface {
	Current(ctx context.Context) (string, error)
	Set(ctx context.Context, name string) error
	Sequence(ctx context.Context) ([]string, error)
}

// HyprctlClient interacts with the Hyprland CLI.
type HyprctlClient interface {
	Dispatch(ctx context.Context, args ...string) error
	Clients(ctx context.Context) ([]byte, error)
	DecodeClients(ctx context.Context, out any) error
}

// Bootstrap assembles the runtime dependencies for a CLI invocation.
func Bootstrap(ctx context.Context, opts Options) (*Runtime, error) {
	cfgLoader := config.NewLoader(config.LoaderOptions{OverridePath: opts.ConfigPath})

	cfg, err := cfgLoader.Load(ctx)
	if err != nil {
		return nil, err
	}

	for _, w := range cfg.Warnings {
		fmt.Fprintf(os.Stderr, "config warning: %s\n", w)
	}

	orbitManager, err := state.NewManager(state.Options{Orbits: cfg.Orbits})
	if err != nil {
		return nil, err
	}

	if _, err := orbitManager.Current(ctx); err != nil {
		return nil, err
	}

	client := hyprctl.NewClient(hyprctl.Options{
		Verbose: opts.Verbose,
	})

	deps := Container{
		ConfigProvider: cfgLoader,
		OrbitTracker:   orbitManager,
		HyprctlClient:  client,
	}

	return &Runtime{options: opts, dependencies: deps, config: cfg}, nil
}

// Options returns the runtime options.
func (r *Runtime) Options() Options {
	if r == nil {
		return Options{}
	}
	return r.options
}

// Dependencies exposes the dependency container.
func (r *Runtime) Dependencies() Container {
	if r == nil {
		return Container{}
	}
	return r.dependencies
}

// Config returns the validated configuration for the current invocation.
func (r *Runtime) Config(ctx context.Context) (*config.EffectiveConfig, error) {
	if r == nil {
		return nil, ErrRuntimeMissing
	}
	if r.config != nil {
		return r.config, nil
	}
	if r.dependencies.ConfigProvider == nil {
		return nil, fmt.Errorf("config provider missing")
	}
	cfg, err := r.dependencies.ConfigProvider.Load(ctx)
	if err != nil {
		return nil, err
	}
	r.config = cfg
	return cfg, nil
}

// ErrRuntimeMissing is returned when a command expects runtime state but none exists.
var ErrRuntimeMissing = errors.New("runtime not initialized")

// contextKey isolates our context storage.
type contextKey struct{}

// WithRuntime attaches the runtime to a context.
func WithRuntime(ctx context.Context, rt *Runtime) context.Context {
	return context.WithValue(ctx, contextKey{}, rt)
}

// FromContext retrieves the runtime from a context.
func FromContext(ctx context.Context) (*Runtime, error) {
	if ctx == nil {
		return nil, ErrRuntimeMissing
	}
	rt, ok := ctx.Value(contextKey{}).(*Runtime)
	if !ok || rt == nil {
		return nil, ErrRuntimeMissing
	}
	return rt, nil
}

// HasRuntime checks whether a context already carries runtime state.
func HasRuntime(ctx context.Context) bool {
	_, err := FromContext(ctx)
	return err == nil
}

// ExitCoder allows errors to surface process exit codes.
type ExitCoder interface {
	ExitCode() int
	error
}

// ErrorWithCode wraps an error with an exit code.
type ErrorWithCode struct {
	err  error
	code int
}

// Error implements the error interface.
func (e *ErrorWithCode) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

// Unwrap returns the wrapped error.
func (e *ErrorWithCode) Unwrap() error { return e.err }

// ExitCode returns the associated exit code.
func (e *ErrorWithCode) ExitCode() int { return e.code }

// WrapError annotates err with an exit code.
func WrapError(err error, code int) error {
	if err == nil {
		return nil
	}
	var cw ExitCoder
	if errors.As(err, &cw) {
		return err
	}
	return &ErrorWithCode{err: err, code: code}
}

// ExitCodeFromError extracts an exit code suitable for os.Exit.
func ExitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var coder ExitCoder
	if errors.As(err, &coder) {
		return coder.ExitCode()
	}
	return 1
}

// ErrNotImplemented signals placeholder functionality.
var ErrNotImplemented = WrapError(errors.New("not implemented"), 1)

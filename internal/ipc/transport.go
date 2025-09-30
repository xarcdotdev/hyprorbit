package ipc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultDialTimeout bounds how long the client waits for socket connections.
	DefaultDialTimeout = 150 * time.Millisecond

	socketEnvVar      = "HYPR_ORBITS_SOCKET"
	xdgRuntimeEnvVar  = "XDG_RUNTIME_DIR"
	defaultSocketName = "hyprorbit.sock"
)

// DialOptions instructs DialContext how to create the IPC connection.
type DialOptions struct {
	SocketPath string
	Timeout    time.Duration
}

// DialContext connects to the daemon's Unix socket, applying sensible defaults.
func DialContext(ctx context.Context, opts DialOptions) (net.Conn, error) {
	path, err := ResolveSocketPath(opts.SocketPath)
	if err != nil {
		return nil, err
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultDialTimeout
	}

	if _, ok := ctx.Deadline(); !ok && timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", path)
	if err != nil {
		if isDaemonOfflineError(err) {
			return nil, &DaemonOfflineError{Path: path, Cause: err}
		}
		return nil, fmt.Errorf("ipc: dial %s: %w", path, err)
	}
	return conn, nil
}

// DaemonOfflineError indicates the absence of a responsive hyprorbit daemon socket.
type DaemonOfflineError struct {
	Path  string
	Cause error
}

// Error implements the error interface.
func (e *DaemonOfflineError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("hyprorbit daemon is not running (expected socket at %s). Start it (e.g. `hyprorbitd`).", e.Path)
}

// Unwrap exposes the underlying dial failure (e.g. ENOENT, ECONNREFUSED).
func (e *DaemonOfflineError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func isDaemonOfflineError(err error) bool {
	return errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, os.ErrPermission) ||
		errors.Is(err, syscall.EPERM) ||
		errors.Is(err, syscall.ECONNREFUSED)
}

// ResolveSocketPath picks a socket location using explicit path, environment, then defaults.
func ResolveSocketPath(explicit string) (string, error) {
	if path := strings.TrimSpace(explicit); path != "" {
		return absolutize(path)
	}

	if envPath := strings.TrimSpace(os.Getenv(socketEnvVar)); envPath != "" {
		return absolutize(envPath)
	}

	if dir := strings.TrimSpace(os.Getenv(xdgRuntimeEnvVar)); dir != "" {
		return filepath.Join(dir, defaultSocketName), nil
	}

	return fallbackSocketPath(), nil
}

func absolutize(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("ipc: resolve socket path: %w", err)
	}
	return abs, nil
}

func fallbackSocketPath() string {
	return filepath.Join("/tmp", fmt.Sprintf("hyprorbit-%d.sock", os.Getuid()))
}

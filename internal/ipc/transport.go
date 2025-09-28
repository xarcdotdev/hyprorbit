package ipc

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultDialTimeout bounds how long the client waits for socket connections.
	DefaultDialTimeout = 150 * time.Millisecond

	socketEnvVar      = "HYPR_ORBITS_SOCKET"
	xdgRuntimeEnvVar  = "XDG_RUNTIME_DIR"
	defaultSocketName = "hypr-orbits.sock"
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
		return nil, fmt.Errorf("ipc: dial %s: %w", path, err)
	}
	return conn, nil
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
	return filepath.Join("/tmp", fmt.Sprintf("hypr-orbits-%d.sock", os.Getuid()))
}

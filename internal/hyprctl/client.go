package hyprctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Options configures the hyprctl client.
type Options struct {
	Verbose      bool
	Timeout      time.Duration
	CacheTTL     time.Duration
	DisableCache bool
}

// Client wraps hyprctl command execution with caching helpers.
type Client struct {
	opts Options

	cacheMu  sync.Mutex
	clients  []byte
	cachedAt time.Time
}

// NewClient constructs a hyprctl client with defaults.
func NewClient(opts Options) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = 500 * time.Millisecond
	}
	if opts.CacheTTL < 0 {
		opts.CacheTTL = 0
	}
	return &Client{opts: opts}
}

// Dispatch issues `hyprctl dispatch` with the provided arguments.
func (c *Client) Dispatch(ctx context.Context, args ...string) error {
	payload := append([]string{"dispatch"}, args...)
	return c.run(ctx, payload...)
}

// Clients returns the cached JSON output from `hyprctl clients -j`.
func (c *Client) Clients(ctx context.Context) ([]byte, error) {
	if c.opts.DisableCache || c.opts.CacheTTL == 0 {
		return c.fetchClients(ctx)
	}

	c.cacheMu.Lock()
	if len(c.clients) > 0 && time.Since(c.cachedAt) < c.opts.CacheTTL {
		data := append([]byte(nil), c.clients...)
		c.cacheMu.Unlock()
		return data, nil
	}
	c.cacheMu.Unlock()

	data, err := c.fetchClients(ctx)
	if err != nil {
		return nil, err
	}

	c.cacheMu.Lock()
	c.clients = append(c.clients[:0], data...)
	c.cachedAt = time.Now()
	c.cacheMu.Unlock()

	return append([]byte(nil), data...), nil
}

// DecodeClients unmarshals the clients JSON into the provided slice pointer.
func (c *Client) DecodeClients(ctx context.Context, out any) error {
	data, err := c.Clients(ctx)
	if err != nil {
		return err
	}
	return DecodeClientsPayload(data, out)
}

func (c *Client) run(ctx context.Context, args ...string) error {
	_, err := c.runCombined(ctx, args...)
	return err
}

func (c *Client) runCombined(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := withTimeout(ctx, c.opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hyprctl", args...) // #nosec G204 - controlled arguments

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, wrapCommandError(err, args, stderr.Bytes())
	}

	if c.opts.Verbose && stderr.Len() > 0 {
		fmt.Fprintf(os.Stderr, "[hyprctl stderr] %s\n", strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

func wrapCommandError(err error, args []string, stderr []byte) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("hyprctl timeout (%s): %w", strings.Join(args, " "), err)
	}
	if ee, ok := err.(*exec.ExitError); ok {
		msg := strings.TrimSpace(string(stderr))
		if msg == "" {
			msg = ee.Error()
		}
		return fmt.Errorf("hyprctl failed (%s): %s", strings.Join(args, " "), msg)
	}
	return fmt.Errorf("hyprctl exec (%s): %w", strings.Join(args, " "), err)
}

func withTimeout(parent context.Context, dur time.Duration) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if dur <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, dur)
}

// InvalidateClients clears any cached clients payload.
func (c *Client) InvalidateClients() {
	c.cacheMu.Lock()
	defer c.cacheMu.Unlock()
	c.clients = nil
	c.cachedAt = time.Time{}
}

func (c *Client) fetchClients(ctx context.Context) ([]byte, error) {
	data, err := c.runCombined(ctx, "clients", "-j")
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), data...), nil
}

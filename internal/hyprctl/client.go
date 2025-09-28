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

// BatchResult captures the output of a hyprctl batch command.
type BatchResult struct {
	Command string
	Output  []byte
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

// Workspaces returns the list of workspaces via `hyprctl workspaces -j`.
func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error) {
	data, err := c.runCombined(ctx, "workspaces", "-j")
	if err != nil {
		return nil, err
	}
	var out []Workspace
	if err := decodePayload(data, &out, "workspaces"); err != nil {
		return nil, err
	}
	if out == nil {
		return []Workspace{}, nil
	}
	return out, nil
}

// ActiveWorkspace returns the currently active workspace via `hyprctl activeworkspace -j`.
func (c *Client) ActiveWorkspace(ctx context.Context) (*Workspace, error) {
	data, err := c.runCombined(ctx, "activeworkspace", "-j")
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var ws Workspace
	if err := decodePayload(data, &ws, "active workspace"); err != nil {
		return nil, err
	}
	return &ws, nil
}

// ActiveWindow returns the currently focused window via `hyprctl activewindow -j`.
func (c *Client) ActiveWindow(ctx context.Context) (*Window, error) {
	data, err := c.runCombined(ctx, "activewindow", "-j")
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var win Window
	if err := decodePayload(data, &win, "active window"); err != nil {
		return nil, err
	}
	return &win, nil
}

// Monitors returns the monitor list via `hyprctl monitors -j`.
func (c *Client) Monitors(ctx context.Context) ([]Monitor, error) {
	data, err := c.runCombined(ctx, "monitors", "-j")
	if err != nil {
		return nil, err
	}
	var out []Monitor
	if err := decodePayload(data, &out, "monitors"); err != nil {
		return nil, err
	}
	if out == nil {
		return []Monitor{}, nil
	}
	return out, nil
}

// FocusWindow dispatches a focus request for the provided window address.
func (c *Client) FocusWindow(ctx context.Context, address string) error {
	return c.Dispatch(ctx, "focuswindow", "address:"+address)
}

// MoveToWorkspace moves a window to the target workspace via dispatch.
func (c *Client) MoveToWorkspace(ctx context.Context, windowAddr, workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("movetoworkspace: workspace name missing")
	}

	if addr := strings.TrimSpace(windowAddr); addr != "" {
		if err := c.FocusWindow(ctx, addr); err != nil {
			return fmt.Errorf("movetoworkspace: focus %s: %w", addr, err)
		}
	}

	return c.Dispatch(ctx, "movetoworkspacesilent", "name:"+workspace)
}

// SwitchWorkspace switches focus to the named workspace.
func (c *Client) SwitchWorkspace(ctx context.Context, workspace string) error {
	return c.Dispatch(ctx, "workspace", "name:"+workspace)
}

// Batch executes multiple hyprctl commands using `hyprctl --batch`.
func (c *Client) Batch(ctx context.Context, commands ...[]string) ([]BatchResult, error) {
	if len(commands) == 0 {
		return nil, nil
	}

	parts := make([]string, 0, len(commands))
	labels := make([]string, 0, len(commands))
	for _, cmd := range commands {
		if len(cmd) == 0 {
			continue
		}
		label := strings.Join(cmd, " ")
		labels = append(labels, label)
		parts = append(parts, label)
	}

	if len(parts) == 0 {
		return nil, nil
	}

	joined := strings.Join(parts, ";")
	output, err := c.runCombined(ctx, "--batch", joined)
	if err != nil {
		return nil, err
	}

	rows := bytes.Split(bytes.TrimRight(output, "\n"), []byte("\n"))
	results := make([]BatchResult, 0, len(labels))
	for i, label := range labels {
		var out []byte
		if i < len(rows) {
			out = append([]byte(nil), rows[i]...)
		}
		results = append(results, BatchResult{Command: label, Output: out})
	}
	return results, nil
}

// BatchDispatch batches multiple dispatch commands.
func (c *Client) BatchDispatch(ctx context.Context, dispatches ...[]string) ([]BatchResult, error) {
	if len(dispatches) == 0 {
		return nil, nil
	}
	commands := make([][]string, 0, len(dispatches))
	for _, args := range dispatches {
		cmd := append([]string{"dispatch"}, args...)
		commands = append(commands, cmd)
	}
	return c.Batch(ctx, commands...)
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

package hyprctl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
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
	opts   Options
	logger *log.Logger

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

// SetLogger sets the debug logger for this client.
func (c *Client) SetLogger(logger *log.Logger) {
	c.logger = logger
}

// debugf logs a debug message if debug logging is enabled.
func (c *Client) debugf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

// Dispatch issues `hyprctl dispatch` with the provided arguments.
func (c *Client) Dispatch(ctx context.Context, args ...string) error {
	if len(args) == 0 {
		return fmt.Errorf("dispatch: dispatcher missing")
	}
	parser, err := c.detectParser(ctx)
	if err != nil {
		return err
	}
	payload, err := serializeDispatch(parser, args)
	if err != nil {
		return err
	}
	out, err := c.runCombined(ctx, payload...)
	if err != nil {
		return err
	}
	return dispatchResult(out)
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
	c.debugf("ActiveWorkspace: querying active workspace")
	data, err := c.runCombined(ctx, "activeworkspace", "-j")
	if err != nil {
		c.debugf("ActiveWorkspace: query failed: %v", err)
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		c.debugf("ActiveWorkspace: no active workspace")
		return nil, nil
	}
	var ws Workspace
	if err := decodePayload(data, &ws, "active workspace"); err != nil {
		c.debugf("ActiveWorkspace: decode failed: %v", err)
		return nil, err
	}
	c.debugf("ActiveWorkspace: active workspace=%q (id=%d)", ws.Name, ws.ID)
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

// MoveToWorkspaceFollow moves a window to the target workspace and follows it.
func (c *Client) MoveToWorkspaceFollow(ctx context.Context, windowAddr, workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("movetoworkspace: workspace name missing")
	}

	if addr := strings.TrimSpace(windowAddr); addr != "" {
		if err := c.FocusWindow(ctx, addr); err != nil {
			return fmt.Errorf("movetoworkspace: focus %s: %w", addr, err)
		}
	}

	return c.Dispatch(ctx, "movetoworkspace", "name:"+workspace)
}

// MoveToWorkspaceSilent moves a window to the target workspace without following it.
func (c *Client) MoveToWorkspaceSilent(ctx context.Context, windowAddr, workspace string) error {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return fmt.Errorf("movetoworkspacesilent: workspace name missing")
	}

	if addr := strings.TrimSpace(windowAddr); addr != "" {
		if err := c.FocusWindow(ctx, addr); err != nil {
			return fmt.Errorf("movetoworkspacesilent: focus %s: %w", addr, err)
		}
	}

	return c.Dispatch(ctx, "movetoworkspacesilent", "name:"+workspace)
}

// SwitchWorkspace switches focus to the named workspace.
func (c *Client) SwitchWorkspace(ctx context.Context, workspace string) error {
	c.debugf("SwitchWorkspace: switching to workspace=%q", workspace)
	err := c.Dispatch(ctx, "workspace", "name:"+workspace)
	if err != nil {
		c.debugf("SwitchWorkspace: failed to switch to workspace=%q: %v", workspace, err)
		return err
	}
	c.debugf("SwitchWorkspace: successfully switched to workspace=%q", workspace)
	return nil
}

// Batch executes multiple hyprctl commands using `hyprctl --batch`.
func (c *Client) Batch(ctx context.Context, commands ...[]string) ([]BatchResult, error) {
	if len(commands) == 0 {
		return nil, nil
	}

	active := make([][]string, 0, len(commands))
	mutation := false
	for _, cmd := range commands {
		if len(cmd) == 0 {
			continue
		}
		active = append(active, cmd)
		if !batchQuery(strings.TrimPrefix(cmd[0], "j/")) {
			mutation = true
		}
	}
	if len(active) == 0 {
		return nil, nil
	}
	parser := legacyParser
	if mutation {
		var err error
		parser, err = c.detectParser(ctx)
		if err != nil {
			return nil, err
		}
	}
	parts := make([]string, 0, len(active))
	labels := make([]string, 0, len(active))
	// Serialize and validate the entire batch before sending any command.
	for _, cmd := range active {
		labels = append(labels, strings.Join(cmd, " "))
		wire := cmd
		if cmd[0] == "dispatch" {
			var err error
			wire, err = serializeDispatch(parser, cmd[1:])
			if err != nil {
				return nil, err
			}
		} else if cmd[0] == "keyword" && parser == luaParser {
			return nil, fmt.Errorf("batch: keyword unsupported with Lua config manager")
		}
		if err := safeBatchCommand(wire); err != nil {
			return nil, err
		}
		parts = append(parts, strings.Join(wire, " "))
	}
	output, err := c.runCombined(ctx, "--batch", strings.Join(parts, ";"))
	if err != nil {
		return nil, err
	}
	// Hyprland separates replies with three newlines, not individual lines.
	rows := bytes.Split(output, []byte("\n\n\n"))
	// Some versions append the separator after the final reply.
	if len(rows) == len(active)+1 && len(bytes.TrimSpace(rows[len(rows)-1])) == 0 {
		rows = rows[:len(rows)-1]
	}
	if len(rows) != len(active) {
		return nil, fmt.Errorf("batch: expected %d replies, got %d", len(active), len(rows))
	}
	results := make([]BatchResult, 0, len(active))
	for i, cmd := range active {
		results = append(results, BatchResult{Command: labels[i], Output: append([]byte(nil), rows[i]...)})
		if cmd[0] == "dispatch" {
			if err := dispatchResult(rows[i]); err != nil {
				return nil, fmt.Errorf("batch command %d (%s): %w", i+1, labels[i], err)
			}
		}
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

func (c *Client) runCombined(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := withTimeout(ctx, c.opts.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "hyprctl", args...) // #nosec G204 - controlled arguments

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			err = errors.Join(ctx.Err(), err)
		}
		diagnostics := append(append([]byte(nil), stdout.Bytes()...), stderr.Bytes()...)
		return nil, wrapCommandError(err, args, diagnostics)
	}

	if c.opts.Verbose && stderr.Len() > 0 {
		fmt.Fprintf(os.Stderr, "[hyprctl stderr] %s\n", strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

func wrapCommandError(err error, args []string, diagnostics []byte) error {
	kind := "failed"
	if errors.Is(err, context.DeadlineExceeded) {
		kind = "timeout"
	}
	if errors.Is(err, context.Canceled) {
		kind = "canceled"
	}
	return fmt.Errorf("hyprctl %s (%s): %s: %w", kind, strings.Join(args, " "), strings.TrimSpace(string(diagnostics)), err)
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

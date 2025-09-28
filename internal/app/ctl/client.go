package ctl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"hypr-orbits/internal/ipc"
	"hypr-orbits/internal/module"
	"hypr-orbits/internal/orbit"
)

// Options configures the behaviour of the control client.
type Options struct {
	SocketPath string
	JSON       bool
	Quiet      bool
	Timeout    time.Duration
}

// Client speaks the IPC protocol with the hypr-orbits daemon.
type Client struct {
	opts Options
}

// NewClient initialises an IPC client with the given options.
func NewClient(opts Options) *Client {
	return &Client{opts: opts}
}

// Options exposes the effective client options.
func (c *Client) Options() Options {
	return c.opts
}

// Call issues an IPC request and optionally decodes the response payload into target.
func (c *Client) Call(ctx context.Context, req ipc.Request, target any) (*ipc.Response, error) {
	conn, err := ipc.DialContext(ctx, ipc.DialOptions{SocketPath: c.opts.SocketPath, Timeout: c.opts.Timeout})
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	if err := enc.Encode(&req); err != nil {
		return nil, fmt.Errorf("ipc: send request: %w", err)
	}

	var resp ipc.Response
	if err := dec.Decode(&resp); err != nil {
		return nil, fmt.Errorf("ipc: decode response: %w", err)
	}

	if resp.Version != ipc.Version {
		return &resp, &Error{Message: fmt.Sprintf("protocol mismatch: expected %d, got %d", ipc.Version, resp.Version), Code: 1}
	}

	if !resp.Success {
		msg := resp.Error
		if msg == "" {
			msg = "daemon returned an error"
		}
		return &resp, &Error{Message: msg, Code: resp.ExitCode}
	}

	if target != nil && len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, target); err != nil {
			return &resp, fmt.Errorf("ipc: decode payload: %w", err)
		}
	}

	return &resp, nil
}

// OrbitGet fetches the current orbit record.
func (c *Client) OrbitGet(ctx context.Context) (*orbit.Record, error) {
	req := ipc.NewRequest("orbit", "get")
	var rec orbit.Record
	if _, err := c.Call(ctx, req, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// OrbitNext switches to the next orbit in sequence.
func (c *Client) OrbitNext(ctx context.Context) (*orbit.Record, error) {
	req := ipc.NewRequest("orbit", "next")
	var rec orbit.Record
	if _, err := c.Call(ctx, req, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// OrbitPrev switches to the previous orbit in sequence.
func (c *Client) OrbitPrev(ctx context.Context) (*orbit.Record, error) {
	req := ipc.NewRequest("orbit", "prev")
	var rec orbit.Record
	if _, err := c.Call(ctx, req, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// OrbitSet selects a specific orbit by name.
func (c *Client) OrbitSet(ctx context.Context, name string) (*orbit.Record, error) {
	req := ipc.NewRequest("orbit", "set")
	req.Args = []string{name}
	var rec orbit.Record
	if _, err := c.Call(ctx, req, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// ModuleFocusOptions customises the module focus request.
type ModuleFocusOptions struct {
	Matcher    string
	Command    []string
	ForceFloat bool
	NoMove     bool
}

// ModuleFocus performs focus-or-launch for the given module.
func (c *Client) ModuleFocus(ctx context.Context, moduleName string, opts ModuleFocusOptions) (*module.Result, error) {
	req := ipc.NewRequest("module", "focus")
	req.Args = []string{moduleName}
	flags := map[string]any{}
	if opts.Matcher != "" {
		flags["matcher"] = opts.Matcher
	}
	if len(opts.Command) > 0 {
		flags["cmd"] = opts.Command
	}
	if opts.ForceFloat {
		flags["force_float"] = true
	}
	if opts.NoMove {
		flags["no_move"] = true
	}
	if len(flags) > 0 {
		req.Flags = flags
	}

	var res module.Result
	if _, err := c.Call(ctx, req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ModuleJump switches to the module workspace within the active orbit.
func (c *Client) ModuleJump(ctx context.Context, moduleName string) (*module.Result, error) {
	req := ipc.NewRequest("module", "jump")
	req.Args = []string{moduleName}
	var res module.Result
	if _, err := c.Call(ctx, req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ModuleSeed sequences the configured seed steps for a module.
func (c *Client) ModuleSeed(ctx context.Context, moduleName string) ([]*module.Result, error) {
	req := ipc.NewRequest("module", "seed")
	req.Args = []string{moduleName}
	var results []*module.Result
	if _, err := c.Call(ctx, req, &results); err != nil {
		return nil, err
	}
	if results == nil {
		results = []*module.Result{}
	}
	return results, nil
}

// ModuleList retrieves workspace summaries for configured and active workspaces.
func (c *Client) ModuleList(ctx context.Context, filter string) ([]module.WorkspaceSummary, error) {
	req := ipc.NewRequest("module", "list")
	if filter != "" {
		req.Flags = map[string]any{"filter": filter}
	}
	var summaries []module.WorkspaceSummary
	if _, err := c.Call(ctx, req, &summaries); err != nil {
		return nil, err
	}
	if summaries == nil {
		summaries = []module.WorkspaceSummary{}
	}
	return summaries, nil
}

// DaemonReload instructs the daemon to reload its configuration.
func (c *Client) DaemonReload(ctx context.Context) error {
	req := ipc.NewRequest("daemon", "reload")
	_, err := c.Call(ctx, req, nil)
	return err
}

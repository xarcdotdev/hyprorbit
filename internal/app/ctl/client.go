package ctl

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"hyprorbit/internal/ipc"
	"hyprorbit/internal/module"
	"hyprorbit/internal/orbit"
)

// Options configures the behaviour of the control client.
type Options struct {
	SocketPath string
	JSON       bool
	Quiet      bool
	Timeout    time.Duration
	NoColor    bool
	ConfigPath string
}

// Client speaks the IPC protocol with the hyprorbit daemon.
type Client struct {
	opts Options
}

type streamReadCloser struct {
	net.Conn
	once sync.Once
}

var dialIPC = ipc.DialContext

func (s *streamReadCloser) Close() error {
	var err error
	s.once.Do(func() {
		err = s.Conn.Close()
	})
	return err
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
	conn, err := dialIPC(ctx, ipc.DialOptions{SocketPath: c.opts.SocketPath, Timeout: c.opts.Timeout})
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
	return c.moduleJump(ctx, "jump", []string{moduleName})
}

// ModuleJumpNext cycles to the next module workspace within the active orbit.
func (c *Client) ModuleJumpNext(ctx context.Context) (*module.Result, error) {
	return c.moduleJump(ctx, "jump-next", nil)
}

// ModuleJumpPrev cycles to the previous module workspace within the active orbit.
func (c *Client) ModuleJumpPrev(ctx context.Context) (*module.Result, error) {
	return c.moduleJump(ctx, "jump-prev", nil)
}

// ModuleJumpCreate creates a temporary workspace in the active orbit and switches to it.
func (c *Client) ModuleJumpCreate(ctx context.Context) (*module.Result, error) {
	return c.moduleJump(ctx, "jump-create", nil)
}

func (c *Client) moduleJump(ctx context.Context, action string, args []string) (*module.Result, error) {
	req := ipc.NewRequest("module", action)
	if len(args) > 0 {
		req.Args = append(req.Args, args...)
	}
	var res module.Result
	if _, err := c.Call(ctx, req, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

// ModuleWatch opens a streaming status feed for module/orbit snapshots.
func (c *Client) ModuleWatch(ctx context.Context) (io.ReadCloser, error) {
	conn, err := dialIPC(ctx, ipc.DialOptions{SocketPath: c.opts.SocketPath, Timeout: c.opts.Timeout})
	if err != nil {
		return nil, err
	}

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)

	req := ipc.NewRequest("module", "status-stream")
	if err := enc.Encode(&req); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ipc: send request: %w", err)
	}

	var resp ipc.Response
	if err := dec.Decode(&resp); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ipc: decode response: %w", err)
	}

	if resp.Version != ipc.Version {
		conn.Close()
		return nil, &Error{Message: fmt.Sprintf("protocol mismatch: expected %d, got %d", ipc.Version, resp.Version), Code: 1}
	}

	if !resp.Success {
		msg := resp.Error
		if msg == "" {
			msg = "daemon returned an error"
		}
		conn.Close()
		return nil, &Error{Message: msg, Code: resp.ExitCode}
	}

	if !resp.Streaming {
		conn.Close()
		return nil, fmt.Errorf("ipc: daemon response is not streaming")
	}

	stream := &streamReadCloser{Conn: conn}
	if ctx != nil {
		go func() {
			<-ctx.Done()
			_ = stream.Close()
		}()
	}

	return stream, nil
}

// ModuleGet returns metadata about the currently active module workspace.
func (c *Client) ModuleGet(ctx context.Context) (*module.Status, error) {
	req := ipc.NewRequest("module", "get")
	var status module.Status
	if _, err := c.Call(ctx, req, &status); err != nil {
		return nil, err
	}
	return &status, nil
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

// WorkspaceReset instructs the daemon to reset active workspaces.
func (c *Client) WorkspaceReset(ctx context.Context) error {
	req := ipc.NewRequest("module", "workspace-reset")
	_, err := c.Call(ctx, req, nil)
	return err
}

// WorkspaceAlign moves focus to the first configured module/orbit workspace.
func (c *Client) WorkspaceAlign(ctx context.Context) error {
	req := ipc.NewRequest("module", "workspace-align")
	_, err := c.Call(ctx, req, nil)
	return err
}

// DaemonStatus checks whether the daemon is responsive.
func (c *Client) DaemonStatus(ctx context.Context) error {
	req := ipc.NewRequest("daemon", "status")
	_, err := c.Call(ctx, req, nil)
	return err
}

// DaemonReload instructs the daemon to reload its configuration.
func (c *Client) DaemonReload(ctx context.Context) error {
	req := ipc.NewRequest("daemon", "reload")
	_, err := c.Call(ctx, req, nil)
	return err
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"hyprorbits/internal/ipc"
)

// Server hosts the hyprorbits daemon lifecycle.
type Server struct {
	opts       Options
	state      *DaemonState
	dispatcher *Dispatcher

	mu       sync.Mutex
	listener net.Listener
	wg       sync.WaitGroup
}

// NewServer constructs and initializes the daemon server.
func NewServer(ctx context.Context, opts Options) (*Server, error) {
	opts = opts.normalize()
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	daemonState, err := NewDaemonState(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &Server{
		opts:       opts,
		state:      daemonState,
		dispatcher: NewDispatcher(daemonState),
	}, nil
}

// Serve listens on the configured socket and processes requests until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	if err := s.state.Start(ctx); err != nil {
		return err
	}
	defer s.state.Stop()

	path, err := ipc.ResolveSocketPath(s.opts.SocketPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("service: create socket dir: %w", err)
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("service: remove stale socket: %w", err)
	}

	listener, err := net.Listen("unix", path)
	if err != nil {
		return fmt.Errorf("service: listen on %s: %w", path, err)
	}

	if err := os.Chmod(path, 0o660); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = listener.Close()
		return fmt.Errorf("service: chmod socket: %w", err)
	}

	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		if s.listener != nil {
			s.listener.Close()
		}
		s.listener = nil
		s.mu.Unlock()
		os.Remove(path)
	}()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		if s.listener != nil {
			s.listener.Close()
		}
		s.mu.Unlock()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			if ctx.Err() != nil {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			return fmt.Errorf("service: accept: %w", err)
		}

		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			s.handleConnection(ctx, c)
		}(conn)
	}
}

// Shutdown closes the listener and waits for in-flight handlers to complete.
func (s *Server) Shutdown(ctx context.Context) error {
	s.state.Stop()

	s.mu.Lock()
	if s.listener != nil {
		s.listener.Close()
	}
	s.listener = nil
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()

	deadline := time.Now().Add(500 * time.Millisecond)
	_ = conn.SetDeadline(deadline)

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	var req ipc.Request
	if err := decoder.Decode(&req); err != nil {
		resp := ipc.NewResponse(false)
		resp.Error = fmt.Sprintf("decode request: %v", err)
		resp.ExitCode = 1
		encoder.Encode(resp)
		return
	}

	resp, stream, err := s.dispatcher.Handle(ctx, req)
	if err != nil {
		resp.Success = false
		resp.Error = err.Error()
		if resp.ExitCode == 0 {
			resp.ExitCode = 1
		}
	}

	if err := encoder.Encode(resp); err != nil {
		return
	}

	if resp.Streaming && stream != nil {
		_ = conn.SetDeadline(time.Time{})
		if err := stream(ctx, conn); err != nil && !errors.Is(err, context.Canceled) {
			s.state.Logf("stream handler: %v", err)
		}
	}
}

// State exposes the daemon state (primarily for testing hooks).
func (s *Server) State() *DaemonState {
	return s.state
}

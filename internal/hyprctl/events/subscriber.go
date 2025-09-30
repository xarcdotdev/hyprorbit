package events

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	// ErrSignatureMissing is returned when HYPRLAND_INSTANCE_SIGNATURE cannot be determined.
	ErrSignatureMissing = errors.New("hyprland instance signature missing")
	// ErrSocketNotFound is returned when no viable socket path could be located.
	ErrSocketNotFound = errors.New("hyprland event socket not found")
)

type dialContextFunc func(ctx context.Context, network, address string) (net.Conn, error)

const (
	defaultBufferSize     = 32
	defaultInitialBackoff = 100 * time.Millisecond
	defaultMaxBackoff     = 5 * time.Second
	defaultDialTimeout    = time.Second
	scannerMaxTokenSize   = 256 * 1024
)

// PathOptions configure how the Hyprland event socket path should be resolved.
type PathOptions struct {
	SocketPath string
	Signature  string
	RuntimeDir string
	CacheDir   string
	HomeDir    string
}

// Options configure the behavior of the event subscriber.
type Options struct {
	PathOptions

	BufferSize     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	DialTimeout    time.Duration

	Dialer dialContextFunc
	Logf   func(format string, args ...any)
}

// Subscriber maintains a streaming connection to Hyprland's event socket.
type Subscriber struct {
	pathOpts       PathOptions
	logf           func(string, ...any)
	dialer         dialContextFunc
	bufferSize     int
	initialBackoff time.Duration
	maxBackoff     time.Duration
	dialTimeout    time.Duration

	events chan Event
	errs   chan error

	start sync.Once
}

// NewSubscriber constructs a subscriber with sane defaults.
func NewSubscriber(opts Options) (*Subscriber, error) {
	if opts.BufferSize <= 0 {
		opts.BufferSize = defaultBufferSize
	}
	if opts.InitialBackoff <= 0 {
		opts.InitialBackoff = defaultInitialBackoff
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = defaultMaxBackoff
	}
	if opts.MaxBackoff < opts.InitialBackoff {
		opts.MaxBackoff = opts.InitialBackoff
	}
	if opts.DialTimeout <= 0 {
		opts.DialTimeout = defaultDialTimeout
	}
	if opts.Logf == nil {
		opts.Logf = func(string, ...any) {}
	}

	dialer := opts.Dialer
	if dialer == nil {
		base := &net.Dialer{}
		dialer = base.DialContext
	}

	sub := &Subscriber{
		pathOpts:       opts.PathOptions,
		logf:           opts.Logf,
		dialer:         dialer,
		bufferSize:     opts.BufferSize,
		initialBackoff: opts.InitialBackoff,
		maxBackoff:     opts.MaxBackoff,
		dialTimeout:    opts.DialTimeout,
		events:         make(chan Event, opts.BufferSize),
		errs:           make(chan error, opts.BufferSize),
	}
	return sub, nil
}

// Start launches the subscriber loop. Safe to call multiple times; only the first invocation has an effect.
func (s *Subscriber) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	s.start.Do(func() {
		go s.run(ctx)
	})
}

// Events exposes the read-only event channel for consumers.
func (s *Subscriber) Events() <-chan Event {
	return s.events
}

// Errors exposes non-fatal errors encountered while streaming events.
func (s *Subscriber) Errors() <-chan error {
	return s.errs
}

func (s *Subscriber) run(ctx context.Context) {
	defer close(s.events)
	defer close(s.errs)

	backoff := s.initialBackoff

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		path, err := ResolveSocketPath(s.pathOpts)
		if err != nil {
			s.reportError(err)
			if !s.wait(ctx, backoff) {
				return
			}
			backoff = s.nextBackoff(backoff)
			continue
		}

		conn, err := s.dial(ctx, path)
		if err != nil {
			s.reportError(fmt.Errorf("connect %s: %w", path, err))
			if !s.wait(ctx, backoff) {
				return
			}
			backoff = s.nextBackoff(backoff)
			continue
		}

		s.logf("hyprctl events: connected to %s", path)
		backoff = s.initialBackoff

		err = s.consume(ctx, conn)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil && !errors.Is(err, io.EOF) {
			s.reportError(err)
		}
		if ctx.Err() != nil {
			return
		}

		if !s.wait(ctx, s.initialBackoff) {
			return
		}
	}
}

func (s *Subscriber) consume(ctx context.Context, conn net.Conn) error {
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-done:
		}
	}()

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), scannerMaxTokenSize)

	for scanner.Scan() {
		line := scanner.Text()
		event, err := ParseEvent(line)
		if err != nil {
			s.reportError(err)
			continue
		}
		s.publish(event)
	}

	close(done)

	if err := scanner.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("events stream read: %w", err)
	}

	if ctx.Err() != nil {
		return ctx.Err()
	}
	return io.EOF
}

func (s *Subscriber) publish(ev Event) {
	select {
	case s.events <- ev:
		return
	default:
	}

	s.reportError(fmt.Errorf("dropping event %q due to slow consumer", ev.Type))

	select {
	case <-s.events:
	default:
	}

	select {
	case s.events <- ev:
	default:
	}
}

func (s *Subscriber) dial(ctx context.Context, path string) (net.Conn, error) {
	dctx, cancel := context.WithTimeout(ctx, s.dialTimeout)
	defer cancel()
	return s.dialer(dctx, "unix", path)
}

func (s *Subscriber) wait(ctx context.Context, dur time.Duration) bool {
	if dur <= 0 {
		return true
	}
	timer := time.NewTimer(dur)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *Subscriber) nextBackoff(current time.Duration) time.Duration {
	doubled := current * 2
	if doubled <= 0 {
		doubled = current
	}
	if doubled > s.maxBackoff {
		return s.maxBackoff
	}
	if doubled < s.initialBackoff {
		return s.initialBackoff
	}
	return doubled
}

func (s *Subscriber) reportError(err error) {
	if err == nil {
		return
	}
	s.logf("hyprctl events: %v", err)
	select {
	case s.errs <- err:
	default:
	}
}

// ResolveSocketPath determines the Hyprland event socket path using common conventions.
func ResolveSocketPath(opts PathOptions) (string, error) {
	if opts.SocketPath != "" {
		return opts.SocketPath, nil
	}

	signature := strings.TrimSpace(opts.Signature)
	if signature == "" {
		signature = strings.TrimSpace(os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"))
	}
	if signature == "" {
		return "", ErrSignatureMissing
	}

	candidates := candidatePaths(opts, signature)
	for _, path := range candidates {
		info, err := os.Stat(path)
		if err == nil {
			if info.Mode()&os.ModeSocket != 0 || info.Mode().IsRegular() {
				return path, nil
			}
			// If the file exists but is neither socket nor regular file, continue.
			continue
		}
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		// Permission-denied or other errors: assume the path is valid and let dial report details.
		return path, nil
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("%w: no candidate paths for signature %q", ErrSocketNotFound, signature)
	}

	return "", fmt.Errorf("%w: tried %s", ErrSocketNotFound, strings.Join(candidates, ", "))
}

func candidatePaths(opts PathOptions, signature string) []string {
	var paths []string

	runtimeDir := strings.TrimSpace(opts.RuntimeDir)
	if runtimeDir == "" {
		runtimeDir = strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	}
	if runtimeDir != "" {
		paths = append(paths, filepath.Join(runtimeDir, "hypr", signature, ".socket2.sock"))
	}

	cacheDir := strings.TrimSpace(opts.CacheDir)
	if cacheDir == "" {
		home := strings.TrimSpace(opts.HomeDir)
		if home == "" {
			if h, err := os.UserHomeDir(); err == nil {
				home = h
			}
		}
		if home != "" {
			cacheDir = filepath.Join(home, ".cache")
		}
	}
	if cacheDir != "" {
		paths = append(paths, filepath.Join(cacheDir, "hypr", signature, "hyprland.sock2"))
	}

	return paths
}

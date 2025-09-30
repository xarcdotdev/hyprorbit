package events

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestResolveSocketPathPrefersRuntimeDir(t *testing.T) {

	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	tmp := t.TempDir()
	signature := "abc123"
	socketPath := filepath.Join(tmp, "hypr", signature, ".socket2.sock")
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(socketPath, []byte{}, 0o600); err != nil {
		t.Fatalf("touch socket: %v", err)
	}

	path, err := ResolveSocketPath(PathOptions{Signature: signature, RuntimeDir: tmp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != socketPath {
		t.Fatalf("expected %q, got %q", socketPath, path)
	}
}

func TestResolveSocketPathFallsBackToCache(t *testing.T) {

	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	tmp := t.TempDir()
	signature := "def456"
	cachePath := filepath.Join(tmp, "hypr", signature, "hyprland.sock2")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte{}, 0o600); err != nil {
		t.Fatalf("touch socket: %v", err)
	}

	path, err := ResolveSocketPath(PathOptions{Signature: signature, CacheDir: tmp})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != cachePath {
		t.Fatalf("expected %q, got %q", cachePath, path)
	}
}

func TestResolveSocketPathMissingSignature(t *testing.T) {
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")

	if _, err := ResolveSocketPath(PathOptions{}); !errors.Is(err, ErrSignatureMissing) {
		t.Fatalf("expected ErrSignatureMissing, got %v", err)
	}
}

func TestResolveSocketPathMissingFiles(t *testing.T) {
	t.Setenv("HYPRLAND_INSTANCE_SIGNATURE", "")
	t.Setenv("XDG_RUNTIME_DIR", "")

	signature := "zzz"
	_, err := ResolveSocketPath(PathOptions{Signature: signature, RuntimeDir: t.TempDir(), CacheDir: t.TempDir()})
	if !errors.Is(err, ErrSocketNotFound) {
		t.Fatalf("expected ErrSocketNotFound, got %v", err)
	}
}

func TestSubscriberReconnects(t *testing.T) {
	dialer := newPipeDialer(2)
	sub, err := NewSubscriber(Options{
		PathOptions:    PathOptions{SocketPath: "/tmp/test.sock"},
		BufferSize:     4,
		InitialBackoff: time.Millisecond,
		MaxBackoff:     5 * time.Millisecond,
		DialTimeout:    50 * time.Millisecond,
		Dialer:         dialer.DialContext,
		Logf:           func(string, ...any) {},
	})
	if err != nil {
		t.Fatalf("new subscriber: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sub.Start(ctx)
	eventsCh := sub.Events()

	if !dialer.WaitDial(0, time.Second) {
		t.Fatalf("first dial did not occur")
	}

	srv0 := dialer.Server(0)
	defer srv0.Close()

	fmt.Fprintln(srv0, "workspace>>1")

	select {
	case ev := <-eventsCh:
		if ev.Type != TypeWorkspace || ev.Workspace == nil || ev.Workspace.ID != "1" {
			t.Fatalf("unexpected event: %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for first event")
	}

	srv0.Close()

	if !dialer.WaitDial(1, time.Second) {
		t.Fatalf("second dial did not occur")
	}

	srv1 := dialer.Server(1)
	defer srv1.Close()

	fmt.Fprintln(srv1, "workspacev2>>2,dev")

	select {
	case ev := <-eventsCh:
		if ev.Type != TypeWorkspaceV2 || ev.Workspace == nil || ev.Workspace.Name != "dev" {
			t.Fatalf("unexpected event after reconnect: %#v", ev)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for second event")
	}
}

type pipeDialer struct {
	mu      sync.Mutex
	servers []net.Conn
	clients []net.Conn
	dialCh  chan int
	idx     int
}

func newPipeDialer(count int) *pipeDialer {
	servers := make([]net.Conn, count)
	clients := make([]net.Conn, count)
	for i := 0; i < count; i++ {
		client, server := net.Pipe()
		clients[i] = client
		servers[i] = server
	}
	return &pipeDialer{
		servers: servers,
		clients: clients,
		dialCh:  make(chan int, count),
	}
}

func (p *pipeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.idx >= len(p.clients) {
		return nil, fmt.Errorf("no connections available")
	}
	conn := p.clients[p.idx]
	p.dialCh <- p.idx
	p.idx++
	return conn, nil
}

func (p *pipeDialer) Server(index int) net.Conn {
	return p.servers[index]
}

func (p *pipeDialer) WaitDial(index int, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case got := <-p.dialCh:
			if got == index {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

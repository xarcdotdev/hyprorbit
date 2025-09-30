package service

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"hyprorbit/internal/ipc"
	"hyprorbit/internal/module"
	"hyprorbit/internal/orbit"
)

func TestDispatcherModuleStatusStream(t *testing.T) {
	state := &DaemonState{
		broadcaster: NewStatusBroadcaster(0),
		moduleSvc:   &module.Service{},
		logger:      func(string, ...any) {},
	}

	dispatcher := NewDispatcher(state)

	req := ipc.Request{Version: ipc.Version, Command: "module", Action: "status-stream"}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, stream, err := dispatcher.Handle(ctx, req)
	if err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	if !resp.Success || !resp.Streaming {
		t.Fatalf("expected streaming success response, got %+v", resp)
	}
	if stream == nil {
		t.Fatalf("expected non-nil stream handler")
	}

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	done := make(chan error, 1)
	go func() {
		done <- stream(ctx, serverConn)
	}()

	snapshot := StatusSnapshot{
		Workspace: "code-alpha",
		Module:    "code",
		Orbit:     &orbit.Record{Name: "alpha"},
	}

	// Give the handler a moment to subscribe before publishing.
	time.Sleep(10 * time.Millisecond)
	state.Broadcaster().Publish(snapshot)

	var got StatusSnapshot
	decoder := json.NewDecoder(clientConn)
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if got.Module != snapshot.Module || got.Workspace != snapshot.Workspace {
		t.Fatalf("unexpected snapshot: %+v", got)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Fatalf("stream returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatalf("stream handler did not exit")
	}
}

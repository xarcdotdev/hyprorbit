package ctl

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"hyprorbits/internal/app/service"
	"hyprorbits/internal/ipc"
	"hyprorbits/internal/orbit"
)

func TestClientModuleWatch(t *testing.T) {
	originalDial := dialIPC
	defer func() { dialIPC = originalDial }()

	clientConn, serverConn := net.Pipe()

	dialIPC = func(ctx context.Context, opts ipc.DialOptions) (net.Conn, error) {
		return clientConn, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer serverConn.Close()

		dec := json.NewDecoder(serverConn)
		enc := json.NewEncoder(serverConn)

		var req ipc.Request
		if err := dec.Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if req.Command != "module" || req.Action != "status-stream" {
			t.Errorf("unexpected request: %+v", req)
			return
		}

		resp := ipc.NewResponse(true)
		resp.Streaming = true
		if err := enc.Encode(&resp); err != nil {
			t.Errorf("encode response: %v", err)
			return
		}

		snapshots := []service.StatusSnapshot{
			{Workspace: "code-alpha", Module: "code", Orbit: &orbit.Record{Name: "alpha"}},
			{Workspace: "comm-alpha", Module: "comm", Orbit: &orbit.Record{Name: "alpha"}},
		}
		for _, snap := range snapshots {
			if err := enc.Encode(snap); err != nil {
				t.Errorf("encode snapshot: %v", err)
				return
			}
		}
	}()

	client := NewClient(Options{Timeout: time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := client.ModuleWatch(ctx)
	if err != nil {
		t.Fatalf("ModuleWatch: %v", err)
	}
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 4096), 256*1024)

	if !scanner.Scan() {
		t.Fatalf("expected first snapshot, got error: %v", scanner.Err())
	}
	var first service.StatusSnapshot
	if err := json.Unmarshal(scanner.Bytes(), &first); err != nil {
		t.Fatalf("unmarshal first snapshot: %v", err)
	}
	if first.Module != "code" || first.Workspace != "code-alpha" {
		t.Fatalf("unexpected first snapshot: %+v", first)
	}

	if !scanner.Scan() {
		t.Fatalf("expected second snapshot, got error: %v", scanner.Err())
	}
	var second service.StatusSnapshot
	if err := json.Unmarshal(scanner.Bytes(), &second); err != nil {
		t.Fatalf("unmarshal second snapshot: %v", err)
	}
	if second.Module != "comm" {
		t.Fatalf("unexpected second snapshot: %+v", second)
	}

	cancel()

	if scanner.Scan() {
		t.Fatalf("unexpected extra snapshot")
	}

	clientConn.Close()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("server goroutine did not exit")
	}
}

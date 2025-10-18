package daemon

import (
	"context"
	"testing"
	"time"

	"hyprorbit/internal/hyprctl/events"
)

func TestStatusBroadcasterPublish(t *testing.T) {
	t.Parallel()

	bc := NewStatusBroadcaster(4)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, unsubscribe := bc.Subscribe(ctx)
	defer unsubscribe()

	snapshot := StatusSnapshot{Workspace: "dev", Module: "editor"}

	bc.Publish(snapshot)

	select {
	case got := <-ch:
		if got.Workspace != snapshot.Workspace || got.Module != snapshot.Module {
			t.Fatalf("unexpected snapshot: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for snapshot")
	}
}

func TestStatusBroadcasterContextCancelClosesChannel(t *testing.T) {
	bc := NewStatusBroadcaster(1)

	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := bc.Subscribe(ctx)

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected channel to be closed")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("channel not closed after context cancellation")
	}
}

func TestShouldPublishSnapshot(t *testing.T) {
	cases := map[events.EventType]bool{
		events.TypeWorkspace:       true,
		events.TypeWorkspaceV2:     true,
		events.TypeActiveWorkspace: true,
		events.TypeActiveWindow:    true,
		events.TypeFocusedMonitor:  true,
		events.TypeUnknown:         false,
		events.EventType("other"):  false,
	}

	for eventType, want := range cases {
		if got := shouldPublishSnapshot(eventType); got != want {
			t.Fatalf("event %q: expected %v, got %v", eventType, want, got)
		}
	}
}

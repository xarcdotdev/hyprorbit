package service

import (
	"context"
	"sync"
	"time"

	"hyprorbits/internal/orbit"
)

const defaultSnapshotBuffer = 16

// StatusSnapshot describes the current module/workspace association for streaming clients.
type StatusSnapshot struct {
	Workspace string        `json:"workspace,omitempty"`
	Module    string        `json:"module,omitempty"`
	Orbit     *orbit.Record `json:"orbit,omitempty"`
	Generated time.Time     `json:"generated"`
}

type statusSubscriber struct {
	id  int
	ctx context.Context
	ch  chan StatusSnapshot
}

// StatusBroadcaster fan-outs snapshots to registered subscribers.
type StatusBroadcaster struct {
	mu     sync.Mutex
	subs   map[int]*statusSubscriber
	nextID int
	buffer int
}

// NewStatusBroadcaster constructs a broadcaster with the provided buffer size per subscriber.
func NewStatusBroadcaster(buffer int) *StatusBroadcaster {
	if buffer <= 0 {
		buffer = defaultSnapshotBuffer
	}
	return &StatusBroadcaster{
		subs:   make(map[int]*statusSubscriber),
		buffer: buffer,
	}
}

// Subscribe registers a new listener that will receive status snapshots until the context ends or the caller unsubscribes.
func (b *StatusBroadcaster) Subscribe(ctx context.Context) (<-chan StatusSnapshot, func()) {
	if ctx == nil {
		ctx = context.Background()
	}

	ch := make(chan StatusSnapshot, b.buffer)

	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subs[id] = &statusSubscriber{ctx: ctx, ch: ch, id: id}
	b.mu.Unlock()

	unsubscribe := func() { b.removeSubscriber(id) }

	go func() {
		<-ctx.Done()
		b.removeSubscriber(id)
	}()

	return ch, unsubscribe
}

// Publish broadcasts a snapshot to all active subscribers.
func (b *StatusBroadcaster) Publish(snapshot StatusSnapshot) {
	b.mu.Lock()
	defer b.mu.Unlock()

	for id, sub := range b.subs {
		select {
		case <-sub.ctx.Done():
			close(sub.ch)
			delete(b.subs, id)
			continue
		default:
		}

		select {
		case sub.ch <- snapshot:
		default:
			select {
			case <-sub.ch:
			default:
			}
			select {
			case sub.ch <- snapshot:
			default:
				close(sub.ch)
				delete(b.subs, id)
			}
		}
	}
}

func (b *StatusBroadcaster) removeSubscriber(id int) {
	b.mu.Lock()
	sub, ok := b.subs[id]
	if ok {
		delete(b.subs, id)
	}
	b.mu.Unlock()
	if ok {
		close(sub.ch)
	}
}

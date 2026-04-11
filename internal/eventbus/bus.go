package eventbus

import (
	"context"
	"sync"
)

// Event represents a system event.
type Event struct {
	Type     string
	TenantID string
	Payload  any
}

// Handler is a function that handles events.
type Handler func(ctx context.Context, event Event)

// Bus is an in-memory publish/subscribe event bus.
type Bus struct {
	mu       sync.RWMutex
	handlers map[string][]Handler

	streamMu sync.RWMutex
	streams  map[*Stream]struct{}
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{
		handlers: make(map[string][]Handler),
		streams:  make(map[*Stream]struct{}),
	}
}

// Subscribe registers a handler for an event type.
func (b *Bus) Subscribe(eventType string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish sends an event to all subscribers and all streams.
func (b *Bus) Publish(ctx context.Context, event Event) {
	b.mu.RLock()
	handlers := make([]Handler, len(b.handlers[event.Type]))
	copy(handlers, b.handlers[event.Type])
	b.mu.RUnlock()

	for _, h := range handlers {
		h(ctx, event)
	}

	// Fan out to all streams (non-blocking)
	b.streamMu.RLock()
	for s := range b.streams {
		s.send(event)
	}
	b.streamMu.RUnlock()
}

// Stream represents a real-time event stream for a specific tenant.
type Stream struct {
	tenantID string
	ch       chan Event
}

// OpenStream creates a new event stream filtered by tenant ID.
// The caller must call CloseStream when done.
func (b *Bus) OpenStream(tenantID string) *Stream {
	s := &Stream{
		tenantID: tenantID,
		ch:       make(chan Event, 64),
	}
	b.streamMu.Lock()
	b.streams[s] = struct{}{}
	b.streamMu.Unlock()
	return s
}

// CloseStream removes and closes a stream.
func (b *Bus) CloseStream(s *Stream) {
	b.streamMu.Lock()
	delete(b.streams, s)
	b.streamMu.Unlock()
}

// Ch returns the channel to receive events from.
func (s *Stream) Ch() <-chan Event {
	return s.ch
}

// send non-blocking send of event to stream, filtered by tenant.
func (s *Stream) send(e Event) {
	if s.tenantID != "" && e.TenantID != s.tenantID {
		return
	}
	select {
	case s.ch <- e:
	default:
		// Drop event if buffer is full (backpressure)
	}
}

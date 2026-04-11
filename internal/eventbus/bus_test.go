package eventbus

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestBus_PublishSubscribe(t *testing.T) {
	bus := New()
	var count atomic.Int32

	bus.Subscribe("test.event", func(ctx context.Context, e Event) {
		count.Add(1)
		if e.TenantID != "t1" {
			t.Errorf("expected tenant t1, got %s", e.TenantID)
		}
	})

	bus.Publish(context.Background(), Event{Type: "test.event", TenantID: "t1", Payload: "hello"})
	bus.Publish(context.Background(), Event{Type: "test.event", TenantID: "t1", Payload: "world"})
	bus.Publish(context.Background(), Event{Type: "other.event", TenantID: "t1"})

	if got := count.Load(); got != 2 {
		t.Errorf("expected 2 events handled, got %d", got)
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := New()
	var count atomic.Int32

	bus.Subscribe("evt", func(ctx context.Context, e Event) { count.Add(1) })
	bus.Subscribe("evt", func(ctx context.Context, e Event) { count.Add(10) })

	bus.Publish(context.Background(), Event{Type: "evt"})
	if got := count.Load(); got != 11 {
		t.Errorf("expected 11, got %d", got)
	}
}

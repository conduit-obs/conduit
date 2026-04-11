package eventbus

import (
	"context"
	"testing"
	"time"
)

func TestBus_Stream_ReceivesEvents(t *testing.T) {
	bus := New()

	stream := bus.OpenStream("tenant-1")
	defer bus.CloseStream(stream)

	// Publish an event for the right tenant
	bus.Publish(context.Background(), Event{
		Type:     "test.event",
		TenantID: "tenant-1",
		Payload:  "hello",
	})

	select {
	case e := <-stream.Ch():
		if e.Type != "test.event" {
			t.Errorf("expected test.event, got %s", e.Type)
		}
		if e.TenantID != "tenant-1" {
			t.Errorf("expected tenant-1, got %s", e.TenantID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestBus_Stream_FiltersByTenant(t *testing.T) {
	bus := New()

	stream := bus.OpenStream("tenant-1")
	defer bus.CloseStream(stream)

	// Publish event for different tenant
	bus.Publish(context.Background(), Event{
		Type:     "test.event",
		TenantID: "tenant-2",
		Payload:  "wrong tenant",
	})

	// Should not receive it
	select {
	case e := <-stream.Ch():
		t.Errorf("should not receive event for other tenant, got %v", e)
	case <-time.After(100 * time.Millisecond):
		// Expected — no event
	}
}

func TestBus_Stream_CloseRemovesStream(t *testing.T) {
	bus := New()

	stream := bus.OpenStream("tenant-1")
	bus.CloseStream(stream)

	// Publish after close — should not panic
	bus.Publish(context.Background(), Event{
		Type:     "test.event",
		TenantID: "tenant-1",
	})

	// Channel should not receive anything
	select {
	case <-stream.Ch():
		t.Error("should not receive event after close")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestBus_Stream_MultipleStreams(t *testing.T) {
	bus := New()

	s1 := bus.OpenStream("tenant-1")
	s2 := bus.OpenStream("tenant-1")
	defer bus.CloseStream(s1)
	defer bus.CloseStream(s2)

	bus.Publish(context.Background(), Event{
		Type:     "test",
		TenantID: "tenant-1",
	})

	// Both should receive
	for _, s := range []*Stream{s1, s2} {
		select {
		case <-s.Ch():
		case <-time.After(time.Second):
			t.Fatal("stream did not receive event")
		}
	}
}

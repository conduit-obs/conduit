package circuitbreaker

import (
	"errors"
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedOnSuccess(t *testing.T) {
	cb := New(3, 1, 100*time.Millisecond)
	err := cb.Wrap(func() error { return nil })
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if cb.State() != StateClosed {
		t.Errorf("expected closed state, got %s", cb.State())
	}
}

func TestCircuitBreaker_OpensAfterThreshold(t *testing.T) {
	cb := New(3, 1, 100*time.Millisecond)
	fail := errors.New("fail")
	for i := 0; i < 3; i++ {
		cb.Wrap(func() error { return fail })
	}
	if cb.State() != StateOpen {
		t.Errorf("expected open state, got %s", cb.State())
	}
	err := cb.Wrap(func() error { return nil })
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_HalfOpenAfterTimeout(t *testing.T) {
	cb := New(2, 1, 50*time.Millisecond)
	fail := errors.New("fail")
	cb.Wrap(func() error { return fail })
	cb.Wrap(func() error { return fail })
	if cb.State() != StateOpen {
		t.Fatalf("expected open, got %s", cb.State())
	}
	time.Sleep(60 * time.Millisecond)
	if cb.State() != StateHalfOpen {
		t.Errorf("expected half-open, got %s", cb.State())
	}
}

func TestCircuitBreaker_ClosesAfterHalfOpenSuccess(t *testing.T) {
	cb := New(2, 1, 50*time.Millisecond)
	fail := errors.New("fail")
	cb.Wrap(func() error { return fail })
	cb.Wrap(func() error { return fail })
	time.Sleep(60 * time.Millisecond)
	cb.Wrap(func() error { return nil })
	if cb.State() != StateClosed {
		t.Errorf("expected closed after half-open success, got %s", cb.State())
	}
}

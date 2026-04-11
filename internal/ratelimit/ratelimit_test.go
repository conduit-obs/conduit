package ratelimit

import (
	"testing"
	"time"
)

func TestTokenBucket_Allow(t *testing.T) {
	tb := New()

	// Rate = 2 req/s
	ok, _ := tb.Allow("tenant-1", 2)
	if !ok {
		t.Error("first request should be allowed")
	}
	ok, _ = tb.Allow("tenant-1", 2)
	if !ok {
		t.Error("second request should be allowed")
	}
	ok, retry := tb.Allow("tenant-1", 2)
	if ok {
		t.Error("third request should be rejected")
	}
	if retry <= 0 {
		t.Errorf("retry-after should be positive, got %f", retry)
	}
}

func TestTokenBucket_Unlimited(t *testing.T) {
	tb := New()
	for i := 0; i < 50; i++ {
		ok, _ := tb.Allow("t", 0)
		if !ok {
			t.Fatalf("request %d should be allowed with rate=0", i)
		}
	}
}

func TestTokenBucket_Refill(t *testing.T) {
	tb := New()

	// Drain the bucket
	tb.Allow("t", 1)
	ok, _ := tb.Allow("t", 1)
	if ok {
		t.Error("should be rate limited after 1 req")
	}

	// Wait for refill
	time.Sleep(1100 * time.Millisecond)
	ok, _ = tb.Allow("t", 1)
	if !ok {
		t.Error("should be allowed after refill")
	}
}

func TestTokenBucket_Usage(t *testing.T) {
	tb := New()

	reqs, limited := tb.Usage("nonexistent")
	if reqs != 0 || limited != 0 {
		t.Error("nonexistent tenant should have 0 usage")
	}

	tb.Allow("t1", 1)
	tb.Allow("t1", 1) // this one is limited

	reqs, limited = tb.Usage("t1")
	if reqs != 2 {
		t.Errorf("expected 2 requests, got %d", reqs)
	}
	if limited != 1 {
		t.Errorf("expected 1 limited, got %d", limited)
	}
}

func TestTokenBucket_Isolation(t *testing.T) {
	tb := New()

	// Drain tenant-1
	tb.Allow("t1", 1)
	ok, _ := tb.Allow("t1", 1)
	if ok {
		t.Error("t1 should be limited")
	}

	// tenant-2 should not be affected
	ok, _ = tb.Allow("t2", 1)
	if !ok {
		t.Error("t2 should be allowed")
	}
}

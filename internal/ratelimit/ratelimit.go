package ratelimit

import (
	"math"
	"sync"
	"time"
)

// TokenBucket implements a per-tenant token bucket rate limiter.
type TokenBucket struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	requests   int64 // total requests counter
	limited    int64 // total limited counter
}

// New creates a new TokenBucket rate limiter.
func New() *TokenBucket {
	return &TokenBucket{
		buckets: make(map[string]*bucket),
	}
}

// Allow checks if a request from the given tenant is allowed.
// rateLimit is requests per second. Returns (allowed, retryAfterSeconds).
func (tb *TokenBucket) Allow(tenantID string, rateLimit int) (bool, float64) {
	if rateLimit <= 0 {
		return true, 0
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	b, ok := tb.buckets[tenantID]
	now := time.Now()

	if !ok || b.refillRate != float64(rateLimit) {
		b = &bucket{
			tokens:     float64(rateLimit),
			maxTokens:  float64(rateLimit),
			refillRate: float64(rateLimit),
			lastRefill: now,
		}
		tb.buckets[tenantID] = b
	}

	// Refill tokens
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens = math.Min(b.maxTokens, b.tokens+elapsed*b.refillRate)
	b.lastRefill = now

	b.requests++

	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}

	b.limited++
	// Calculate retry-after: time until 1 token is available
	retryAfter := (1 - b.tokens) / b.refillRate
	return false, retryAfter
}

// Usage returns usage stats for a tenant.
func (tb *TokenBucket) Usage(tenantID string) (requests, limited int64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	b, ok := tb.buckets[tenantID]
	if !ok {
		return 0, 0
	}
	return b.requests, b.limited
}

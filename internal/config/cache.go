package config

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
)

// CachingCompiler wraps a Compiler with content-addressable caching.
type CachingCompiler struct {
	compiler *Compiler
	mu       sync.RWMutex
	cache    map[string]string // intentHash -> compiledYAML
	hits     atomic.Int64
	misses   atomic.Int64
}

// NewCachingCompiler creates a compiler with caching.
func NewCachingCompiler() *CachingCompiler {
	return &CachingCompiler{
		compiler: NewCompiler(),
		cache:    make(map[string]string),
	}
}

// Compile compiles an intent, using cache if available.
func (cc *CachingCompiler) Compile(intent *Intent) (string, error) {
	// Compute hash of intent JSON
	intentJSON, _ := json.Marshal(intent)
	hash := fmt.Sprintf("%x", sha256.Sum256(intentJSON))

	// Check cache
	cc.mu.RLock()
	if cached, ok := cc.cache[hash]; ok {
		cc.mu.RUnlock()
		cc.hits.Add(1)
		return cached, nil
	}
	cc.mu.RUnlock()

	// Cache miss — compile
	cc.misses.Add(1)
	result, err := cc.compiler.Compile(intent)
	if err != nil {
		return "", err
	}

	// Store in cache
	cc.mu.Lock()
	cc.cache[hash] = result
	cc.mu.Unlock()

	return result, nil
}

// Stats returns cache hit/miss statistics.
func (cc *CachingCompiler) Stats() (hits, misses int64) {
	return cc.hits.Load(), cc.misses.Load()
}

// Inner returns the underlying Compiler for direct use.
func (cc *CachingCompiler) Inner() *Compiler {
	return cc.compiler
}

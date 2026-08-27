package tokenbucket

import (
	"math"
	"sync"
	"time"
)

// TokenBucket represents a thread-safe token bucket rate limiter
type TokenBucket struct {
	mu          sync.Mutex
	capacity    float64
	refillRate  float64 // tokens per second
	tokens      float64
	lastUpdated time.Time
}

// NewTokenBucket initializes a token bucket
func NewTokenBucket(capacity float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:    capacity,
		refillRate:  refillRate,
		tokens:      capacity,
		lastUpdated: time.Now(),
	}
}

// Allow checks if the requested tokens are available and consumes them
func (tb *TokenBucket) Allow(tokens float64) (bool, float64) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastUpdated).Seconds()
	tb.lastUpdated = now

	// Refill tokens
	tb.tokens = math.Min(tb.capacity, tb.tokens+elapsed*tb.refillRate)

	if tb.tokens >= tokens {
		tb.tokens -= tokens
		return true, tb.tokens
	}

	return false, tb.tokens
}

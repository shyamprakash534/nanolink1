package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/nanolink/nanolink/internal/pkg/tokenbucket"
	"github.com/nanolink/nanolink/internal/storage/redis"
)

type RateLimiterService struct {
	redisClient *redis.Client
	localMu     sync.RWMutex
	localLimits map[string]*tokenbucket.TokenBucket
	capacity    float64
	refillRate  float64
}

func NewRateLimiterService(rdb *redis.Client, capacity, refillRate float64) *RateLimiterService {
	return &RateLimiterService{
		redisClient: rdb,
		localLimits: make(map[string]*tokenbucket.TokenBucket),
		capacity:    capacity,
		refillRate:  refillRate,
	}
}

func (s *RateLimiterService) Allow(ctx context.Context, clientKey string, tokens int) (bool, int64, error) {
	if s.redisClient != nil {
		redisKey := fmt.Sprintf("rate_limit:%s", clientKey)
		allowed, remaining, err := s.redisClient.EvaluateTokenBucket(ctx, redisKey, s.capacity, s.refillRate, tokens)
		if err == nil {
			return allowed, remaining, nil
		}
	}

	// In-memory fallback
	s.localMu.Lock()
	tb, exists := s.localLimits[clientKey]
	if !exists {
		tb = tokenbucket.NewTokenBucket(s.capacity, s.refillRate)
		s.localLimits[clientKey] = tb
	}
	s.localMu.Unlock()

	allowed, rem := tb.Allow(float64(tokens))
	return allowed, int64(rem), nil
}

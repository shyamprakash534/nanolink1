package service

import (
	"context"

	"github.com/nanolink/nanolink/internal/pkg/bloom"
	"github.com/nanolink/nanolink/internal/storage/redis"
)

type BloomService struct {
	localFilter *bloom.BloomFilter
	redisClient *redis.Client
}

func NewBloomService(expected uint64, fpRate float64, rdb *redis.Client) *BloomService {
	return &BloomService{
		localFilter: bloom.NewBloomFilter(expected, fpRate),
		redisClient: rdb,
	}
}

func (s *BloomService) Add(ctx context.Context, shortCode string) {
	s.localFilter.Add(shortCode)
	if s.redisClient != nil {
		_ = s.redisClient.AddToBloom(ctx, shortCode)
	}
}

func (s *BloomService) MightContain(ctx context.Context, shortCode string) bool {
	if !s.localFilter.Contains(shortCode) {
		return false
	}
	if s.redisClient != nil {
		exists, err := s.redisClient.ExistsInBloom(ctx, shortCode)
		if err == nil && !exists {
			return false
		}
	}
	return true
}

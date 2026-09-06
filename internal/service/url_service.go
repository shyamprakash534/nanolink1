package service

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nanolink/nanolink/internal/models"
	"github.com/nanolink/nanolink/internal/pkg/base62"
	"github.com/nanolink/nanolink/internal/pkg/snowflake"
	"github.com/nanolink/nanolink/internal/storage/postgres"
	"github.com/nanolink/nanolink/internal/storage/redis"
)

var aliasRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{3,30}$`)

type URLService struct {
	urlRepo      *postgres.URLRepository
	redisClient  *redis.Client
	bloomService *BloomService
	node         *snowflake.Node
	baseURL      string
}

func NewURLService(
	urlRepo *postgres.URLRepository,
	rdb *redis.Client,
	bloomService *BloomService,
	baseURL string,
) *URLService {
	node, _ := snowflake.NewNode(1)
	return &URLService{
		urlRepo:      urlRepo,
		redisClient:  rdb,
		bloomService: bloomService,
		node:         node,
		baseURL:      strings.TrimRight(baseURL, "/"),
	}
}

func (s *URLService) ShortenURL(ctx context.Context, req models.ShortenRequest, userID *uuid.UUID) (*models.ShortenResponse, error) {
	var shortCode string
	isCustom := false

	if req.CustomAlias != "" {
		if !aliasRegex.MatchString(req.CustomAlias) {
			return nil, errors.New("custom alias must be 3-30 alphanumeric characters, hyphens or underscores")
		}
		// Check uniqueness
		existing, err := s.urlRepo.GetByShortCode(req.CustomAlias)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, errors.New("custom alias already taken")
		}
		shortCode = req.CustomAlias
		isCustom = true
	} else {
		// Generate Snowflake ID and encode to Base62
		sfID := s.node.Generate()
		shortCode = base62.Base62Encode(sfID)
		if len(shortCode) < 6 {
			// Pad or hash if needed
			shortCode = fmt.Sprintf("%06s", shortCode)
		}
	}

	var expiresAt *time.Time
	if req.TTLSeconds != nil && *req.TTLSeconds > 0 {
		exp := time.Now().UTC().Add(time.Duration(*req.TTLSeconds) * time.Second)
		expiresAt = &exp
	}

	urlObj := &models.URL{
		ShortCode:   shortCode,
		LongURL:     req.LongURL,
		CustomAlias: isCustom,
		UserID:      userID,
		CreatedAt:   time.Now().UTC(),
		ExpiresAt:   expiresAt,
		ClickCount:  0,
		IsActive:    true,
	}

	if err := s.urlRepo.CreateURL(urlObj); err != nil {
		// Fallback MD5 hash resolution if collision occurs
		if !isCustom {
			h := md5.Sum([]byte(req.LongURL + time.Now().String()))
			shortCode = hex.EncodeToString(h[:])[:7]
			urlObj.ShortCode = shortCode
			if err := s.urlRepo.CreateURL(urlObj); err != nil {
				return nil, fmt.Errorf("failed to persist short url: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create custom alias: %w", err)
		}
	}

	// Add to Bloom Filter
	s.bloomService.Add(ctx, shortCode)

	// Pre-warm Redis Cache with TTL
	if s.redisClient != nil {
		ttl := 24 * time.Hour
		if expiresAt != nil {
			dur := time.Until(*expiresAt)
			if dur > 0 && dur < ttl {
				ttl = dur
			}
		}
		_ = s.redisClient.SetURL(ctx, shortCode, req.LongURL, ttl)
	}

	return &models.ShortenResponse{
		ShortCode: shortCode,
		ShortURL:  fmt.Sprintf("%s/%s", s.baseURL, shortCode),
		LongURL:   req.LongURL,
		CreatedAt: urlObj.CreatedAt,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *URLService) ResolveURL(ctx context.Context, shortCode string) (string, error) {
	// Step 1: Bloom filter fast reject
	if !s.bloomService.MightContain(ctx, shortCode) {
		return "", errors.New("url not found")
	}

	// Step 2: Redis Cache Lookup
	if s.redisClient != nil {
		longURL, err := s.redisClient.GetURL(ctx, shortCode)
		if err == nil && longURL != "" {
			return longURL, nil
		}
	}

	// Step 3: PostgreSQL Database Query
	urlObj, err := s.urlRepo.GetByShortCode(shortCode)
	if err != nil {
		return "", err
	}
	if urlObj == nil || !urlObj.IsActive {
		return "", errors.New("url not found")
	}

	// Check expiration
	if urlObj.ExpiresAt != nil && time.Now().UTC().After(*urlObj.ExpiresAt) {
		return "", errors.New("url expired")
	}

	// Populate Redis Cache
	if s.redisClient != nil {
		ttl := 24 * time.Hour
		if urlObj.ExpiresAt != nil {
			dur := time.Until(*urlObj.ExpiresAt)
			if dur > 0 && dur < ttl {
				ttl = dur
			}
		}
		_ = s.redisClient.SetURL(ctx, shortCode, urlObj.LongURL, ttl)
	}

	return urlObj.LongURL, nil
}

// RecordClick increments the persistent click counter for a successful redirect.
// It is intentionally separate from ResolveURL so cache hits are counted too,
// while non-redirect lookups such as QR generation are not counted as clicks.
func (s *URLService) RecordClick(shortCode string) {
	go func() {
		_ = s.urlRepo.IncrementClickCount(shortCode)
	}()
}

func (s *URLService) DeleteURL(ctx context.Context, shortCode string, userID *uuid.UUID) error {
	if err := s.urlRepo.DeleteURL(shortCode, userID); err != nil {
		return err
	}
	if s.redisClient != nil {
		_ = s.redisClient.InvalidateURL(ctx, shortCode)
	}
	return nil
}

func (s *URLService) ListUserURLs(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.URL, error) {
	return s.urlRepo.ListURLsByUser(userID, limit, offset)
}

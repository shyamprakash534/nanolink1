package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/nanolink/nanolink/internal/models"
	"github.com/nanolink/nanolink/internal/storage/clickhouse"
	"github.com/nanolink/nanolink/internal/storage/redis"
)

type AnalyticsService struct {
	redisClient      *redis.Client
	clickHouseClient *clickhouse.Client
}

func NewAnalyticsService(rdb *redis.Client, ch *clickhouse.Client) *AnalyticsService {
	return &AnalyticsService{
		redisClient:      rdb,
		clickHouseClient: ch,
	}
}

func (s *AnalyticsService) RecordClick(ctx context.Context, shortCode, ip, userAgent, referrer string) {
	ipHashBytes := sha256.Sum256([]byte(ip))
	ipHash := hex.EncodeToString(ipHashBytes[:])

	deviceType := "desktop"
	uaLower := strings.ToLower(userAgent)
	if strings.Contains(uaLower, "mobile") || strings.Contains(uaLower, "android") || strings.Contains(uaLower, "iphone") {
		deviceType = "mobile"
	} else if strings.Contains(uaLower, "tablet") || strings.Contains(uaLower, "ipad") {
		deviceType = "tablet"
	}

	browser := "other"
	if strings.Contains(uaLower, "chrome") {
		browser = "chrome"
	} else if strings.Contains(uaLower, "safari") {
		browser = "safari"
	} else if strings.Contains(uaLower, "firefox") {
		browser = "firefox"
	} else if strings.Contains(uaLower, "edge") {
		browser = "edge"
	}

	os := "other"
	if strings.Contains(uaLower, "windows") {
		os = "windows"
	} else if strings.Contains(uaLower, "macintosh") || strings.Contains(uaLower, "mac os") {
		os = "macos"
	} else if strings.Contains(uaLower, "linux") {
		os = "linux"
	} else if strings.Contains(uaLower, "android") {
		os = "android"
	} else if strings.Contains(uaLower, "ios") {
		os = "ios"
	}

	event := &models.ClickEvent{
		ShortCode:   shortCode,
		ClickedAt:   time.Now().UTC(),
		IPHash:      ipHash,
		CountryCode: "US", // In production, resolved via MaxMind GeoIP / Cloudflare CF-IPCountry header
		UserAgent:   userAgent,
		DeviceType:  deviceType,
		Browser:     browser,
		OS:          os,
		Referrer:    referrer,
	}

	if s.redisClient != nil {
	go func() {
    bgCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    if err := s.redisClient.PublishClickEvent(bgCtx, event); err != nil {
        log.Printf("[Analytics] Failed to publish click for %s: %v", shortCode, err)
    }
}()
	}
}

func (s *AnalyticsService) GetStats(ctx context.Context, shortCode string) (*models.URLStatsResponse, error) {
	if s.clickHouseClient != nil {
		return s.clickHouseClient.GetStatsByShortCode(ctx, shortCode)
	}
	return &models.URLStatsResponse{
		ShortCode: shortCode,
	}, nil
}

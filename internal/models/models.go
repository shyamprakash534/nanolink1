package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID              uuid.UUID `json:"id"`
	Email           string    `json:"email"`
	APIKeyHash      string    `json:"-"`
	Tier            string    `json:"tier"`
	RateLimitPerHr  int       `json:"rate_limit_per_hr"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type URL struct {
	ID          uint64     `json:"id"`
	ShortCode   string     `json:"short_code"`
	LongURL     string     `json:"long_url"`
	CustomAlias bool       `json:"custom_alias"`
	UserID      *uuid.UUID `json:"user_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ClickCount  int64      `json:"click_count"`
	IsActive    bool       `json:"is_active"`
}

type ShortenRequest struct {
	LongURL     string `json:"long_url" binding:"required,url"`
	CustomAlias string `json:"custom_alias,omitempty"`
	TTLSeconds  *int64 `json:"ttl_seconds,omitempty"`
}

type ShortenResponse struct {
	ShortCode string     `json:"short_code"`
	ShortURL  string     `json:"short_url"`
	LongURL   string     `json:"long_url"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type ClickEvent struct {
	ID          uint64    `json:"id"`
	ShortCode   string    `json:"short_code"`
	ClickedAt   time.Time `json:"clicked_at"`
	IPHash      string    `json:"ip_hash"`
	CountryCode string    `json:"country_code"`
	UserAgent   string    `json:"user_agent"`
	DeviceType  string    `json:"device_type"`
	Browser     string    `json:"browser"`
	OS          string    `json:"os"`
	Referrer    string    `json:"referrer"`
}

type URLStatsResponse struct {
	ShortCode           string            `json:"short_code"`
	LongURL             string            `json:"long_url"`
	TotalClicks         int64             `json:"total_clicks"`
	CreatedAt           time.Time         `json:"created_at"`
	ClicksByCountry     map[string]int64  `json:"clicks_by_country"`
	ClicksByDevice      map[string]int64  `json:"clicks_by_device"`
	ClicksByReferrer    map[string]int64  `json:"clicks_by_referrer"`
	HourlyClicks        []HourlyClickStat `json:"hourly_clicks"`
}

type HourlyClickStat struct {
	Hour  string `json:"hour"`
	Count int64  `json:"count"`
}

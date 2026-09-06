package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port              string
	BaseURL            string
	PostgresDSN        string
	RedisAddr          string
	RedisPassword      string
	ClickHouseDSN      string
	RateLimitCapacity  float64
	RateLimitRefill    float64
	BloomCapacity      uint64
	BloomFPRate        float64
}

func LoadConfig() *Config {
	return &Config{
		Port:              getEnv("PORT", "8080"),
		BaseURL:           getEnv("BASE_URL", "http://localhost:8080"),
		PostgresDSN:       getEnv("DATABASE_URL", "postgres://nanolink:nanolink_secret@localhost:5432/nanolink?sslmode=disable"),
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		// ClickHouse is optional. When it is not configured, NanoLink still
		// starts and serves URL-shortening functionality without analytics storage.
		ClickHouseDSN:     getEnv("CLICKHOUSE_DSN", ""),
		RateLimitCapacity: getEnvFloat("RATE_LIMIT_CAPACITY", 100.0),
		RateLimitRefill:   getEnvFloat("RATE_LIMIT_REFILL", 10.0),
		BloomCapacity:     getEnvUint("BLOOM_CAPACITY", 10000000),
		BloomFPRate:       getEnvFloat("BLOOM_FP_RATE", 0.01),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvUint(key string, defaultVal uint64) uint64 {
	if val := os.Getenv(key); val != "" {
		if u, err := strconv.ParseUint(val, 10, 64); err == nil {
			return u
		}
	}
	return defaultVal
}

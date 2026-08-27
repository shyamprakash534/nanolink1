package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	rdb *redis.Client
}

func NewRedisClient(addr, password string) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           0,
		PoolSize:     200,
		MinIdleConns: 50,
		ReadTimeout:  20 * time.Millisecond,
		WriteTimeout: 20 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		// Log warning or return client
		return &Client{rdb: rdb}, nil
	}

	return &Client{rdb: rdb}, nil
}

func (c *Client) SetURL(ctx context.Context, shortCode, longURL string, ttl time.Duration) error {
	key := fmt.Sprintf("url:%s", shortCode)
	return c.rdb.Set(ctx, key, longURL, ttl).Err()
}

func (c *Client) GetURL(ctx context.Context, shortCode string) (string, error) {
	key := fmt.Sprintf("url:%s", shortCode)
	return c.rdb.Get(ctx, key).Result()
}

func (c *Client) InvalidateURL(ctx context.Context, shortCode string) error {
	key := fmt.Sprintf("url:%s", shortCode)
	return c.rdb.Del(ctx, key).Err()
}

// Token bucket Lua script for atomic rate limiting
const tokenBucketScript = `
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

local data = redis.call("HMGET", key, "tokens", "last_updated")
local tokens = tonumber(data[1])
local last_updated = tonumber(data[2])

if not tokens then
    tokens = capacity
    last_updated = now
else
    local delta = math.max(0, now - last_updated)
    tokens = math.min(capacity, tokens + delta * refill_rate)
    last_updated = now
end

if tokens >= requested then
    tokens = tokens - requested
    redis.call("HMSET", key, "tokens", tokens, "last_updated", last_updated)
    redis.call("EXPIRE", key, math.ceil(capacity / refill_rate))
    return {1, math.floor(tokens)}
else
    redis.call("HMSET", key, "tokens", tokens, "last_updated", last_updated)
    return {0, math.floor(tokens)}
end
`

func (c *Client) EvaluateTokenBucket(ctx context.Context, key string, capacity, refillRate float64, requested int) (bool, int64, error) {
	now := time.Now().Unix()
	res, err := c.rdb.Eval(ctx, tokenBucketScript, []string{key}, capacity, refillRate, now, requested).Result()
	if err != nil {
		return false, 0, err
	}

	resSlice, ok := res.([]interface{})
	if !ok || len(resSlice) < 2 {
		return false, 0, fmt.Errorf("unexpected script response format")
	}

	allowed := resSlice[0].(int64) == 1
	remainingTokens := resSlice[1].(int64)
	return allowed, remainingTokens, nil
}

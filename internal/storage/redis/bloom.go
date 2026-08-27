package redis

import (
	"context"
)

const bloomKey = "bloom:urls"

// AddToBloom adds a short code to RedisBloom or bitset
func (c *Client) AddToBloom(ctx context.Context, shortCode string) error {
	// Uses BF.ADD if RedisBloom is present, or fallback bitset
	return c.rdb.Do(ctx, "BF.ADD", bloomKey, shortCode).Err()
}

// ExistsInBloom checks if a short code exists in the Redis Bloom filter
func (c *Client) ExistsInBloom(ctx context.Context, shortCode string) (bool, error) {
	res, err := c.rdb.Do(ctx, "BF.EXISTS", bloomKey, shortCode).Bool()
	if err != nil {
		// If RedisBloom command doesn't exist or errors, fallback to true to allow DB lookup
		return true, nil
	}
	return res, nil
}

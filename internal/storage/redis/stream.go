package redis
import (
	"context"
	"encoding/json"
	"time"

	"github.com/nanolink/nanolink/internal/models"
	"github.com/redis/go-redis/v9"
)

const clickStreamKey = "stream:clicks"
const clickConsumerGroup = "analytics_group"

// PublishClickEvent publishes click telemetry event to Redis Streams
func (c *Client) PublishClickEvent(ctx context.Context, event *models.ClickEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return c.rdb.XAdd(ctx, &redis.XAddArgs{
		Stream: clickStreamKey,
		Values: map[string]interface{}{
			"payload": string(data),
		},
	}).Err()
}

// ReadClickEventsBatch reads a batch of click events from Redis Streams consumer group
func (c *Client) ReadClickEventsBatch(ctx context.Context, consumerName string, count int64) ([]models.ClickEvent, []string, error) {
	entries, err := c.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    clickConsumerGroup,
		Consumer: consumerName,
		Streams:  []string{clickStreamKey, ">"},
		Count:    count,
		Block:    1000 * time.Millisecond,
	}).Result()

	if err != nil {
		if err == redis.Nil {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	var events []models.ClickEvent
	var messageIDs []string

	for _, stream := range entries {
		for _, message := range stream.Messages {
			messageIDs = append(messageIDs, message.ID)
			if payload, ok := message.Values["payload"].(string); ok {
				var ev models.ClickEvent
				if err := json.Unmarshal([]byte(payload), &ev); err == nil {
					events = append(events, ev)
				}
			}
		}
	}

	return events, messageIDs, nil
}

// AckClickEvents acknowledges processed stream messages
func (c *Client) AckClickEvents(ctx context.Context, messageIDs []string) error {
	if len(messageIDs) == 0 {
		return nil
	}
	return c.rdb.XAck(ctx, clickStreamKey, clickConsumerGroup, messageIDs...).Err()
}



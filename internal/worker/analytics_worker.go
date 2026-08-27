package worker

import (
	"context"
	"log"
	"time"

	"github.com/nanolink/nanolink/internal/storage/clickhouse"
	"github.com/nanolink/nanolink/internal/storage/redis"
)

type AnalyticsWorker struct {
	redisClient      *redis.Client
	clickHouseClient *clickhouse.Client
	consumerID       string
	batchSize        int64
	flushInterval    time.Duration
	stopChan         chan struct{}
}

func NewAnalyticsWorker(rdb *redis.Client, ch *clickhouse.Client, consumerID string) *AnalyticsWorker {
	return &AnalyticsWorker{
		redisClient:      rdb,
		clickHouseClient: ch,
		consumerID:       consumerID,
		batchSize:        1000,
		flushInterval:    1 * time.Second,
		stopChan:         make(chan struct{}),
	}
}

func (w *AnalyticsWorker) Start(ctx context.Context) {
	log.Printf("[AnalyticsWorker] Starting stream consumer %s...", w.consumerID)
	ticker := time.NewTicker(w.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[AnalyticsWorker] Stopping consumer...")
			return
		case <-w.stopChan:
			log.Println("[AnalyticsWorker] Stopped.")
			return
		case <-ticker.C:
			w.processBatch(ctx)
		}
	}
}

func (w *AnalyticsWorker) processBatch(ctx context.Context) {
	if w.redisClient == nil || w.clickHouseClient == nil {
		return
	}

	events, messageIDs, err := w.redisClient.ReadClickEventsBatch(ctx, w.consumerID, w.batchSize)
	if err != nil {
		log.Printf("[AnalyticsWorker] Error reading click batch: %v", err)
		return
	}

	if len(events) == 0 {
		return
	}

	// Flush batch to ClickHouse
	if err := w.clickHouseClient.InsertClickBatch(ctx, events); err != nil {
		log.Printf("[AnalyticsWorker] Error writing batch to ClickHouse: %v", err)
		return
	}

	// Acknowledge processed stream entries
	if err := w.redisClient.AckClickEvents(ctx, messageIDs); err != nil {
		log.Printf("[AnalyticsWorker] Error acknowledging stream batch: %v", err)
	} else {
		log.Printf("[AnalyticsWorker] Successfully processed and flushed %d click events", len(events))
	}
}

func (w *AnalyticsWorker) Stop() {
	close(w.stopChan)
}

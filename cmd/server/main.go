package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nanolink/nanolink/internal/api"
	"github.com/nanolink/nanolink/internal/config"
	"github.com/nanolink/nanolink/internal/service"
	"github.com/nanolink/nanolink/internal/storage/clickhouse"
	"github.com/nanolink/nanolink/internal/storage/postgres"
	"github.com/nanolink/nanolink/internal/storage/redis"
	"github.com/nanolink/nanolink/internal/worker"
)

func main() {
	cfg := config.LoadConfig()
	log.Printf("Starting NanoLink API Service on port %s...", cfg.Port)

	// Initialize Postgres
	pgDB, err := postgres.NewPostgresDB(cfg.PostgresDSN)
	if err != nil {
		log.Printf("Warning: Postgres connection error: %v (continuing with degraded/in-memory mode)", err)
	}
	urlRepo := postgres.NewURLRepository(pgDB)

	// Initialize Redis
	redisClient, err := redis.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword)
	if err != nil {
		log.Printf("Warning: Redis connection error: %v", err)
	}

	// Initialize ClickHouse only when explicitly configured.
	// Render does not provide a ClickHouse service by default, so the API
	// must not attempt to connect to localhost:9000 in production.
	var chClient *clickhouse.Client
	if cfg.ClickHouseDSN != "" {
		chClient, err = clickhouse.NewClickHouseClient(cfg.ClickHouseDSN)
		if err != nil {
			log.Printf("Warning: ClickHouse connection error: %v (analytics storage disabled)", err)
			chClient = nil
		}
	} else {
		log.Printf("ClickHouse not configured; analytics storage is disabled")
	}

	// Initialize Services
	bloomService := service.NewBloomService(cfg.BloomCapacity, cfg.BloomFPRate, redisClient)
	rateLimiterService := service.NewRateLimiterService(redisClient, cfg.RateLimitCapacity, cfg.RateLimitRefill)
	analyticsService := service.NewAnalyticsService(redisClient, chClient)
	urlService := service.NewURLService(urlRepo, redisClient, bloomService, cfg.BaseURL)

	// Start Background Analytics Stream Worker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	analyticsWorker := worker.NewAnalyticsWorker(redisClient, chClient, "worker-node-1")
	go analyticsWorker.Start(ctx)

	// Setup HTTP Router
	router := api.SetupRouter(urlService, analyticsService, rateLimiterService, pgDB)

	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	log.Printf("NanoLink API Service is listening on :%s", cfg.Port)

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down NanoLink API Service...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("NanoLink API Service exited cleanly")
}

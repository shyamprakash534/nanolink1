package api

import (
	"github.com/gin-gonic/gin"
	"github.com/nanolink/nanolink/internal/api/handlers"
	"github.com/nanolink/nanolink/internal/api/middleware"
	"github.com/nanolink/nanolink/internal/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func SetupRouter(
	urlService *service.URLService,
	analyticsService *service.AnalyticsService,
	rlService *service.RateLimiterService,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.MetricsMiddleware())

	urlHandler := handlers.NewURLHandler(urlService, analyticsService)
	analyticsHandler := handlers.NewAnalyticsHandler(analyticsService, urlService)
	healthHandler := handlers.NewHealthHandler()

	// Public Health & Metrics
	r.GET("/health", healthHandler.HealthCheck)
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Fast Redirect Resolution Path (sub-10ms)
	r.GET("/:code", urlHandler.Redirect)

	// API V1 Group
	v1 := r.Group("/api/v1")
	{
		// URL Management
		v1.POST("/urls", middleware.RateLimiter(rlService), middleware.OptionalAuth(), urlHandler.ShortenURL)
		v1.GET("/urls", middleware.RequireAuth(), urlHandler.ListURLs)
		v1.GET("/urls/:code", urlHandler.Redirect)
		v1.DELETE("/urls/:code", middleware.RequireAuth(), urlHandler.DeleteURL)
		v1.GET("/urls/:code/stats", analyticsHandler.GetStats)
		v1.GET("/urls/:code/qr", urlHandler.GenerateQRCode)
	}

	return r
}

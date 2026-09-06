package api

import (
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/nanolink/nanolink/internal/api/handlers"
    "github.com/nanolink/nanolink/internal/api/middleware"
    "github.com/nanolink/nanolink/internal/service"
    "github.com/nanolink/nanolink/internal/storage/postgres"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func SetupRouter(
    urlService *service.URLService,
    analyticsService *service.AnalyticsService,
    rlService *service.RateLimiterService,
    pgDB *postgres.DB,
) *gin.Engine {
    r := gin.New()
    r.Use(gin.Recovery())
    r.Use(middleware.MetricsMiddleware())

    urlHandler := handlers.NewURLHandler(urlService, analyticsService)
    analyticsHandler := handlers.NewAnalyticsHandler(analyticsService, urlService)
    healthHandler := handlers.NewHealthHandler()
    authMiddleware := middleware.NewAuthMiddleware(pgDB)

    // Public service information
    r.GET("/", func(c *gin.Context) {
        c.JSON(http.StatusOK, gin.H{
            "service": "nanolink-api",
            "status":  "running",
            "message": "NanoLink URL Shortener API is running",
            "health":  "/health",
            "metrics": "/metrics",
            "api":     "/api/v1/urls",
        })
    })

    // Public Health & Metrics
    r.GET("/health", healthHandler.HealthCheck)
    r.GET("/metrics", gin.WrapH(promhttp.Handler()))

    // Fast Redirect Resolution Path
    r.GET("/:code", urlHandler.Redirect)

    // API V1 Group
    v1 := r.Group("/api/v1")
    {
        v1.POST("/urls", middleware.RateLimiter(rlService), authMiddleware.OptionalAuth(), urlHandler.ShortenURL)
        v1.GET("/urls", authMiddleware.RequireAuth(), urlHandler.ListURLs)
        v1.GET("/urls/:code", urlHandler.Redirect)
        v1.DELETE("/urls/:code", authMiddleware.RequireAuth(), urlHandler.DeleteURL)
        v1.GET("/urls/:code/stats", analyticsHandler.GetStats)
        v1.GET("/urls/:code/qr", urlHandler.GenerateQRCode)
    }

    return r
}

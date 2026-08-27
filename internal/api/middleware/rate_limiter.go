package middleware

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/nanolink/nanolink/internal/service"
)

func RateLimiter(rlService *service.RateLimiterService) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		apiKey := c.GetHeader("X-API-Key")

		clientKey := "ip:" + clientIP
		if apiKey != "" {
			clientKey = "key:" + apiKey
		}

		allowed, remaining, err := rlService.Allow(c.Request.Context(), clientKey, 1)
		if err != nil {
			// On rate limiter error, fail-open to avoid service disruption
			c.Next()
			return
		}

		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if !allowed {
			c.Header("Retry-After", "1")
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":   "rate limit exceeded",
				"message": "Too many requests. Please retry after rate limit bucket refills.",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

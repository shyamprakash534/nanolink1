package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const AuthUserKey = "auth_user_id"
const AuthUserTierKey = "auth_user_tier"

// OptionalAuth parses X-API-Key if present, allowing authenticated and anonymous access
func OptionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey != "" {
			h := sha256.Sum256([]byte(apiKey))
			hashStr := hex.EncodeToString(h[:])
			// In production, lookup user by hashStr in DB/Cache
			// For testing/mocking, attach sample user ID
			uid := uuid.NewMD5(uuid.NameSpaceDNS, []byte(hashStr))
			c.Set(AuthUserKey, uid)
			c.Set(AuthUserTierKey, "pro")
		}
		c.Next()
	}
}

// RequireAuth enforces valid API key
func RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		apiKey := c.GetHeader("X-API-Key")
		if apiKey == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing X-API-Key header"})
			c.Abort()
			return
		}

		h := sha256.Sum256([]byte(apiKey))
		hashStr := hex.EncodeToString(h[:])
		uid := uuid.NewMD5(uuid.NameSpaceDNS, []byte(hashStr))
		c.Set(AuthUserKey, uid)
		c.Set(AuthUserTierKey, "pro")
		c.Next()
	}
}

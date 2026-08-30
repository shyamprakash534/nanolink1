package middleware

import (
    "crypto/sha256"
    "encoding/hex"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "github.com/nanolink/nanolink/internal/storage/postgres"
)

const AuthUserKey = "auth_user_id"
const AuthUserTierKey = "auth_user_tier"

type AuthMiddleware struct {
    db *postgres.DB
}

func NewAuthMiddleware(db *postgres.DB) *AuthMiddleware {
    return &AuthMiddleware{db: db}
}

func (m *AuthMiddleware) lookupUser(apiKey string) (uuid.UUID, string, error) {
    h := sha256.Sum256([]byte(apiKey))
    hashStr := hex.EncodeToString(h[:])

    var userID uuid.UUID
    var tier string

    err := m.db.QueryRow(
        "SELECT id, tier FROM users WHERE api_key_hash = $1",
        hashStr,
    ).Scan(&userID, &tier)

    return userID, tier, err
}

// OptionalAuth parses X-API-Key if present.
func (m *AuthMiddleware) OptionalAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        apiKey := c.GetHeader("X-API-Key")

        if apiKey != "" {
            userID, tier, err := m.lookupUser(apiKey)
            if err == nil {
                c.Set(AuthUserKey, userID)
                c.Set(AuthUserTierKey, tier)
            }
        }

        c.Next()
    }
}

// RequireAuth enforces a valid API key.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
    return func(c *gin.Context) {
        apiKey := c.GetHeader("X-API-Key")
        if apiKey == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "missing X-API-Key header"})
            c.Abort()
            return
        }

        userID, tier, err := m.lookupUser(apiKey)
        if err != nil {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
            c.Abort()
            return
        }

        c.Set(AuthUserKey, userID)
        c.Set(AuthUserTierKey, tier)
        c.Next()
    }
}

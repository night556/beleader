package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"beleader/gateway/db"

	"github.com/gin-gonic/gin"
)

// AuthContext stores the resolved auth info for a request.
type AuthContext struct {
	TenantID int64  `json:"tenant_id"`
	APIKeyID int64  `json:"api_key_id"`
	Scope    string `json:"scope"` // "admin", "console", "api"
	UserID   string `json:"user_id"`
}

// UserToken is the JWT-like payload signed by the tenant.
type UserToken struct {
	TenantID int64  `json:"tenant_id"`
	UserID   string `json:"user_id"`
	Exp      int64  `json:"exp"`
}

// AuthMiddleware resolves API key or signed user token.
func AuthMiddleware(database *db.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("Authorization")
		if len(key) > 7 && key[:7] == "Bearer " {
			key = key[7:]
		}
		if key == "" {
			key = c.Query("token")
		}
		if key == "" {
			c.Next()
			return
		}

		// Try API key first (admin or console)
		if strings.HasPrefix(key, "bl_") || strings.HasPrefix(key, "sk_") {
			ak, err := database.GetAPIKeyByKey(key)
			if err == nil {
				auth := &AuthContext{
					TenantID: ak.TenantID,
					APIKeyID: ak.ID,
					Scope:    ak.Scope,
					UserID:   c.GetHeader("X-User-ID"),
				}
				c.Set("auth", auth)
				c.Next()
				return
			}
		}

		// Try signed user token (tenant-issued JWT)
		if auth := verifyUserToken(database, key); auth != nil {
			c.Set("auth", auth)
			c.Next()
			return
		}

		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key or token"})
		c.Abort()
	}
}

// verifyUserToken verifies a tenant-signed user token.
func verifyUserToken(database *db.DB, token string) *AuthContext {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil
	}

	var ut UserToken
	if err := json.Unmarshal(payloadJSON, &ut); err != nil {
		return nil
	}

	// Check expiration
	if ut.Exp > 0 && time.Now().Unix() > ut.Exp {
		return nil
	}

	// Verify signature from tenant_secrets
	secret, err := database.GetTenantSecret(ut.TenantID)
	if err != nil || secret.SigningKey == "" {
		return nil
	}

	mac := hmac.New(sha256.New, []byte(secret.SigningKey))
	mac.Write([]byte(parts[0]))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(parts[1]), []byte(expectedSig)) {
		return nil
	}

	return &AuthContext{
		TenantID: ut.TenantID,
		Scope:    "api",
		UserID:   ut.UserID,
	}
}

// GenerateUserToken creates a signed token for a tenant's user.
func GenerateUserToken(secret *db.TenantSecret, tenantID int64, userID string, expiry time.Duration) (string, error) {
	ut := UserToken{
		TenantID: tenantID,
		UserID:   userID,
		Exp:      time.Now().Add(expiry).Unix(),
	}
	payload, _ := json.Marshal(ut)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)

	mac := hmac.New(sha256.New, []byte(secret.SigningKey))
	mac.Write([]byte(payloadB64))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return payloadB64 + "." + sig, nil
}

// GetAuth extracts the auth context from the gin context.
func GetAuth(c *gin.Context) *AuthContext {
	if a, ok := c.Get("auth"); ok {
		return a.(*AuthContext)
	}
	return nil
}

// RequireScope creates middleware that requires a minimum scope.
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := GetAuth(c)
		if auth == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}
		if auth.Scope != scope && auth.Scope != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			c.Abort()
			return
		}
		c.Next()
	}
}

// RequireAdmin requires admin scope.
func RequireAdmin(c *gin.Context) {
	auth := GetAuth(c)
	if auth == nil || auth.Scope != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		c.Abort()
		return
	}
	c.Next()
}

// TenantIDFromAuth returns the tenant ID from auth, or 0 if no auth.
func TenantIDFromAuth(c *gin.Context) int64 {
	auth := GetAuth(c)
	if auth != nil {
		return auth.TenantID
	}
	return 0
}

// TenantIDPtr returns a pointer to the tenant ID, or nil if no auth.
func TenantIDPtr(c *gin.Context) *int64 {
	auth := GetAuth(c)
	if auth != nil && auth.TenantID > 0 {
		tid := auth.TenantID
		return &tid
	}
	return nil
}
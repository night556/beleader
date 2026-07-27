package api

import (
	"net/http"
	"strconv"

	"beleader/gateway/db"

	"github.com/gin-gonic/gin"
)

// AuthContext stores the resolved auth info for a request.
type AuthContext struct {
	TenantID int64
	APIKeyID int64
	Scope    string // "admin", "console", "api"
	UserID   string // from X-User-ID header
}

// AuthMiddleware resolves the API key and injects auth info into the context.
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

		ak, err := database.GetAPIKeyByKey(key)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid API key"})
			c.Abort()
			return
		}

		auth := &AuthContext{
			TenantID: ak.TenantID,
			APIKeyID: ak.ID,
			Scope:    ak.Scope,
			UserID:   c.GetHeader("X-User-ID"),
		}
		c.Set("auth", auth)
		c.Next()
	}
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

// ParseTenantID returns the tenant ID from auth, with admin override via query param.
func ParseTenantID(c *gin.Context) int64 {
	auth := GetAuth(c)
	if auth == nil {
		return 0
	}
	if auth.Scope == "admin" {
		if v := c.Query("tenant_id"); v != "" {
			if id, err := strconv.ParseInt(v, 10, 64); err == nil {
				return id
			}
		}
	}
	return auth.TenantID
}
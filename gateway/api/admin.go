package api

import (
	"fmt"
	"strconv"
	"time"

	"beleader/gateway/db"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

// ── Admin: Tenant CRUD ──

func (h *Handler) handleAdminListTenants(c *gin.Context) {
	tenants, err := h.DB.ListTenants()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if tenants == nil {
		tenants = []db.Tenant{}
	}
	c.JSON(200, tenants)
}

func (h *Handler) handleAdminCreateTenant(c *gin.Context) {
	var req struct {
		Name     string `json:"name" binding:"required"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	t, err := h.DB.CreateTenant(req.Name)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if req.Email != "" {
		h.DB.SetTenantEmail(t.ID, req.Email)
		t.Email = req.Email
	}
	if req.Password != "" {
		hash, _ := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		h.DB.SetTenantPassword(t.ID, string(hash))
	}
	c.JSON(201, t)
}

func (h *Handler) handleAdminUpdateTenant(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req map[string]any
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	// Handle email and password separately
	if email, ok := req["email"].(string); ok {
		delete(req, "email")
		h.DB.SetTenantEmail(id, email)
	}
	if password, ok := req["password"].(string); ok && password != "" {
		delete(req, "password")
		hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		h.DB.SetTenantPassword(id, string(hash))
	}
	if len(req) > 0 {
		if err := h.DB.UpdateTenant(id, req); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handleAdminDeleteTenant(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	if err := h.DB.DeleteTenant(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}

func (h *Handler) handleAdminRechargeTenant(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Amount float64 `json:"amount" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	tenant, err := h.DB.GetTenant(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "tenant not found"})
		return
	}
	newBalance := tenant.Balance + req.Amount
	if err := h.DB.UpdateTenant(id, map[string]any{"balance": newBalance}); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"balance": newBalance})
}

// ── Admin: API Key Management ──

func (h *Handler) handleAdminCreateTenantKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Name  string `json:"name"`
		Scope string `json:"scope"` // "console" or "api"
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Scope == "" {
		req.Scope = "api"
	}
	ak, err := h.DB.CreateAPIKey(id, req.Name, req.Scope)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, ak)
}

func (h *Handler) handleAdminListTenantKeys(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	keys, err := h.DB.ListAPIKeys(id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if keys == nil {
		keys = []db.APIKey{}
	}
	c.JSON(200, keys)
}

func (h *Handler) handleAdminDeleteTenantKey(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	kid, err := strconv.ParseInt(c.Param("kid"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid key id"})
		return
	}
	if err := h.DB.DeleteAPIKey(kid); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	_ = id
	c.JSON(200, gin.H{"status": "deleted"})
}

// ── Admin: Usage ──

func (h *Handler) handleAdminTenantUsage(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	days := 30
	if v := c.Query("days"); v != "" {
		fmt.Sscanf(v, "%d", &days)
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	records, err := h.DB.GetTenantUsage(id, since)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	totalTokens, _ := h.DB.GetTenantUsageSummary(id, since)
	c.JSON(200, gin.H{
		"records":      records,
		"total_tokens": totalTokens,
		"days":         days,
	})
}

// ── Console: API Key Management ──

func (h *Handler) handleConsoleCreateKey(c *gin.Context) {
	auth := GetAuth(c)
	if auth == nil {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	var req struct {
		Name  string `json:"name"`
		Scope string `json:"scope"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Scope == "" {
		req.Scope = "api"
	}
	ak, err := h.DB.CreateAPIKey(auth.TenantID, req.Name, req.Scope)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, ak)
}

func (h *Handler) handleConsoleListKeys(c *gin.Context) {
	auth := GetAuth(c)
	if auth == nil {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	keys, err := h.DB.ListAPIKeys(auth.TenantID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if keys == nil {
		keys = []db.APIKey{}
	}
	c.JSON(200, keys)
}

func (h *Handler) handleConsoleDeleteKey(c *gin.Context) {
	auth := GetAuth(c)
	if auth == nil {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	kid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	// Verify the key belongs to the tenant
	keys, _ := h.DB.ListAPIKeys(auth.TenantID)
	found := false
	for _, k := range keys {
		if k.ID == kid {
			found = true
			break
		}
	}
	if !found {
		c.JSON(404, gin.H{"error": "key not found"})
		return
	}
	if err := h.DB.DeleteAPIKey(kid); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}

// ── Console: Usage ──

func (h *Handler) handleConsoleUsage(c *gin.Context) {
	auth := GetAuth(c)
	if auth == nil {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	days := 30
	if v := c.Query("days"); v != "" {
		fmt.Sscanf(v, "%d", &days)
	}
	since := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	records, err := h.DB.GetTenantUsage(auth.TenantID, since)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	totalTokens, _ := h.DB.GetTenantUsageSummary(auth.TenantID, since)
	c.JSON(200, gin.H{
		"records":      records,
		"total_tokens": totalTokens,
		"days":         days,
	})
}

// ── Console: Dashboard ──

func (h *Handler) handleConsoleDashboard(c *gin.Context) {
	auth := GetAuth(c)
	if auth == nil {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}
	tenant, err := h.DB.GetTenant(auth.TenantID)
	if err != nil {
		c.JSON(404, gin.H{"error": "tenant not found"})
		return
	}
	secret, _ := h.DB.GetTenantSecret(auth.TenantID)
	sk := ""
	if secret != nil {
		sk = secret.SigningKey
	}
	since := time.Now().Add(-24 * time.Hour)
	total24h, _ := h.DB.GetTenantUsageSummary(auth.TenantID, since)
	c.JSON(200, gin.H{
		"tenant":      tenant,
		"tokens_24h":  total24h,
		"signing_key": sk,
	})
}

// ── Console: Login ──

func (h *Handler) handleConsoleLogin(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	tenant, err := h.DB.GetTenantByEmail(req.Email)
	if err != nil {
		c.JSON(401, gin.H{"error": "invalid email or password"})
		return
	}

	secret, err := h.DB.GetTenantSecret(tenant.ID)
	if err != nil {
		c.JSON(401, gin.H{"error": "invalid email or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(secret.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(401, gin.H{"error": "invalid email or password"})
		return
	}

	h.DB.LogSecretAudit(tenant.ID, "login", c.ClientIP())

	ak, err := h.DB.CreateAPIKey(tenant.ID, "console", "console")
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"token":       ak.Key,
		"tenant_id":   tenant.ID,
		"tenant":      tenant.Name,
		"signing_key": secret.SigningKey,
	})
}

// ── Console: Generate User Token ──

func (h *Handler) handleConsoleGenerateToken(c *gin.Context) {
	auth := GetAuth(c)
	if auth == nil {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		UserID string `json:"user_id" binding:"required"`
		Expiry int    `json:"expiry"` // hours, default 24
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Expiry <= 0 {
		req.Expiry = 24
	}

	secret, err := h.DB.GetTenantSecret(auth.TenantID)
	if err != nil {
		c.JSON(404, gin.H{"error": "tenant secret not found"})
		return
	}

	token, err := GenerateUserToken(secret, auth.TenantID, req.UserID, time.Duration(req.Expiry)*time.Hour)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"token":   token,
		"user_id": req.UserID,
		"expires": time.Now().Add(time.Duration(req.Expiry) * time.Hour).Format(time.RFC3339),
	})
}

// ── Admin: Tenant Secret ──

func (h *Handler) handleAdminTenantSecret(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	secret, err := h.DB.GetTenantSecret(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "secret not found"})
		return
	}
	h.DB.LogSecretAudit(id, "viewed", c.ClientIP())
	c.JSON(200, gin.H{"signing_key": secret.SigningKey})
}

func (h *Handler) handleAdminRotateSecret(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	newKey, err := h.DB.RotateSigningKey(id)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	h.DB.LogSecretAudit(id, "rotated", c.ClientIP())
	c.JSON(200, gin.H{"signing_key": newKey})
}
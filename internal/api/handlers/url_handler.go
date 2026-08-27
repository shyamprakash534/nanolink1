package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/nanolink/nanolink/internal/api/middleware"
	"github.com/nanolink/nanolink/internal/models"
	"github.com/nanolink/nanolink/internal/service"
	"github.com/skip2/go-qrcode"
)

type URLHandler struct {
	urlService       *service.URLService
	analyticsService *service.AnalyticsService
}

func NewURLHandler(urlService *service.URLService, analyticsService *service.AnalyticsService) *URLHandler {
	return &URLHandler{
		urlService:       urlService,
		analyticsService: analyticsService,
	}
}

func (h *URLHandler) ShortenURL(c *gin.Context) {
	var req models.ShortenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var userID *uuid.UUID
	if val, exists := c.Get(middleware.AuthUserKey); exists {
		if uid, ok := val.(uuid.UUID); ok {
			userID = &uid
		}
	}

	resp, err := h.urlService.ShortenURL(c.Request.Context(), req, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, resp)
}

func (h *URLHandler) Redirect(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid short code"})
		return
	}

	longURL, err := h.urlService.ResolveURL(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "short code not found or expired"})
		return
	}

	// Asynchronous click telemetry ingestion
	go func(shortCode, ip, ua, ref string) {
		h.analyticsService.RecordClick(c.Request.Context(), shortCode, ip, ua, ref)
	}(code, c.ClientIP(), c.GetHeader("User-Agent"), c.GetHeader("Referer"))

	c.Redirect(http.StatusFound, longURL)
}

func (h *URLHandler) DeleteURL(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid short code"})
		return
	}

	var userID *uuid.UUID
	if val, exists := c.Get(middleware.AuthUserKey); exists {
		if uid, ok := val.(uuid.UUID); ok {
			userID = &uid
		}
	}

	if err := h.urlService.DeleteURL(c.Request.Context(), code, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "URL successfully deactivated", "short_code": code})
}

func (h *URLHandler) ListURLs(c *gin.Context) {
	val, exists := c.Get(middleware.AuthUserKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID := val.(uuid.UUID)

	limit := 20
	offset := 0
	if l := c.Query("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 && val <= 100 {
			limit = val
		}
	}
	if o := c.Query("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			offset = val
		}
	}

	urls, err := h.urlService.ListUserURLs(c.Request.Context(), userID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"urls": urls, "limit": limit, "offset": offset})
}

func (h *URLHandler) GenerateQRCode(c *gin.Context) {
	code := c.Param("code")
	longURL, err := h.urlService.ResolveURL(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "short code not found"})
		return
	}

	png, err := qrcode.Encode(longURL, qrcode.Medium, 256)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate qr code"})
		return
	}

	c.Data(http.StatusOK, "image/png", png)
}

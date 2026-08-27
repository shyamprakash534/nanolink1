package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nanolink/nanolink/internal/service"
)

type AnalyticsHandler struct {
	analyticsService *service.AnalyticsService
	urlService       *service.URLService
}

func NewAnalyticsHandler(analyticsService *service.AnalyticsService, urlService *service.URLService) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: analyticsService,
		urlService:       urlService,
	}
}

func (h *AnalyticsHandler) GetStats(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid short code"})
		return
	}

	stats, err := h.analyticsService.GetStats(c.Request.Context(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

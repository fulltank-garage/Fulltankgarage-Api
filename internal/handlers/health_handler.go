package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type HealthHandler struct {
	startedAt time.Time
}

func NewHealthHandler() *HealthHandler {
	return &HealthHandler{startedAt: time.Now()}
}

func (h *HealthHandler) Check(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"service":   "fulltankgarage-api",
		"startedAt": h.startedAt.Format(time.RFC3339),
	})
}

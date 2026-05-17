package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type HealthHandler struct {
	startedAt time.Time
	db        *gorm.DB
}

func NewHealthHandler(db *gorm.DB) *HealthHandler {
	return &HealthHandler{startedAt: time.Now(), db: db}
}

func (h *HealthHandler) Check(c *gin.Context) {
	checks := gin.H{"api": "ok"}
	status := "ok"

	if h.db != nil {
		sqlDB, err := h.db.DB()
		if err != nil || sqlDB.PingContext(c.Request.Context()) != nil {
			checks["database"] = "error"
			status = "degraded"
		} else {
			checks["database"] = "ok"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"health":    status,
		"service":   "fulltankgarage-api",
		"checks":    checks,
		"startedAt": h.startedAt.Format(time.RFC3339),
	})
}

package handlers

import (
	"strings"

	"gorm.io/gorm"

	"github.com/fulltank-garage/fulltankgarage-api/internal/cache"
	"github.com/fulltank-garage/fulltankgarage-api/internal/realtime"
	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

type FulltankHandler struct {
	db        *gorm.DB
	cache     *cache.Store
	uploadDir string
	baseURL   string
	richMenu  *services.RichMenuService
	richQueue *services.RichMenuSyncQueue
	events    *realtime.Hub
}

func NewFulltankHandler(db *gorm.DB, cacheStore *cache.Store, uploadDir string, baseURL string, richMenu *services.RichMenuService, richQueue *services.RichMenuSyncQueue, events *realtime.Hub) *FulltankHandler {
	return &FulltankHandler{
		db:        db,
		cache:     cacheStore,
		uploadDir: uploadDir,
		baseURL:   strings.TrimRight(baseURL, "/"),
		richMenu:  richMenu,
		richQueue: richQueue,
		events:    events,
	}
}

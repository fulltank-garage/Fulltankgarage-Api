package handlers

import (
	"log"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (h *FulltankHandler) allowSerialCheck(c *gin.Context, serial string) bool {
	if h.cache == nil {
		return true
	}

	keys := []string{"rate:serial-check:ip:" + c.ClientIP()}
	if serial != "" {
		keys = append(keys, "rate:serial-check:serial:"+serial)
	}

	for _, key := range keys {
		allowed, _, err := h.cache.RateLimit(c.Request.Context(), key, 60, time.Minute)
		if err != nil {
			continue
		}
		if !allowed {
			return false
		}
	}

	return true
}

func (h *FulltankHandler) acquireSerialLock(c *gin.Context, key string) (string, bool, error) {
	if h.cache == nil {
		return "", true, nil
	}

	return h.cache.AcquireLock(c.Request.Context(), key, 30*time.Second)
}

func (h *FulltankHandler) releaseSerialLock(c *gin.Context, key string, token string) {
	if h.cache == nil {
		return
	}

	if err := h.cache.ReleaseLock(c.Request.Context(), key, token); err != nil {
		log.Printf("release serial registration lock: %v", err)
	}
}

func (h *FulltankHandler) getCachedList(c *gin.Context, key string, dest any) bool {
	if h.cache == nil {
		return false
	}

	ok, err := h.cache.GetJSON(c.Request.Context(), key, dest)
	if err != nil {
		log.Printf("read list cache %s: %v", key, err)
		return false
	}

	return ok
}

func (h *FulltankHandler) setCachedList(c *gin.Context, key string, value any) {
	if h.cache == nil {
		return
	}

	if err := h.cache.SetJSON(c.Request.Context(), key, value); err != nil {
		log.Printf("write list cache %s: %v", key, err)
	}
}

func (h *FulltankHandler) filmCacheKey(c *gin.Context) string {
	if isPublicListRequest(c) {
		return "cache:films:public"
	}

	return "cache:films:admin"
}

func (h *FulltankHandler) promotionCacheKey(c *gin.Context) string {
	if isPublicListRequest(c) {
		return "cache:promotions:public"
	}

	return "cache:promotions:admin"
}

func (h *FulltankHandler) clearFilmCache(c *gin.Context) {
	if h.cache == nil {
		return
	}

	if err := h.cache.Delete(c.Request.Context(), "cache:films:public", "cache:films:admin"); err != nil {
		log.Printf("clear film cache: %v", err)
	}
}

func (h *FulltankHandler) clearPromotionCache(c *gin.Context) {
	if h.cache == nil {
		return
	}

	if err := h.cache.Delete(c.Request.Context(), "cache:promotions:public", "cache:promotions:admin"); err != nil {
		log.Printf("clear promotion cache: %v", err)
	}
}

func isPublicListRequest(c *gin.Context) bool {
	return c.Query("public") == "true" || strings.HasPrefix(c.FullPath(), "/api/public/")
}

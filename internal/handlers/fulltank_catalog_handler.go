package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/httpx"
	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
)

type promotionPayload struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Detail      string `json:"detail"`
	ImageURL    string `json:"imageUrl"`
	IsActive    *bool  `json:"isActive"`
	StartsAt    string `json:"startsAt"`
	EndsAt      string `json:"endsAt"`
}

func (payload promotionPayload) toModel() (models.Promotion, error) {
	startsAt, err := parseFlexibleDate(payload.StartsAt)
	if err != nil {
		return models.Promotion{}, err
	}

	endsAt, err := parseFlexibleDate(payload.EndsAt)
	if err != nil {
		return models.Promotion{}, err
	}

	isActive := true
	if payload.IsActive != nil {
		isActive = *payload.IsActive
	}

	return models.Promotion{
		Title:       strings.TrimSpace(payload.Title),
		Description: strings.TrimSpace(payload.Description),
		Detail:      strings.TrimSpace(payload.Detail),
		ImageURL:    strings.TrimSpace(payload.ImageURL),
		IsActive:    isActive,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
	}, nil
}

func (h *FulltankHandler) ListFilms(c *gin.Context) {
	cacheKey := h.filmCacheKey(c)
	var cached []models.FilmProduct
	if h.getCachedList(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	var items []models.FilmProduct
	query := h.db.Order("created_at DESC")
	if isPublicListRequest(c) {
		query = query.Where("is_active = ?", true)
	}
	if err := query.Find(&items).Error; err != nil {
		httpx.Internal(c, "โหลดข้อมูลฟิล์มไม่สำเร็จ")
		return
	}
	h.setCachedList(c, cacheKey, items)
	c.JSON(http.StatusOK, items)
}

func (h *FulltankHandler) CreateFilm(c *gin.Context) {
	var input models.FilmProduct
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลฟิล์มไม่ถูกต้อง")
		return
	}
	if input.Slug == "" {
		input.Slug = slugify(input.Name)
	}
	if err := h.db.Create(&input).Error; err != nil {
		httpx.Internal(c, "บันทึกข้อมูลฟิล์มไม่สำเร็จ")
		return
	}
	h.clearFilmCache(c)
	h.publishEvent("film.created", input)
	c.JSON(http.StatusCreated, input)
}

func (h *FulltankHandler) UpdateFilm(c *gin.Context) {
	var input models.FilmProduct
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลฟิล์มไม่ถูกต้อง")
		return
	}
	var item models.FilmProduct
	if err := h.db.First(&item, c.Param("id")).Error; err != nil {
		httpx.NotFound(c, "ไม่พบข้อมูลฟิล์ม")
		return
	}
	if err := h.db.Model(&item).Updates(input).Error; err != nil {
		httpx.Internal(c, "อัปเดตข้อมูลฟิล์มไม่สำเร็จ")
		return
	}
	if err := h.db.First(&item, item.ID).Error; err != nil {
		httpx.Internal(c, "โหลดข้อมูลฟิล์มที่อัปเดตไม่สำเร็จ")
		return
	}
	h.clearFilmCache(c)
	h.publishEvent("film.updated", item)
	c.JSON(http.StatusOK, item)
}

func (h *FulltankHandler) DeleteFilm(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&models.FilmProduct{}, c.Param("id")).Error; err != nil {
		httpx.Internal(c, "ลบข้อมูลฟิล์มไม่สำเร็จ")
		return
	}
	h.clearFilmCache(c)
	h.publishEvent("film.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

func (h *FulltankHandler) ListPromotions(c *gin.Context) {
	cacheKey := h.promotionCacheKey(c)
	var cached []models.Promotion
	if h.getCachedList(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	var items []models.Promotion
	query := h.db.Order("created_at DESC")
	if isPublicListRequest(c) {
		query = query.Where("is_active = ?", true)
	}
	if err := query.Find(&items).Error; err != nil {
		httpx.Internal(c, "โหลดโปรโมชันไม่สำเร็จ")
		return
	}
	h.setCachedList(c, cacheKey, items)
	c.JSON(http.StatusOK, items)
}

func (h *FulltankHandler) CreatePromotion(c *gin.Context) {
	var payload promotionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลโปรโมชันไม่ถูกต้อง")
		return
	}
	input, err := payload.toModel()
	if err != nil {
		httpx.BadRequest(c, "รูปแบบวันที่โปรโมชันไม่ถูกต้อง")
		return
	}
	if input.Title == "" {
		httpx.BadRequest(c, "กรุณากรอกชื่อโปรโมชัน")
		return
	}
	if err := h.db.Create(&input).Error; err != nil {
		httpx.Internal(c, "บันทึกโปรโมชันไม่สำเร็จ")
		return
	}
	h.clearPromotionCache(c)
	h.publishEvent("promotion.created", input)
	c.JSON(http.StatusCreated, input)
}

func (h *FulltankHandler) UpdatePromotion(c *gin.Context) {
	var payload promotionPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลโปรโมชันไม่ถูกต้อง")
		return
	}
	input, err := payload.toModel()
	if err != nil {
		httpx.BadRequest(c, "รูปแบบวันที่โปรโมชันไม่ถูกต้อง")
		return
	}
	if input.Title == "" {
		httpx.BadRequest(c, "กรุณากรอกชื่อโปรโมชัน")
		return
	}
	var item models.Promotion
	if err := h.db.First(&item, c.Param("id")).Error; err != nil {
		httpx.NotFound(c, "ไม่พบโปรโมชัน")
		return
	}
	updates := map[string]interface{}{
		"title":       input.Title,
		"description": input.Description,
		"detail":      input.Detail,
		"image_url":   input.ImageURL,
		"is_active":   input.IsActive,
		"starts_at":   input.StartsAt,
		"ends_at":     input.EndsAt,
	}
	if err := h.db.Model(&item).Updates(updates).Error; err != nil {
		httpx.Internal(c, "อัปเดตโปรโมชันไม่สำเร็จ")
		return
	}
	if err := h.db.First(&item, item.ID).Error; err != nil {
		httpx.Internal(c, "โหลดโปรโมชันที่อัปเดตไม่สำเร็จ")
		return
	}
	h.clearPromotionCache(c)
	h.publishEvent("promotion.updated", item)
	c.JSON(http.StatusOK, item)
}

func (h *FulltankHandler) DeletePromotion(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.Delete(&models.Promotion{}, id).Error; err != nil {
		httpx.Internal(c, "ลบโปรโมชันไม่สำเร็จ")
		return
	}
	h.clearPromotionCache(c)
	h.publishEvent("promotion.deleted", gin.H{"id": id})
	c.Status(http.StatusNoContent)
}

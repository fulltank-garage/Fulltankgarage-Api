package handlers

import (
	"errors"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/httpx"
)

func (h *FulltankHandler) UploadImage(c *gin.Context) {
	file, err := c.FormFile("image")
	if err != nil {
		httpx.BadRequest(c, "กรุณาเลือกไฟล์รูปภาพ")
		return
	}
	if !isImageUpload(file.Filename, file.Header.Get("Content-Type")) {
		httpx.BadRequest(c, "รองรับเฉพาะไฟล์รูปภาพเท่านั้น")
		return
	}

	imageDir := filepath.Join(h.uploadDir, "images")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		httpx.Internal(c, "เตรียมพื้นที่จัดเก็บรูปไม่สำเร็จ")
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == ".jpeg" {
		ext = ".jpg"
	}
	filename := time.Now().Format("20060102150405") + "-" + slugify(strings.TrimSuffix(file.Filename, filepath.Ext(file.Filename))) + ext
	target := filepath.Join(imageDir, filename)
	if err := c.SaveUploadedFile(file, target); err != nil {
		httpx.Internal(c, "อัปโหลดรูปไม่สำเร็จ")
		return
	}

	publicPath := "/" + path.Join("uploads", "images", filename)
	c.JSON(http.StatusCreated, gin.H{"imageUrl": h.absoluteURL(publicPath)})
}

func (h *FulltankHandler) saveReceipt(c *gin.Context) (string, error) {
	file, err := c.FormFile("receiptFile")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil
		}
		return "", err
	}

	receiptDir := filepath.Join(h.uploadDir, "receipts")
	if err := os.MkdirAll(receiptDir, 0o755); err != nil {
		return "", err
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext == "" {
		ext = ".jpg"
	}
	filename := time.Now().Format("20060102150405") + "-" + slugify(strings.TrimSuffix(file.Filename, ext)) + ext
	target := filepath.Join(receiptDir, filename)
	if err := c.SaveUploadedFile(file, target); err != nil {
		return "", err
	}

	return "/uploads/receipts/" + filename, nil
}

func (h *FulltankHandler) absoluteURL(publicPath string) string {
	if h.baseURL == "" {
		return publicPath
	}

	return h.baseURL + publicPath
}

func isImageUpload(filename string, contentType string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	allowedExt := ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".webp" || ext == ".gif"
	return allowedExt && strings.HasPrefix(strings.ToLower(contentType), "image/")
}

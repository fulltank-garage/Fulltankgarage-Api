package services

import (
	"errors"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

func removeStorefrontImage(path string) {
	if path == "" {
		return
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("remove storefront image: %v", err)
	}
}

func (s *MemberService) saveStorefrontImage(fileHeader *multipart.FileHeader) (string, string, error) {
	src, err := fileHeader.Open()
	if err != nil {
		return "", "", err
	}
	defer src.Close()

	ext := imageExtension(fileHeader)
	month := time.Now().UTC().Format("200601")
	filename := uuid.NewString() + ext
	targetDir := filepath.Join(s.cfg.UploadDir, "storefronts", month)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return "", "", err
	}

	targetPath := filepath.Join(targetDir, filename)
	dst, err := os.Create(targetPath)
	if err != nil {
		return "", "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", "", err
	}

	publicPath := path.Join("uploads", "storefronts", month, filename)
	publicURL := s.cfg.BaseURL + "/" + publicPath

	return filepath.ToSlash(targetPath), publicURL, nil
}

func isAllowedImage(fileHeader *multipart.FileHeader) bool {
	contentType := strings.ToLower(strings.TrimSpace(fileHeader.Header.Get("Content-Type")))
	if strings.HasPrefix(contentType, "image/") {
		return true
	}

	switch strings.ToLower(filepath.Ext(fileHeader.Filename)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func imageExtension(fileHeader *multipart.FileHeader) string {
	switch strings.ToLower(strings.TrimSpace(fileHeader.Header.Get("Content-Type"))) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}

	switch strings.ToLower(filepath.Ext(fileHeader.Filename)) {
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".png":
		return ".png"
	case ".webp":
		return ".webp"
	case ".gif":
		return ".gif"
	default:
		return ".jpg"
	}
}

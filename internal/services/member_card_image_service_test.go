package services

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
)

func TestMemberCardImageServiceGenerate(t *testing.T) {
	uploadDir := t.TempDir()
	service, err := NewMemberCardImageService(config.Config{
		BaseURL:   "https://example.com",
		UploadDir: uploadDir,
	})
	if err != nil {
		t.Fatalf("new member card image service: %v", err)
	}

	result, err := service.Generate("member-1", "ทดสอบ ระบบ")
	if err != nil {
		t.Fatalf("generate member card: %v", err)
	}

	if result.ImageURL != "https://example.com/uploads/line-member-cards/member-1-91379bad046c.png" {
		t.Fatalf("unexpected image url: %s", result.ImageURL)
	}
	if result.PreviewURL != "https://example.com/uploads/line-member-cards/member-1-91379bad046c-preview.png" {
		t.Fatalf("unexpected preview url: %s", result.PreviewURL)
	}

	assertPNG(t, filepath.Join(uploadDir, "line-member-cards", "member-1-91379bad046c.png"))
	assertPNG(t, filepath.Join(uploadDir, "line-member-cards", "member-1-91379bad046c-preview.png"))
}

func assertPNG(t *testing.T, filename string) {
	t.Helper()

	file, err := os.Open(filename)
	if err != nil {
		t.Fatalf("open generated png: %v", err)
	}
	defer file.Close()

	if _, err := png.Decode(file); err != nil {
		t.Fatalf("decode generated png: %v", err)
	}
}

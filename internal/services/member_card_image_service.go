package services

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path"
	"path/filepath"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
)

const (
	memberCardTemplatePath = "assets/line/registration-success-template.png"
	memberCardFontPath     = "assets/fonts/NotoSansThai.ttf"
)

type MemberCardImageService struct {
	baseURL   string
	uploadDir string
	font      *opentype.Font
}

type MemberCardImageResult struct {
	ImageURL   string
	PreviewURL string
}

func NewMemberCardImageService(cfg config.Config) (*MemberCardImageService, error) {
	fontBytes, err := os.ReadFile(resolveAssetPath(memberCardFontPath))
	if err != nil {
		return nil, err
	}

	parsedFont, err := opentype.Parse(fontBytes)
	if err != nil {
		return nil, err
	}

	return &MemberCardImageService{
		baseURL:   cfg.BaseURL,
		uploadDir: cfg.UploadDir,
		font:      parsedFont,
	}, nil
}

func (s *MemberCardImageService) Generate(memberID string, customerName string) (MemberCardImageResult, error) {
	memberID = strings.TrimSpace(memberID)
	customerName = strings.Join(strings.Fields(customerName), " ")
	if memberID == "" || customerName == "" {
		return MemberCardImageResult{}, validationError("ไม่พบข้อมูลชื่อสำหรับสร้างรูป member")
	}

	templateFile, err := os.Open(resolveAssetPath(memberCardTemplatePath))
	if err != nil {
		return MemberCardImageResult{}, err
	}
	defer templateFile.Close()

	templateImage, _, err := image.Decode(templateFile)
	if err != nil {
		return MemberCardImageResult{}, err
	}

	card := image.NewRGBA(templateImage.Bounds())
	draw.Draw(card, card.Bounds(), templateImage, image.Point{}, draw.Src)
	s.drawNamePill(card, customerName)

	filename := safeMemberCardFilename(memberID, customerName)
	targetDir := filepath.Join(s.uploadDir, "line-member-cards")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return MemberCardImageResult{}, err
	}

	imagePath := filepath.Join(targetDir, filename+".png")
	if err := writePNG(imagePath, card); err != nil {
		return MemberCardImageResult{}, err
	}

	previewPath := filepath.Join(targetDir, filename+"-preview.png")
	if err := writePNG(previewPath, resizeImage(card, 400, 400)); err != nil {
		return MemberCardImageResult{}, err
	}

	publicImagePath := path.Join("uploads", "line-member-cards", filename+".png")
	publicPreviewPath := path.Join("uploads", "line-member-cards", filename+"-preview.png")

	return MemberCardImageResult{
		ImageURL:   s.baseURL + "/" + publicImagePath,
		PreviewURL: s.baseURL + "/" + publicPreviewPath,
	}, nil
}

func (s *MemberCardImageService) drawNamePill(img *image.RGBA, customerName string) {
	pill := image.Rect(367, 833, 887, 933)

	face := s.fontFaceForText(customerName, pill.Dx()-72)
	defer face.Close()

	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA{R: 112, G: 82, B: 56, A: 255}),
		Face: face,
	}
	textWidth := d.MeasureString(customerName).Round()
	metrics := face.Metrics()
	textHeight := (metrics.Ascent + metrics.Descent).Round()
	x := pill.Min.X + (pill.Dx()-textWidth)/2
	y := pill.Min.Y + (pill.Dy()-textHeight)/2 + metrics.Ascent.Round() - 2
	d.Dot = fixed.P(x, y)
	d.DrawString(customerName)
}

func (s *MemberCardImageService) fontFaceForText(text string, maxWidth int) font.Face {
	for size := 58.0; size >= 30; size -= 2 {
		face, err := opentype.NewFace(s.font, &opentype.FaceOptions{
			Size:    size,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err != nil {
			continue
		}

		d := &font.Drawer{Face: face}
		if d.MeasureString(text).Round() <= maxWidth {
			return face
		}
		_ = face.Close()
	}

	face, _ := opentype.NewFace(s.font, &opentype.FaceOptions{
		Size:    30,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	return face
}

func resizeImage(src image.Image, width int, height int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

func writePNG(filename string, img image.Image) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

func safeMemberCardFilename(memberID string, customerName string) string {
	sum := sha1.Sum([]byte(memberID + ":" + customerName))
	return fmt.Sprintf("%s-%s", memberID, hex.EncodeToString(sum[:])[:12])
}

func resolveAssetPath(filename string) string {
	if _, err := os.Stat(filename); err == nil {
		return filename
	}

	for _, prefix := range []string{"..", "../..", "../../.."} {
		candidate := filepath.Join(prefix, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return filename
}

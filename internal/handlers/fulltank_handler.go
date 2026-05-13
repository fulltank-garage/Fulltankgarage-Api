package handlers

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/fulltank-garage/fulltankgarage-api/internal/httpx"
	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

type FulltankHandler struct {
	db        *gorm.DB
	uploadDir string
	baseURL   string
	richMenu  *services.RichMenuService
}

func NewFulltankHandler(db *gorm.DB, uploadDir string, baseURL string, richMenu *services.RichMenuService) *FulltankHandler {
	return &FulltankHandler{
		db:        db,
		uploadDir: uploadDir,
		baseURL:   strings.TrimRight(baseURL, "/"),
		richMenu:  richMenu,
	}
}

func (h *FulltankHandler) CheckSerial(c *gin.Context) {
	serial := normalizeSerial(c.Param("serial"))
	var item models.SerialNumber
	if err := h.db.Where("serial_number = ?", serial).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, gin.H{"serialNumber": serial, "status": "missing"})
			return
		}
		httpx.Internal(c, "ตรวจสอบ Serial Number ไม่สำเร็จ")
		return
	}

	c.JSON(http.StatusOK, item)
}

func (h *FulltankHandler) RegisterWarranty(c *gin.Context) {
	serial := normalizeSerial(c.PostForm("serialNumber"))
	if serial == "" {
		httpx.BadRequest(c, "กรุณากรอก Serial Number")
		return
	}

	installDate, err := parseDate(c.PostForm("installDate"))
	if err != nil {
		httpx.BadRequest(c, "รูปแบบวันที่ติดตั้งไม่ถูกต้อง")
		return
	}

	var created models.WarrantyRegistration
	err = h.db.Transaction(func(tx *gorm.DB) error {
		var serialRecord models.SerialNumber
		if err := tx.Clauses().Where("serial_number = ?", serial).First(&serialRecord).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errSerialMissing
			}
			return err
		}
		if serialRecord.Status != "available" {
			return errSerialUsed
		}

		receiptFile, err := h.saveReceipt(c)
		if err != nil {
			return err
		}

		created = models.WarrantyRegistration{
			SerialNumber:    serial,
			CustomerName:    strings.TrimSpace(c.PostForm("customerName")),
			Phone:           strings.TrimSpace(c.PostForm("phone")),
			CarModel:        strings.TrimSpace(c.PostForm("carModel")),
			LicensePlate:    strings.TrimSpace(c.PostForm("licensePlate")),
			FilmBrand:       strings.TrimSpace(c.PostForm("filmBrand")),
			FilmModel:       strings.TrimSpace(c.PostForm("filmModel")),
			InstallDate:     installDate,
			Branch:          strings.TrimSpace(c.PostForm("branch")),
			InstallerName:   strings.TrimSpace(c.PostForm("installerName")),
			ReceiptFile:     receiptFile,
			Remarks:         strings.TrimSpace(c.PostForm("remarks")),
			LineUserID:      strings.TrimSpace(c.PostForm("lineUserId")),
			LineDisplayName: strings.TrimSpace(c.PostForm("lineDisplayName")),
			LinePictureURL:  strings.TrimSpace(c.PostForm("linePictureUrl")),
		}

		if created.CustomerName == "" || created.Phone == "" {
			return errRequiredCustomer
		}

		if err := tx.Create(&created).Error; err != nil {
			return err
		}

		return tx.Model(&serialRecord).Update("status", "used").Error
	})
	if err != nil {
		switch {
		case errors.Is(err, errSerialMissing):
			httpx.NotFound(c, "ไม่พบ Serial Number นี้")
		case errors.Is(err, errSerialUsed):
			httpx.Conflict(c, "Serial Number นี้ถูกใช้งานแล้ว")
		case errors.Is(err, errRequiredCustomer):
			httpx.BadRequest(c, "กรุณากรอกชื่อลูกค้าและเบอร์โทร")
		default:
			httpx.Internal(c, "ลงทะเบียนรับประกันไม่สำเร็จ")
		}
		return
	}

	richMenuSynced := false
	currentRichMenuID := ""
	if h.richMenu != nil && strings.TrimSpace(created.LineUserID) != "" {
		if err := h.richMenu.LinkMemberRichMenu(c.Request.Context(), created.LineUserID); err != nil {
			log.Printf("link member rich menu after warranty registration failed lineUserID=%s serial=%s: %v", created.LineUserID, created.SerialNumber, err)
		} else {
			richMenuSynced = true
			currentRichMenuID = h.richMenu.MemberRichMenuID()
			if linkedRichMenuID, err := h.richMenu.GetUserRichMenuID(c.Request.Context(), created.LineUserID); err != nil {
				log.Printf("get rich menu after warranty registration failed lineUserID=%s serial=%s: %v", created.LineUserID, created.SerialNumber, err)
			} else if linkedRichMenuID != "" {
				currentRichMenuID = linkedRichMenuID
			}
			log.Printf("link member rich menu after warranty registration succeeded lineUserID=%s serial=%s richMenuID=%s", created.LineUserID, created.SerialNumber, currentRichMenuID)
		}
	}

	c.JSON(http.StatusCreated, gin.H{
		"data":             created,
		"richMenuSynced":   richMenuSynced,
		"linkedRichMenuId": currentRichMenuID,
		"targetRichMenuId": h.targetMemberRichMenuID(),
	})
}

func (h *FulltankHandler) LinkWarranty(c *gin.Context) {
	var input struct {
		SerialNumber    string `json:"serialNumber"`
		LineUserID      string `json:"lineUserId"`
		LineDisplayName string `json:"lineDisplayName"`
		LinePictureURL  string `json:"linePictureUrl"`
	}
	_ = c.ShouldBindJSON(&input)

	serial := normalizeSerial(input.SerialNumber)
	lineUserID := strings.TrimSpace(input.LineUserID)
	if serial == "" || lineUserID == "" {
		httpx.BadRequest(c, "กรุณาเปิดผ่าน LINE และกรอก Serial Number")
		return
	}

	var item models.WarrantyRegistration
	if err := h.db.Where("serial_number = ?", serial).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.NotFound(c, "ยังไม่พบข้อมูลบัตรรับประกัน")
			return
		}
		httpx.Internal(c, "โหลดข้อมูลบัตรรับประกันไม่สำเร็จ")
		return
	}

	if item.LineUserID != "" && item.LineUserID != lineUserID {
		httpx.Conflict(c, "Serial Number นี้ผูกกับ LINE อื่นแล้ว")
		return
	}

	updates := map[string]any{"line_user_id": lineUserID}
	if strings.TrimSpace(input.LineDisplayName) != "" {
		updates["line_display_name"] = strings.TrimSpace(input.LineDisplayName)
	}
	if strings.TrimSpace(input.LinePictureURL) != "" {
		updates["line_picture_url"] = strings.TrimSpace(input.LinePictureURL)
	}
	if err := h.db.Model(&item).Updates(updates).Error; err != nil {
		httpx.Internal(c, "ผูกบัตรรับประกันกับ LINE ไม่สำเร็จ")
		return
	}
	if err := h.db.Where("serial_number = ?", serial).First(&item).Error; err != nil {
		httpx.Internal(c, "โหลดข้อมูลบัตรรับประกันไม่สำเร็จ")
		return
	}

	richMenuSynced := false
	currentRichMenuID := ""
	if h.richMenu != nil {
		if err := h.richMenu.LinkMemberRichMenu(c.Request.Context(), lineUserID); err != nil {
			log.Printf("link member rich menu after warranty link failed lineUserID=%s serial=%s: %v", lineUserID, serial, err)
		} else {
			richMenuSynced = true
			currentRichMenuID = h.richMenu.MemberRichMenuID()
			if linkedRichMenuID, err := h.richMenu.GetUserRichMenuID(c.Request.Context(), lineUserID); err != nil {
				log.Printf("get rich menu after warranty link failed lineUserID=%s serial=%s: %v", lineUserID, serial, err)
			} else if linkedRichMenuID != "" {
				currentRichMenuID = linkedRichMenuID
			}
			log.Printf("link member rich menu after warranty link succeeded lineUserID=%s serial=%s richMenuID=%s", lineUserID, serial, currentRichMenuID)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":             item,
		"richMenuSynced":   richMenuSynced,
		"linkedRichMenuId": currentRichMenuID,
		"targetRichMenuId": h.targetMemberRichMenuID(),
	})
}

func (h *FulltankHandler) targetMemberRichMenuID() string {
	if h.richMenu == nil {
		return ""
	}

	return h.richMenu.MemberRichMenuID()
}

func (h *FulltankHandler) WarrantyStatus(c *gin.Context) {
	lineUserID := strings.TrimSpace(c.Query("lineUserId"))
	if lineUserID == "" {
		httpx.BadRequest(c, "ไม่พบข้อมูล LINE สำหรับตรวจสอบบัตรรับประกัน")
		return
	}

	var item models.WarrantyRegistration
	if err := h.db.Where("line_user_id = ?", lineUserID).Order("created_at DESC").First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.NotFound(c, "ยังไม่พบข้อมูลบัตรรับประกัน")
			return
		}
		httpx.Internal(c, "โหลดข้อมูลบัตรรับประกันไม่สำเร็จ")
		return
	}

	richMenuSynced := false
	currentRichMenuID := ""
	if h.richMenu != nil {
		if err := h.richMenu.LinkMemberRichMenu(c.Request.Context(), lineUserID); err != nil {
			log.Printf("link member rich menu during warranty status failed lineUserID=%s serial=%s: %v", lineUserID, item.SerialNumber, err)
		} else {
			richMenuSynced = true
			currentRichMenuID = h.richMenu.MemberRichMenuID()
			if linkedRichMenuID, err := h.richMenu.GetUserRichMenuID(c.Request.Context(), lineUserID); err != nil {
				log.Printf("get rich menu during warranty status failed lineUserID=%s serial=%s: %v", lineUserID, item.SerialNumber, err)
			} else if linkedRichMenuID != "" {
				currentRichMenuID = linkedRichMenuID
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":             item,
		"richMenuSynced":   richMenuSynced,
		"linkedRichMenuId": currentRichMenuID,
		"targetRichMenuId": h.targetMemberRichMenuID(),
	})
}

func (h *FulltankHandler) ListRegistrations(c *gin.Context) {
	var items []models.WarrantyRegistration
	if err := h.db.Order("created_at DESC").Find(&items).Error; err != nil {
		httpx.Internal(c, "โหลดข้อมูลลูกค้าไม่สำเร็จ")
		return
	}

	c.JSON(http.StatusOK, items)
}

func (h *FulltankHandler) ListSerials(c *gin.Context) {
	var items []models.SerialNumber
	if err := h.db.Order("created_at DESC").Find(&items).Error; err != nil {
		httpx.Internal(c, "โหลด Serial Number ไม่สำเร็จ")
		return
	}

	c.JSON(http.StatusOK, items)
}

func (h *FulltankHandler) CreateSerial(c *gin.Context) {
	var input struct {
		SerialNumber string `json:"serialNumber"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูล Serial Number ไม่ถูกต้อง")
		return
	}

	item := models.SerialNumber{SerialNumber: normalizeSerial(input.SerialNumber), Status: "available"}
	if item.SerialNumber == "" {
		httpx.BadRequest(c, "กรุณากรอก Serial Number")
		return
	}
	if err := h.db.Create(&item).Error; err != nil {
		httpx.Conflict(c, "Serial Number นี้มีอยู่แล้ว")
		return
	}

	c.JSON(http.StatusCreated, item)
}

func (h *FulltankHandler) ListFilms(c *gin.Context) {
	var items []models.FilmProduct
	query := h.db.Order("created_at DESC")
	if c.Query("public") == "true" || strings.HasPrefix(c.FullPath(), "/api/public/") {
		query = query.Where("is_active = ?", true)
	}
	if err := query.Find(&items).Error; err != nil {
		httpx.Internal(c, "โหลดข้อมูลฟิล์มไม่สำเร็จ")
		return
	}
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
	c.JSON(http.StatusOK, item)
}

func (h *FulltankHandler) DeleteFilm(c *gin.Context) {
	if err := h.db.Delete(&models.FilmProduct{}, c.Param("id")).Error; err != nil {
		httpx.Internal(c, "ลบข้อมูลฟิล์มไม่สำเร็จ")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *FulltankHandler) ListPromotions(c *gin.Context) {
	var items []models.Promotion
	query := h.db.Order("created_at DESC")
	if c.Query("public") == "true" || strings.HasPrefix(c.FullPath(), "/api/public/") {
		query = query.Where("is_active = ?", true)
	}
	if err := query.Find(&items).Error; err != nil {
		httpx.Internal(c, "โหลดโปรโมชันไม่สำเร็จ")
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *FulltankHandler) CreatePromotion(c *gin.Context) {
	var input models.Promotion
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลโปรโมชันไม่ถูกต้อง")
		return
	}
	if err := h.db.Create(&input).Error; err != nil {
		httpx.Internal(c, "บันทึกโปรโมชันไม่สำเร็จ")
		return
	}
	c.JSON(http.StatusCreated, input)
}

func (h *FulltankHandler) UpdatePromotion(c *gin.Context) {
	var input models.Promotion
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลโปรโมชันไม่ถูกต้อง")
		return
	}
	var item models.Promotion
	if err := h.db.First(&item, c.Param("id")).Error; err != nil {
		httpx.NotFound(c, "ไม่พบโปรโมชัน")
		return
	}
	if err := h.db.Model(&item).Updates(input).Error; err != nil {
		httpx.Internal(c, "อัปเดตโปรโมชันไม่สำเร็จ")
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *FulltankHandler) DeletePromotion(c *gin.Context) {
	if err := h.db.Delete(&models.Promotion{}, c.Param("id")).Error; err != nil {
		httpx.Internal(c, "ลบโปรโมชันไม่สำเร็จ")
		return
	}
	c.Status(http.StatusNoContent)
}

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

var (
	errSerialMissing    = errors.New("serial missing")
	errSerialUsed       = errors.New("serial used")
	errRequiredCustomer = errors.New("required customer")
)

func parseDate(value string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}

	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, err
	}

	return &parsed, nil
}

func normalizeSerial(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "-", "_", "-", "/", "-", "\\", "-", ".", "-")
	value = replacer.Replace(value)
	value = strings.Trim(value, "-")
	if value == "" {
		return "item"
	}
	return value
}

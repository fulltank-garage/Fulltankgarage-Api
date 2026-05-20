package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/fulltank-garage/fulltankgarage-api/internal/httpx"
	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
)

func (h *FulltankHandler) CheckSerial(c *gin.Context) {
	serial := normalizeSerial(c.Param("serial"))
	if !h.allowSerialCheck(c, serial) {
		httpx.TooManyRequests(c, "ตรวจสอบ Serial Number บ่อยเกินไป กรุณารอสักครู่แล้วลองใหม่")
		return
	}

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

	lockKey := "lock:warranty-registration:serial:" + serial
	lockToken, locked, err := h.acquireSerialLock(c, lockKey)
	if err != nil {
		httpx.Internal(c, "เตรียมลงทะเบียนรับประกันไม่สำเร็จ")
		return
	}
	if !locked {
		httpx.Conflict(c, "Serial Number นี้กำลังถูกลงทะเบียน กรุณารอสักครู่แล้วลองใหม่")
		return
	}
	defer h.releaseSerialLock(c, lockKey, lockToken)

	installDate, err := parseDate(c.PostForm("installDate"))
	if err != nil {
		httpx.BadRequest(c, "รูปแบบวันที่ติดตั้งไม่ถูกต้อง")
		return
	}

	var created models.WarrantyRegistration
	var usedSerial models.SerialNumber
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

		if err := tx.Model(&serialRecord).Update("status", "used").Error; err != nil {
			return err
		}
		serialRecord.Status = "used"
		usedSerial = serialRecord
		return nil
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
			h.publishRichMenuEvent(created.LineUserID, created.SerialNumber, false, currentRichMenuID, "warranty_registration", err.Error())
			h.enqueueRichMenuRetry(c, created.LineUserID, created.SerialNumber, "warranty_registration_retry")
		} else {
			richMenuSynced = true
			currentRichMenuID = h.richMenu.MemberRichMenuID()
			if linkedRichMenuID, err := h.richMenu.GetUserRichMenuID(c.Request.Context(), created.LineUserID); err != nil {
				log.Printf("get rich menu after warranty registration failed lineUserID=%s serial=%s: %v", created.LineUserID, created.SerialNumber, err)
			} else if linkedRichMenuID != "" {
				currentRichMenuID = linkedRichMenuID
			}
			log.Printf("link member rich menu after warranty registration succeeded lineUserID=%s serial=%s richMenuID=%s", created.LineUserID, created.SerialNumber, currentRichMenuID)
			h.publishRichMenuEvent(created.LineUserID, created.SerialNumber, true, currentRichMenuID, "warranty_registration", "")
		}
	}

	h.publishEvent("warranty_registration.created", created)
	h.publishEvent("serial_number.updated", usedSerial)

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
			h.publishRichMenuEvent(lineUserID, serial, false, currentRichMenuID, "warranty_link", err.Error())
			h.enqueueRichMenuRetry(c, lineUserID, serial, "warranty_link_retry")
		} else {
			richMenuSynced = true
			currentRichMenuID = h.richMenu.MemberRichMenuID()
			if linkedRichMenuID, err := h.richMenu.GetUserRichMenuID(c.Request.Context(), lineUserID); err != nil {
				log.Printf("get rich menu after warranty link failed lineUserID=%s serial=%s: %v", lineUserID, serial, err)
			} else if linkedRichMenuID != "" {
				currentRichMenuID = linkedRichMenuID
			}
			log.Printf("link member rich menu after warranty link succeeded lineUserID=%s serial=%s richMenuID=%s", lineUserID, serial, currentRichMenuID)
			h.publishRichMenuEvent(lineUserID, serial, true, currentRichMenuID, "warranty_link", "")
		}
	}

	h.publishEvent("warranty_registration.linked", item)

	c.JSON(http.StatusOK, gin.H{
		"data":             item,
		"richMenuSynced":   richMenuSynced,
		"linkedRichMenuId": currentRichMenuID,
		"targetRichMenuId": h.targetMemberRichMenuID(),
	})
}

func (h *FulltankHandler) WarrantyStatus(c *gin.Context) {
	lineUserID := strings.TrimSpace(c.Query("lineUserId"))
	if lineUserID == "" {
		httpx.BadRequest(c, "ไม่พบข้อมูล LINE สำหรับตรวจสอบบัตรรับประกัน")
		return
	}

	var items []models.WarrantyRegistration
	if err := h.db.Where("line_user_id = ?", lineUserID).Order("created_at DESC").Find(&items).Error; err != nil {
		httpx.Internal(c, "โหลดข้อมูลบัตรรับประกันไม่สำเร็จ")
		return
	}
	if len(items) == 0 {
		richMenuSynced := false
		currentRichMenuID := ""
		if h.richMenu != nil {
			if err := h.richMenu.LinkRegisterRichMenu(c.Request.Context(), lineUserID); err != nil {
				log.Printf("link register rich menu during empty warranty status failed lineUserID=%s: %v", lineUserID, err)
				h.publishRichMenuEventWithTarget(lineUserID, "", false, currentRichMenuID, h.targetRegisterRichMenuID(), "warranty_status_empty", err.Error())
				h.enqueueRegisterRichMenuRetry(c, lineUserID, "", "warranty_status_empty_retry")
			} else {
				richMenuSynced = true
				currentRichMenuID = h.richMenu.RegisterRichMenuID()
				if linkedRichMenuID, err := h.richMenu.GetUserRichMenuID(c.Request.Context(), lineUserID); err != nil {
					log.Printf("get rich menu during empty warranty status failed lineUserID=%s: %v", lineUserID, err)
				} else if linkedRichMenuID != "" {
					currentRichMenuID = linkedRichMenuID
				}
				h.publishRichMenuEventWithTarget(lineUserID, "", true, currentRichMenuID, h.targetRegisterRichMenuID(), "warranty_status_empty", "")
			}
		}

		c.JSON(http.StatusNotFound, gin.H{
			"message":          "ยังไม่พบข้อมูลบัตรรับประกัน",
			"richMenuSynced":   richMenuSynced,
			"linkedRichMenuId": currentRichMenuID,
			"targetRichMenuId": h.targetRegisterRichMenuID(),
		})
		return
	}

	richMenuSynced := false
	currentRichMenuID := ""
	if h.richMenu != nil {
		if err := h.richMenu.LinkMemberRichMenu(c.Request.Context(), lineUserID); err != nil {
			log.Printf("link member rich menu during warranty status failed lineUserID=%s warranties=%d: %v", lineUserID, len(items), err)
			h.publishRichMenuEvent(lineUserID, "", false, currentRichMenuID, "warranty_status", err.Error())
			h.enqueueRichMenuRetry(c, lineUserID, "", "warranty_status_retry")
		} else {
			richMenuSynced = true
			currentRichMenuID = h.richMenu.MemberRichMenuID()
			if linkedRichMenuID, err := h.richMenu.GetUserRichMenuID(c.Request.Context(), lineUserID); err != nil {
				log.Printf("get rich menu during warranty status failed lineUserID=%s warranties=%d: %v", lineUserID, len(items), err)
			} else if linkedRichMenuID != "" {
				currentRichMenuID = linkedRichMenuID
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":             items[0],
		"items":            items,
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

	h.publishEvent("serial_number.created", item)

	c.JSON(http.StatusCreated, item)
}

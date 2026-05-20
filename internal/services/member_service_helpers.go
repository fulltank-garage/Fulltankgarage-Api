package services

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
)

func validateRegisterInput(input RegisterMemberInput, maxUploadBytes int64) error {
	if input.FirstName == "" {
		return validationError("กรุณากรอกชื่อ")
	}
	if input.LastName == "" {
		return validationError("กรุณากรอกนามสกุล")
	}
	if input.Nickname == "" {
		return validationError("กรุณากรอกชื่อเล่น")
	}
	if len(input.Phone) < 9 || len(input.Phone) > 10 {
		return validationError("กรุณากรอกเบอร์โทรศัพท์ 9-10 หลัก")
	}
	if len(input.CitizenID) != 13 {
		return validationError("กรุณากรอกเลขบัตรประชาชน 13 หลัก")
	}
	if !isHTTPURL(input.ShopPageURL) {
		return validationError("กรุณากรอกลิงก์ร้าน/เพจให้ถูกต้อง")
	}
	if input.StorefrontImage == nil {
		return validationError("กรุณาอัปโหลดรูปหน้าร้าน")
	}
	if input.StorefrontImage.Size > maxUploadBytes {
		return validationError(fmt.Sprintf("รูปภาพต้องมีขนาดไม่เกิน %dMB", maxUploadBytes/1024/1024))
	}
	if !isAllowedImage(input.StorefrontImage) {
		return validationError("ไฟล์ต้องเป็นรูปภาพเท่านั้น")
	}

	return nil
}

func allowedMemberStatus(value string) bool {
	switch value {
	case "pending", "approved", "rejected":
		return true
	default:
		return false
	}
}

func normalizeRejectionReasons(reasons []string) []string {
	normalizedReasons := make([]string, 0, len(reasons))
	seenReasons := map[string]struct{}{}

	for _, reason := range reasons {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			continue
		}
		if _, ok := seenReasons[reason]; ok {
			continue
		}

		seenReasons[reason] = struct{}{}
		normalizedReasons = append(normalizedReasons, reason)
	}

	return normalizedReasons
}

func applicationToResponse(application *models.MemberApplication) MemberResponse {
	lineUserID := ""
	if application.LineUserID != nil {
		lineUserID = *application.LineUserID
	}

	return MemberResponse{
		ID:                 application.ID,
		FirstName:          application.FirstName,
		LastName:           application.LastName,
		Nickname:           application.Nickname,
		Phone:              application.Phone,
		CitizenID:          application.CitizenID,
		ShopPageURL:        application.ShopPageURL,
		StorefrontImage:    application.StorefrontImageURL,
		StorefrontImageURL: application.StorefrontImageURL,
		Status:             application.Status,
		LineUserID:         lineUserID,
	}
}

func memberToResponse(member *models.Member) MemberResponse {
	lineUserID := ""
	if member.LineUserID != nil {
		lineUserID = *member.LineUserID
	}

	return MemberResponse{
		ID:                 member.ID,
		FirstName:          member.FirstName,
		LastName:           member.LastName,
		Nickname:           member.Nickname,
		Phone:              member.Phone,
		CitizenID:          member.CitizenID,
		ShopPageURL:        member.ShopPageURL,
		StorefrontImage:    member.StorefrontImageURL,
		StorefrontImageURL: member.StorefrontImageURL,
		Status:             "approved",
		LineUserID:         lineUserID,
	}
}

func isHTTPURL(value string) bool {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}

	return parsed.Scheme == "http" || parsed.Scheme == "https"
}

func memberCacheKey(lineUserID string) string {
	return "members:line:" + lineUserID
}

func isDuplicateKey(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}

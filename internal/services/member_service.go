package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/fulltank-garage/fulltankgarage-api/internal/cache"
	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
	"github.com/fulltank-garage/fulltankgarage-api/internal/realtime"
	"github.com/fulltank-garage/fulltankgarage-api/internal/repositories"
)

var digitsOnly = regexp.MustCompile(`\D+`)

const (
	memberRichMenuSyncScope        = "member"
	memberRichMenuAutoSyncInterval = 10 * time.Minute
	memberRichMenuSyncBatchSize    = 100
	memberRichMenuSyncBatchPause   = 500 * time.Millisecond
)

type RegisterMemberInput struct {
	FirstName       string
	LastName        string
	Nickname        string
	Phone           string
	CitizenID       string
	ShopPageURL     string
	StorefrontImage *multipart.FileHeader

	LineUserID      string
	LineIDToken     string
	LineDisplayName string
	LinePictureURL  string
}

type MemberResponse struct {
	ID string `json:"id,omitempty"`

	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
	Nickname    string `json:"nickname"`
	Phone       string `json:"phone"`
	CitizenID   string `json:"citizenId"`
	ShopPageURL string `json:"shopPageUrl"`

	StorefrontImage    string `json:"storefrontImage,omitempty"`
	StorefrontImageURL string `json:"storefrontImageUrl,omitempty"`
	Status             string `json:"status,omitempty"`
	LineUserID         string `json:"lineUserId,omitempty"`
}

type RichMenuSyncResponse struct {
	Status     string `json:"status"`
	RichMenu   string `json:"richMenu"`
	LineUserID string `json:"lineUserId,omitempty"`
}

type PointWalletResponse struct {
	Member       MemberResponse `json:"member"`
	Available    int            `json:"available"`
	Pending      int            `json:"pending"`
	Redeemed     int            `json:"redeemed"`
	Lifetime     int            `json:"lifetime"`
	Tier         string         `json:"tier"`
	NextRewardAt int            `json:"nextRewardAt"`
}

type MemberService struct {
	cfg          config.Config
	repo         *repositories.MemberRepository
	cache        *cache.Store
	lineVerifier *LineVerifier
	richMenu     *RichMenuService
	memberCard   *MemberCardImageService
	events       *realtime.Hub
	push         *PushNotificationService

	memberRichMenuSyncMu sync.Mutex
}

func NewMemberService(cfg config.Config, repo *repositories.MemberRepository, cacheStore *cache.Store, lineVerifier *LineVerifier, richMenu *RichMenuService, memberCard *MemberCardImageService, events *realtime.Hub, push *PushNotificationService) *MemberService {
	return &MemberService{
		cfg:          cfg,
		repo:         repo,
		cache:        cacheStore,
		lineVerifier: lineVerifier,
		richMenu:     richMenu,
		memberCard:   memberCard,
		events:       events,
		push:         push,
	}
}

func (s *MemberService) StartMemberRichMenuAutoSync(ctx context.Context) {
	if s.richMenu == nil {
		return
	}

	go func() {
		s.AutoSyncMemberRichMenu(ctx)

		ticker := time.NewTicker(memberRichMenuAutoSyncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.AutoSyncMemberRichMenu(ctx)
			}
		}
	}()
}

func (s *MemberService) AutoSyncMemberRichMenu(ctx context.Context) {
	richMenuID := strings.TrimSpace(s.cfg.LineMemberRichMenuID)
	if richMenuID == "" {
		log.Println("member rich menu auto sync skipped: LINE_MEMBER_RICH_MENU_ID is empty")
		return
	}

	s.memberRichMenuSyncMu.Lock()
	defer s.memberRichMenuSyncMu.Unlock()

	state, err := s.repo.FindRichMenuSyncState(memberRichMenuSyncScope)
	if err == nil && state.RichMenuID == richMenuID && state.Status == "completed" {
		return
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("member rich menu auto sync skipped: load sync state: %v", err)
		return
	}

	members, err := s.repo.ListMembersWithLineUserID()
	if err != nil {
		log.Printf("member rich menu auto sync skipped: list members: %v", err)
		s.saveMemberRichMenuSyncState(richMenuID, "failed", 0, 0, 0, err.Error())
		return
	}

	total := len(members)
	if total == 0 {
		s.saveMemberRichMenuSyncState(richMenuID, "completed", 0, 0, 0, "")
		log.Printf("member rich menu auto sync completed richMenuID=%s total=0 success=0 failed=0", richMenuID)
		return
	}

	s.saveMemberRichMenuSyncState(richMenuID, "running", total, 0, 0, "")
	log.Printf("member rich menu auto sync started richMenuID=%s total=%d", richMenuID, total)

	success := 0
	failed := 0
	lastError := ""

	for index, member := range members {
		select {
		case <-ctx.Done():
			s.saveMemberRichMenuSyncState(richMenuID, "cancelled", total, success, failed, ctx.Err().Error())
			return
		default:
		}

		lineUserID := ""
		if member.LineUserID != nil {
			lineUserID = strings.TrimSpace(*member.LineUserID)
		}
		if lineUserID == "" {
			continue
		}

		requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		err := s.richMenu.LinkMemberRichMenu(requestCtx, lineUserID)
		cancel()
		if err != nil {
			failed++
			lastError = err.Error()
			logLineSyncError("auto_sync_member_rich_menu", lineUserID, err)
		} else {
			success++
		}

		if (index+1)%memberRichMenuSyncBatchSize == 0 && !sleepWithContext(ctx, memberRichMenuSyncBatchPause) {
			s.saveMemberRichMenuSyncState(richMenuID, "cancelled", total, success, failed, ctx.Err().Error())
			return
		}
	}

	status := "completed"
	if success == 0 && failed > 0 {
		status = "failed"
	}

	s.saveMemberRichMenuSyncState(richMenuID, status, total, success, failed, lastError)
	log.Printf("member rich menu auto sync %s richMenuID=%s total=%d success=%d failed=%d", status, richMenuID, total, success, failed)
}

func (s *MemberService) saveMemberRichMenuSyncState(richMenuID string, status string, total int, success int, failed int, lastError string) {
	now := time.Now()
	if len(lastError) > 2048 {
		lastError = lastError[:2048]
	}

	state := models.RichMenuSyncState{
		Scope:      memberRichMenuSyncScope,
		RichMenuID: richMenuID,
		Status:     status,
		Total:      total,
		Success:    success,
		Failed:     failed,
		LastError:  lastError,
		SyncedAt:   &now,
	}
	if err := s.repo.SaveRichMenuSyncState(&state); err != nil {
		log.Printf("member rich menu auto sync state save failed: %v", err)
	}
}

func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (s *MemberService) Register(ctx context.Context, input RegisterMemberInput) (*MemberResponse, error) {
	input.FirstName = strings.TrimSpace(input.FirstName)
	input.LastName = strings.TrimSpace(input.LastName)
	input.Nickname = strings.TrimSpace(input.Nickname)
	input.Phone = digitsOnly.ReplaceAllString(input.Phone, "")
	input.CitizenID = digitsOnly.ReplaceAllString(input.CitizenID, "")
	input.ShopPageURL = strings.TrimSpace(input.ShopPageURL)

	if err := validateRegisterInput(input, s.cfg.MaxUploadBytes); err != nil {
		return nil, err
	}

	lineProfile, err := s.lineVerifier.Resolve(ctx, input.LineUserID, input.LineIDToken, input.LineDisplayName, input.LinePictureURL)
	if err != nil {
		return nil, err
	}

	if existing, err := s.repo.FindMemberConflict(lineProfile.UserID, input.CitizenID, input.Phone); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, &ServiceError{
			Kind:    ErrDuplicateMember,
			Message: "ผู้ใช้นี้ลงทะเบียนแล้ว",
		}
	}

	if existing, err := s.repo.FindApplicationConflict(lineProfile.UserID, input.CitizenID, input.Phone); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, &ServiceError{
			Kind:    ErrDuplicateMember,
			Message: "ผู้ใช้นี้ลงทะเบียนแล้ว",
		}
	}

	imagePath, imageURL, err := s.saveStorefrontImage(input.StorefrontImage)
	if err != nil {
		return nil, err
	}

	var lineUserID *string
	if lineProfile.UserID != "" {
		lineUserID = &lineProfile.UserID
	}

	application := &models.MemberApplication{
		LineUserID:          lineUserID,
		LineDisplayName:     lineProfile.DisplayName,
		LinePictureURL:      lineProfile.PictureURL,
		FirstName:           input.FirstName,
		LastName:            input.LastName,
		Nickname:            input.Nickname,
		Phone:               input.Phone,
		CitizenID:           input.CitizenID,
		ShopPageURL:         input.ShopPageURL,
		StorefrontImagePath: imagePath,
		StorefrontImageURL:  imageURL,
		Status:              "pending",
	}

	if err := s.repo.CreateApplication(application); err != nil {
		_ = os.Remove(imagePath)
		if isDuplicateKey(err) {
			return nil, &ServiceError{
				Kind:    ErrDuplicateMember,
				Message: "ผู้ใช้นี้ลงทะเบียนแล้ว",
				Err:     err,
			}
		}

		return nil, err
	}

	response := applicationToResponse(application)
	s.publishMemberEvent("member_application.created", response)
	if s.push != nil {
		s.push.NotifyNewApplication(ctx, response)
	}
	if application.LineUserID != nil {
		s.syncLineAfterRegistration(ctx, *application.LineUserID)
	}

	return &response, nil
}

func (s *MemberService) Profile(ctx context.Context, lineUserID, lineIDToken, id string) (*MemberResponse, error) {
	lineProfile, err := s.lineVerifier.Resolve(ctx, lineUserID, lineIDToken, "", "")
	if err != nil {
		return nil, err
	}

	if lineProfile.UserID != "" {
		cacheKey := memberCacheKey(lineProfile.UserID)
		var cached MemberResponse
		if ok, err := s.cache.GetJSON(ctx, cacheKey, &cached); err == nil && ok {
			return &cached, nil
		}

		member, err := s.repo.FindMemberByLineUserID(lineProfile.UserID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				application, appErr := s.repo.FindApplicationByLineUserID(lineProfile.UserID)
				if appErr == nil {
					response := applicationToResponse(application)
					if response.Status == "approved" {
						_ = s.cache.SetJSON(ctx, cacheKey, response)
					}
					return &response, nil
				}
				if errors.Is(appErr, gorm.ErrRecordNotFound) {
					return nil, notFoundError("ไม่พบข้อมูลสมาชิกของ LINE userId นี้")
				}
				return nil, appErr
			}
			return nil, err
		}

		response := memberToResponse(member)
		_ = s.cache.SetJSON(ctx, cacheKey, response)
		return &response, nil
	}

	if strings.TrimSpace(id) != "" {
		member, err := s.repo.FindMemberByID(strings.TrimSpace(id))
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, notFoundError("ไม่พบข้อมูลสมาชิก")
			}
			return nil, err
		}

		response := memberToResponse(member)
		return &response, nil
	}

	if s.cfg.AllowProfileFallback {
		member, err := s.repo.FindLatestMember()
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, notFoundError("ยังไม่มีข้อมูลสมาชิก")
			}
			return nil, err
		}

		response := memberToResponse(member)
		return &response, nil
	}

	return nil, validationError("กรุณาส่ง lineUserId หรือ lineIdToken จาก LIFF")
}

func (s *MemberService) PointWallet(ctx context.Context, lineUserID, lineIDToken string) (*PointWalletResponse, error) {
	member, err := s.Profile(ctx, lineUserID, lineIDToken, "")
	if err != nil {
		return nil, err
	}
	if member.Status != "approved" {
		return nil, notFoundError("ยังไม่พบข้อมูลสมาชิกที่ผ่านการยืนยัน")
	}

	return &PointWalletResponse{
		Member:       *member,
		Available:    0,
		Pending:      0,
		Redeemed:     0,
		Lifetime:     0,
		Tier:         "Member",
		NextRewardAt: 100,
	}, nil
}

func (s *MemberService) RegistrationStatus(ctx context.Context, lineUserID, lineIDToken string) (*MemberResponse, error) {
	lineProfile, err := s.lineVerifier.Resolve(ctx, lineUserID, lineIDToken, "", "")
	if err != nil {
		return nil, err
	}

	if lineProfile.UserID == "" {
		return nil, validationError("กรุณาส่ง lineUserId หรือ lineIdToken จาก LIFF")
	}

	member, err := s.repo.FindMemberByLineUserID(lineProfile.UserID)
	if err == nil {
		response := memberToResponse(member)
		response.Status = "approved"
		return &response, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	application, err := s.repo.FindApplicationByLineUserID(lineProfile.UserID)
	if err == nil {
		response := applicationToResponse(application)
		return &response, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, notFoundError("ไม่พบข้อมูลการสมัครของ LINE userId นี้")
	}

	return nil, err
}

func (s *MemberService) SyncLineRichMenuForFollow(ctx context.Context, lineUserID string) error {
	_, err := s.syncLineRichMenuByLineUserID(ctx, lineUserID)
	return err
}

func (s *MemberService) SyncLineRichMenu(ctx context.Context, lineUserID, lineIDToken string) (*RichMenuSyncResponse, error) {
	lineProfile, err := s.lineVerifier.Resolve(ctx, lineUserID, lineIDToken, "", "")
	if err != nil {
		return nil, err
	}

	if lineProfile.UserID == "" {
		return nil, validationError("กรุณาส่ง lineUserId หรือ lineIdToken จาก LIFF")
	}

	return s.syncLineRichMenuByLineUserID(ctx, lineProfile.UserID)
}

func (s *MemberService) syncLineRichMenuByLineUserID(ctx context.Context, lineUserID string) (*RichMenuSyncResponse, error) {
	lineUserID = strings.TrimSpace(lineUserID)
	if lineUserID == "" {
		return nil, validationError("ไม่พบ LINE userId")
	}
	if s.richMenu == nil {
		return &RichMenuSyncResponse{Status: "unknown", RichMenu: "none", LineUserID: lineUserID}, nil
	}

	if _, err := s.repo.FindMemberByLineUserID(lineUserID); err == nil {
		if err := s.richMenu.LinkMemberRichMenu(ctx, lineUserID); err != nil {
			return nil, err
		}
		return &RichMenuSyncResponse{Status: "approved", RichMenu: "member", LineUserID: lineUserID}, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	application, err := s.repo.FindApplicationByLineUserID(lineUserID)
	if err == nil {
		switch application.Status {
		case "pending":
			if err := s.richMenu.LinkVerifyRichMenu(ctx, lineUserID); err != nil {
				return nil, err
			}
			return &RichMenuSyncResponse{Status: "pending", RichMenu: "verify", LineUserID: lineUserID}, nil
		case "approved":
			if err := s.richMenu.LinkMemberRichMenu(ctx, lineUserID); err != nil {
				return nil, err
			}
			return &RichMenuSyncResponse{Status: "approved", RichMenu: "member", LineUserID: lineUserID}, nil
		case "rejected":
			if err := s.richMenu.LinkRejectedRichMenu(ctx, lineUserID); err != nil {
				return nil, err
			}
			return &RichMenuSyncResponse{Status: "rejected", RichMenu: "rejected", LineUserID: lineUserID}, nil
		default:
			if err := s.richMenu.LinkRegisterRichMenu(ctx, lineUserID); err != nil {
				return nil, err
			}
			return &RichMenuSyncResponse{Status: "register", RichMenu: "register", LineUserID: lineUserID}, nil
		}
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	if err := s.richMenu.LinkRegisterRichMenu(ctx, lineUserID); err != nil {
		return nil, err
	}
	return &RichMenuSyncResponse{Status: "register", RichMenu: "register", LineUserID: lineUserID}, nil
}

func (s *MemberService) ListApplications(ctx context.Context) ([]MemberResponse, error) {
	applications, err := s.repo.ListApplications()
	if err != nil {
		return nil, err
	}

	response := make([]MemberResponse, 0, len(applications))
	for _, application := range applications {
		response = append(response, applicationToResponse(&application))
	}

	return response, nil
}

func (s *MemberService) ListMembers(ctx context.Context) ([]MemberResponse, error) {
	members, err := s.repo.ListMembers()
	if err != nil {
		return nil, err
	}

	response := make([]MemberResponse, 0, len(members))
	for _, member := range members {
		response = append(response, memberToResponse(&member))
	}

	return response, nil
}

func (s *MemberService) UpdateApplicationStatus(ctx context.Context, id string, status string, rejectionReasons []string) (*MemberResponse, error) {
	id = strings.TrimSpace(id)
	status = strings.TrimSpace(status)
	rejectionReasons = normalizeRejectionReasons(rejectionReasons)

	if id == "" {
		return nil, validationError("ไม่พบรหัสใบสมัคร")
	}
	if !allowedMemberStatus(status) {
		return nil, validationError("สถานะใบสมัครไม่ถูกต้อง")
	}
	if status == "rejected" && len(rejectionReasons) == 0 {
		return nil, validationError("กรุณาระบุเหตุผลที่ไม่ผ่านเกณฑ์อย่างน้อย 1 ข้อ")
	}

	application, err := s.repo.FindApplicationByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, notFoundError("ไม่พบข้อมูลการสมัคร")
		}
		return nil, err
	}

	if status == "rejected" {
		application.Status = status
		response := applicationToResponse(application)
		lineUserID := ""
		if application.LineUserID != nil {
			lineUserID = *application.LineUserID
		}

		if err := s.repo.DeleteApplication(application); err != nil {
			return nil, err
		}

		if lineUserID != "" {
			_ = s.cache.Delete(ctx, memberCacheKey(lineUserID))
			s.syncLineAfterStatusChange(ctx, lineUserID, status, application, rejectionReasons)
		}

		removeStorefrontImage(application.StorefrontImagePath)
		s.publishMemberEvent("member_application.deleted", response)

		return &response, nil
	}

	if status == "approved" {
		application.Status = status
		member, err := s.repo.ApproveApplication(application)
		if err != nil {
			return nil, err
		}

		response := memberToResponse(member)
		if application.LineUserID != nil {
			_ = s.cache.Delete(ctx, memberCacheKey(*application.LineUserID))
			s.syncLineAfterStatusChange(ctx, *application.LineUserID, status, application, nil)
		}
		s.publishMemberEvent("member_application.updated", response)

		return &response, nil
	}

	application.Status = status
	if err := s.repo.SaveApplication(application); err != nil {
		return nil, err
	}

	response := applicationToResponse(application)
	s.publishMemberEvent("member_application.updated", response)

	return &response, nil
}

func (s *MemberService) DeleteMember(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return validationError("ไม่พบรหัสใบสมัคร")
	}

	member, err := s.repo.FindMemberByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFoundError("ไม่พบข้อมูลสมาชิก")
		}
		return err
	}

	lineUserID := ""
	if member.LineUserID != nil {
		lineUserID = *member.LineUserID
	}

	if err := s.repo.DeleteMemberWithApplication(member); err != nil {
		return err
	}

	if lineUserID != "" {
		_ = s.cache.Delete(ctx, memberCacheKey(lineUserID))
		s.syncLineAfterDeletion(ctx, lineUserID)
	}

	removeStorefrontImage(member.StorefrontImagePath)
	s.publishMemberEvent("member_application.deleted", memberToResponse(member))

	return nil
}

func (s *MemberService) DeleteApplication(ctx context.Context, id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return validationError("ไม่พบรหัสใบสมัคร")
	}

	application, err := s.repo.FindApplicationByID(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return notFoundError("ไม่พบข้อมูลการสมัคร")
		}
		return err
	}

	lineUserID := ""
	if application.LineUserID != nil {
		lineUserID = *application.LineUserID
	}

	if err := s.repo.DeleteApplication(application); err != nil {
		return err
	}

	if lineUserID != "" {
		_ = s.cache.Delete(ctx, memberCacheKey(lineUserID))
		s.syncLineAfterDeletion(ctx, lineUserID)
	}

	removeStorefrontImage(application.StorefrontImagePath)
	s.publishMemberEvent("member_application.deleted", applicationToResponse(application))

	return nil
}

func (s *MemberService) publishMemberEvent(eventType string, member MemberResponse) {
	if s.events == nil {
		return
	}

	s.events.Publish(realtime.Event{
		Type: eventType,
		Data: member,
	})
}

func removeStorefrontImage(path string) {
	if path == "" {
		return
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("remove storefront image: %v", err)
	}
}

func logLineSyncError(operation string, lineUserID string, err error) {
	log.Printf("line sync failed operation=%s lineUserId=%s error=%v", operation, lineUserID, err)
}

func (s *MemberService) syncLineAfterRegistration(ctx context.Context, lineUserID string) {
	if s.richMenu == nil {
		return
	}

	if err := s.richMenu.LinkVerifyRichMenu(ctx, lineUserID); err != nil {
		logLineSyncError("link_verify_rich_menu", lineUserID, err)
	}
	if err := s.richMenu.PushPendingReviewMessage(ctx, lineUserID); err != nil {
		logLineSyncError("push_pending_review_message", lineUserID, err)
	}
}

func (s *MemberService) syncLineAfterStatusChange(ctx context.Context, lineUserID string, status string, application *models.MemberApplication, rejectionReasons []string) {
	if s.richMenu == nil {
		return
	}

	switch status {
	case "approved":
		if err := s.richMenu.LinkMemberRichMenu(ctx, lineUserID); err != nil {
			logLineSyncError("link_member_rich_menu", lineUserID, err)
		}
		cardImage, err := s.generateApprovedCard(application)
		if err != nil {
			logLineSyncError("generate_approved_member_card", lineUserID, err)
		}
		if err := s.richMenu.PushApprovedMessage(ctx, lineUserID, cardImage.ImageURL, cardImage.PreviewURL); err != nil {
			logLineSyncError("push_approved_message", lineUserID, err)
		}
	case "rejected":
		if err := s.richMenu.LinkRegisterRichMenu(ctx, lineUserID); err != nil {
			logLineSyncError("link_register_rich_menu_after_rejection", lineUserID, err)
		}
		if err := s.richMenu.PushRejectedMessage(ctx, lineUserID, rejectionReasons); err != nil {
			logLineSyncError("push_rejected_message", lineUserID, err)
		}
	}
}

func (s *MemberService) syncLineAfterDeletion(ctx context.Context, lineUserID string) {
	if s.richMenu == nil {
		return
	}

	if err := s.richMenu.LinkRegisterRichMenu(ctx, lineUserID); err != nil {
		logLineSyncError("link_register_rich_menu_after_deletion", lineUserID, err)
	}
}

func (s *MemberService) generateApprovedCard(application *models.MemberApplication) (MemberCardImageResult, error) {
	if s.memberCard == nil {
		return MemberCardImageResult{
			ImageURL:   s.cfg.BaseURL + "/assets/line/registration-success.png",
			PreviewURL: s.cfg.BaseURL + "/assets/line/registration-success-preview.png",
		}, nil
	}

	customerName := strings.Join(strings.Fields(application.FirstName+" "+application.LastName), " ")
	if customerName == "" {
		customerName = application.Nickname
	}
	return s.memberCard.Generate(application.ID, customerName)
}

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

func memberCacheKey(lineUserID string) string {
	return "members:line:" + lineUserID
}

func isDuplicateKey(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "duplicate key")
}

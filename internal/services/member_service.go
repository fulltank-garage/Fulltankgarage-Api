package services

import (
	"context"
	"errors"
	"mime/multipart"
	"os"
	"regexp"
	"strings"
	"sync"

	"gorm.io/gorm"

	"github.com/fulltank-garage/fulltankgarage-api/internal/cache"
	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
	"github.com/fulltank-garage/fulltankgarage-api/internal/realtime"
	"github.com/fulltank-garage/fulltankgarage-api/internal/repositories"
)

var digitsOnly = regexp.MustCompile(`\D+`)

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

func (s *MemberService) publishMemberEvent(eventType string, member MemberResponse) {
	if s.events == nil {
		return
	}

	s.events.Publish(realtime.Event{
		Type: eventType,
		Data: member,
	})
}

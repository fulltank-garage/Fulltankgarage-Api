package services

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
)

const (
	memberRichMenuSyncScope        = "member"
	memberRichMenuAutoSyncInterval = 10 * time.Minute
	memberRichMenuSyncBatchSize    = 100
	memberRichMenuSyncBatchPause   = 500 * time.Millisecond
)

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

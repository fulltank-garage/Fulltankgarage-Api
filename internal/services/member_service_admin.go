package services

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
)

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

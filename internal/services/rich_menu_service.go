package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
)

type RichMenuService struct {
	channelAccessToken string
	registerRichMenuID string
	memberRichMenuID   string
	verifyRichMenuID   string
	rejectedRichMenuID string
	menuSyncRichMenuID string
	pointWalletMenuID  string
	verifyLiffID       string
	endpoint           string
	httpClient         *http.Client
}

func NewRichMenuService(cfg config.Config) *RichMenuService {
	return &RichMenuService{
		channelAccessToken: cfg.LineChannelAccessToken,
		registerRichMenuID: cfg.LineRegisterRichMenuID,
		memberRichMenuID:   cfg.LineMemberRichMenuID,
		verifyRichMenuID:   cfg.LineVerifyRichMenuID,
		rejectedRichMenuID: cfg.LineRejectedRichMenuID,
		menuSyncRichMenuID: cfg.LineMenuSyncRichMenuID,
		pointWalletMenuID:  cfg.LinePointWalletMenuID,
		verifyLiffID:       cfg.LineVerifyLiffID,
		endpoint:           cfg.LineRichMenuEndpoint,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

func (s *RichMenuService) ValidateConfiguredRichMenus(ctx context.Context) []string {
	configuredIDs := map[string]string{
		"LINE_REGISTER_RICH_MENU_ID":     s.registerRichMenuID,
		"LINE_MEMBER_RICH_MENU_ID":       s.memberRichMenuID,
		"LINE_VERIFY_RICH_MENU_ID":       s.verifyRichMenuID,
		"LINE_REJECTED_RICH_MENU_ID":     s.rejectedRichMenuID,
		"LINE_MENU_SYNC_RICH_MENU_ID":    s.menuSyncRichMenuID,
		"LINE_POINT_WALLET_RICH_MENU_ID": s.pointWalletMenuID,
	}

	if s.channelAccessToken == "" {
		return []string{"LINE_CHANNEL_ACCESS_TOKEN is empty, rich menu validation skipped"}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.endpoint+"/richmenu/list", nil)
	if err != nil {
		return []string{err.Error()}
	}
	req.Header.Set("Authorization", "Bearer "+s.channelAccessToken)

	res, err := s.httpClient.Do(req)
	if err != nil {
		return []string{err.Error()}
	}
	defer res.Body.Close()

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		message, err := decodeLineError(res)
		if err != nil {
			return []string{err.Error()}
		}
		return []string{message}
	}

	var payload struct {
		RichMenus []struct {
			ID string `json:"richMenuId"`
		} `json:"richmenus"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return []string{err.Error()}
	}

	availableIDs := make(map[string]struct{}, len(payload.RichMenus))
	for _, richMenu := range payload.RichMenus {
		availableIDs[richMenu.ID] = struct{}{}
	}

	warnings := []string{}
	for envName, richMenuID := range configuredIDs {
		richMenuID = strings.TrimSpace(richMenuID)
		if richMenuID == "" {
			continue
		}
		if _, ok := availableIDs[richMenuID]; !ok {
			warnings = append(warnings, fmt.Sprintf("%s=%s is not found in LINE rich menu list", envName, richMenuID))
		}
	}

	return warnings
}

func (s *RichMenuService) LinkRegisterRichMenu(ctx context.Context, lineUserID string) error {
	return s.linkRichMenu(ctx, lineUserID, s.registerRichMenuID)
}

func (s *RichMenuService) RegisterRichMenuID() string {
	return strings.TrimSpace(s.registerRichMenuID)
}

func (s *RichMenuService) LinkMemberRichMenu(ctx context.Context, lineUserID string) error {
	return s.linkRichMenu(ctx, lineUserID, s.memberRichMenuID)
}

func (s *RichMenuService) MemberRichMenuID() string {
	return strings.TrimSpace(s.memberRichMenuID)
}

func (s *RichMenuService) GetUserRichMenuID(ctx context.Context, lineUserID string) (string, error) {
	lineUserID = strings.TrimSpace(lineUserID)
	if lineUserID == "" || s.channelAccessToken == "" {
		return "", nil
	}

	endpoint := fmt.Sprintf(
		"%s/user/%s/richmenu",
		s.endpoint,
		url.PathEscape(lineUserID),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+s.channelAccessToken)

	res, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		message, err := decodeLineError(res)
		if err != nil {
			return "", err
		}
		return "", validationError(message)
	}

	var payload struct {
		RichMenuID string `json:"richMenuId"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", err
	}

	return strings.TrimSpace(payload.RichMenuID), nil
}

func (s *RichMenuService) LinkVerifyRichMenu(ctx context.Context, lineUserID string) error {
	return s.linkRichMenu(ctx, lineUserID, s.verifyRichMenuID)
}

func (s *RichMenuService) LinkRejectedRichMenu(ctx context.Context, lineUserID string) error {
	return s.linkRichMenu(ctx, lineUserID, s.rejectedRichMenuID)
}

func (s *RichMenuService) PushPendingReviewMessage(ctx context.Context, lineUserID string) error {
	return s.pushText(ctx, lineUserID, "สมัครสำเร็จแล้วค่ะ ขณะนี้ข้อมูลของคุณอยู่ระหว่างรอตรวจสอบจากร้าน กรุณารอสักครู่")
}

func (s *RichMenuService) PushApprovedMessage(ctx context.Context, lineUserID string, imageURL string, previewURL string) error {
	imageURL = strings.TrimSpace(imageURL)
	previewURL = strings.TrimSpace(previewURL)
	if previewURL == "" {
		previewURL = imageURL
	}

	messages := []map[string]string{}
	if imageURL != "" {
		messages = append(messages, map[string]string{
			"type":               "image",
			"originalContentUrl": imageURL,
			"previewImageUrl":    previewURL,
		})
	}

	messages = append(messages, map[string]string{
		"type": "text",
		"text": "ข้อมูลของคุณผ่านการตรวจสอบแล้วค่ะ ตอนนี้สามารถใช้งานเมนูสมาชิกได้แล้ว",
	})

	return s.pushMessages(ctx, lineUserID, messages)
}

func (s *RichMenuService) PushRejectedMessage(ctx context.Context, lineUserID string, reasons []string) error {
	message := "ขออภัยค่ะ ข้อมูลของคุณไม่ผ่านเกณฑ์ที่ร้านกำหนด"
	reasons = normalizeLineMessageItems(reasons)
	if len(reasons) > 0 {
		message += "\n\nเหตุผลที่ไม่ผ่านเกณฑ์:"
		for _, reason := range reasons {
			message += "\n- " + reason
		}
	}
	message += "\n\nกรุณาติดต่อร้านผ่านแชท LINE เพื่อสอบถามรายละเอียดเพิ่มเติม"

	return s.pushText(ctx, lineUserID, message)
}

func (s *RichMenuService) linkRichMenu(ctx context.Context, lineUserID string, richMenuID string) error {
	lineUserID = strings.TrimSpace(lineUserID)
	richMenuID = strings.TrimSpace(richMenuID)
	if lineUserID == "" || s.channelAccessToken == "" || richMenuID == "" {
		return nil
	}

	endpoint := fmt.Sprintf(
		"%s/user/%s/richmenu/%s",
		s.endpoint,
		url.PathEscape(lineUserID),
		url.PathEscape(richMenuID),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.channelAccessToken)

	res, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusOK && res.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	message, err := decodeLineError(res)
	if err != nil {
		return err
	}

	return validationError(message)
}

func (s *RichMenuService) pushText(ctx context.Context, lineUserID string, text string) error {
	lineUserID = strings.TrimSpace(lineUserID)
	text = strings.TrimSpace(text)
	if lineUserID == "" || text == "" || s.channelAccessToken == "" {
		return nil
	}

	return s.pushMessages(ctx, lineUserID, []map[string]string{
		{
			"type": "text",
			"text": text,
		},
	})
}

func (s *RichMenuService) pushMessages(ctx context.Context, lineUserID string, messages []map[string]string) error {
	lineUserID = strings.TrimSpace(lineUserID)
	if lineUserID == "" || len(messages) == 0 || s.channelAccessToken == "" {
		return nil
	}

	body, err := json.Marshal(map[string]any{
		"to":       lineUserID,
		"messages": messages,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint+"/message/push", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.channelAccessToken)
	req.Header.Set("Content-Type", "application/json")

	res, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusOK && res.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	message, err := decodeLineError(res)
	if err != nil {
		return err
	}

	return validationError(message)
}

func normalizeLineMessageItems(items []string) []string {
	normalizedItems := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item != "" {
			normalizedItems = append(normalizedItems, item)
		}
	}

	return normalizedItems
}

func decodeLineError(res *http.Response) (string, error) {
	var payload struct {
		Message string `json:"message"`
		Details []struct {
			Message string `json:"message"`
		} `json:"details"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return "", fmt.Errorf("LINE API request failed with status %d", res.StatusCode)
	}

	message := strings.TrimSpace(payload.Message)
	if message == "" && len(payload.Details) > 0 {
		message = strings.TrimSpace(payload.Details[0].Message)
	}
	if message == "" {
		message = fmt.Sprintf("LINE API request failed with status %d", res.StatusCode)
	}

	return message, nil
}

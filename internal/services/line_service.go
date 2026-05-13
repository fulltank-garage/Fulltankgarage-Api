package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
)

type LineProfile struct {
	UserID      string
	DisplayName string
	PictureURL  string
}

type LineVerifier struct {
	channelID      string
	requireToken   bool
	verifyEndpoint string
	httpClient     *http.Client
}

func NewLineVerifier(cfg config.Config) *LineVerifier {
	return &LineVerifier{
		channelID:      cfg.LineChannelID,
		requireToken:   cfg.RequireLineIDToken,
		verifyEndpoint: cfg.LineVerifyEndpoint,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

func (v *LineVerifier) Resolve(ctx context.Context, lineUserID, lineIDToken, displayName, pictureURL string) (LineProfile, error) {
	lineUserID = strings.TrimSpace(lineUserID)
	lineIDToken = strings.TrimSpace(lineIDToken)

	if lineIDToken != "" {
		if v.channelID == "" {
			return LineProfile{}, validationError("ตั้งค่า LINE_CHANNEL_ID ก่อนใช้ lineIdToken")
		}

		return v.verifyIDToken(ctx, lineIDToken)
	}

	if v.requireToken {
		return LineProfile{}, validationError("กรุณาส่ง lineIdToken จาก LIFF เพื่อยืนยันตัวตน LINE")
	}

	return LineProfile{
		UserID:      lineUserID,
		DisplayName: strings.TrimSpace(displayName),
		PictureURL:  strings.TrimSpace(pictureURL),
	}, nil
}

func (v *LineVerifier) verifyIDToken(ctx context.Context, token string) (LineProfile, error) {
	form := url.Values{}
	form.Set("id_token", token)
	form.Set("client_id", v.channelID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, v.verifyEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return LineProfile{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	res, err := v.httpClient.Do(req)
	if err != nil {
		return LineProfile{}, err
	}
	defer res.Body.Close()

	var payload struct {
		Sub     string `json:"sub"`
		Name    string `json:"name"`
		Picture string `json:"picture"`
		Error   string `json:"error"`
		Message string `json:"error_description"`
	}
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return LineProfile{}, err
	}

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		message := payload.Message
		if message == "" {
			message = payload.Error
		}
		if message == "" {
			message = fmt.Sprintf("LINE token verify failed with status %d", res.StatusCode)
		}

		return LineProfile{}, validationError(message)
	}

	if strings.TrimSpace(payload.Sub) == "" {
		return LineProfile{}, validationError("LINE token ไม่พบ userId")
	}

	return LineProfile{
		UserID:      strings.TrimSpace(payload.Sub),
		DisplayName: strings.TrimSpace(payload.Name),
		PictureURL:  strings.TrimSpace(payload.Picture),
	}, nil
}

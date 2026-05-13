package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

type LineWebhookHandler struct {
	memberService *services.MemberService
	channelSecret string
}

type lineWebhookRequest struct {
	Events []lineWebhookEvent `json:"events"`
}

type lineWebhookEvent struct {
	Type   string            `json:"type"`
	Source lineWebhookSource `json:"source"`
}

type lineWebhookSource struct {
	UserID string `json:"userId"`
}

func NewLineWebhookHandler(memberService *services.MemberService, channelSecret string) *LineWebhookHandler {
	return &LineWebhookHandler{
		memberService: memberService,
		channelSecret: strings.TrimSpace(channelSecret),
	}
}

func (h *LineWebhookHandler) Handle(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "อ่านข้อมูล webhook ไม่สำเร็จ"})
		return
	}

	if !h.validSignature(c.GetHeader("X-Line-Signature"), body) {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "LINE webhook signature ไม่ถูกต้อง"})
		return
	}

	var payload lineWebhookRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "รูปแบบ webhook ไม่ถูกต้อง"})
		return
	}

	for _, event := range payload.Events {
		if event.Type != "follow" || strings.TrimSpace(event.Source.UserID) == "" {
			continue
		}

		if err := h.memberService.SyncLineRichMenuForFollow(c.Request.Context(), event.Source.UserID); err != nil {
			log.Printf("line webhook follow sync failed lineUserId=%s error=%v", event.Source.UserID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *LineWebhookHandler) validSignature(signature string, body []byte) bool {
	signature = strings.TrimSpace(signature)
	if signature == "" || h.channelSecret == "" {
		return false
	}

	mac := hmac.New(sha256.New, []byte(h.channelSecret))
	_, _ = mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expected))
}

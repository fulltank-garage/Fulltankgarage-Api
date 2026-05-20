package handlers

import (
	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/realtime"
)

func (h *FulltankHandler) targetMemberRichMenuID() string {
	if h.richMenu == nil {
		return ""
	}

	return h.richMenu.MemberRichMenuID()
}

func (h *FulltankHandler) publishEvent(eventType string, data any) {
	if h.events == nil {
		return
	}

	h.events.Publish(realtime.Event{
		Type: eventType,
		Data: data,
	})
}

func (h *FulltankHandler) publishRichMenuEvent(lineUserID string, serialNumber string, success bool, linkedRichMenuID string, source string, message string) {
	h.publishEvent("rich_menu.sync", gin.H{
		"lineUserId":       lineUserID,
		"serialNumber":     serialNumber,
		"success":          success,
		"linkedRichMenuId": linkedRichMenuID,
		"targetRichMenuId": h.targetMemberRichMenuID(),
		"source":           source,
		"message":          message,
	})
}

func (h *FulltankHandler) enqueueRichMenuRetry(c *gin.Context, lineUserID string, serialNumber string, source string) {
	if h.richQueue == nil {
		return
	}

	h.richQueue.EnqueueMemberLink(c.Request.Context(), lineUserID, serialNumber, source)
}

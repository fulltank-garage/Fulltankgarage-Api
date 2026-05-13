package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/httpx"
	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

type PushNotificationHandler struct {
	pushService *services.PushNotificationService
}

func NewPushNotificationHandler(pushService *services.PushNotificationService) *PushNotificationHandler {
	return &PushNotificationHandler{pushService: pushService}
}

func (h *PushNotificationHandler) PublicKey(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"configured": h.pushService.IsConfigured(),
		"publicKey":  h.pushService.PublicKey(),
	})
}

func (h *PushNotificationHandler) Subscribe(c *gin.Context) {
	var input struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256DH string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลการแจ้งเตือนไม่ถูกต้อง")
		return
	}

	if err := h.pushService.Subscribe(services.PushSubscriptionInput{
		Endpoint: input.Endpoint,
		P256DH:   input.Keys.P256DH,
		Auth:     input.Keys.Auth,
	}); err != nil {
		httpx.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *PushNotificationHandler) Unsubscribe(c *gin.Context) {
	var input struct {
		Endpoint string `json:"endpoint"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.BadRequest(c, "รูปแบบข้อมูลการแจ้งเตือนไม่ถูกต้อง")
		return
	}

	if err := h.pushService.Unsubscribe(input.Endpoint); err != nil {
		httpx.Error(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

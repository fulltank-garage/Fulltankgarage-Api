package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/fulltank-garage/fulltankgarage-api/internal/realtime"
	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

type MemberEventsHandler struct {
	authService *services.AuthService
	hub         *realtime.Hub
	upgrader    websocket.Upgrader
}

func NewMemberEventsHandler(authService *services.AuthService, hub *realtime.Hub) *MemberEventsHandler {
	return &MemberEventsHandler{
		authService: authService,
		hub:         hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}
}

func (h *MemberEventsHandler) Subscribe(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "ไม่พบ token"})
		return
	}

	if _, err := h.authService.ValidateToken(token); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "token ไม่ถูกต้องหรือหมดอายุ"})
		return
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	events := h.hub.Subscribe()
	defer h.hub.Unsubscribe(events)

	conn.SetReadLimit(512)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.NextReader(); err != nil {
				return
			}
		}
	}()

	ticker := time.NewTicker(25 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, event); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-done:
			return
		case <-c.Request.Context().Done():
			return
		}
	}
}

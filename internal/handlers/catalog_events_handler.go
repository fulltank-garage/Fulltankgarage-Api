package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/fulltank-garage/fulltankgarage-api/internal/realtime"
)

type CatalogEventsHandler struct {
	hub      *realtime.Hub
	upgrader websocket.Upgrader
}

func NewCatalogEventsHandler(hub *realtime.Hub) *CatalogEventsHandler {
	return &CatalogEventsHandler{
		hub: hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool {
				return true
			},
		},
	}
}

func (h *CatalogEventsHandler) Subscribe(c *gin.Context) {
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
			if !isPublicCatalogEvent(event) {
				continue
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

func isPublicCatalogEvent(payload []byte) bool {
	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return false
	}

	return strings.HasPrefix(event.Type, "film.") || strings.HasPrefix(event.Type, "promotion.")
}

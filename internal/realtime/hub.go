package realtime

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/fulltank-garage/fulltankgarage-api/internal/cache"
)

const memberApplicationsChannel = "fulltankgarage:member_applications"

type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
	redis   *redis.Client
	channel string
}

func NewHub(ctx context.Context, cacheStore *cache.Store) *Hub {
	hub := &Hub{
		clients: map[chan []byte]struct{}{},
		channel: memberApplicationsChannel,
	}
	if cacheStore != nil {
		hub.redis = cacheStore.Client()
	}

	if hub.redis != nil {
		go hub.subscribeRedis(ctx)
	}

	return hub
}

func (h *Hub) Subscribe() chan []byte {
	ch := make(chan []byte, 8)

	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()

	return ch
}

func (h *Hub) Unsubscribe(ch chan []byte) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *Hub) Publish(event Event) {
	payload, err := json.Marshal(event)
	if err != nil {
		return
	}

	if h.redis != nil {
		if err := h.redis.Publish(context.Background(), h.channel, payload).Err(); err == nil {
			return
		}

		log.Printf("publish realtime event via redis: %v", err)
	}

	h.broadcast(payload)
}

func (h *Hub) subscribeRedis(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		pubsub := h.redis.Subscribe(ctx, h.channel)
		_, err := pubsub.Receive(ctx)
		if err != nil {
			_ = pubsub.Close()
			log.Printf("subscribe realtime events via redis: %v", err)
			if !sleepOrDone(ctx) {
				return
			}
			continue
		}

		messages := pubsub.Channel()
		shouldReconnect := false
		for !shouldReconnect {
			select {
			case <-ctx.Done():
				_ = pubsub.Close()
				return
			case message, ok := <-messages:
				if !ok {
					_ = pubsub.Close()
					log.Print("redis realtime subscription closed")
					if !sleepOrDone(ctx) {
						return
					}
					shouldReconnect = true
					continue
				}

				h.broadcast([]byte(message.Payload))
			}
		}
	}
}

func sleepOrDone(ctx context.Context) bool {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (h *Hub) broadcast(payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for ch := range h.clients {
		select {
		case ch <- payload:
		default:
		}
	}
}

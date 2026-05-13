package services

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"strings"

	webpush "github.com/SherClockHolmes/webpush-go"

	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
	"github.com/fulltank-garage/fulltankgarage-api/internal/models"
	"github.com/fulltank-garage/fulltankgarage-api/internal/repositories"
)

type PushSubscriptionInput struct {
	Endpoint string
	P256DH   string
	Auth     string
}

type PushNotificationService struct {
	cfg  config.Config
	repo *repositories.PushSubscriptionRepository
}

const defaultWebPushSubscriber = "https://miki-japan-admin.vercel.app"

func NewPushNotificationService(cfg config.Config, repo *repositories.PushSubscriptionRepository) *PushNotificationService {
	return &PushNotificationService{
		cfg:  cfg,
		repo: repo,
	}
}

func (s *PushNotificationService) PublicKey() string {
	return strings.TrimSpace(s.cfg.WebPushVAPIDPublicKey)
}

func (s *PushNotificationService) IsConfigured() bool {
	return strings.TrimSpace(s.cfg.WebPushVAPIDPublicKey) != "" &&
		strings.TrimSpace(s.cfg.WebPushVAPIDPrivateKey) != ""
}

func (s *PushNotificationService) subscriber() string {
	subscriber := strings.TrimSpace(s.cfg.WebPushSubscriber)
	normalized := strings.ToLower(subscriber)
	if subscriber == "" ||
		strings.Contains(normalized, "localhost") ||
		strings.Contains(normalized, ".local") ||
		(!strings.HasPrefix(normalized, "mailto:") &&
			!strings.HasPrefix(normalized, "https://") &&
			!strings.HasPrefix(normalized, "http://")) {
		return defaultWebPushSubscriber
	}

	return subscriber
}

func (s *PushNotificationService) Subscribe(input PushSubscriptionInput) error {
	input.Endpoint = strings.TrimSpace(input.Endpoint)
	input.P256DH = strings.TrimSpace(input.P256DH)
	input.Auth = strings.TrimSpace(input.Auth)

	if input.Endpoint == "" || input.P256DH == "" || input.Auth == "" {
		return validationError("ข้อมูลการแจ้งเตือนไม่ครบถ้วน")
	}

	return s.repo.Upsert(&models.PushSubscription{
		Endpoint: input.Endpoint,
		P256DH:   input.P256DH,
		Auth:     input.Auth,
	})
}

func (s *PushNotificationService) Unsubscribe(endpoint string) error {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil
	}

	return s.repo.DeleteByEndpoint(endpoint)
}

func (s *PushNotificationService) NotifyNewApplication(ctx context.Context, application MemberResponse) {
	if !s.IsConfigured() {
		return
	}

	subscriptions, err := s.repo.List()
	if err != nil {
		log.Printf("list push subscriptions: %v", err)
		return
	}
	if len(subscriptions) == 0 {
		return
	}

	payload, err := json.Marshal(map[string]string{
		"title": "มีข้อมูลการสมัครใหม่",
		"body":  application.FirstName + " " + application.LastName + " ส่งข้อมูลสมัครเข้ามาใหม่",
		"url":   "/",
	})
	if err != nil {
		log.Printf("marshal push payload: %v", err)
		return
	}

	for _, subscription := range subscriptions {
		if err := s.send(ctx, subscription, payload); err != nil {
			log.Printf("send push notification: %v", err)
			_ = s.repo.DeleteByEndpoint(subscription.Endpoint)
		}
	}
}

func (s *PushNotificationService) send(ctx context.Context, subscription models.PushSubscription, payload []byte) error {
	response, err := webpush.SendNotificationWithContext(ctx, payload, &webpush.Subscription{
		Endpoint: subscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: subscription.P256DH,
			Auth:   subscription.Auth,
		},
	}, &webpush.Options{
		Subscriber:      s.subscriber(),
		VAPIDPublicKey:  strings.TrimSpace(s.cfg.WebPushVAPIDPublicKey),
		VAPIDPrivateKey: strings.TrimSpace(s.cfg.WebPushVAPIDPrivateKey),
		TTL:             60,
	})
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		log.Printf(
			"push notification rejected: status=%d body=%q",
			response.StatusCode,
			strings.TrimSpace(string(body)),
		)
		return validationError("ส่ง push notification ไม่สำเร็จ")
	}

	return nil
}

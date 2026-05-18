package main

import (
	"context"
	"log"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/cache"
	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
	"github.com/fulltank-garage/fulltankgarage-api/internal/database"
	"github.com/fulltank-garage/fulltankgarage-api/internal/realtime"
	"github.com/fulltank-garage/fulltankgarage-api/internal/repositories"
	"github.com/fulltank-garage/fulltankgarage-api/internal/router"
	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

func main() {
	appCtx := context.Background()

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}
	for _, warning := range cfg.Warnings() {
		log.Printf("configuration warning: %s", warning)
	}
	if err := ensureUploadDir(cfg.UploadDir); err != nil {
		log.Fatalf("prepare upload directory: %v", err)
	}

	if cfg.AppEnv == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}

	cacheStore, err := cache.New(appCtx, cfg)
	if err != nil {
		log.Fatalf("connect redis: %v", err)
	}

	memberRepo := repositories.NewMemberRepository(db)
	pushSubscriptionRepo := repositories.NewPushSubscriptionRepository(db)

	lineVerifier := services.NewLineVerifier(cfg)
	richMenuService := services.NewRichMenuService(cfg)
	for _, warning := range richMenuService.ValidateConfiguredRichMenus(context.Background()) {
		log.Printf("rich menu configuration warning: %s", warning)
	}
	memberCardService, err := services.NewMemberCardImageService(cfg)
	if err != nil {
		log.Fatalf("load member card image service: %v", err)
	}
	memberEvents := realtime.NewHub(context.Background(), cacheStore)
	richMenuQueue := services.NewRichMenuSyncQueue(cacheStore, richMenuService, memberEvents)
	richMenuQueue.Start(appCtx)
	pushNotificationService := services.NewPushNotificationService(cfg, pushSubscriptionRepo)
	memberService := services.NewMemberService(cfg, memberRepo, cacheStore, lineVerifier, richMenuService, memberCardService, memberEvents, pushNotificationService)
	memberService.StartMemberRichMenuAutoSync(appCtx)
	authService := services.NewAuthService(cfg)

	engine := router.New(router.Dependencies{
		Config:        cfg,
		Cache:         cacheStore,
		DB:            db,
		MemberService: memberService,
		AuthService:   authService,
		RichMenu:      richMenuService,
		RichMenuQueue: richMenuQueue,
		PushService:   pushNotificationService,
		MemberEvents:  memberEvents,
	})

	log.Printf("FULLTANK Garage API listening on :%s", cfg.Port)
	if err := engine.Run(":" + cfg.Port); err != nil {
		log.Fatalf("run api: %v", err)
	}
}

func ensureUploadDir(uploadDir string) error {
	if err := os.MkdirAll(uploadDir, 0o755); err != nil {
		return err
	}

	file, err := os.CreateTemp(uploadDir, ".write-check-*")
	if err != nil {
		return err
	}
	filename := file.Name()
	if err := file.Close(); err != nil {
		return err
	}

	return os.Remove(filename)
}

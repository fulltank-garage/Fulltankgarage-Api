package router

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
	"github.com/fulltank-garage/fulltankgarage-api/internal/handlers"
	"github.com/fulltank-garage/fulltankgarage-api/internal/middleware"
	"github.com/fulltank-garage/fulltankgarage-api/internal/realtime"
	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

type Dependencies struct {
	Config        config.Config
	DB            *gorm.DB
	MemberService *services.MemberService
	AuthService   *services.AuthService
	RichMenu      *services.RichMenuService
	PushService   *services.PushNotificationService
	MemberEvents  *realtime.Hub
}

func New(deps Dependencies) *gin.Engine {
	engine := gin.New()
	_ = engine.SetTrustedProxies(nil)
	engine.Use(gin.Logger(), gin.Recovery(), middleware.CORS(deps.Config.CORSAllowedOrigins))
	engine.MaxMultipartMemory = deps.Config.MaxUploadBytes
	engine.Static("/assets", "assets")
	engine.Static("/uploads", deps.Config.UploadDir)

	healthHandler := handlers.NewHealthHandler(deps.DB)
	authHandler := handlers.NewAuthHandler(deps.MemberService, deps.AuthService)
	lineWebhookHandler := handlers.NewLineWebhookHandler(deps.MemberService, deps.Config.LineChannelSecret)
	memberHandler := handlers.NewMemberHandler(deps.MemberService)
	memberEventsHandler := handlers.NewMemberEventsHandler(deps.AuthService, deps.MemberEvents)
	pushNotificationHandler := handlers.NewPushNotificationHandler(deps.PushService)
	fulltankHandler := handlers.NewFulltankHandler(deps.DB, deps.Config.UploadDir, deps.Config.BaseURL, deps.RichMenu, deps.MemberEvents)

	engine.GET("/health", healthHandler.Check)

	api := engine.Group("/api")
	api.GET("/health", healthHandler.Check)

	auth := api.Group("/auth")
	auth.POST("/register", authHandler.Register)
	auth.GET("/registration-status", authHandler.RegistrationStatus)
	auth.POST("/rich-menu/sync", authHandler.SyncRichMenu)
	auth.GET("/profile", authHandler.Profile)
	auth.GET("/profile/:id", authHandler.Profile)
	auth.POST("/login", authHandler.Login)
	auth.POST("/refresh", authHandler.Refresh)

	api.GET("/points/wallet", authHandler.PointWallet)

	api.POST("/line/webhook", lineWebhookHandler.Handle)

	api.GET("/serial-numbers/:serial", fulltankHandler.CheckSerial)
	api.POST("/warranty/link", fulltankHandler.LinkWarranty)
	api.GET("/warranty/status", fulltankHandler.WarrantyStatus)
	api.POST("/warranty/register", fulltankHandler.RegisterWarranty)
	api.GET("/public/films", fulltankHandler.ListFilms)
	api.GET("/public/promotions", fulltankHandler.ListPromotions)

	applications := api.Group("/member-applications", middleware.AdminAuth(deps.AuthService))
	applications.GET("", memberHandler.ListApplications)
	applications.PATCH("/:id/status", memberHandler.UpdateStatus)
	applications.DELETE("/:id", memberHandler.DeleteApplication)

	members := api.Group("/members", middleware.AdminAuth(deps.AuthService))
	members.GET("", memberHandler.ListMembers)
	members.PATCH("/:id/status", memberHandler.UpdateStatus)
	members.DELETE("/:id", memberHandler.DeleteMember)
	api.GET("/members/events", memberEventsHandler.Subscribe)

	adminSerials := api.Group("/serial-numbers", middleware.AdminAuth(deps.AuthService))
	adminSerials.GET("", fulltankHandler.ListSerials)
	adminSerials.POST("", fulltankHandler.CreateSerial)

	adminWarranty := api.Group("/warranty", middleware.AdminAuth(deps.AuthService))
	adminWarranty.GET("/registrations", fulltankHandler.ListRegistrations)

	adminFilms := api.Group("/films", middleware.AdminAuth(deps.AuthService))
	adminFilms.GET("", fulltankHandler.ListFilms)
	adminFilms.POST("", fulltankHandler.CreateFilm)
	adminFilms.PATCH("/:id", fulltankHandler.UpdateFilm)
	adminFilms.DELETE("/:id", fulltankHandler.DeleteFilm)

	adminPromotions := api.Group("/promotions", middleware.AdminAuth(deps.AuthService))
	adminPromotions.GET("", fulltankHandler.ListPromotions)
	adminPromotions.POST("", fulltankHandler.CreatePromotion)
	adminPromotions.PATCH("/:id", fulltankHandler.UpdatePromotion)
	adminPromotions.DELETE("/:id", fulltankHandler.DeletePromotion)

	adminUploads := api.Group("/uploads", middleware.AdminAuth(deps.AuthService))
	adminUploads.POST("/images", fulltankHandler.UploadImage)

	pushNotifications := api.Group("/push-notifications", middleware.AdminAuth(deps.AuthService))
	pushNotifications.GET("/public-key", pushNotificationHandler.PublicKey)
	pushNotifications.POST("/subscriptions", pushNotificationHandler.Subscribe)
	pushNotifications.DELETE("/subscriptions", pushNotificationHandler.Unsubscribe)

	return engine
}

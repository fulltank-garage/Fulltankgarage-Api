package config

import (
	"strings"
	"testing"
	"time"
)

func productionConfig() Config {
	return Config{
		AppEnv:                 "production",
		BaseURL:                "https://api.example.com",
		DatabaseURL:            "postgres://user:pass@example.com:5432/app?sslmode=require",
		JWTSecret:              strings.Repeat("a", 32),
		JWTTTL:                 24 * time.Hour,
		JWTRefreshTTL:          30 * 24 * time.Hour,
		LineChannelID:          "2010003223",
		LineChannelAccessToken: "token",
		LineChannelSecret:      "secret",
		LineRegisterRichMenuID: "richmenu-register",
		LineMemberRichMenuID:   "richmenu-member",
		LineVerifyRichMenuID:   "richmenu-verify",
		LineRejectedRichMenuID: "richmenu-rejected",
		LineVerifyLiffID:       "2010003223-verify",
		MaxUploadBytes:         2 * 1024 * 1024,
		CacheTTL:               5 * time.Minute,
		CORSAllowedOrigins:     []string{"https://admin.example.com"},
		UploadDir:              "/mnt/uploads",
		RedisRequired:          true,
		AdminPassword:          "changed-password",
	}
}

func TestValidateAllowsCompleteProductionConfig(t *testing.T) {
	cfg := productionConfig()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected production config to be valid: %v", err)
	}
}

func TestValidateRejectsUnsafeProductionConfig(t *testing.T) {
	cfg := productionConfig()
	cfg.BaseURL = "http://localhost:8080"
	cfg.DatabaseURL = "postgres://fulltankgarage:fulltankgarage@localhost:5432/fulltankgarage?sslmode=disable"
	cfg.JWTSecret = "change-me-in-production"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected production config validation error")
	}

	message := err.Error()
	for _, expected := range []string{
		"BASE_URL must be a public HTTPS URL",
		"DATABASE_URL must not point to localhost",
		"JWT_SECRET must be changed",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("expected validation message to contain %q, got %q", expected, message)
		}
	}
}

func TestWarningsFlagProductionRisks(t *testing.T) {
	cfg := productionConfig()
	cfg.CORSAllowedOrigins = []string{"*"}
	cfg.UploadDir = "uploads"
	cfg.RedisRequired = false
	cfg.AdminPassword = "admin1234"

	warnings := strings.Join(cfg.Warnings(), "\n")
	for _, expected := range []string{
		"CORS_ALLOWED_ORIGINS is set to *",
		"UPLOAD_DIR uses local storage",
		"REDIS_REQUIRED=false",
		"ADMIN_PASSWORD is still the default",
	} {
		if !strings.Contains(warnings, expected) {
			t.Fatalf("expected warnings to contain %q, got %q", expected, warnings)
		}
	}
}

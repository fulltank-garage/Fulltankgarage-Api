package config

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	AppEnv string
	Port   string

	BaseURL string

	DatabaseURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int
	RedisRequired bool
	CacheTTL      time.Duration

	JWTSecret     string
	JWTTTL        time.Duration
	JWTRefreshTTL time.Duration
	AdminEmail    string
	AdminPassword string
	AdminName     string

	CORSAllowedOrigins []string

	UploadDir      string
	MaxUploadBytes int64

	LineChannelID          string
	LineChannelAccessToken string
	LineChannelSecret      string
	LineRegisterRichMenuID string
	LineMemberRichMenuID   string
	LineVerifyRichMenuID   string
	LineRejectedRichMenuID string
	LineMenuSyncRichMenuID string
	LinePointWalletMenuID  string
	LineVerifyLiffID       string
	LineRichMenuEndpoint   string
	RequireLineIDToken     bool
	LineVerifyEndpoint     string
	AllowProfileFallback   bool

	WebPushVAPIDPublicKey  string
	WebPushVAPIDPrivateKey string
	WebPushSubscriber      string
}

func Load() Config {
	loadDotEnv(".env")

	maxUploadMB := envInt("MAX_UPLOAD_MB", 2)

	return Config{
		AppEnv: getenv("APP_ENV", "development"),
		Port:   getenv("PORT", "8080"),

		BaseURL: strings.TrimRight(getenv("BASE_URL", "http://localhost:8080"), "/"),

		DatabaseURL: getenv("DATABASE_URL", "postgres://fulltankgarage:fulltankgarage@localhost:5432/fulltankgarage?sslmode=disable"),

		RedisAddr:     getenv("REDIS_ADDR", "localhost:6379"),
		RedisPassword: getenv("REDIS_PASSWORD", ""),
		RedisDB:       envInt("REDIS_DB", 0),
		RedisRequired: envBool("REDIS_REQUIRED", false),
		CacheTTL:      time.Duration(envInt("CACHE_TTL_SECONDS", 300)) * time.Second,

		JWTSecret:     getenv("JWT_SECRET", "change-me-in-production"),
		JWTTTL:        time.Duration(envInt("JWT_TTL_HOURS", 24)) * time.Hour,
		JWTRefreshTTL: time.Duration(envInt("JWT_REFRESH_TTL_DAYS", 30)) * 24 * time.Hour,
		AdminEmail:    getenv("ADMIN_EMAIL", "admin@fulltankgarage.local"),
		AdminPassword: getenv("ADMIN_PASSWORD", "admin1234"),
		AdminName:     getenv("ADMIN_NAME", "FULLTANK Garage Admin"),

		CORSAllowedOrigins: splitCSV(getenv("CORS_ALLOWED_ORIGINS", "*")),

		UploadDir:      getenv("UPLOAD_DIR", "uploads"),
		MaxUploadBytes: int64(maxUploadMB) * 1024 * 1024,

		LineChannelID:          getenv("LINE_CHANNEL_ID", ""),
		LineChannelAccessToken: getenv("LINE_CHANNEL_ACCESS_TOKEN", ""),
		LineChannelSecret:      getenv("LINE_CHANNEL_SECRET", ""),
		LineRegisterRichMenuID: getenv("LINE_REGISTER_RICH_MENU_ID", ""),
		LineMemberRichMenuID:   getenv("LINE_MEMBER_RICH_MENU_ID", ""),
		LineVerifyRichMenuID:   getenv("LINE_VERIFY_RICH_MENU_ID", ""),
		LineRejectedRichMenuID: getenv("LINE_REJECTED_RICH_MENU_ID", ""),
		LineMenuSyncRichMenuID: getenv("LINE_MENU_SYNC_RICH_MENU_ID", ""),
		LinePointWalletMenuID:  getenv("LINE_POINT_WALLET_RICH_MENU_ID", ""),
		LineVerifyLiffID:       getenv("LINE_VERIFY_LIFF_ID", ""),
		LineRichMenuEndpoint:   strings.TrimRight(getenv("LINE_RICH_MENU_ENDPOINT", "https://api.line.me/v2/bot"), "/"),
		RequireLineIDToken:     envBool("REQUIRE_LINE_ID_TOKEN", false),
		LineVerifyEndpoint:     getenv("LINE_VERIFY_ENDPOINT", "https://api.line.me/oauth2/v2.1/verify"),
		AllowProfileFallback:   envBool("ALLOW_PROFILE_WITHOUT_LINE_ID", false),

		WebPushVAPIDPublicKey:  getenv("WEB_PUSH_VAPID_PUBLIC_KEY", ""),
		WebPushVAPIDPrivateKey: getenv("WEB_PUSH_VAPID_PRIVATE_KEY", ""),
		WebPushSubscriber:      getenv("WEB_PUSH_SUBSCRIBER", "mailto:admin@fulltankgarage.local"),
	}
}

func (c Config) Validate() error {
	if c.AppEnv != "production" {
		return nil
	}

	var problems []string
	requireValue := func(name string, value string) {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, name+" is required in production")
		}
	}

	requireValue("BASE_URL", c.BaseURL)
	requireValue("DATABASE_URL", c.DatabaseURL)
	requireValue("JWT_SECRET", c.JWTSecret)

	if strings.Contains(c.BaseURL, "localhost") || strings.Contains(c.BaseURL, "127.0.0.1") {
		problems = append(problems, "BASE_URL must be a public HTTPS URL in production")
	}
	if parsedBaseURL, err := url.Parse(c.BaseURL); err != nil || parsedBaseURL.Scheme != "https" || parsedBaseURL.Host == "" {
		problems = append(problems, "BASE_URL must be a valid HTTPS URL in production")
	}
	if strings.Contains(c.DatabaseURL, "localhost") || strings.Contains(c.DatabaseURL, "127.0.0.1") {
		problems = append(problems, "DATABASE_URL must not point to localhost in production")
	}
	if c.JWTSecret == "change-me-in-production" || len(c.JWTSecret) < 32 {
		problems = append(problems, "JWT_SECRET must be changed and at least 32 characters in production")
	}
	if c.RequireLineIDToken && c.LineChannelID == "" {
		problems = append(problems, "LINE_CHANNEL_ID is required when REQUIRE_LINE_ID_TOKEN=true")
	}
	if c.MaxUploadBytes <= 0 {
		problems = append(problems, "MAX_UPLOAD_MB must be greater than 0")
	}
	if c.CacheTTL <= 0 {
		problems = append(problems, "CACHE_TTL_SECONDS must be greater than 0")
	}
	if strings.TrimSpace(c.RedisAddr) == "" {
		problems = append(problems, "REDIS_ADDR is required in production")
	}
	if !c.RedisRequired {
		problems = append(problems, "REDIS_REQUIRED must be true in production")
	}
	if c.JWTRefreshTTL <= c.JWTTTL {
		problems = append(problems, "JWT_REFRESH_TTL_DAYS must be longer than JWT_TTL_HOURS")
	}

	if len(problems) > 0 {
		return fmt.Errorf("%s", strings.Join(problems, "; "))
	}

	return nil
}

func (c Config) Warnings() []string {
	warnings := []string{}
	if c.AppEnv == "production" {
		for _, origin := range c.CORSAllowedOrigins {
			if origin == "*" {
				warnings = append(warnings, "CORS_ALLOWED_ORIGINS is set to *, use the exact Vercel domains when the frontend domains are stable")
				break
			}
		}
		if c.UploadDir == "uploads" {
			warnings = append(warnings, "UPLOAD_DIR uses local storage; keep a Railway Volume mounted here or move uploads to object storage")
		}
		if c.AdminPassword == "admin1234" {
			warnings = append(warnings, "ADMIN_PASSWORD is still the default development password")
		}
	}

	return warnings
}

func loadDotEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}

		_ = os.Setenv(key, value)
	}
}

func getenv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item != "" {
			result = append(result, item)
		}
	}

	if len(result) == 0 {
		return []string{"*"}
	}

	return result
}

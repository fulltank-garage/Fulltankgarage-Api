package services

import (
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/fulltank-garage/fulltankgarage-api/internal/config"
)

type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthSession struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	User         struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"user"`
}

type RefreshInput struct {
	RefreshToken string `json:"refreshToken"`
}

type AdminClaims struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Role  string `json:"role"`
	jwt.RegisteredClaims
}

type AuthService struct {
	cfg config.Config
}

func NewAuthService(cfg config.Config) *AuthService {
	return &AuthService{cfg: cfg}
}

func (s *AuthService) Login(input LoginInput) (*AuthSession, error) {
	email := strings.TrimSpace(strings.ToLower(input.Email))
	password := strings.TrimSpace(input.Password)

	if email == "" || password == "" {
		return nil, validationError("กรุณากรอกอีเมลและรหัสผ่าน")
	}
	if email != strings.ToLower(s.cfg.AdminEmail) || password != s.cfg.AdminPassword {
		return nil, &ServiceError{Kind: ErrUnauthorized, Message: "อีเมลหรือรหัสผ่านไม่ถูกต้อง"}
	}

	return s.issueSession(email)
}

func (s *AuthService) Refresh(input RefreshInput) (*AuthSession, error) {
	tokenText := strings.TrimSpace(input.RefreshToken)
	if tokenText == "" {
		return nil, validationError("ไม่พบ refresh token")
	}

	claims := &AdminClaims{}
	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, &ServiceError{Kind: ErrUnauthorized, Message: "refresh token ไม่ถูกต้องหรือหมดอายุ"}
	}
	if claims.Role != "admin_refresh" || claims.Subject != strings.ToLower(s.cfg.AdminEmail) {
		return nil, &ServiceError{Kind: ErrUnauthorized, Message: "ไม่มีสิทธิ์ต่ออายุ token"}
	}

	return s.issueSession(strings.ToLower(s.cfg.AdminEmail))
}

func (s *AuthService) issueSession(email string) (*AuthSession, error) {
	accessToken, err := s.signAdminToken(email, "admin", s.cfg.JWTTTL)
	if err != nil {
		return nil, err
	}

	refreshToken, err := s.signAdminToken(email, "admin_refresh", s.cfg.JWTRefreshTTL)
	if err != nil {
		return nil, err
	}

	session := &AuthSession{
		Token:        accessToken,
		RefreshToken: refreshToken,
	}
	session.User.Name = s.cfg.AdminName
	session.User.Email = email

	return session, nil
}

func (s *AuthService) signAdminToken(email string, role string, ttl time.Duration) (string, error) {
	now := time.Now()
	expiresAt := now.Add(ttl)
	claims := AdminClaims{
		Email: email,
		Name:  s.cfg.AdminName,
		Role:  role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   email,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.cfg.JWTSecret))
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s *AuthService) ValidateToken(tokenText string) (*AdminClaims, error) {
	claims := &AdminClaims{}
	token, err := jwt.ParseWithClaims(tokenText, claims, func(token *jwt.Token) (any, error) {
		return []byte(s.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, &ServiceError{Kind: ErrUnauthorized, Message: "token ไม่ถูกต้องหรือหมดอายุ"}
	}
	if claims.Role != "admin" {
		return nil, &ServiceError{Kind: ErrUnauthorized, Message: "ไม่มีสิทธิ์ใช้งาน"}
	}

	return claims, nil
}

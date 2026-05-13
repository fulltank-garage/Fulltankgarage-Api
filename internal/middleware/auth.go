package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/httpx"
	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

func AdminAuth(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		token := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if header == "" || token == header {
			httpx.Error(c, &services.ServiceError{
				Kind:    services.ErrUnauthorized,
				Message: "กรุณาเข้าสู่ระบบ",
			})
			c.Abort()
			return
		}

		claims, err := authService.ValidateToken(token)
		if err != nil {
			httpx.Error(c, err)
			c.Abort()
			return
		}

		c.Set("adminEmail", claims.Email)
		c.Next()
	}
}

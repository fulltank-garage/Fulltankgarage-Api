package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/fulltank-garage/fulltankgarage-api/internal/services"
)

func Error(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "เกิดข้อผิดพลาดภายในระบบ"

	var serviceErr *services.ServiceError
	if errors.As(err, &serviceErr) {
		message = serviceErr.Message
		switch {
		case errors.Is(serviceErr.Kind, services.ErrValidation):
			status = http.StatusBadRequest
		case errors.Is(serviceErr.Kind, services.ErrDuplicateMember), errors.Is(serviceErr.Kind, services.ErrConflict):
			status = http.StatusConflict
		case errors.Is(serviceErr.Kind, services.ErrNotFound):
			status = http.StatusNotFound
		case errors.Is(serviceErr.Kind, services.ErrUnauthorized):
			status = http.StatusUnauthorized
		}
	}

	c.JSON(status, gin.H{"message": message})
}

func BadRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, gin.H{"message": message})
}

func Conflict(c *gin.Context, message string) {
	c.JSON(http.StatusConflict, gin.H{"message": message})
}

func NotFound(c *gin.Context, message string) {
	c.JSON(http.StatusNotFound, gin.H{"message": message})
}

func TooManyRequests(c *gin.Context, message string) {
	c.JSON(http.StatusTooManyRequests, gin.H{"message": message})
}

func Internal(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, gin.H{"message": message})
}

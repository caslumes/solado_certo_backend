package authmiddleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type userIDKey struct{}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey{}, userID)
}

func UserIDFrom(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDKey{}).(string)
	return userID, ok && userID != ""
}

type service interface {
	ParseToken(token string) (string, error)
}

type AuthMiddleware struct {
	authService service
}

func NewAuthMiddleware(authService service) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
	}
}

func (a *AuthMiddleware) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authorization := c.GetHeader("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		token := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		userID, err := a.authService.ParseToken(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		c.Request = c.Request.WithContext(WithUserID(c.Request.Context(), userID))
		c.Next()
	}
}

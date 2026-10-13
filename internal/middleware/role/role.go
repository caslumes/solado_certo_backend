package rolemiddleware

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

type service interface {
	HasRole(ctx context.Context, userID string, role enum.Role) (bool, error)
}

type RoleMiddleware struct {
	service service
}

func NewRoleMiddleware(service service) *RoleMiddleware {
	return &RoleMiddleware{service: service}
}

// RequireRole must run after the auth middleware. Roles are read from the
// database on every request, so a new role applies without a new token.
func (m *RoleMiddleware) RequireRole(role enum.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, ok := authmiddleware.UserIDFrom(c.Request.Context())
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}

		hasRole, err := m.service.HasRole(c.Request.Context(), userID, role)
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "RequireRole failed", "user_id", userID, "role", role, "error", err)
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "could not check role"})
			return
		}
		if !hasRole {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}

		c.Next()
	}
}

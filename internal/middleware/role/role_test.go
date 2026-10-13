package rolemiddleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService map[string][]enum.Role

func (f fakeService) HasRole(_ context.Context, userID string, role enum.Role) (bool, error) {
	if userID == "broken" {
		return false, errors.New("database unavailable")
	}
	for _, r := range f[userID] {
		if r == role {
			return true, nil
		}
	}
	return false, nil
}

func TestRequireRole(t *testing.T) {
	middleware := NewRoleMiddleware(fakeService{
		"seller-1": {enum.RoleBuyer, enum.RoleSeller},
		"buyer-1":  {enum.RoleBuyer},
	})

	tests := []struct {
		name       string
		userID     string
		wantStatus int
	}{
		{"has role", "seller-1", http.StatusOK},
		{"missing role", "buyer-1", http.StatusForbidden},
		{"no roles", "user-2", http.StatusForbidden},
		{"no user", "", http.StatusUnauthorized},
		{"lookup fails", "broken", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reached := false
			router := gin.New()
			router.Use(func(c *gin.Context) {
				if tt.userID != "" {
					c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), tt.userID))
				}
			})
			router.GET("/seller/ads", middleware.RequireRole(enum.RoleSeller), func(c *gin.Context) {
				reached = true
			})

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/seller/ads", nil))

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if reached != (tt.wantStatus == http.StatusOK) {
				t.Fatalf("handler reached = %v", reached)
			}
		})
	}
}

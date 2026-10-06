package authmiddleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService map[string]string

func (f fakeService) ParseToken(token string) (string, error) {
	userID, ok := f[token]
	if !ok {
		return "", errors.New("invalid token")
	}
	return userID, nil
}

func TestMiddleware(t *testing.T) {
	middleware := NewAuthMiddleware(fakeService{"access-token": "user-1"})

	tests := []struct {
		name          string
		authorization string
		wantStatus    int
	}{
		{"valid token", "Bearer access-token", http.StatusOK},
		{"rejected token", "Bearer refresh-token", http.StatusUnauthorized},
		{"missing header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic access-token", http.StatusUnauthorized},
		{"empty token", "Bearer ", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotUserID string
			router := gin.New()
			router.GET("/me/profile", middleware.Middleware(), func(c *gin.Context) {
				gotUserID, _ = UserIDFrom(c.Request.Context())
			})

			request := httptest.NewRequest(http.MethodGet, "/me/profile", nil)
			if tt.authorization != "" {
				request.Header.Set("Authorization", tt.authorization)
			}
			recorder := httptest.NewRecorder()

			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK && gotUserID != "user-1" {
				t.Fatalf("userID = %q, want user-1", gotUserID)
			}
		})
	}
}

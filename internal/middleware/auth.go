package middleware

import (
	"context"
	"net/http"
)

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

func (a *AuthMiddleware) Middleware() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "Authorization header is required", 401)
			return
		}

		userID, err := a.authService.ParseToken(token)
		if err != nil {
			http.Error(w, "Invalid token", 401)
			return
		}

		ctx := context.WithValue(r.Context(), "userID", userID)
		r = r.WithContext(ctx)
	}
}

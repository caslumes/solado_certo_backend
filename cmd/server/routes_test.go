package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/config"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	rolemiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/role"
	"github.com/gin-gonic/gin"
)

type tokens map[string]string

func (t tokens) ParseToken(token string) (string, error) {
	userID, ok := t[token]
	if !ok {
		return "", errors.New("invalid token")
	}
	return userID, nil
}

type noRoles struct{}

func (noRoles) HasRole(context.Context, string, enum.Role) (bool, error) {
	return false, nil
}

func TestAdminRoutesRefuseNonAdmins(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	initRoutes(router, &config.App{
		AuthMiddleware: authmiddleware.NewAuthMiddleware(tokens{"buyer-token": "buyer-1"}),
		RoleMiddleware: rolemiddleware.NewRoleMiddleware(noRoles{}),
	})

	var checked int
	for _, route := range router.Routes() {
		if !strings.HasPrefix(route.Path, "/admin/") {
			continue
		}
		checked++
		path := strings.NewReplacer(":userID", "u-1", ":advertisementID", "a-1", ":ticketID", "t-1").Replace(route.Path)

		for authorization, want := range map[string]int{"Bearer buyer-token": http.StatusForbidden, "": http.StatusUnauthorized} {
			request := httptest.NewRequest(route.Method, path, strings.NewReader(`{"reason":"teste de permissão"}`))
			if authorization != "" {
				request.Header.Set("Authorization", authorization)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != want {
				t.Errorf("%s %s with %q: status = %d, want %d", route.Method, route.Path, authorization, recorder.Code, want)
			}
		}
	}
	if checked < 13 {
		t.Fatalf("checked %d admin routes, want at least 13", checked)
	}
}

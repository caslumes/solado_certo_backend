package httpauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func serve(handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	router := gin.New()
	router.POST("/auth/register", handler.Register)
	router.POST("/auth/refresh-token", handler.RefreshToken)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

type registerUseCaseFunc func(context.Context, *auth.RegisterInput) error
type loginUseCaseFunc func(context.Context, *auth.LoginInput) (auth.TokenPair, error)
type logoutUseCaseFunc func(context.Context, string) error
type refreshTokenUseCaseFunc func(context.Context, string) (auth.TokenPair, error)

func nilRegisterUseCaseFunc(context.Context, *auth.RegisterInput) error {
	return nil
}

func nilLoginUseCaseFunc(context.Context, *auth.LoginInput) (auth.TokenPair, error) {
	return auth.TokenPair{}, nil
}

func nilLogoutUseCaseFunc(context.Context, string) error {
	return nil
}

func nilRefreshTokenUseCaseFunc(context.Context, string) (auth.TokenPair, error) {
	return auth.TokenPair{}, nil
}

func (f registerUseCaseFunc) Register(ctx context.Context, input *auth.RegisterInput) error {
	return f(ctx, input)
}

func (f loginUseCaseFunc) Login(ctx context.Context, input *auth.LoginInput) (auth.TokenPair, error) {
	return f(ctx, input)
}

func (f logoutUseCaseFunc) Logout(ctx context.Context, refreshToken string) error {
	return f(ctx, refreshToken)
}

func (f refreshTokenUseCaseFunc) RefreshToken(ctx context.Context, refreshToken string) (auth.TokenPair, error) {
	return f(ctx, refreshToken)
}

func TestRegisterDecodesRequestAndPassesContext(t *testing.T) {
	requestContext := context.WithValue(context.Background(), struct{}{}, "request")
	var received *auth.RegisterInput

	handler := NewHandler(
		registerUseCaseFunc(func(ctx context.Context, input *auth.RegisterInput) error {
			if ctx.Value(struct{}{}) != "request" {
				t.Errorf("context was not passed to use case")
			}
			received = input
			return nil
		}),
		loginUseCaseFunc(nilLoginUseCaseFunc),
		logoutUseCaseFunc(nilLogoutUseCaseFunc),
		refreshTokenUseCaseFunc(nilRefreshTokenUseCaseFunc),
	)

	request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"name":"Ana","email":"ana@example.com","phone":"123","password":"password"}`)).WithContext(requestContext)
	recorder := serve(handler, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if received == nil || received.Name != "Ana" || received.Email != "ana@example.com" {
		t.Fatalf("received input = %#v", received)
	}
}

func TestRegisterRejectsMalformedJSON(t *testing.T) {
	called := false
	handler := NewHandler(
		registerUseCaseFunc(func(context.Context, *auth.RegisterInput) error {
			called = true
			return nil
		}),
		loginUseCaseFunc(nilLoginUseCaseFunc),
		logoutUseCaseFunc(nilLogoutUseCaseFunc),
		refreshTokenUseCaseFunc(nilRefreshTokenUseCaseFunc),
	)

	recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader("{")))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("use case was called for malformed JSON")
	}
}

func TestRegisterMapsApplicationErrors(t *testing.T) {
	handler := NewHandler(
		registerUseCaseFunc(func(context.Context, *auth.RegisterInput) error {
			return app.ErrInvalidInput
		}),
		loginUseCaseFunc(nilLoginUseCaseFunc),
		logoutUseCaseFunc(nilLogoutUseCaseFunc),
		refreshTokenUseCaseFunc(nilRefreshTokenUseCaseFunc),
	)
	body := `{"name":"Ana","email":"ana@example.com","phone":"123","password":"password"}`

	recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRegisterMapsUnexpectedErrors(t *testing.T) {
	handler := NewHandler(
		registerUseCaseFunc(func(context.Context, *auth.RegisterInput) error {
			return errors.New("database unavailable")
		}),
		loginUseCaseFunc(nilLoginUseCaseFunc),
		logoutUseCaseFunc(nilLogoutUseCaseFunc),
		refreshTokenUseCaseFunc(nilRefreshTokenUseCaseFunc),
	)
	body := `{"name":"Ana","email":"ana@example.com","phone":"123","password":"password"}`

	recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body)))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestRefreshTokenMapsTokenErrorsToUnauthorized(t *testing.T) {
	for _, tokenErr := range []error{app.ErrInvalidToken, app.ErrTokenExpired, app.ErrTokenNotFound, app.ErrTokenRevoked} {
		t.Run(tokenErr.Error(), func(t *testing.T) {
			handler := NewHandler(
				registerUseCaseFunc(nilRegisterUseCaseFunc),
				loginUseCaseFunc(nilLoginUseCaseFunc),
				logoutUseCaseFunc(nilLogoutUseCaseFunc),
				refreshTokenUseCaseFunc(func(context.Context, string) (auth.TokenPair, error) {
					return auth.TokenPair{}, tokenErr
				}),
			)

			request := httptest.NewRequest(http.MethodPost, "/auth/refresh-token", strings.NewReader(`{"refresh_token":"token"}`))
			recorder := serve(handler, request)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestRefreshTokenReturnsRotatedTokens(t *testing.T) {
	handler := NewHandler(
		registerUseCaseFunc(nilRegisterUseCaseFunc),
		loginUseCaseFunc(nilLoginUseCaseFunc),
		logoutUseCaseFunc(nilLogoutUseCaseFunc),
		refreshTokenUseCaseFunc(func(context.Context, string) (auth.TokenPair, error) {
			return auth.TokenPair{AccessToken: "new-access", RefreshToken: "new-refresh"}, nil
		}),
	)

	request := httptest.NewRequest(http.MethodPost, "/auth/refresh-token", strings.NewReader(`{"refresh_token":"old-refresh"}`))
	recorder := serve(handler, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body := recorder.Body.String(); !strings.Contains(body, `"refresh_token":"new-refresh"`) {
		t.Fatalf("body = %s, want the rotated refresh token", body)
	}
}

func TestRefreshTokenRequiresToken(t *testing.T) {
	called := false
	handler := NewHandler(
		registerUseCaseFunc(nilRegisterUseCaseFunc),
		loginUseCaseFunc(nilLoginUseCaseFunc),
		logoutUseCaseFunc(nilLogoutUseCaseFunc),
		refreshTokenUseCaseFunc(func(context.Context, string) (auth.TokenPair, error) {
			called = true
			return auth.TokenPair{}, nil
		}),
	)

	for _, body := range []string{`{}`, `{"refresh_token":""}`} {
		recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/refresh-token", strings.NewReader(body)))

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want %d", body, recorder.Code, http.StatusBadRequest)
		}
	}
	if called {
		t.Fatal("use case was called without a refresh token")
	}
}

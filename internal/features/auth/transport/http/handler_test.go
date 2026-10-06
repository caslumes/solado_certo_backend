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

type fakeService struct {
	register     func(context.Context, *auth.RegisterInput) error
	login        func(context.Context, *auth.LoginInput) (auth.TokenPair, error)
	logout       func(context.Context, string) error
	refreshToken func(context.Context, string) (auth.TokenPair, error)
}

func (f fakeService) Register(ctx context.Context, input *auth.RegisterInput) error {
	return f.register(ctx, input)
}

func (f fakeService) Login(ctx context.Context, input *auth.LoginInput) (auth.TokenPair, error) {
	return f.login(ctx, input)
}

func (f fakeService) Logout(ctx context.Context, refreshToken string) error {
	return f.logout(ctx, refreshToken)
}

func (f fakeService) RefreshToken(ctx context.Context, refreshToken string) (auth.TokenPair, error) {
	return f.refreshToken(ctx, refreshToken)
}

func init() {
	gin.SetMode(gin.TestMode)
}

func serve(handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	router := gin.New()
	router.POST("/auth/register", handler.Register)
	router.POST("/auth/login", handler.Login)
	router.POST("/auth/refresh-token", handler.RefreshToken)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

const registerBody = `{"name":"Ana","email":"ana@example.com","phone":"123","password":"password"}`

func TestRegisterDecodesRequestAndPassesContext(t *testing.T) {
	requestContext := context.WithValue(context.Background(), struct{}{}, "request")
	var received *auth.RegisterInput

	handler := NewHandler(fakeService{
		register: func(ctx context.Context, input *auth.RegisterInput) error {
			if ctx.Value(struct{}{}) != "request" {
				t.Errorf("context was not passed to use case")
			}
			received = input
			return nil
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(registerBody)).WithContext(requestContext)
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
	handler := NewHandler(fakeService{
		register: func(context.Context, *auth.RegisterInput) error {
			called = true
			return nil
		},
	})

	recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader("{")))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("use case was called for malformed JSON")
	}
}

func TestRegisterMapsApplicationErrors(t *testing.T) {
	handler := NewHandler(fakeService{
		register: func(context.Context, *auth.RegisterInput) error {
			return app.ErrInvalidInput
		},
	})

	recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(registerBody)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRegisterMapsUnexpectedErrors(t *testing.T) {
	handler := NewHandler(fakeService{
		register: func(context.Context, *auth.RegisterInput) error {
			return errors.New("database unavailable")
		},
	})

	recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(registerBody)))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

func TestLoginMapsInvalidCredentialsToUnauthorized(t *testing.T) {
	handler := NewHandler(fakeService{
		login: func(context.Context, *auth.LoginInput) (auth.TokenPair, error) {
			return auth.TokenPair{}, app.ErrInvalidCredentials
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{"email":"ana@example.com","password":"wrong"}`))
	recorder := serve(handler, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestRefreshTokenMapsTokenErrorsToUnauthorized(t *testing.T) {
	for _, tokenErr := range []error{app.ErrInvalidToken, app.ErrTokenExpired, app.ErrTokenNotFound, app.ErrTokenRevoked} {
		t.Run(tokenErr.Error(), func(t *testing.T) {
			handler := NewHandler(fakeService{
				refreshToken: func(context.Context, string) (auth.TokenPair, error) {
					return auth.TokenPair{}, tokenErr
				},
			})

			request := httptest.NewRequest(http.MethodPost, "/auth/refresh-token", strings.NewReader(`{"refresh_token":"token"}`))
			recorder := serve(handler, request)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestRefreshTokenReturnsRotatedTokens(t *testing.T) {
	handler := NewHandler(fakeService{
		refreshToken: func(context.Context, string) (auth.TokenPair, error) {
			return auth.TokenPair{AccessToken: "new-access", RefreshToken: "new-refresh"}, nil
		},
	})

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
	handler := NewHandler(fakeService{
		refreshToken: func(context.Context, string) (auth.TokenPair, error) {
			called = true
			return auth.TokenPair{}, nil
		},
	})

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

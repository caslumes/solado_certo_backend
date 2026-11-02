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
	forgot       func(context.Context, string) error
	reset        func(context.Context, *auth.ResetPasswordInput) error
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

func (f fakeService) ForgotPassword(ctx context.Context, email string) error {
	return f.forgot(ctx, email)
}

func (f fakeService) ResetPassword(ctx context.Context, input *auth.ResetPasswordInput) error {
	return f.reset(ctx, input)
}

func init() {
	gin.SetMode(gin.TestMode)
}

func serve(handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	router := gin.New()
	router.POST("/auth/register", handler.Register)
	router.POST("/auth/login", handler.Login)
	router.POST("/auth/refresh-token", handler.RefreshToken)
	router.POST("/auth/password/forgot", handler.ForgotPassword)
	router.POST("/auth/password/reset", handler.ResetPassword)

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

func TestRegisterMapsDuplicateUserToConflict(t *testing.T) {
	handler := NewHandler(fakeService{
		register: func(context.Context, *auth.RegisterInput) error {
			return app.ErrUserAlreadyExists
		},
	})

	recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(registerBody)))

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusConflict)
	}
	if !strings.Contains(recorder.Body.String(), "email or phone already registered") {
		t.Fatalf("body = %s", recorder.Body.String())
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

func TestForgotPasswordAnswersAccepted(t *testing.T) {
	var received string
	handler := NewHandler(fakeService{
		forgot: func(_ context.Context, email string) error {
			received = email
			return nil
		},
	})

	request := httptest.NewRequest(http.MethodPost, "/auth/password/forgot", strings.NewReader(`{"email":"ana@example.com"}`))
	recorder := serve(handler, request)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusAccepted)
	}
	if received != "ana@example.com" {
		t.Fatalf("email = %q", received)
	}
}

func TestForgotPasswordMapsErrors(t *testing.T) {
	tests := map[string]struct {
		body string
		err  error
		want int
	}{
		"malformed JSON": {"{", nil, http.StatusBadRequest},
		"invalid e-mail": {`{"email":"ana"}`, app.ErrInvalidInput, http.StatusBadRequest},
		"unexpected":     {`{"email":"ana@example.com"}`, errors.New("database unavailable"), http.StatusInternalServerError},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewHandler(fakeService{
				forgot: func(context.Context, string) error { return tt.err },
			})

			recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/password/forgot", strings.NewReader(tt.body)))

			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}
}

func TestResetPasswordDecodesRequest(t *testing.T) {
	var received *auth.ResetPasswordInput
	handler := NewHandler(fakeService{
		reset: func(_ context.Context, input *auth.ResetPasswordInput) error {
			received = input
			return nil
		},
	})

	body := `{"email":"ana@example.com","code":"123456","new_password":"new-password"}`
	recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/password/reset", strings.NewReader(body)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if received == nil || received.Email != "ana@example.com" || received.Code != "123456" || received.NewPassword != "new-password" {
		t.Fatalf("received = %#v", received)
	}
}

func TestResetPasswordMapsErrors(t *testing.T) {
	tests := map[string]struct {
		body     string
		err      error
		want     int
		wantBody string
	}{
		"malformed JSON":    {"{", nil, http.StatusBadRequest, "invalid request body"},
		"invalid input":     {`{}`, app.ErrInvalidInput, http.StatusBadRequest, "invalid input"},
		"wrong code":        {`{}`, app.ErrInvalidResetCode, http.StatusBadRequest, "invalid or expired code"},
		"too many attempts": {`{}`, app.ErrResetAttemptsExceeded, http.StatusBadRequest, "too many attempts"},
		"unexpected":        {`{}`, errors.New("database unavailable"), http.StatusInternalServerError, "could not reset"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			handler := NewHandler(fakeService{
				reset: func(context.Context, *auth.ResetPasswordInput) error { return tt.err },
			})

			recorder := serve(handler, httptest.NewRequest(http.MethodPost, "/auth/password/reset", strings.NewReader(tt.body)))

			if recorder.Code != tt.want || !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("status = %d, body = %s; want %d with %q", recorder.Code, recorder.Body.String(), tt.want, tt.wantBody)
			}
		})
	}
}

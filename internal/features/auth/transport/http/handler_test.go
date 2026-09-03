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
)

type registerUseCaseFunc func(context.Context, *auth.RegisterInput) error
type loginUseCaseFunc func(context.Context, *auth.LoginInput) (auth.TokenPair, error)

func (f registerUseCaseFunc) Register(ctx context.Context, input *auth.RegisterInput) error {
	return f(ctx, input)
}

func (f loginUseCaseFunc) Login(ctx context.Context, input *auth.LoginInput) (auth.TokenPair, error) {
	return f(ctx, input)
}

func TestRegisterDecodesRequestAndPassesContext(t *testing.T) {
	requestContext := context.WithValue(context.Background(), struct{}{}, "request")
	var received *auth.RegisterInput

	handler := NewHandler(registerUseCaseFunc(func(ctx context.Context, input *auth.RegisterInput) error {
		if ctx.Value(struct{}{}) != "request" {
			t.Errorf("context was not passed to use case")
		}
		received = input
		return nil
	}), loginUseCaseFunc(func(ctx context.Context, input *auth.LoginInput) (auth.TokenPair, error) {
		return auth.TokenPair{}, nil
	}))

	request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"nome":"Ana","email":"ana@example.com","phone":"123","password":"password"}`)).WithContext(requestContext)
	recorder := httptest.NewRecorder()

	handler.Register(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if received == nil || received.Name != "Ana" || received.Email != "ana@example.com" {
		t.Fatalf("received input = %#v", received)
	}
}

func TestRegisterRejectsMalformedJSON(t *testing.T) {
	called := false
	handler := NewHandler(registerUseCaseFunc(func(context.Context, *auth.RegisterInput) error {
		called = true
		return nil
	}), loginUseCaseFunc(func(ctx context.Context, input *auth.LoginInput) (auth.TokenPair, error) {
		return auth.TokenPair{}, nil
	}))
	recorder := httptest.NewRecorder()

	handler.Register(recorder, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader("{")))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	if called {
		t.Fatal("use case was called for malformed JSON")
	}
}

func TestRegisterMapsApplicationErrors(t *testing.T) {
	handler := NewHandler(registerUseCaseFunc(func(context.Context, *auth.RegisterInput) error {
		return app.ErrInvalidInput
	}), loginUseCaseFunc(func(ctx context.Context, input *auth.LoginInput) (auth.TokenPair, error) {
		return auth.TokenPair{}, nil
	}))
	recorder := httptest.NewRecorder()
	body := `{"nome":"Ana","email":"ana@example.com","phone":"123","password":"password"}`

	handler.Register(recorder, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body)))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestRegisterMapsUnexpectedErrors(t *testing.T) {
	handler := NewHandler(registerUseCaseFunc(func(context.Context, *auth.RegisterInput) error {
		return errors.New("database unavailable")
	}), loginUseCaseFunc(func(ctx context.Context, input *auth.LoginInput) (auth.TokenPair, error) {
		return auth.TokenPair{}, nil
	}))
	recorder := httptest.NewRecorder()
	body := `{"nome":"Ana","email":"ana@example.com","phone":"123","password":"password"}`

	handler.Register(recorder, httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body)))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
}

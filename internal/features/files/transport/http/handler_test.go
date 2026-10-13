package httpfiles

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/pkg/storage"
	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type failingStorage struct {
	storage.Storage
}

func (failingStorage) Open(context.Context, storage.Area, string) (*storage.File, error) {
	return nil, errors.New("disk unavailable")
}

func serve(s storage.Storage, target string) *httptest.ResponseRecorder {
	router := gin.New()
	router.GET("/files/public/*path", NewHandler(s).ServePublic)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

func newStorage(t *testing.T) *storage.LocalStorage {
	t.Helper()
	s, err := storage.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalStorage: %v", err)
	}
	return s
}

func TestServePublicReturnsFile(t *testing.T) {
	s := newStorage(t)
	key, err := s.Save(context.Background(), storage.AreaPublic, "ads", ".png", strings.NewReader("png bytes"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	recorder := serve(s, storage.PublicPath(key))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if body, _ := io.ReadAll(recorder.Body); string(body) != "png bytes" {
		t.Fatalf("body = %q", body)
	}
	headers := map[string]string{
		"Content-Type":           "image/png",
		"Content-Length":         "9",
		"X-Content-Type-Options": "nosniff",
		"Cache-Control":          "public, max-age=31536000, immutable",
	}
	for name, want := range headers {
		if got := recorder.Header().Get(name); got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
}

func TestServePublicDoesNotServePrivateFiles(t *testing.T) {
	s := newStorage(t)
	key, err := s.Save(context.Background(), storage.AreaPrivate, "prescriptions", ".pdf", strings.NewReader("pdf"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	if recorder := serve(s, storage.PublicPath(key)); recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestServePublicReturnsNotFound(t *testing.T) {
	targets := []string{
		"/files/public/ads/0123456789abcdef0123456789abcdef.jpg",
		"/files/public/ads/photo.jpg",
		"/files/public/",
		"/files/public/..%2F..%2Fgo.mod",
	}

	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			recorder := serve(newStorage(t), target)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestServePublicHidesUnexpectedErrors(t *testing.T) {
	recorder := serve(failingStorage{}, "/files/public/ads/0123456789abcdef0123456789abcdef.jpg")

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), "disk unavailable") {
		t.Fatalf("body leaks the internal error: %s", recorder.Body.String())
	}
}

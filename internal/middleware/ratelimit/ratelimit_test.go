package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newTestHandler(l *Limiter) http.Handler {
	router := gin.New()
	router.POST("/auth/login", l.Middleware(), func(*gin.Context) {})
	return router
}

func newTestLimiter(now *time.Time) *Limiter {
	l := New(3, time.Minute)
	l.now = func() time.Time { return *now }
	return l
}

func request(t *testing.T, handler http.Handler, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
	r.RemoteAddr = remoteAddr
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, r)

	return recorder
}

func TestLimiterBlocksAfterBurstPerIP(t *testing.T) {
	now := time.Now()
	handler := newTestHandler(newTestLimiter(&now))

	for i := range 3 {
		if code := request(t, handler, "10.0.0.1:1234").Code; code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want %d", i+1, code, http.StatusOK)
		}
	}

	blocked := request(t, handler, "10.0.0.1:5678")
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", blocked.Code, http.StatusTooManyRequests)
	}
	if got := blocked.Header().Get("Retry-After"); got != "20" {
		t.Fatalf("Retry-After = %q, want %q", got, "20")
	}

	if code := request(t, handler, "10.0.0.2:1234").Code; code != http.StatusOK {
		t.Fatalf("other IP: status = %d, want %d", code, http.StatusOK)
	}
}

func TestLimiterIgnoresForwardedFor(t *testing.T) {
	now := time.Now()
	handler := newTestHandler(newTestLimiter(&now))

	for i := range 4 {
		r := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		r.RemoteAddr = "10.0.0.1:1234"
		r.Header.Set("X-Forwarded-For", "192.168.0."+string(rune('1'+i)))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, r)

		if i == 3 && recorder.Code != http.StatusTooManyRequests {
			t.Fatalf("spoofed header bypassed the limit: status = %d", recorder.Code)
		}
	}
}

func TestLimiterRefillsOverTime(t *testing.T) {
	now := time.Now()
	handler := newTestHandler(newTestLimiter(&now))

	for range 3 {
		request(t, handler, "10.0.0.1:1234")
	}

	now = now.Add(20 * time.Second)
	if code := request(t, handler, "10.0.0.1:1234").Code; code != http.StatusOK {
		t.Fatalf("status after refill = %d, want %d", code, http.StatusOK)
	}
}

func TestLimiterRemovesIdleClients(t *testing.T) {
	now := time.Now()
	l := newTestLimiter(&now)

	l.allow("10.0.0.1")
	now = now.Add(idleTimeout + sweepInterval)
	l.allow("10.0.0.2")

	if _, ok := l.clients["10.0.0.1"]; ok {
		t.Fatal("idle client was not removed")
	}
}

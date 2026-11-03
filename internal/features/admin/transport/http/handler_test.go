package httpadmin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/admin"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

const adminID = "admin-1"

var now = time.Date(2026, 11, 2, 15, 0, 0, 0, time.UTC)

type fakeService struct {
	users      func(context.Context, admin.UserQuery) (*admin.Page[*admin.UserEntity], error)
	moderate   func(string, string, string, admin.ModerationInput) (*admin.UserEntity, error)
	ads        func(context.Context, admin.AdQuery) (*admin.Page[*admin.AdEntity], error)
	removeAd   func(context.Context, string, string, string) (*admin.AdEntity, error)
	orders     func(context.Context, admin.OrderQuery) (*admin.Page[*admin.OrderEntity], error)
	payments   func(context.Context, admin.PaymentQuery) (*admin.Page[*admin.PaymentEntity], error)
	audit      func(context.Context, admin.AuditQuery) (*admin.Page[*admin.AuditEntity], error)
	indicators func(context.Context, string, string) (*admin.Indicators, error)
}

func (f fakeService) ListUsers(ctx context.Context, query admin.UserQuery) (*admin.Page[*admin.UserEntity], error) {
	return f.users(ctx, query)
}

func (f fakeService) BlockUser(_ context.Context, adminID string, userID string, input admin.ModerationInput) (*admin.UserEntity, error) {
	return f.moderate("block", adminID, userID, input)
}

func (f fakeService) SuspendUser(_ context.Context, adminID string, userID string, input admin.ModerationInput) (*admin.UserEntity, error) {
	return f.moderate("suspend", adminID, userID, input)
}

func (f fakeService) ReactivateUser(_ context.Context, adminID string, userID string, input admin.ModerationInput) (*admin.UserEntity, error) {
	return f.moderate("reactivate", adminID, userID, input)
}

func (f fakeService) ListAds(ctx context.Context, query admin.AdQuery) (*admin.Page[*admin.AdEntity], error) {
	return f.ads(ctx, query)
}

func (f fakeService) RemoveAd(ctx context.Context, adminID string, adID string, reason string) (*admin.AdEntity, error) {
	return f.removeAd(ctx, adminID, adID, reason)
}

func (f fakeService) ListOrders(ctx context.Context, query admin.OrderQuery) (*admin.Page[*admin.OrderEntity], error) {
	return f.orders(ctx, query)
}

func (f fakeService) ListPayments(ctx context.Context, query admin.PaymentQuery) (*admin.Page[*admin.PaymentEntity], error) {
	return f.payments(ctx, query)
}

func (f fakeService) ListAuditLogs(ctx context.Context, query admin.AuditQuery) (*admin.Page[*admin.AuditEntity], error) {
	return f.audit(ctx, query)
}

func (f fakeService) GetIndicators(ctx context.Context, from string, to string) (*admin.Indicators, error) {
	return f.indicators(ctx, from, to)
}

func init() {
	gin.SetMode(gin.TestMode)
}

func serve(service fakeService, request *http.Request) *httptest.ResponseRecorder {
	handler := NewHandler(service)
	handler.now = func() time.Time { return now }
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), adminID))
	})
	router.GET("/admin/indicators", handler.GetIndicators)
	router.GET("/admin/users", handler.ListUsers)
	router.POST("/admin/users/:userID/block", handler.BlockUser)
	router.POST("/admin/users/:userID/suspend", handler.SuspendUser)
	router.POST("/admin/users/:userID/reactivate", handler.ReactivateUser)
	router.GET("/admin/ads", handler.ListAds)
	router.POST("/admin/ads/:advertisementID/remove", handler.RemoveAd)
	router.GET("/admin/orders", handler.ListOrders)
	router.GET("/admin/payments", handler.ListPayments)
	router.GET("/admin/audit-logs", handler.ListAuditLogs)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestListUsersBindsQueryAndShowsSuspension(t *testing.T) {
	until := now.Add(24 * time.Hour)
	service := fakeService{users: func(_ context.Context, query admin.UserQuery) (*admin.Page[*admin.UserEntity], error) {
		if query.Search != "ana" || query.Status != "suspended" || query.Role != "buyer" || query.Page != 2 || query.Limit != 5 {
			t.Errorf("query = %+v", query)
		}
		user := &admin.UserEntity{ID: "u-1", Name: "Ana", Status: "active", SuspendedUntil: &until, CreatedAt: now}
		return &admin.Page[*admin.UserEntity]{Items: []*admin.UserEntity{user}, CurrentPage: 2, TotalPages: 2, TotalItems: 6}, nil
	}}

	recorder := serve(service, httptest.NewRequest(http.MethodGet, "/admin/users?q=ana&status=suspended&role=buyer&page=2&limit=5", nil))

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"status":"suspended"`) || !strings.Contains(body, `"roles":[]`) || !strings.Contains(body, `"has_previous":true`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, body)
	}

	if recorder := serve(service, httptest.NewRequest(http.MethodGet, "/admin/users?limit=0", nil)); recorder.Code != http.StatusBadRequest {
		t.Fatalf("limit 0: status = %d", recorder.Code)
	}
}

func TestModerationRoutes(t *testing.T) {
	for _, action := range []string{"block", "suspend", "reactivate"} {
		t.Run(action, func(t *testing.T) {
			service := fakeService{moderate: func(got string, gotAdmin string, userID string, input admin.ModerationInput) (*admin.UserEntity, error) {
				if got != action || gotAdmin != adminID || userID != "u-1" || input.Reason != "Fraude comprovada" {
					t.Errorf("action = %s, admin = %s, user = %s, input = %+v", got, gotAdmin, userID, input)
				}
				if action == "suspend" && (input.Until == nil || !input.Until.Equal(now.Add(48*time.Hour))) {
					t.Errorf("until = %v", input.Until)
				}
				return &admin.UserEntity{ID: userID, Status: "blocked"}, nil
			}}

			body := `{"reason":"Fraude comprovada","until":"2026-11-04T15:00:00Z"}`
			recorder := serve(service, httptest.NewRequest(http.MethodPost, "/admin/users/u-1/"+action, strings.NewReader(body)))

			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"blocked"`) {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
		})
	}
}

func TestModerationMapsErrors(t *testing.T) {
	tests := map[string]struct {
		body string
		err  error
		want int
	}{
		"malformed":  {"{", nil, http.StatusBadRequest},
		"bad until":  {`{"reason":"Fraude","until":"amanhã"}`, nil, http.StatusBadRequest},
		"no reason":  {"{}", &app.MissingFieldsError{Fields: []string{"reason"}}, http.StatusUnprocessableEntity},
		"short":      {"{}", app.ErrInvalidInput, http.StatusBadRequest},
		"own":        {"{}", app.ErrForbidden, http.StatusForbidden},
		"unknown":    {"{}", app.ErrUserNotFound, http.StatusNotFound},
		"conflict":   {"{}", app.ErrInvalidStatusTransition, http.StatusConflict},
		"unexpected": {"{}", errors.New("boom"), http.StatusInternalServerError},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			service := fakeService{moderate: func(string, string, string, admin.ModerationInput) (*admin.UserEntity, error) {
				return nil, tt.err
			}}
			if recorder := serve(service, httptest.NewRequest(http.MethodPost, "/admin/users/u-1/block", strings.NewReader(tt.body))); recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}
		})
	}
}

func TestRemoveAd(t *testing.T) {
	service := fakeService{removeAd: func(_ context.Context, gotAdmin string, id string, reason string) (*admin.AdEntity, error) {
		if gotAdmin != adminID || id != "ad-1" || reason != "Promessa de cura" {
			t.Errorf("admin = %s, id = %s, reason = %s", gotAdmin, id, reason)
		}
		return &admin.AdEntity{ID: id, Status: "removed", SellerID: "s-1", SellerName: "Loja", RemovedAt: &now}, nil
	}}

	recorder := serve(service, httptest.NewRequest(http.MethodPost, "/admin/ads/ad-1/remove", strings.NewReader(`{"reason":"Promessa de cura"}`)))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"seller":{"id":"s-1","name":"Loja"}`) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}

	service.removeAd = func(context.Context, string, string, string) (*admin.AdEntity, error) {
		return nil, app.ErrAdvertisementNotFound
	}
	if recorder := serve(service, httptest.NewRequest(http.MethodPost, "/admin/ads/ad-1/remove", strings.NewReader(`{}`))); recorder.Code != http.StatusNotFound {
		t.Fatalf("unknown: status = %d", recorder.Code)
	}
}

func TestListsBindQueries(t *testing.T) {
	service := fakeService{
		ads: func(_ context.Context, query admin.AdQuery) (*admin.Page[*admin.AdEntity], error) {
			if query.Search != "palmilha" || query.Status != "published" || query.SellerID != "s-1" {
				t.Errorf("ads query = %+v", query)
			}
			return &admin.Page[*admin.AdEntity]{CurrentPage: 1}, nil
		},
		orders: func(_ context.Context, query admin.OrderQuery) (*admin.Page[*admin.OrderEntity], error) {
			if query.Search != "SC-1" || query.Status != "paid" || query.From != "2026-11-01" || query.To != "2026-11-02" {
				t.Errorf("orders query = %+v", query)
			}
			seller := "s-1"
			order := &admin.OrderEntity{ID: "o-1", BuyerID: "b-1", BuyerName: "Ana", SellerID: &seller}
			return &admin.Page[*admin.OrderEntity]{Items: []*admin.OrderEntity{order}, CurrentPage: 1, TotalPages: 1, TotalItems: 1}, nil
		},
		payments: func(_ context.Context, query admin.PaymentQuery) (*admin.Page[*admin.PaymentEntity], error) {
			if !query.RefundGivenUp || query.Method != "pix" {
				t.Errorf("payments query = %+v", query)
			}
			return &admin.Page[*admin.PaymentEntity]{CurrentPage: 1}, nil
		},
		audit: func(_ context.Context, query admin.AuditQuery) (*admin.Page[*admin.AuditEntity], error) {
			if query.Entity != "user" || query.Action != "admin." || query.UserID != "u-1" {
				t.Errorf("audit query = %+v", query)
			}
			entry := &admin.AuditEntity{ID: "l-1", Entity: "user", Action: "admin.user_blocked", Context: json.RawMessage(`{"reason":"Fraude"}`)}
			return &admin.Page[*admin.AuditEntity]{Items: []*admin.AuditEntity{entry}, CurrentPage: 1, TotalPages: 1, TotalItems: 1}, nil
		},
	}

	tests := map[string]struct {
		url  string
		want string
	}{
		"ads":      {"/admin/ads?q=palmilha&status=published&seller_id=s-1", `"items":[]`},
		"orders":   {"/admin/orders?q=SC-1&status=paid&from=2026-11-01&to=2026-11-02", `"seller":{"id":"s-1","name":""}`},
		"payments": {"/admin/payments?refund_given_up=true&method=pix", `"total_items":0`},
		"audit":    {"/admin/audit-logs?entity=user&action=admin.&user_id=u-1", `"context":{"reason":"Fraude"},"created_at"`},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			recorder := serve(service, httptest.NewRequest(http.MethodGet, tt.url, nil))
			if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), tt.want) {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
			}
		})
	}

	if recorder := serve(service, httptest.NewRequest(http.MethodGet, "/admin/payments?refund_given_up=talvez", nil)); recorder.Code != http.StatusBadRequest {
		t.Fatalf("refund_given_up: status = %d", recorder.Code)
	}
}

func TestGetIndicators(t *testing.T) {
	service := fakeService{indicators: func(_ context.Context, from string, to string) (*admin.Indicators, error) {
		if from != "2026-10-01" || to != "2026-10-31" {
			return nil, app.ErrInvalidInput
		}
		start := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
		return &admin.Indicators{
			Period: admin.Period{From: start, Until: start.AddDate(0, 0, 31)}, NewUsers: 4, PaidAmount: "250.00",
			OrdersByStatus: map[string]uint{"paid": 2}, PaymentsByStatus: map[string]admin.Amount{"captured": {Count: 2, Amount: "250.00"}},
		}, nil
	}}

	recorder := serve(service, httptest.NewRequest(http.MethodGet, "/admin/indicators?from=2026-10-01&to=2026-10-31", nil))
	body := recorder.Body.String()
	for _, want := range []string{`"from":"2026-10-01"`, `"to":"2026-10-31"`, `"new_users":4`, `"paid":2`, `"captured":{"count":2,"amount":"250.00"}`, `"refunded":{"count":0,"amount":"0.00"}`} {
		if !strings.Contains(body, want) {
			t.Fatalf("status = %d, body = %s, want %s", recorder.Code, body, want)
		}
	}

	if recorder := serve(service, httptest.NewRequest(http.MethodGet, "/admin/indicators?from=x", nil)); recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid: status = %d", recorder.Code)
	}
}

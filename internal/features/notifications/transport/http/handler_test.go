package httpnotifications

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

const (
	testUserID         = "user-1"
	testNotificationID = "00000000-0000-0000-0000-000000000901"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService struct {
	list    func(ctx context.Context, userID string, page uint, limit uint) (*notifications.NotificationPage, error)
	read    func(ctx context.Context, userID string, id string) error
	readAll func(ctx context.Context, userID string) error
}

func (f *fakeService) ListNotifications(ctx context.Context, userID string, page uint, limit uint) (*notifications.NotificationPage, error) {
	return f.list(ctx, userID, page, limit)
}

func (f *fakeService) MarkRead(ctx context.Context, userID string, id string) error {
	return f.read(ctx, userID, id)
}

func (f *fakeService) MarkAllRead(ctx context.Context, userID string) error {
	return f.readAll(ctx, userID)
}

func newRouter(handler *Handler, authenticated bool) *gin.Engine {
	router := gin.New()
	group := router.Group("", func(c *gin.Context) {
		if authenticated {
			c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), testUserID))
		}
	})
	group.GET("/notifications", handler.ListNotifications)
	group.POST("/notifications/read-all", handler.MarkAllRead)
	group.POST("/notifications/:notificationID/read", handler.MarkRead)
	return router
}

func serve(handler *Handler, authenticated bool, method, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	newRouter(handler, authenticated).ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

func TestListNotifications(t *testing.T) {
	var gotUser string
	var gotPage, gotLimit uint
	handler := NewHandler(&fakeService{
		list: func(_ context.Context, userID string, page uint, limit uint) (*notifications.NotificationPage, error) {
			gotUser, gotPage, gotLimit = userID, page, limit
			return &notifications.NotificationPage{
				Items: []*notifications.NotificationEntity{{
					ID: testNotificationID, Type: notifications.TypeOrderPaid, Title: "Novo pedido pago", Body: "O pedido SC-1 foi pago.",
					Data: map[string]string{"order_id": "order-1"}, CreatedAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
				}},
				CurrentPage: 1, TotalPages: 1, TotalItems: 1, UnreadCount: 1,
			}, nil
		},
	})

	recorder := serve(handler, true, http.MethodGet, "/notifications")

	if recorder.Code != http.StatusOK || gotUser != testUserID || gotPage != 1 || gotLimit != 20 {
		t.Fatalf("status = %d, user = %q, page = %d, limit = %d", recorder.Code, gotUser, gotPage, gotLimit)
	}
	var body notifications.NotificationPageOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.UnreadCount != 1 || len(body.Items) != 1 || body.Items[0].Type != "order_paid" || body.Items[0].Read || body.Items[0].Data["order_id"] != "order-1" {
		t.Errorf("body = %s", recorder.Body.String())
	}

	serve(handler, true, http.MethodGet, "/notifications?page=2&limit=5")
	if gotPage != 2 || gotLimit != 5 {
		t.Errorf("page = %d, limit = %d, want 2 and 5", gotPage, gotLimit)
	}
}

func TestListNotificationsRejectsBadQueries(t *testing.T) {
	handler := NewHandler(&fakeService{})
	for _, query := range []string{"page=0", "limit=0", "limit=101", "page=x"} {
		if recorder := serve(handler, true, http.MethodGet, "/notifications?"+query); recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, recorder.Code)
		}
	}
}

func TestListNotificationsFailure(t *testing.T) {
	handler := NewHandler(&fakeService{
		list: func(context.Context, string, uint, uint) (*notifications.NotificationPage, error) {
			return nil, errors.New("database down")
		},
	})
	if recorder := serve(handler, true, http.MethodGet, "/notifications"); recorder.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", recorder.Code)
	}
}

func TestMarkRead(t *testing.T) {
	var gotUser, gotID string
	var result error
	handler := NewHandler(&fakeService{
		read: func(_ context.Context, userID string, id string) error {
			gotUser, gotID = userID, id
			return result
		},
	})

	recorder := serve(handler, true, http.MethodPost, "/notifications/"+testNotificationID+"/read")
	if recorder.Code != http.StatusNoContent || gotUser != testUserID || gotID != testNotificationID {
		t.Errorf("status = %d, user = %q, id = %q", recorder.Code, gotUser, gotID)
	}

	result = app.ErrNotificationNotFound
	if recorder := serve(handler, true, http.MethodPost, "/notifications/"+testNotificationID+"/read"); recorder.Code != http.StatusNotFound {
		t.Errorf("unknown notification: status = %d, want 404", recorder.Code)
	}
}

func TestMarkAllRead(t *testing.T) {
	var gotUser string
	var result error
	handler := NewHandler(&fakeService{
		readAll: func(_ context.Context, userID string) error {
			gotUser = userID
			return result
		},
	})

	if recorder := serve(handler, true, http.MethodPost, "/notifications/read-all"); recorder.Code != http.StatusNoContent || gotUser != testUserID {
		t.Errorf("status = %d, user = %q", recorder.Code, gotUser)
	}

	result = errors.New("database down")
	if recorder := serve(handler, true, http.MethodPost, "/notifications/read-all"); recorder.Code != http.StatusInternalServerError {
		t.Errorf("failure: status = %d, want 500", recorder.Code)
	}
}

func TestRequiresUser(t *testing.T) {
	handler := NewHandler(&fakeService{})
	for _, request := range []struct{ method, target string }{
		{http.MethodGet, "/notifications"},
		{http.MethodPost, "/notifications/read-all"},
		{http.MethodPost, "/notifications/" + testNotificationID + "/read"},
	} {
		if recorder := serve(handler, false, request.method, request.target); recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: status = %d, want 401", request.method, request.target, recorder.Code)
		}
	}
}

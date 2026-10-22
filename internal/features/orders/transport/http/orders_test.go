package httporders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
)

const testOrderID = "00000000-0000-0000-0000-000000000801"

func sampleDetail(status orders.OrderStatus) *orders.OrderDetail {
	order := sampleCheckout().Orders[0]
	order.ID = testOrderID
	order.Status = status
	reason := "checkout"
	return &orders.OrderDetail{Order: order, History: []orders.StatusChangeEntity{{To: orders.OrderStatusAwaitingPayment, Reason: &reason}}}
}

func ordersService(err error) fakeService {
	service := fakeService{}
	service.list = func(context.Context, string, orders.HistoryFilter, uint, uint) (*orders.OrderPage, error) {
		if err != nil {
			return nil, err
		}
		return &orders.OrderPage{Items: sampleCheckout().Orders, CurrentPage: 1, TotalPages: 1, TotalItems: 1}, nil
	}
	service.getOrder = func(context.Context, string, string) (*orders.OrderDetail, error) {
		if err != nil {
			return nil, err
		}
		return sampleDetail(orders.OrderStatusAwaitingPayment), nil
	}
	service.cancel = func(context.Context, string, string) (*orders.OrderDetail, error) {
		if err != nil {
			return nil, err
		}
		return sampleDetail(orders.OrderStatusCancelled), nil
	}
	detail := func(status orders.OrderStatus) func(context.Context, string, string) (*orders.OrderDetail, error) {
		return func(context.Context, string, string) (*orders.OrderDetail, error) {
			if err != nil {
				return nil, err
			}
			return sampleDetail(status), nil
		}
	}
	service.sellerList = func(context.Context, string, orders.HistoryFilter, uint, uint) (*orders.OrderPage, error) {
		if err != nil {
			return nil, err
		}
		return &orders.OrderPage{Items: sampleCheckout().Orders, CurrentPage: 1, TotalPages: 1, TotalItems: 1}, nil
	}
	service.sellerGet = detail(orders.OrderStatusPaid)
	service.processing = detail(orders.OrderStatusProcessing)
	service.ship = func(ctx context.Context, userID string, id string, _ orders.ShipmentInput) (*orders.OrderDetail, error) {
		return detail(orders.OrderStatusShipped)(ctx, userID, id)
	}
	service.deliver = detail(orders.OrderStatusDelivered)
	return service
}

var orderEndpoints = map[string]endpoint{
	"list":        {http.MethodGet, "/orders", ""},
	"get":         {http.MethodGet, "/orders/" + testOrderID, ""},
	"cancel":      {http.MethodPost, "/orders/" + testOrderID + "/cancel", ""},
	"deliver":     {http.MethodPost, "/orders/" + testOrderID + "/delivered", ""},
	"seller list": {http.MethodGet, "/seller/orders", ""},
	"seller get":  {http.MethodGet, "/seller/orders/" + testOrderID, ""},
	"processing":  {http.MethodPost, "/seller/orders/" + testOrderID + "/processing", ""},
	"ship":        {http.MethodPost, "/seller/orders/" + testOrderID + "/shipment", `{"carrier":"Correios","tracking_code":"AA123456789BR"}`},
}

func TestListOrders(t *testing.T) {
	var gotFilter orders.HistoryFilter
	var gotPage, gotLimit uint
	service := ordersService(nil)
	inner := service.list
	service.list = func(ctx context.Context, userID string, filter orders.HistoryFilter, page uint, limit uint) (*orders.OrderPage, error) {
		if userID != testBuyerID {
			t.Errorf("user = %q", userID)
		}
		gotFilter, gotPage, gotLimit = filter, page, limit
		return inner(ctx, userID, filter, page, limit)
	}

	recorder := serve(NewHandler(service), http.MethodGet, "/orders", "")
	if recorder.Code != http.StatusOK || gotFilter != (orders.HistoryFilter{}) || gotPage != 1 || gotLimit != 10 {
		t.Fatalf("status = %d, filter = %+v, page = %d, limit = %d", recorder.Code, gotFilter, gotPage, gotLimit)
	}
	var body orders.OrderPageOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Number != "SC-20261019-ABC123" || body.TotalItems != 1 {
		t.Errorf("body = %s", recorder.Body.String())
	}

	serve(NewHandler(service), http.MethodGet, "/orders?page=3&limit=5&status=delivered&from=2026-10-01&to=2026-10-31", "")
	want := orders.HistoryFilter{Status: orders.OrderStatusDelivered, From: "2026-10-01", To: "2026-10-31"}
	if gotPage != 3 || gotLimit != 5 || gotFilter != want {
		t.Errorf("page = %d, limit = %d, filter = %+v", gotPage, gotLimit, gotFilter)
	}
}

func TestListOrdersRejectsBadQueries(t *testing.T) {
	for _, query := range []string{"page=0", "limit=0", "limit=101", "page=x"} {
		if recorder := serve(NewHandler(ordersService(nil)), http.MethodGet, "/orders?"+query, ""); recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, recorder.Code)
		}
	}
}

func TestGetAndCancelOrder(t *testing.T) {
	var gotID string
	service := ordersService(nil)
	service.cancel = func(_ context.Context, userID string, id string) (*orders.OrderDetail, error) {
		gotID = id
		return sampleDetail(orders.OrderStatusCancelled), nil
	}

	recorder := serve(NewHandler(service), http.MethodGet, "/orders/"+testOrderID, "")
	var body orders.OrderDetailOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if recorder.Code != http.StatusOK || body.ID != testOrderID || !body.CanCancel || len(body.History) != 1 || body.History[0].To != "awaiting_payment" {
		t.Errorf("get: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = serve(NewHandler(service), http.MethodPost, "/orders/"+testOrderID+"/cancel", "")
	body = orders.OrderDetailOutput{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if recorder.Code != http.StatusOK || gotID != testOrderID || body.Status != "cancelled" || body.CanCancel {
		t.Errorf("cancel: status = %d, id = %q, body = %s", recorder.Code, gotID, recorder.Body.String())
	}
}

func TestOrderEndpointsMapErrors(t *testing.T) {
	errorsByStatus := map[int]error{
		http.StatusBadRequest:          app.ErrInvalidInput,
		http.StatusNotFound:            app.ErrOrderNotFound,
		http.StatusConflict:            app.ErrInvalidStatusTransition,
		http.StatusInternalServerError: errors.New("database unavailable"),
	}
	for status, err := range errorsByStatus {
		for name, e := range orderEndpoints {
			recorder := serve(NewHandler(ordersService(err)), e.method, e.target, e.body)
			if recorder.Code != status {
				t.Errorf("%s %v: status = %d, want %d", name, err, recorder.Code, status)
			}
			if strings.Contains(recorder.Body.String(), "database unavailable") {
				t.Errorf("body leaks the internal error: %s", recorder.Body.String())
			}
		}
	}
}

func TestOrderEndpointsRequireAuthenticatedUser(t *testing.T) {
	for name, e := range orderEndpoints {
		if recorder := serveAs(NewHandler(fakeService{}), false, e.method, e.target, e.body); recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, recorder.Code)
		}
	}
}

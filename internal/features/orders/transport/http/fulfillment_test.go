package httporders

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
)

func TestListSellerOrders(t *testing.T) {
	var gotUser string
	var gotStatus orders.OrderStatus
	var gotPage, gotLimit uint
	service := ordersService(nil)
	inner := service.sellerList
	service.sellerList = func(ctx context.Context, userID string, status orders.OrderStatus, page uint, limit uint) (*orders.OrderPage, error) {
		gotUser, gotStatus, gotPage, gotLimit = userID, status, page, limit
		return inner(ctx, userID, status, page, limit)
	}

	recorder := serve(NewHandler(service), http.MethodGet, "/seller/orders", "")
	if recorder.Code != http.StatusOK || gotUser != testBuyerID || gotStatus != "" || gotPage != 1 || gotLimit != 10 {
		t.Fatalf("status = %d, user = %q, filter = %q, page = %d, limit = %d", recorder.Code, gotUser, gotStatus, gotPage, gotLimit)
	}
	var body orders.SellerOrderPageOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Number != "SC-20261019-ABC123" {
		t.Errorf("body = %s", recorder.Body.String())
	}

	serve(NewHandler(service), http.MethodGet, "/seller/orders?status=shipped&page=2&limit=5", "")
	if gotStatus != orders.OrderStatusShipped || gotPage != 2 || gotLimit != 5 {
		t.Errorf("filter = %q, page = %d, limit = %d", gotStatus, gotPage, gotLimit)
	}

	for _, query := range []string{"page=0", "limit=101"} {
		if recorder := serve(NewHandler(service), http.MethodGet, "/seller/orders?"+query, ""); recorder.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", query, recorder.Code)
		}
	}
}

func TestSellerOrderDetailOutput(t *testing.T) {
	service := ordersService(nil)
	service.sellerGet = func(context.Context, string, string) (*orders.OrderDetail, error) {
		detail := sampleDetail(orders.OrderStatusProcessing)
		detail.Order.BuyerName = "Maria"
		return detail, nil
	}

	recorder := serve(NewHandler(service), http.MethodGet, "/seller/orders/"+testOrderID, "")

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	buyer, _ := body["buyer"].(map[string]any)
	if buyer["name"] != "Maria" || body["can_ship"] != true || body["can_start_processing"] != false || body["shipment"] != nil {
		t.Errorf("body = %s", recorder.Body.String())
	}
}

func TestShipOrderPassesTheInputOn(t *testing.T) {
	var gotID string
	var gotInput orders.ShipmentInput
	service := ordersService(nil)
	service.ship = func(_ context.Context, _ string, id string, input orders.ShipmentInput) (*orders.OrderDetail, error) {
		gotID, gotInput = id, input
		detail := sampleDetail(orders.OrderStatusShipped)
		posted := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
		detail.Shipment = &orders.ShipmentEntity{Carrier: "Correios", TrackingCode: "AA123456789BR", Status: "posted", PostedAt: &posted}
		return detail, nil
	}

	recorder := serve(NewHandler(service), http.MethodPost, "/seller/orders/"+testOrderID+"/shipment", `{"carrier":"Correios","tracking_code":"aa123456789br"}`)

	if recorder.Code != http.StatusOK || gotID != testOrderID || gotInput != (orders.ShipmentInput{Carrier: "Correios", TrackingCode: "aa123456789br"}) {
		t.Fatalf("status = %d, id = %q, input = %+v", recorder.Code, gotID, gotInput)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	shipment, _ := body["shipment"].(map[string]any)
	if shipment["tracking_code"] != "AA123456789BR" || shipment["status"] != "posted" {
		t.Errorf("body = %s", recorder.Body.String())
	}

	if recorder := serve(NewHandler(service), http.MethodPost, "/seller/orders/"+testOrderID+"/shipment", `{`); recorder.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: status = %d, want 400", recorder.Code)
	}
}

func TestShipOrderMapsTrackingErrors(t *testing.T) {
	for err, status := range map[error]int{app.ErrInvalidTrackingCode: http.StatusUnprocessableEntity, app.ErrTrackingCodeInUse: http.StatusConflict} {
		service := ordersService(nil)
		service.ship = func(context.Context, string, string, orders.ShipmentInput) (*orders.OrderDetail, error) {
			return nil, err
		}
		recorder := serve(NewHandler(service), http.MethodPost, "/seller/orders/"+testOrderID+"/shipment", `{"carrier":"Correios","tracking_code":"1"}`)
		if recorder.Code != status {
			t.Errorf("%v: status = %d, want %d", err, recorder.Code, status)
		}
	}
}

func TestConfirmDeliveryOutput(t *testing.T) {
	recorder := serve(NewHandler(ordersService(nil)), http.MethodPost, "/orders/"+testOrderID+"/delivered", "")

	var body orders.OrderDetailOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if recorder.Code != http.StatusOK || body.Status != "delivered" || body.CanConfirmDelivery || body.CanCancel {
		t.Errorf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

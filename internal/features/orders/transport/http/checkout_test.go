package httporders

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

const (
	testAddressID = "00000000-0000-0000-0000-000000000901"
	checkoutBody  = `{"address_id":"` + testAddressID + `","shipping":[{"advertisement_id":"` + testAdID + `","shipping_option_id":"pac"}]}`
)

func samplePlan() *orders.CheckoutPlan {
	item := sampleCart().Items[0]
	return &orders.CheckoutPlan{
		CartID:    "cart-1",
		AddressID: testAddressID,
		Address:   orders.AddressSnapshot{Receiver: "Maria", City: "Campinas"},
		Orders: []orders.PlannedOrder{{
			SellerID:      item.SellerID,
			SellerName:    item.SellerName,
			Items:         []orders.PlannedItem{{CartItemEntity: item, ShippingOption: orders.ShippingOptionEntity{ID: "pac", Name: "PAC", Price: "19.90"}}},
			Subtotal:      helpers.MustParseCents("179.80"),
			ShippingTotal: helpers.MustParseCents("19.90"),
		}},
	}
}

func sampleCheckout() *orders.CheckoutEntity {
	return &orders.CheckoutEntity{ID: "checkout-1", Orders: []*orders.OrderEntity{{
		ID:       "order-1",
		Number:   "SC-20261019-ABC123",
		Status:   orders.OrderStatusAwaitingPayment,
		SellerID: "seller-1",
		Total:    "199.70",
		Items: []orders.OrderItemEntity{{
			AdvertisementID: testAdID,
			Quantity:        2,
			UnitPrice:       "89.90",
			Subtotal:        "179.80",
			Snapshot:        orders.ProductSnapshot{Title: "Palmilha", ShippingOption: orders.ShippingOptionOutput{ID: "pac", Price: "19.90"}},
		}},
	}}}
}

func checkoutService(err error) fakeService {
	service := serviceReturning(sampleCart(), nil)
	service.preview = func(context.Context, string, orders.CheckoutInput) (*orders.CheckoutPlan, error) {
		if err != nil {
			return nil, err
		}
		return samplePlan(), nil
	}
	service.checkout = func(context.Context, string, string, orders.CheckoutInput) (*orders.CheckoutEntity, bool, error) {
		if err != nil {
			return nil, false, err
		}
		return sampleCheckout(), true, nil
	}
	return service
}

func serveCheckout(handler *Handler, key string, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/checkout", strings.NewReader(body))
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	newRouter(handler, true).ServeHTTP(recorder, request)
	return recorder
}

func TestPreviewCheckout(t *testing.T) {
	var got orders.CheckoutInput
	service := checkoutService(nil)
	service.preview = func(_ context.Context, userID string, input orders.CheckoutInput) (*orders.CheckoutPlan, error) {
		if userID != testBuyerID {
			t.Errorf("user = %q", userID)
		}
		got = input
		return samplePlan(), nil
	}

	recorder := serve(NewHandler(service), http.MethodPost, "/checkout/preview", checkoutBody)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", recorder.Code, recorder.Body.String())
	}
	if got.AddressID != testAddressID || len(got.Shipping) != 1 || got.Shipping[0] != (orders.ShippingSelection{AdvertisementID: testAdID, ShippingOptionID: "pac"}) {
		t.Errorf("input = %+v", got)
	}
	var body orders.CheckoutPreviewOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Total != "199.70" || body.ShippingTotal != "19.90" || body.Address.City != "Campinas" || len(body.Orders) != 1 {
		t.Errorf("body = %s", recorder.Body.String())
	}
}

func TestCheckoutStatuses(t *testing.T) {
	var gotKey string
	service := checkoutService(nil)
	created := true
	service.checkout = func(_ context.Context, _ string, key string, _ orders.CheckoutInput) (*orders.CheckoutEntity, bool, error) {
		gotKey = key
		return sampleCheckout(), created, nil
	}

	recorder := serveCheckout(NewHandler(service), "key-1", checkoutBody)
	if recorder.Code != http.StatusCreated || gotKey != "key-1" {
		t.Fatalf("status = %d, key = %q", recorder.Code, gotKey)
	}
	var body orders.CheckoutOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != "checkout-1" || body.Total != "199.70" || len(body.Orders) != 1 || body.Orders[0].Number != "SC-20261019-ABC123" ||
		body.Orders[0].Status != "awaiting_payment" || body.Orders[0].Items[0].Title != "Palmilha" {
		t.Errorf("body = %s", recorder.Body.String())
	}

	created = false
	if recorder := serveCheckout(NewHandler(service), "key-1", checkoutBody); recorder.Code != http.StatusOK {
		t.Errorf("repeat: status = %d, want 200", recorder.Code)
	}
}

func TestCheckoutErrors(t *testing.T) {
	tests := []struct {
		err      error
		status   int
		withCart bool
	}{
		{&app.OutOfStockError{Items: []app.StockShortage{{AdvertisementID: testAdID, Requested: 2, Available: 1}}}, http.StatusConflict, true},
		{app.ErrPricesChanged, http.StatusConflict, true},
		{app.ErrCartChanged, http.StatusConflict, true},
		{&app.ShippingRequiredError{AdvertisementIDs: []string{testAdID}}, http.StatusUnprocessableEntity, false},
		{app.ErrCartEmpty, http.StatusUnprocessableEntity, false},
		{app.ErrAddressNotFound, http.StatusNotFound, false},
		{app.ErrInvalidInput, http.StatusBadRequest, false},
		{errors.New("database unavailable"), http.StatusInternalServerError, false},
	}
	for _, tt := range tests {
		for _, target := range []string{"/checkout", "/checkout/preview"} {
			var recorder *httptest.ResponseRecorder
			if target == "/checkout" {
				recorder = serveCheckout(NewHandler(checkoutService(tt.err)), "key-1", checkoutBody)
			} else {
				recorder = serve(NewHandler(checkoutService(tt.err)), http.MethodPost, target, checkoutBody)
			}

			if recorder.Code != tt.status {
				t.Errorf("%s %v: status = %d, want %d", target, tt.err, recorder.Code, tt.status)
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if _, hasCart := body["cart"]; hasCart != tt.withCart {
				t.Errorf("%s %v: has cart = %v, want %v", target, tt.err, hasCart, tt.withCart)
			}
			if strings.Contains(recorder.Body.String(), "database unavailable") {
				t.Errorf("body leaks the internal error: %s", recorder.Body.String())
			}
		}
	}
}

func TestCheckoutErrorBodies(t *testing.T) {
	recorder := serveCheckout(NewHandler(checkoutService(&app.ShippingRequiredError{AdvertisementIDs: []string{testAdID}})), "key-1", checkoutBody)
	var shipping struct {
		Error            string   `json:"error"`
		AdvertisementIDs []string `json:"advertisement_ids"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &shipping); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if shipping.Error != "shipping option required" || len(shipping.AdvertisementIDs) != 1 {
		t.Errorf("body = %s", recorder.Body.String())
	}

	recorder = serveCheckout(NewHandler(checkoutService(&app.OutOfStockError{Items: []app.StockShortage{{AdvertisementID: testAdID, Requested: 2, Available: 1}}})), "key-1", checkoutBody)
	var stock struct {
		Error string              `json:"error"`
		Items []app.StockShortage `json:"items"`
		Cart  orders.CartOutput   `json:"cart"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &stock); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if stock.Error != "out of stock" || len(stock.Items) != 1 || stock.Cart.Subtotal != "179.80" {
		t.Errorf("body = %s", recorder.Body.String())
	}
}

func TestCheckoutConflictWithoutCartWhenTheCartFails(t *testing.T) {
	service := checkoutService(app.ErrPricesChanged)
	service.get = func(context.Context, string) (*orders.CartEntity, error) { return nil, errors.New("db down") }

	recorder := serveCheckout(NewHandler(service), "key-1", checkoutBody)

	if recorder.Code != http.StatusConflict || strings.Contains(recorder.Body.String(), `"cart"`) {
		t.Errorf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestCheckoutBadBodiesAndAuth(t *testing.T) {
	for _, target := range []string{"/checkout", "/checkout/preview"} {
		if recorder := serve(NewHandler(checkoutService(nil)), http.MethodPost, target, `{`); recorder.Code != http.StatusBadRequest {
			t.Errorf("%s bad body: status = %d", target, recorder.Code)
		}
		if recorder := serveAs(NewHandler(fakeService{}), false, http.MethodPost, target, checkoutBody); recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s without user: status = %d", target, recorder.Code)
		}
	}
}

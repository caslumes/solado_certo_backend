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
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

const (
	testBuyerID = "buyer-1"
	testAdID    = "00000000-0000-0000-0000-000000000601"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService struct {
	Service
	get      func(context.Context, string) (*orders.CartEntity, error)
	add      func(context.Context, string, string, int) (*orders.CartEntity, error)
	change   func(context.Context, string, string, int) (*orders.CartEntity, error)
	remove   func(context.Context, string, string) (*orders.CartEntity, error)
	preview  func(context.Context, string, orders.CheckoutInput) (*orders.CheckoutPlan, error)
	checkout func(context.Context, string, string, orders.CheckoutInput) (*orders.CheckoutEntity, bool, error)
	list     func(context.Context, string, orders.HistoryFilter, uint, uint) (*orders.OrderPage, error)
	getOrder func(context.Context, string, string) (*orders.OrderDetail, error)
	cancel   func(context.Context, string, string) (*orders.OrderDetail, error)

	sellerList func(context.Context, string, orders.HistoryFilter, uint, uint) (*orders.OrderPage, error)
	sellerGet  func(context.Context, string, string) (*orders.OrderDetail, error)
	processing func(context.Context, string, string) (*orders.OrderDetail, error)
	ship       func(context.Context, string, string, orders.ShipmentInput) (*orders.OrderDetail, error)
	deliver    func(context.Context, string, string) (*orders.OrderDetail, error)
}

func (f fakeService) ListSellerOrders(ctx context.Context, userID string, filter orders.HistoryFilter, page uint, limit uint) (*orders.OrderPage, error) {
	return f.sellerList(ctx, userID, filter, page, limit)
}

func (f fakeService) GetSellerOrder(ctx context.Context, userID string, id string) (*orders.OrderDetail, error) {
	return f.sellerGet(ctx, userID, id)
}

func (f fakeService) StartProcessing(ctx context.Context, userID string, id string) (*orders.OrderDetail, error) {
	return f.processing(ctx, userID, id)
}

func (f fakeService) ShipOrder(ctx context.Context, userID string, id string, input orders.ShipmentInput) (*orders.OrderDetail, error) {
	return f.ship(ctx, userID, id, input)
}

func (f fakeService) ConfirmDelivery(ctx context.Context, userID string, id string) (*orders.OrderDetail, error) {
	return f.deliver(ctx, userID, id)
}

func (f fakeService) ListOrders(ctx context.Context, userID string, filter orders.HistoryFilter, page uint, limit uint) (*orders.OrderPage, error) {
	return f.list(ctx, userID, filter, page, limit)
}

func (f fakeService) GetOrder(ctx context.Context, userID string, id string) (*orders.OrderDetail, error) {
	return f.getOrder(ctx, userID, id)
}

func (f fakeService) CancelOrder(ctx context.Context, userID string, id string) (*orders.OrderDetail, error) {
	return f.cancel(ctx, userID, id)
}

func (f fakeService) PreviewCheckout(ctx context.Context, userID string, input orders.CheckoutInput) (*orders.CheckoutPlan, error) {
	return f.preview(ctx, userID, input)
}

func (f fakeService) Checkout(ctx context.Context, userID string, key string, input orders.CheckoutInput) (*orders.CheckoutEntity, bool, error) {
	return f.checkout(ctx, userID, key, input)
}

func (f fakeService) GetCart(ctx context.Context, userID string) (*orders.CartEntity, error) {
	return f.get(ctx, userID)
}

func (f fakeService) AddCartItem(ctx context.Context, userID string, advertisementID string, quantity int) (*orders.CartEntity, error) {
	return f.add(ctx, userID, advertisementID, quantity)
}

func (f fakeService) ChangeCartItemQuantity(ctx context.Context, userID string, advertisementID string, quantity int) (*orders.CartEntity, error) {
	return f.change(ctx, userID, advertisementID, quantity)
}

func (f fakeService) RemoveCartItem(ctx context.Context, userID string, advertisementID string) (*orders.CartEntity, error) {
	return f.remove(ctx, userID, advertisementID)
}

func newRouter(handler *Handler, authenticated bool) *gin.Engine {
	router := gin.New()
	if authenticated {
		router.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), testBuyerID))
		})
	}
	router.GET("/cart", handler.GetCart)
	router.POST("/cart/items", handler.AddCartItem)
	router.PATCH("/cart/items/:advertisementID", handler.ChangeCartItemQuantity)
	router.DELETE("/cart/items/:advertisementID", handler.RemoveCartItem)
	router.POST("/checkout/preview", handler.PreviewCheckout)
	router.POST("/checkout", handler.Checkout)
	router.GET("/orders", handler.ListOrders)
	router.GET("/orders/:orderID", handler.GetOrder)
	router.POST("/orders/:orderID/cancel", handler.CancelOrder)
	router.POST("/orders/:orderID/delivered", handler.ConfirmDelivery)
	router.GET("/seller/orders", handler.ListSellerOrders)
	router.GET("/seller/orders/:orderID", handler.GetSellerOrder)
	router.POST("/seller/orders/:orderID/processing", handler.StartProcessing)
	router.POST("/seller/orders/:orderID/shipment", handler.ShipOrder)
	return router
}

func serve(handler *Handler, method, target, body string) *httptest.ResponseRecorder {
	return serveAs(handler, true, method, target, body)
}

func serveAs(handler *Handler, authenticated bool, method, target, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	newRouter(handler, authenticated).ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader(body)))
	return recorder
}

func sampleCart() *orders.CartEntity {
	return &orders.CartEntity{ID: "cart-1", Items: []orders.CartItemEntity{{
		AdvertisementID: testAdID,
		SellerID:        "seller-1",
		SellerName:      "Loja",
		Title:           "Palmilha",
		Quantity:        2,
		AddedPrice:      "89.90",
		Price:           "89.90",
		Stock:           5,
		AdStatus:        orders.AdvertisementStatusPublished,
	}}}
}

type endpoint struct {
	method string
	target string
	body   string
}

var endpoints = map[string]endpoint{
	"get":    {http.MethodGet, "/cart", ""},
	"add":    {http.MethodPost, "/cart/items", `{"advertisement_id":"` + testAdID + `","quantity":1}`},
	"change": {http.MethodPatch, "/cart/items/" + testAdID, `{"quantity":2}`},
	"remove": {http.MethodDelete, "/cart/items/" + testAdID, ""},
}

func serviceReturning(cart *orders.CartEntity, err error) fakeService {
	return fakeService{
		get:    func(context.Context, string) (*orders.CartEntity, error) { return cart, err },
		add:    func(context.Context, string, string, int) (*orders.CartEntity, error) { return cart, err },
		change: func(context.Context, string, string, int) (*orders.CartEntity, error) { return cart, err },
		remove: func(context.Context, string, string) (*orders.CartEntity, error) { return cart, err },
	}
}

func TestEndpointsRequireAuthenticatedUser(t *testing.T) {
	for name, e := range endpoints {
		t.Run(name, func(t *testing.T) {
			recorder := serveAs(NewHandler(fakeService{}), false, e.method, e.target, e.body)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestEndpointsReturnTheCart(t *testing.T) {
	for name, e := range endpoints {
		t.Run(name, func(t *testing.T) {
			recorder := serve(NewHandler(serviceReturning(sampleCart(), nil)), e.method, e.target, e.body)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			var body orders.CartOutput
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Subtotal != "179.80" || len(body.Groups) != 1 || body.Groups[0].Items[0].AdvertisementID != testAdID {
				t.Errorf("body = %s", recorder.Body.String())
			}
		})
	}
}

func TestEndpointsMapErrors(t *testing.T) {
	errorsByStatus := map[int]error{
		http.StatusBadRequest:          app.ErrInvalidInput,
		http.StatusNotFound:            app.ErrAdvertisementNotFound,
		http.StatusConflict:            &app.OutOfStockError{},
		http.StatusInternalServerError: errors.New("database unavailable"),
	}

	for status, err := range errorsByStatus {
		for name, e := range endpoints {
			t.Run(name+" "+http.StatusText(status), func(t *testing.T) {
				recorder := serve(NewHandler(serviceReturning(nil, err)), e.method, e.target, e.body)

				if recorder.Code != status {
					t.Fatalf("status = %d, want %d", recorder.Code, status)
				}
				if strings.Contains(recorder.Body.String(), "database unavailable") {
					t.Fatalf("body leaks the internal error: %s", recorder.Body.String())
				}
			})
		}
	}
}

func TestCartItemErrors(t *testing.T) {
	tests := map[error]int{
		app.ErrCartItemNotFound: http.StatusNotFound,
		app.ErrOwnAdvertisement: http.StatusConflict,
	}
	for err, status := range tests {
		recorder := serve(NewHandler(serviceReturning(nil, err)), http.MethodPost, "/cart/items", `{"advertisement_id":"`+testAdID+`"}`)
		if recorder.Code != status {
			t.Errorf("%v: status = %d, want %d", err, recorder.Code, status)
		}
	}
}

func TestOutOfStockListsTheItems(t *testing.T) {
	err := &app.OutOfStockError{Items: []app.StockShortage{{AdvertisementID: testAdID, Requested: 3, Available: 1}}}

	recorder := serve(NewHandler(serviceReturning(nil, err)), http.MethodPost, "/cart/items", `{"advertisement_id":"`+testAdID+`","quantity":3}`)

	var body struct {
		Error string              `json:"error"`
		Items []app.StockShortage `json:"items"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "out of stock" || len(body.Items) != 1 || body.Items[0].Available != 1 || body.Items[0].Requested != 3 {
		t.Errorf("body = %s", recorder.Body.String())
	}
}

func TestAddCartItemDefaultsToOneUnit(t *testing.T) {
	var gotQuantity int
	var gotAdID string
	service := serviceReturning(sampleCart(), nil)
	service.add = func(_ context.Context, _ string, adID string, quantity int) (*orders.CartEntity, error) {
		gotAdID, gotQuantity = adID, quantity
		return sampleCart(), nil
	}

	recorder := serve(NewHandler(service), http.MethodPost, "/cart/items", `{"advertisement_id":"`+testAdID+`"}`)

	if recorder.Code != http.StatusOK || gotQuantity != 1 || gotAdID != testAdID {
		t.Errorf("status = %d, ad = %q, quantity = %d", recorder.Code, gotAdID, gotQuantity)
	}
}

func TestChangeCartItemQuantityPassesTheQuantity(t *testing.T) {
	var gotQuantity = -1
	service := serviceReturning(sampleCart(), nil)
	service.change = func(_ context.Context, userID string, adID string, quantity int) (*orders.CartEntity, error) {
		if userID != testBuyerID || adID != testAdID {
			t.Errorf("user = %q, ad = %q", userID, adID)
		}
		gotQuantity = quantity
		return sampleCart(), nil
	}

	serve(NewHandler(service), http.MethodPatch, "/cart/items/"+testAdID, `{"quantity":0}`)

	if gotQuantity != 0 {
		t.Errorf("quantity = %d, want 0", gotQuantity)
	}
}

func TestBadBodies(t *testing.T) {
	tests := []endpoint{
		{http.MethodPost, "/cart/items", `{`},
		{http.MethodPatch, "/cart/items/" + testAdID, `{`},
		{http.MethodPatch, "/cart/items/" + testAdID, `{}`},
	}
	for _, e := range tests {
		recorder := serve(NewHandler(serviceReturning(sampleCart(), nil)), e.method, e.target, e.body)
		if recorder.Code != http.StatusBadRequest {
			t.Errorf("%s %s %s: status = %d, want 400", e.method, e.target, e.body, recorder.Code)
		}
	}
}

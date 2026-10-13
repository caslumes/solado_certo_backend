package httpseller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/seller"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

const (
	testSellerID = "seller-1"
	testAdID     = "00000000-0000-0000-0000-000000000601"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService struct {
	Service
	list         func(context.Context, string, string) ([]*seller.AdvertisementEntity, error)
	get          func(context.Context, string, string) (*seller.AdvertisementEntity, error)
	create       func(context.Context, string, seller.AdvertisementInput) (*seller.AdvertisementEntity, error)
	edit         func(context.Context, string, string, seller.AdvertisementInput) (*seller.AdvertisementEntity, error)
	changeStatus func(context.Context, string, string, seller.StatusAction) (*seller.AdvertisementEntity, error)
	remove       func(context.Context, string, string) error
}

func (f fakeService) ListAdvertisements(ctx context.Context, sellerID string, status string) ([]*seller.AdvertisementEntity, error) {
	return f.list(ctx, sellerID, status)
}

func (f fakeService) GetAdvertisement(ctx context.Context, sellerID string, id string) (*seller.AdvertisementEntity, error) {
	return f.get(ctx, sellerID, id)
}

func (f fakeService) CreateAdvertisement(ctx context.Context, sellerID string, input seller.AdvertisementInput) (*seller.AdvertisementEntity, error) {
	return f.create(ctx, sellerID, input)
}

func (f fakeService) EditAdvertisement(ctx context.Context, sellerID string, id string, input seller.AdvertisementInput) (*seller.AdvertisementEntity, error) {
	return f.edit(ctx, sellerID, id, input)
}

func (f fakeService) ChangeAdvertisementStatus(ctx context.Context, sellerID string, id string, action seller.StatusAction) (*seller.AdvertisementEntity, error) {
	return f.changeStatus(ctx, sellerID, id, action)
}

func (f fakeService) RemoveAdvertisement(ctx context.Context, sellerID string, id string) error {
	return f.remove(ctx, sellerID, id)
}

func newRouter(handler *Handler, authenticated bool) *gin.Engine {
	router := gin.New()
	if authenticated {
		router.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), testSellerID))
		})
	}
	router.GET("/seller/ads", handler.ListAdvertisements)
	router.POST("/seller/ads", handler.CreateAdvertisement)
	router.GET("/seller/ads/:advertisementID", handler.GetAdvertisement)
	router.PATCH("/seller/ads/:advertisementID", handler.EditAdvertisement)
	router.DELETE("/seller/ads/:advertisementID", handler.RemoveAdvertisement)
	router.POST("/seller/ads/:advertisementID/publish", handler.PublishAdvertisement)
	router.POST("/seller/ads/:advertisementID/pause", handler.PauseAdvertisement)
	router.POST("/seller/ads/:advertisementID/reactivate", handler.ReactivateAdvertisement)
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

func sampleAd(status seller.AdvertisementStatus, stock int) *seller.AdvertisementEntity {
	return &seller.AdvertisementEntity{ID: testAdID, Title: "Palmilha", Price: "89.90", Stock: stock, Status: status}
}

type endpoint struct {
	method string
	target string
	body   string
}

var endpoints = map[string]endpoint{
	"list":       {http.MethodGet, "/seller/ads", ""},
	"create":     {http.MethodPost, "/seller/ads", `{"title":"Palmilha"}`},
	"get":        {http.MethodGet, "/seller/ads/" + testAdID, ""},
	"edit":       {http.MethodPatch, "/seller/ads/" + testAdID, `{"title":"Palmilha"}`},
	"remove":     {http.MethodDelete, "/seller/ads/" + testAdID, ""},
	"publish":    {http.MethodPost, "/seller/ads/" + testAdID + "/publish", ""},
	"pause":      {http.MethodPost, "/seller/ads/" + testAdID + "/pause", ""},
	"reactivate": {http.MethodPost, "/seller/ads/" + testAdID + "/reactivate", ""},
}

func serviceReturning(err error) fakeService {
	return fakeService{
		list: func(context.Context, string, string) ([]*seller.AdvertisementEntity, error) { return nil, err },
		get:  func(context.Context, string, string) (*seller.AdvertisementEntity, error) { return nil, err },
		create: func(context.Context, string, seller.AdvertisementInput) (*seller.AdvertisementEntity, error) {
			return nil, err
		},
		edit: func(context.Context, string, string, seller.AdvertisementInput) (*seller.AdvertisementEntity, error) {
			return nil, err
		},
		changeStatus: func(context.Context, string, string, seller.StatusAction) (*seller.AdvertisementEntity, error) {
			return nil, err
		},
		remove: func(context.Context, string, string) error { return err },
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

func TestEndpointsMapErrors(t *testing.T) {
	errorsByStatus := map[int]error{
		http.StatusBadRequest:          app.ErrInvalidInput,
		http.StatusNotFound:            app.ErrAdvertisementNotFound,
		http.StatusForbidden:           app.ErrForbidden,
		http.StatusConflict:            app.ErrInvalidStatusTransition,
		http.StatusUnprocessableEntity: &app.MissingFieldsError{Fields: []string{"images"}},
		http.StatusInternalServerError: errors.New("database unavailable"),
	}

	for status, err := range errorsByStatus {
		for name, e := range endpoints {
			t.Run(name+" "+http.StatusText(status), func(t *testing.T) {
				recorder := serve(NewHandler(serviceReturning(err)), e.method, e.target, e.body)

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

func TestMissingFieldsAreListed(t *testing.T) {
	handler := NewHandler(serviceReturning(&app.MissingFieldsError{Fields: []string{"material", "images"}}))

	recorder := serve(handler, http.MethodPost, "/seller/ads/"+testAdID+"/publish", "")

	var body struct {
		Error  string   `json:"error"`
		Fields []string `json:"fields"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error != "missing required fields" || len(body.Fields) != 2 || body.Fields[0] != "material" || body.Fields[1] != "images" {
		t.Fatalf("body = %+v", body)
	}
}

func TestBodyEndpointsRejectMalformedJSON(t *testing.T) {
	for _, name := range []string{"create", "edit"} {
		e := endpoints[name]
		t.Run(name, func(t *testing.T) {
			recorder := serve(NewHandler(fakeService{}), e.method, e.target, `{"title":`)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestListPassesStatusAndReturnsItems(t *testing.T) {
	var receivedSeller, receivedStatus string
	handler := NewHandler(fakeService{
		list: func(_ context.Context, sellerID string, status string) ([]*seller.AdvertisementEntity, error) {
			receivedSeller, receivedStatus = sellerID, status
			return []*seller.AdvertisementEntity{sampleAd(seller.AdvertisementStatusPublished, 0)}, nil
		},
	})

	recorder := serve(handler, http.MethodGet, "/seller/ads?status=published", "")

	if recorder.Code != http.StatusOK || receivedSeller != testSellerID || receivedStatus != "published" {
		t.Fatalf("status = %d, seller = %q, filter = %q", recorder.Code, receivedSeller, receivedStatus)
	}
	var body seller.AdvertisementsOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 || !body.Items[0].SoldOut {
		t.Fatalf("items = %+v", body.Items)
	}
}

func TestListReturnsEmptyItems(t *testing.T) {
	handler := NewHandler(fakeService{
		list: func(context.Context, string, string) ([]*seller.AdvertisementEntity, error) { return nil, nil },
	})

	if body := serve(handler, http.MethodGet, "/seller/ads", "").Body.String(); body != `{"items":[]}` {
		t.Fatalf("body = %s", body)
	}
}

func TestCreateDecodesRequestAndReturnsCreated(t *testing.T) {
	var received seller.AdvertisementInput
	handler := NewHandler(fakeService{
		create: func(_ context.Context, _ string, input seller.AdvertisementInput) (*seller.AdvertisementEntity, error) {
			received = input
			return sampleAd(seller.AdvertisementStatusDraft, 1), nil
		},
	})
	body := `{"category_id":"c1","title":"Palmilha","price":"89.90","stock":3,"side":"both","purposes":["pain_relief"],` +
		`"condition_ids":[],"shipping_options":[{"id":"o1","name":"PAC","carrier":"Correios","price":"19.90","delivery_days":7}]}`

	recorder := serve(handler, http.MethodPost, "/seller/ads", body)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusCreated)
	}
	if *received.CategoryID != "c1" || *received.Price != "89.90" || *received.Stock != 3 || *received.Side != "both" {
		t.Fatalf("input = %+v", received)
	}
	if received.Description != nil || received.Size != nil {
		t.Fatal("absent fields must stay nil")
	}
	if (*received.PurposeCodes)[0] != "pain_relief" || received.ConditionIDs == nil || len(*received.ConditionIDs) != 0 {
		t.Fatalf("purposes = %v, conditions = %v", received.PurposeCodes, received.ConditionIDs)
	}
	option := (*received.ShippingOptions)[0]
	if *option.ID != "o1" || option.Name != "PAC" || *option.Carrier != "Correios" || option.Price != "19.90" || option.DeliveryDays != 7 {
		t.Fatalf("shipping option = %+v", option)
	}
}

func TestEditPassesID(t *testing.T) {
	var receivedID string
	handler := NewHandler(fakeService{
		edit: func(_ context.Context, _ string, id string, _ seller.AdvertisementInput) (*seller.AdvertisementEntity, error) {
			receivedID = id
			return sampleAd(seller.AdvertisementStatusDraft, 1), nil
		},
	})

	recorder := serve(handler, http.MethodPatch, "/seller/ads/"+testAdID, `{"stock":0}`)

	if recorder.Code != http.StatusOK || receivedID != testAdID {
		t.Fatalf("status = %d, id = %q", recorder.Code, receivedID)
	}
}

func TestStatusEndpointsPassTheirAction(t *testing.T) {
	actions := map[string]seller.StatusAction{
		"publish":    seller.StatusActionPublish,
		"pause":      seller.StatusActionPause,
		"reactivate": seller.StatusActionReactivate,
	}

	for name, want := range actions {
		t.Run(name, func(t *testing.T) {
			var received seller.StatusAction
			handler := NewHandler(fakeService{
				changeStatus: func(_ context.Context, _ string, _ string, action seller.StatusAction) (*seller.AdvertisementEntity, error) {
					received = action
					return sampleAd(seller.AdvertisementStatusPublished, 1), nil
				},
			})

			recorder := serve(handler, http.MethodPost, "/seller/ads/"+testAdID+"/"+name, "")

			if recorder.Code != http.StatusOK || received != want {
				t.Fatalf("status = %d, action = %q", recorder.Code, received)
			}
		})
	}
}

func TestRemoveReturnsNoContent(t *testing.T) {
	handler := NewHandler(fakeService{remove: func(context.Context, string, string) error { return nil }})

	if recorder := serve(handler, http.MethodDelete, "/seller/ads/"+testAdID, ""); recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

package httpcustomorders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/customorders"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

const (
	testBuyerID   = "buyer-1"
	testSellerID  = "seller-1"
	testRequestID = "00000000-0000-0000-0000-000000000901"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// fakeService records the last call and answers with err, or with a request in status.
type fakeService struct {
	call   string
	userID string
	id     string
	input  any
	status customorders.Status
	err    error
}

func (f *fakeService) answer(call string, userID string, id string, input any) (*customorders.RequestDetailEntity, error) {
	f.call, f.userID, f.id, f.input = call, userID, id, input
	if f.err != nil {
		return nil, f.err
	}
	return &customorders.RequestDetailEntity{RequestEntity: customorders.RequestEntity{
		ID: testRequestID, BuyerID: testBuyerID, SellerID: testSellerID, Status: f.status, AdvertisementTitle: "Palmilha sob medida",
	}}, nil
}

func (f *fakeService) CreateRequest(_ context.Context, buyerID string, input customorders.RequestInput) (*customorders.RequestDetailEntity, error) {
	return f.answer("create", buyerID, "", input)
}

func (f *fakeService) ListRequests(_ context.Context, buyerID string, status customorders.Status, page uint, limit uint) (*customorders.RequestPage, error) {
	f.call, f.userID, f.input = "list", buyerID, fmt.Sprintf("%s %d %d", status, page, limit)
	return &customorders.RequestPage{CurrentPage: page, TotalPages: 1, TotalItems: 0}, f.err
}

func (f *fakeService) ListSellerRequests(_ context.Context, sellerID string, status customorders.Status, page uint, limit uint) (*customorders.RequestPage, error) {
	f.call, f.userID, f.input = "seller list", sellerID, fmt.Sprintf("%s %d %d", status, page, limit)
	return &customorders.RequestPage{CurrentPage: page, TotalPages: 1}, f.err
}

func (f *fakeService) GetRequest(_ context.Context, buyerID string, id string) (*customorders.RequestDetailEntity, error) {
	return f.answer("get", buyerID, id, nil)
}

func (f *fakeService) GetSellerRequest(_ context.Context, sellerID string, id string) (*customorders.RequestDetailEntity, error) {
	return f.answer("seller get", sellerID, id, nil)
}

func (f *fakeService) ComplementRequest(_ context.Context, buyerID string, id string, input customorders.ComplementInput) (*customorders.RequestDetailEntity, error) {
	return f.answer("complement", buyerID, id, input)
}

func (f *fakeService) CancelRequest(_ context.Context, buyerID string, id string, input customorders.CancelInput) (*customorders.RequestDetailEntity, error) {
	return f.answer("cancel", buyerID, id, input)
}

func (f *fakeService) ReviewRequest(_ context.Context, sellerID string, id string) (*customorders.RequestDetailEntity, error) {
	return f.answer("review", sellerID, id, nil)
}

func (f *fakeService) AcceptRequest(_ context.Context, sellerID string, id string, input customorders.AcceptInput) (*customorders.RequestDetailEntity, error) {
	return f.answer("accept", sellerID, id, input)
}

func (f *fakeService) RejectRequest(_ context.Context, sellerID string, id string, input customorders.ReasonInput) (*customorders.RequestDetailEntity, error) {
	return f.answer("reject", sellerID, id, input)
}

func (f *fakeService) RequestInfo(_ context.Context, sellerID string, id string, input customorders.ReasonInput) (*customorders.RequestDetailEntity, error) {
	return f.answer("request info", sellerID, id, input)
}

func serve(service *fakeService, userID string, method string, target string, body string) *httptest.ResponseRecorder {
	handler := NewHandler(service)
	router := gin.New()
	group := router.Group("", func(c *gin.Context) {
		if userID != "" {
			c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), userID))
		}
	})
	group.GET("/custom-orders", handler.ListRequests)
	group.POST("/custom-orders", handler.CreateRequest)
	group.GET("/custom-orders/:requestID", handler.GetRequest)
	group.POST("/custom-orders/:requestID/complement", handler.ComplementRequest)
	group.POST("/custom-orders/:requestID/cancel", handler.CancelRequest)
	group.GET("/seller/custom-orders", handler.ListSellerRequests)
	group.GET("/seller/custom-orders/:requestID", handler.GetSellerRequest)
	group.POST("/seller/custom-orders/:requestID/review", handler.ReviewRequest)
	group.POST("/seller/custom-orders/:requestID/accept", handler.AcceptRequest)
	group.POST("/seller/custom-orders/:requestID/reject", handler.RejectRequest)
	group.POST("/seller/custom-orders/:requestID/request-info", handler.RequestInfo)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func decode(t *testing.T, recorder *httptest.ResponseRecorder) customorders.RequestDetailOutput {
	t.Helper()
	var body customorders.RequestDetailOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.String(), err)
	}
	return body
}

func TestCreateRequest(t *testing.T) {
	service := &fakeService{status: customorders.StatusRequested}

	recorder := serve(service, testBuyerID, http.MethodPost, "/custom-orders",
		`{"advertisement_id":"ad-1","measurements":{"left":{"length_cm":25,"width_cm":9.5},"right":{"length_cm":25.2,"width_cm":9.6},"arch_type":"flat","shoe_size":39},"technical_notes":"Esporão"}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	input := service.input.(customorders.RequestInput)
	if service.userID != testBuyerID || input.AdvertisementID != "ad-1" || input.Measurements.Right.LengthCm != 25.2 ||
		input.Measurements.ArchType != customorders.ArchTypeFlat || input.Measurements.ShoeSize != 39 || *input.TechnicalNotes != "Esporão" {
		t.Errorf("user = %q, input = %+v", service.userID, input)
	}
	body := decode(t, recorder)
	if body.ID != testRequestID || body.Status != "requested" || len(body.Actions) != 1 || body.Actions[0] != "cancel" {
		t.Errorf("body = %s", recorder.Body.String())
	}
}

func TestSellerActions(t *testing.T) {
	for _, test := range []struct {
		path  string
		body  string
		call  string
		input any
	}{
		{"review", "", "review", nil},
		{"accept", `{"quoted_price":"189.90"}`, "accept", customorders.AcceptInput{QuotedPrice: "189.90"}},
		{"reject", `{"reason":"Sem material"}`, "reject", customorders.ReasonInput{Reason: "Sem material"}},
		{"request-info", `{"reason":"Qual o arco?"}`, "request info", customorders.ReasonInput{Reason: "Qual o arco?"}},
	} {
		t.Run(test.path, func(t *testing.T) {
			service := &fakeService{status: customorders.StatusUnderReview}
			recorder := serve(service, testSellerID, http.MethodPost, "/seller/custom-orders/"+testRequestID+"/"+test.path, test.body)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if service.call != test.call || service.userID != testSellerID || service.id != testRequestID || service.input != test.input {
				t.Errorf("call = %q, user = %q, id = %q, input = %+v", service.call, service.userID, service.id, service.input)
			}
			if body := decode(t, recorder); len(body.Actions) != 3 {
				t.Errorf("seller actions = %v", body.Actions)
			}
		})
	}
}

func TestBuyerActions(t *testing.T) {
	reason := "Mudei de ideia"
	for _, test := range []struct {
		path string
		body string
		call string
	}{
		{"complement", `{"message":"Arco alto","measurements":{"arch_type":"high"}}`, "complement"},
		{"cancel", `{"reason":"Mudei de ideia"}`, "cancel"},
		{"cancel", "", "cancel"},
	} {
		t.Run(test.path+" "+test.body, func(t *testing.T) {
			service := &fakeService{status: customorders.StatusNeedsInfo}
			recorder := serve(service, testBuyerID, http.MethodPost, "/custom-orders/"+testRequestID+"/"+test.path, test.body)

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
			if service.call != test.call || service.userID != testBuyerID || service.id != testRequestID {
				t.Errorf("call = %q, user = %q, id = %q", service.call, service.userID, service.id)
			}
			switch input := service.input.(type) {
			case customorders.ComplementInput:
				if input.Message != "Arco alto" || input.Measurements.ArchType != customorders.ArchTypeHigh {
					t.Errorf("input = %+v", input)
				}
			case customorders.CancelInput:
				if test.body != "" && (input.Reason == nil || *input.Reason != reason) || test.body == "" && input.Reason != nil {
					t.Errorf("input = %+v", input)
				}
			}
		})
	}
}

func TestGetAndListRequests(t *testing.T) {
	service := &fakeService{status: customorders.StatusApproved}

	if recorder := serve(service, testBuyerID, http.MethodGet, "/custom-orders/"+testRequestID, ""); recorder.Code != http.StatusOK || service.call != "get" {
		t.Errorf("get: status = %d, call = %q", recorder.Code, service.call)
	}
	if recorder := serve(service, testSellerID, http.MethodGet, "/seller/custom-orders/"+testRequestID, ""); recorder.Code != http.StatusOK || service.call != "seller get" {
		t.Errorf("seller get: status = %d, call = %q", recorder.Code, service.call)
	}
	if recorder := serve(service, testBuyerID, http.MethodGet, "/custom-orders?status=approved&page=2&limit=5", ""); recorder.Code != http.StatusOK || service.input != "approved 2 5" {
		t.Errorf("list: status = %d, input = %v", recorder.Code, service.input)
	}
	if recorder := serve(service, testSellerID, http.MethodGet, "/seller/custom-orders", ""); recorder.Code != http.StatusOK || service.input != " 1 10" {
		t.Errorf("seller list: status = %d, input = %v", recorder.Code, service.input)
	}
	if recorder := serve(service, testBuyerID, http.MethodGet, "/custom-orders?limit=500", ""); recorder.Code != http.StatusBadRequest {
		t.Errorf("bad query: status = %d", recorder.Code)
	}
}

func TestErrors(t *testing.T) {
	for name, test := range map[string]struct {
		err  error
		want int
	}{
		"missing fields":  {&app.MissingFieldsError{Fields: []string{"reason"}}, http.StatusUnprocessableEntity},
		"invalid input":   {fmt.Errorf("%w: bad", app.ErrInvalidInput), http.StatusBadRequest},
		"no consent":      {app.ErrConsentRequired, http.StatusForbidden},
		"no ad":           {app.ErrAdvertisementNotFound, http.StatusNotFound},
		"no request":      {app.ErrCustomOrderNotFound, http.StatusNotFound},
		"not custom made": {app.ErrCustomMadeNotAllowed, http.StatusConflict},
		"own ad":          {app.ErrOwnAdvertisement, http.StatusConflict},
		"wrong status":    {app.ErrInvalidStatusTransition, http.StatusConflict},
		"unexpected":      {errors.New("boom"), http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			recorder := serve(&fakeService{err: test.err}, testSellerID, http.MethodPost, "/seller/custom-orders/"+testRequestID+"/reject", `{"reason":"x"}`)
			if recorder.Code != test.want {
				t.Errorf("status = %d, want %d, body = %s", recorder.Code, test.want, recorder.Body.String())
			}
		})
	}

	if recorder := serve(&fakeService{}, testBuyerID, http.MethodPost, "/custom-orders", "{"); recorder.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: status = %d", recorder.Code)
	}
	if recorder := serve(&fakeService{}, "", http.MethodGet, "/custom-orders", ""); recorder.Code != http.StatusUnauthorized {
		t.Errorf("no user: status = %d", recorder.Code)
	}
}

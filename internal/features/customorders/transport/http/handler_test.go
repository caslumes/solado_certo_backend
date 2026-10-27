package httpcustomorders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/customorders"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
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

type fakeService struct {
	call    string
	userID  string
	id      string
	input   any
	status  customorders.Status
	err     error
	key     string
	created bool
	content []byte
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

func (f *fakeService) Checkout(_ context.Context, buyerID string, id string, key string, input customorders.CheckoutInput) (*customorders.RequestDetailEntity, bool, error) {
	f.key = key
	request, err := f.answer("checkout", buyerID, id, input)
	return request, f.created, err
}

func (f *fakeService) ReportRequest(_ context.Context, sellerID string, id string, instructions string, content io.Reader) (*customorders.RequestDetailEntity, error) {
	f.content, _ = io.ReadAll(content)
	return f.answer("report", sellerID, id, instructions)
}

func (f *fakeService) SendPhoto(_ context.Context, buyerID string, id string, content io.Reader) (*customorders.RequestDetailEntity, error) {
	f.content, _ = io.ReadAll(content)
	return f.answer("send photo", buyerID, id, nil)
}

func (f *fakeService) OpenPhoto(_ context.Context, buyerID string, id string) (*customorders.PhotoFile, error) {
	return f.photo("open photo", buyerID, id)
}

func (f *fakeService) OpenSellerPhoto(_ context.Context, sellerID string, id string) (*customorders.PhotoFile, error) {
	return f.photo("seller open photo", sellerID, id)
}

func (f *fakeService) photo(call string, userID string, id string) (*customorders.PhotoFile, error) {
	f.call, f.userID, f.id = call, userID, id
	if f.err != nil {
		return nil, f.err
	}
	content := []byte("png bytes")
	return &customorders.PhotoFile{
		File:        &storage.File{ReadCloser: io.NopCloser(bytes.NewReader(content)), Size: int64(len(content))},
		ContentType: "image/png",
		Name:        "foot-photo.png",
	}, nil
}

func serve(service *fakeService, userID string, method string, target string, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	return serveRequest(service, userID, request)
}

func serveMultipart(t *testing.T, service *fakeService, userID string, method string, target string, file []byte, fields map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if file != nil {
		part, err := writer.CreateFormFile("file", "upload")
		if err != nil {
			t.Fatal(err)
		}
		part.Write(file)
	}
	writer.Close()
	request := httptest.NewRequest(method, target, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return serveRequest(service, userID, request)
}

func serveRequest(service *fakeService, userID string, request *http.Request) *httptest.ResponseRecorder {
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
	group.POST("/custom-orders/:requestID/checkout", handler.Checkout)
	group.PUT("/custom-orders/:requestID/photo", handler.SendPhoto)
	group.GET("/custom-orders/:requestID/photo", handler.OpenPhoto)
	group.POST("/seller/custom-orders/:requestID/report", handler.ReportRequest)
	group.GET("/seller/custom-orders/:requestID/photo", handler.OpenSellerPhoto)

	recorder := httptest.NewRecorder()
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
	if body.ID != testRequestID || body.Status != "requested" || strings.Join(body.Actions, ",") != "cancel,send_photo" {
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
		"prices changed":  {app.ErrPricesChanged, http.StatusConflict},
		"no address":      {app.ErrAddressNotFound, http.StatusNotFound},
		"no shipping":     {&app.ShippingRequiredError{AdvertisementIDs: []string{"ad-1"}}, http.StatusUnprocessableEntity},
		"no photo":        {app.ErrFootPhotoNotFound, http.StatusNotFound},
		"unreadable":      {fmt.Errorf("%w: the PDF is incomplete", app.ErrUnreadableDocument), http.StatusUnprocessableEntity},
		"too large":       {storage.ErrTooLarge, http.StatusRequestEntityTooLarge},
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

func TestCheckout(t *testing.T) {
	for _, test := range []struct {
		created bool
		want    int
	}{{true, http.StatusCreated}, {false, http.StatusOK}} {
		service := &fakeService{status: customorders.StatusApproved, created: test.created}
		request := httptest.NewRequest(http.MethodPost, "/custom-orders/"+testRequestID+"/checkout", strings.NewReader(`{"address_id":"a-1","shipping_option_id":"s-1"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Idempotency-Key", "key-1")

		recorder := serveRequest(service, testBuyerID, request)

		if recorder.Code != test.want {
			t.Fatalf("created = %v: status = %d, want %d, body = %s", test.created, recorder.Code, test.want, recorder.Body.String())
		}
		input := service.input.(customorders.CheckoutInput)
		if service.call != "checkout" || service.userID != testBuyerID || service.id != testRequestID || service.key != "key-1" ||
			input.AddressID != "a-1" || input.ShippingOptionID != "s-1" {
			t.Errorf("call = %q, key = %q, input = %+v", service.call, service.key, input)
		}
	}
	if recorder := serve(&fakeService{}, testBuyerID, http.MethodPost, "/custom-orders/"+testRequestID+"/checkout", "{"); recorder.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: status = %d", recorder.Code)
	}
}

func TestReportRequest(t *testing.T) {
	service := &fakeService{status: customorders.StatusCompleted}

	recorder := serveMultipart(t, service, testSellerID, http.MethodPost, "/seller/custom-orders/"+testRequestID+"/report",
		[]byte("%PDF report"), map[string]string{"adaptation_instructions": "Use 2 horas por dia."})

	if recorder.Code != http.StatusOK || decode(t, recorder).Status != string(customorders.StatusCompleted) {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if service.call != "report" || service.userID != testSellerID || service.input != "Use 2 horas por dia." || string(service.content) != "%PDF report" {
		t.Errorf("call = %q, input = %v, content = %q", service.call, service.input, service.content)
	}

	if recorder := serveMultipart(t, service, testSellerID, http.MethodPost, "/seller/custom-orders/"+testRequestID+"/report", nil, nil); recorder.Code != http.StatusBadRequest {
		t.Errorf("no file: status = %d", recorder.Code)
	}
	service.err = storage.ErrUnsupportedType
	recorder = serveMultipart(t, service, testSellerID, http.MethodPost, "/seller/custom-orders/"+testRequestID+"/report", []byte("x"), nil)
	if recorder.Code != http.StatusUnsupportedMediaType || !strings.Contains(recorder.Body.String(), "PDF, JPEG or PNG") {
		t.Errorf("wrong type: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestFootPhoto(t *testing.T) {
	service := &fakeService{status: customorders.StatusRequested}

	recorder := serveMultipart(t, service, testBuyerID, http.MethodPut, "/custom-orders/"+testRequestID+"/photo", []byte("png bytes"), nil)

	if recorder.Code != http.StatusOK || service.call != "send photo" || string(service.content) != "png bytes" {
		t.Fatalf("send: status = %d, call = %q, body = %s", recorder.Code, service.call, recorder.Body.String())
	}
	service.err = storage.ErrUnsupportedType
	recorder = serveMultipart(t, service, testBuyerID, http.MethodPut, "/custom-orders/"+testRequestID+"/photo", []byte("%PDF"), nil)
	if recorder.Code != http.StatusUnsupportedMediaType || !strings.Contains(recorder.Body.String(), "JPEG or PNG") {
		t.Errorf("wrong type: status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	service.err = nil

	for _, open := range []struct{ userID, path, call string }{
		{testBuyerID, "/custom-orders/", "open photo"},
		{testSellerID, "/seller/custom-orders/", "seller open photo"},
	} {
		recorder := serve(service, open.userID, http.MethodGet, open.path+testRequestID+"/photo", "")
		if recorder.Code != http.StatusOK || service.call != open.call || recorder.Body.String() != "png bytes" {
			t.Errorf("%s: status = %d, call = %q, body = %q", open.call, recorder.Code, service.call, recorder.Body.String())
		}
		if recorder.Header().Get("Cache-Control") != "private, no-store" || recorder.Header().Get("Content-Type") != "image/png" {
			t.Errorf("%s: headers = %v", open.call, recorder.Header())
		}
	}
	service.err = app.ErrFootPhotoNotFound
	if recorder := serve(service, testBuyerID, http.MethodGet, "/custom-orders/"+testRequestID+"/photo", ""); recorder.Code != http.StatusNotFound {
		t.Errorf("no photo: status = %d", recorder.Code)
	}
}

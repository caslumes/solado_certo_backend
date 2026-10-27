package httppayments

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	authmiddleware "github.com/caslumes/solado_certo_backend/internal/middleware/auth"
	"github.com/gin-gonic/gin"
)

const (
	testBuyerID  = "buyer-1"
	testOrderID  = "00000000-0000-0000-0000-000000000801"
	webhookToken = "webhook-token-for-tests"
)

func init() {
	gin.SetMode(gin.TestMode)
}

type fakeService struct {
	create func(ctx context.Context, userID, orderID, key string, input payments.CreatePaymentInput) (*payments.PaymentEntity, bool, error)
	list   func(ctx context.Context, userID, orderID string) ([]*payments.PaymentEntity, error)

	mu      sync.Mutex
	applied []string
	apply   error
}

func (f *fakeService) CreatePayment(ctx context.Context, userID, orderID, key string, input payments.CreatePaymentInput) (*payments.PaymentEntity, bool, error) {
	return f.create(ctx, userID, orderID, key, input)
}

func (f *fakeService) ListPayments(ctx context.Context, userID, orderID string) ([]*payments.PaymentEntity, error) {
	return f.list(ctx, userID, orderID)
}

func (f *fakeService) ApplyPaymentStatus(_ context.Context, transactionID string, source string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.applied = append(f.applied, source+":"+transactionID)
	return f.apply
}

func newRouter(handler *Handler) *gin.Engine {
	router := gin.New()
	router.POST("/webhooks/asaas", handler.AsaasWebhook)
	authenticated := router.Group("", func(c *gin.Context) {
		c.Request = c.Request.WithContext(authmiddleware.WithUserID(c.Request.Context(), testBuyerID))
	})
	authenticated.POST("/orders/:orderID/payments", handler.CreatePayment)
	authenticated.GET("/orders/:orderID/payments", handler.ListPayments)
	return router
}

func serve(handler *Handler, request *http.Request) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	newRouter(handler).ServeHTTP(recorder, request)
	return recorder
}

func pixPayment() *payments.PaymentEntity {
	due := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	transaction := "pay_1"
	return &payments.PaymentEntity{
		ID: "payment-1", OrderID: testOrderID, Gateway: "fake", TransactionID: &transaction,
		Method: payments.MethodPix, Status: payments.StatusPending, Amount: "115.00", DueDate: &due,
		Details:   payments.PaymentDetails{PixPayload: "00020126pix", InvoiceURL: "https://example.com/i/1", GatewayStatus: "PENDING"},
		CreatedAt: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC),
	}
}

func createRequest(body string, key string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/orders/"+testOrderID+"/payments", strings.NewReader(body))
	request.RemoteAddr = "203.0.113.7:51000"
	if key != "" {
		request.Header.Set("Idempotency-Key", key)
	}
	return request
}

func TestCreatePaymentPassesTheRequestOn(t *testing.T) {
	var gotUser, gotOrder, gotKey string
	var gotInput payments.CreatePaymentInput
	handler := NewHandler(&fakeService{
		create: func(_ context.Context, userID, orderID, key string, input payments.CreatePaymentInput) (*payments.PaymentEntity, bool, error) {
			gotUser, gotOrder, gotKey, gotInput = userID, orderID, key, input
			return pixPayment(), true, nil
		},
	}, webhookToken)

	body := `{"method":"credit_card","tax_id":"529.982.247-25","card":{"holder_name":"MARIA S","number":"4111111111111111","expiry_month":"12","expiry_year":"2030","cvv":"123"},"remote_ip":"1.1.1.1"}`
	recorder := serve(handler, createRequest(body, "key-1"))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if gotUser != testBuyerID || gotOrder != testOrderID || gotKey != "key-1" {
		t.Errorf("user, order, key = %q, %q, %q", gotUser, gotOrder, gotKey)
	}
	want := payments.CardInput{HolderName: "MARIA S", Number: "4111111111111111", ExpiryMonth: "12", ExpiryYear: "2030", CVV: "123"}
	if gotInput.Method != payments.MethodCreditCard || gotInput.TaxID != "529.982.247-25" || gotInput.Card == nil || *gotInput.Card != want {
		t.Errorf("input = %+v", gotInput)
	}
	if gotInput.RemoteIP != "203.0.113.7" {
		t.Errorf("RemoteIP = %q, want the connection's address", gotInput.RemoteIP)
	}
}

func TestCreatePaymentOutput(t *testing.T) {
	handler := NewHandler(&fakeService{
		create: func(context.Context, string, string, string, payments.CreatePaymentInput) (*payments.PaymentEntity, bool, error) {
			return pixPayment(), false, nil
		},
	}, webhookToken)

	recorder := serve(handler, createRequest(`{"method":"pix"}`, "key-1"))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a repeated key", recorder.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	pix, _ := body["pix"].(map[string]any)
	if body["status"] != "pending" || body["amount"] != "115.00" || body["due_date"] != "2026-10-18" || pix["payload"] != "00020126pix" {
		t.Errorf("body = %s", recorder.Body.String())
	}
	if body["card"] != nil || body["boleto_url"] != nil || body["failure_message"] != nil || body["invoice_url"] != "https://example.com/i/1" {
		t.Errorf("body = %s, want only the Pix fields", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "gateway_status") || strings.Contains(recorder.Body.String(), "pay_1") {
		t.Errorf("body = %s, want no gateway internals", recorder.Body.String())
	}
}

func TestCreatePaymentRefusedCardOutput(t *testing.T) {
	reason := payments.FailureCardRefused
	handler := NewHandler(&fakeService{
		create: func(context.Context, string, string, string, payments.CreatePaymentInput) (*payments.PaymentEntity, bool, error) {
			return &payments.PaymentEntity{
				ID: "payment-2", OrderID: testOrderID, Method: payments.MethodCreditCard, Status: payments.StatusFailed,
				Amount: "120.00", FailureReason: &reason,
				Details: payments.PaymentDetails{GatewayError: "Transação não autorizada.", CardLast4: "0002", CardBrand: "VISA"},
			}, true, nil
		},
	}, webhookToken)

	recorder := serve(handler, createRequest(`{"method":"credit_card"}`, "key-2"))

	body := recorder.Body.String()
	if recorder.Code != http.StatusCreated || !strings.Contains(body, `"failure_reason":"card_refused"`) ||
		!strings.Contains(body, `"failure_message":"Transação não autorizada."`) || !strings.Contains(body, `"card":{"last4":"0002","brand":"VISA"}`) {
		t.Errorf("status = %d, body = %s", recorder.Code, body)
	}
}

func TestCreatePaymentRejectsInvalidJSON(t *testing.T) {
	handler := NewHandler(&fakeService{}, webhookToken)
	if recorder := serve(handler, createRequest(`{"method":`, "key-1")); recorder.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", recorder.Code)
	}
}

func TestPaymentErrors(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{app.ErrInvalidInput, http.StatusBadRequest},
		{app.ErrOrderNotFound, http.StatusNotFound},
		{app.ErrInvalidStatusTransition, http.StatusConflict},
		{app.ErrOrderExpired, http.StatusConflict},
		{app.ErrPaymentInProgress, http.StatusConflict},
		{&app.PrescriptionRequiredError{ItemIDs: []string{"item-1"}}, http.StatusConflict},
		{app.ErrTaxIDInUse, http.StatusConflict},
		{app.ErrTaxIDRequired, http.StatusUnprocessableEntity},
		{app.ErrIdempotencyKeyReused, http.StatusUnprocessableEntity},
		{app.ErrGatewayUnavailable, http.StatusServiceUnavailable},
		{errors.New("database down"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			handler := NewHandler(&fakeService{
				create: func(context.Context, string, string, string, payments.CreatePaymentInput) (*payments.PaymentEntity, bool, error) {
					return nil, false, tt.err
				},
				list: func(context.Context, string, string) ([]*payments.PaymentEntity, error) { return nil, tt.err },
			}, webhookToken)

			created := serve(handler, createRequest(`{"method":"pix"}`, "key-1"))
			listed := serve(handler, httptest.NewRequest(http.MethodGet, "/orders/"+testOrderID+"/payments", nil))

			if created.Code != tt.want || listed.Code != tt.want {
				t.Errorf("status = %d (create), %d (list), want %d", created.Code, listed.Code, tt.want)
			}
			if tt.want == http.StatusInternalServerError && strings.Contains(created.Body.String(), "database down") {
				t.Errorf("body = %s, want the internal error hidden", created.Body.String())
			}
		})
	}
}

func TestListPayments(t *testing.T) {
	handler := NewHandler(&fakeService{
		list: func(_ context.Context, userID, orderID string) ([]*payments.PaymentEntity, error) {
			if userID != testBuyerID || orderID != testOrderID {
				t.Errorf("user, order = %q, %q", userID, orderID)
			}
			return []*payments.PaymentEntity{pixPayment()}, nil
		},
	}, webhookToken)

	recorder := serve(handler, httptest.NewRequest(http.MethodGet, "/orders/"+testOrderID+"/payments", nil))

	var body []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || recorder.Code != http.StatusOK || len(body) != 1 || body[0]["id"] != "payment-1" {
		t.Errorf("status = %d, body = %s, err = %v", recorder.Code, recorder.Body.String(), err)
	}
}

func TestListPaymentsEmpty(t *testing.T) {
	handler := NewHandler(&fakeService{
		list: func(context.Context, string, string) ([]*payments.PaymentEntity, error) { return nil, nil },
	}, webhookToken)

	recorder := serve(handler, httptest.NewRequest(http.MethodGet, "/orders/"+testOrderID+"/payments", nil))

	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != "[]" {
		t.Errorf("status = %d, body = %s, want an empty list", recorder.Code, recorder.Body.String())
	}
}

func webhookRequest(token string, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/webhooks/asaas", strings.NewReader(body))
	if token != "" {
		request.Header.Set("asaas-access-token", token)
	}
	return request
}

const receivedEvent = `{"id":"evt_1","event":"PAYMENT_RECEIVED","payment":{"object":"payment","id":"pay_1","status":"RECEIVED"}}`

func TestAsaasWebhookAppliesTheCharge(t *testing.T) {
	service := &fakeService{}
	handler := NewHandler(service, webhookToken)

	recorder := serve(handler, webhookRequest(webhookToken, receivedEvent))
	handler.Wait()

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if len(service.applied) != 1 || service.applied[0] != payments.SourceWebhook+":pay_1" {
		t.Errorf("applied = %v, want pay_1 from the webhook", service.applied)
	}
}

func TestAsaasWebhookAnswersOKWhenApplyingFails(t *testing.T) {
	for _, err := range []error{app.ErrPaymentNotFound, app.ErrGatewayUnavailable} {
		service := &fakeService{apply: err}
		handler := NewHandler(service, webhookToken)

		recorder := serve(handler, webhookRequest(webhookToken, receivedEvent))
		handler.Wait()

		if recorder.Code != http.StatusOK || len(service.applied) != 1 {
			t.Errorf("%v: status = %d, applied = %v, want 200 after trying once", err, recorder.Code, service.applied)
		}
	}
}

func TestAsaasWebhookChecksTheToken(t *testing.T) {
	tests := map[string]struct {
		configured string
		sent       string
	}{
		"missing":        {webhookToken, ""},
		"wrong":          {webhookToken, "another-token"},
		"not configured": {"", ""},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			service := &fakeService{}
			handler := NewHandler(service, tt.configured)

			recorder := serve(handler, webhookRequest(tt.sent, receivedEvent))
			handler.Wait()

			if recorder.Code != http.StatusUnauthorized || len(service.applied) != 0 {
				t.Errorf("status = %d, applied = %v, want 401 and nothing applied", recorder.Code, service.applied)
			}
		})
	}
}

func TestAsaasWebhookIgnoresOtherEvents(t *testing.T) {
	service := &fakeService{}
	handler := NewHandler(service, webhookToken)

	for _, body := range []string{
		`{"event":"TRANSFER_DONE","transfer":{"id":"tra_1"}}`,
		`{"event":"PAYMENT_RECEIVED","payment":{}}`,
	} {
		recorder := serve(handler, webhookRequest(webhookToken, body))
		if recorder.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200", body, recorder.Code)
		}
	}
	handler.Wait()

	if len(service.applied) != 0 {
		t.Errorf("applied = %v, want nothing", service.applied)
	}
	if recorder := serve(handler, webhookRequest(webhookToken, `not json`)); recorder.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON: status = %d, want 400", recorder.Code)
	}
}

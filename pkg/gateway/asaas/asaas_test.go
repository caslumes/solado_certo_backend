package asaas

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

type call struct {
	method string
	path   string
	query  string
	body   map[string]any
	header http.Header
}

// fakeAPI answers each request with the next response scripted for "METHOD /path".
type fakeAPI struct {
	t         *testing.T
	mu        sync.Mutex
	responses map[string][]response
	calls     []call
}

type response struct {
	status int
	body   string
}

func newFakeAPI(t *testing.T) (*fakeAPI, *Client) {
	t.Helper()
	api := &fakeAPI{t: t, responses: map[string][]response{}}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)
	client := New(server.URL+"/v3/", "test-key")
	return api, client
}

func (a *fakeAPI) on(route string, responses ...response) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.responses[route] = append(a.responses[route], responses...)
}

func (a *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := call{method: r.Method, path: strings.TrimPrefix(r.URL.Path, "/v3"), query: r.URL.RawQuery, header: r.Header}
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		if err := json.Unmarshal(data, &c.body); err != nil {
			a.t.Errorf("body of %s %s is not JSON: %s", r.Method, r.URL.Path, data)
		}
	}
	a.calls = append(a.calls, c)

	route := c.method + " " + c.path
	queue := a.responses[route]
	if len(queue) == 0 {
		a.t.Errorf("unexpected request %s", route)
		w.WriteHeader(http.StatusTeapot)
		return
	}
	next := queue[0]
	if len(queue) > 1 {
		a.responses[route] = queue[1:]
	}
	w.WriteHeader(next.status)
	_, _ = io.WriteString(w, next.body)
}

func (a *fakeAPI) count(route string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for _, c := range a.calls {
		if c.method+" "+c.path == route {
			n++
		}
	}
	return n
}

// noWait records the backoff instead of sleeping.
func noWait(c *Client) *[]time.Duration {
	var waits []time.Duration
	c.sleep = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	return &waits
}

const emptyList = `{"object": "list", "data": []}`

func chargeRequest(method gateway.Method) gateway.ChargeRequest {
	return gateway.ChargeRequest{
		CustomerID:  "cus_1",
		Method:      method,
		Amount:      "115.00",
		DueDate:     time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC),
		Description: "Pedido SC-20261015-AAAAAA",
		Reference:   "payment-1",
		RemoteIP:    "200.1.2.3",
	}
}

func TestMapStatus(t *testing.T) {
	cases := map[string]gateway.Status{
		"PENDING":                gateway.StatusPending,
		"AWAITING_RISK_ANALYSIS": gateway.StatusPending,
		"CONFIRMED":              gateway.StatusCaptured,
		"RECEIVED":               gateway.StatusCaptured,
		"RECEIVED_IN_CASH":       gateway.StatusCaptured,
		"REFUND_REQUESTED":       gateway.StatusCaptured,
		"CHARGEBACK_REQUESTED":   gateway.StatusCaptured,
		"OVERDUE":                gateway.StatusFailed,
		"DUNNING_REQUESTED":      gateway.StatusFailed,
		"REFUNDED":               gateway.StatusRefunded,
		"SOMETHING_NEW":          gateway.StatusPending,
	}
	for status, want := range cases {
		if got := MapStatus(status, false); got != want {
			t.Errorf("MapStatus(%s) = %s, want %s", status, got, want)
		}
	}
	if got := MapStatus("PENDING", true); got != gateway.StatusCancelled {
		t.Errorf("a deleted payment = %s, want cancelled", got)
	}
}

func TestCreateCustomer(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("GET /customers", response{200, emptyList})
	api.on("POST /customers", response{200, `{"id": "cus_000001"}`})

	id, err := client.CreateCustomer(context.Background(), gateway.Customer{Name: "Maria", Email: "maria@example.com", Phone: "11999990000", TaxID: "52998224725"})

	if err != nil || id != "cus_000001" {
		t.Fatalf("CreateCustomer = %q, %v", id, err)
	}
	lookup, created := api.calls[0], api.calls[1]
	if lookup.query != "cpfCnpj=52998224725" {
		t.Errorf("lookup query = %q", lookup.query)
	}
	want := map[string]any{"name": "Maria", "cpfCnpj": "52998224725", "email": "maria@example.com", "mobilePhone": "11999990000", "notificationDisabled": true}
	for key, value := range want {
		if created.body[key] != value {
			t.Errorf("body[%s] = %v, want %v", key, created.body[key], value)
		}
	}
	if created.header.Get("access_token") != "test-key" || created.header.Get("User-Agent") == "" || created.header.Get("Content-Type") != "application/json" {
		t.Errorf("headers = %v", created.header)
	}
}

func TestCreateCustomerReusesExisting(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("GET /customers", response{200, `{"data": [{"id": "cus_old", "deleted": true}, {"id": "cus_000002", "deleted": false}]}`})

	id, err := client.CreateCustomer(context.Background(), gateway.Customer{Name: "Maria", TaxID: "52998224725"})

	if err != nil || id != "cus_000002" || api.count("POST /customers") != 0 {
		t.Errorf("CreateCustomer = %q, %v after %d creations, want the existing customer", id, err, api.count("POST /customers"))
	}
}

func TestCreatePixCharge(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("POST /payments", response{200, `{"id": "pay_1", "status": "PENDING", "invoiceUrl": "https://sandbox.asaas.com/i/1"}`})

	charge, err := client.CreateCharge(context.Background(), chargeRequest(gateway.MethodPix))

	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}
	if charge.ID != "pay_1" || charge.Status != gateway.StatusPending || charge.GatewayStatus != "PENDING" || charge.InvoiceURL == "" {
		t.Errorf("charge = %+v", charge)
	}
	body := api.calls[0].body
	want := map[string]any{"customer": "cus_1", "billingType": "PIX", "value": 115.0, "dueDate": "2026-10-18",
		"description": "Pedido SC-20261015-AAAAAA", "externalReference": "payment-1"}
	for key, value := range want {
		if body[key] != value {
			t.Errorf("body[%s] = %v, want %v", key, body[key], value)
		}
	}
	if _, ok := body["creditCard"]; ok {
		t.Error("a Pix charge sent card data")
	}
}

func TestCreateBoletoCharge(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("POST /payments", response{200, `{"id": "pay_2", "status": "PENDING", "bankSlipUrl": "https://sandbox.asaas.com/b/2"}`})

	charge, err := client.CreateCharge(context.Background(), chargeRequest(gateway.MethodBoleto))

	if err != nil || charge.BoletoURL != "https://sandbox.asaas.com/b/2" || api.calls[0].body["billingType"] != "BOLETO" {
		t.Errorf("CreateCharge = %+v, %v", charge, err)
	}
}

func cardRequest() gateway.ChargeRequest {
	request := chargeRequest(gateway.MethodCreditCard)
	request.Card = &gateway.Card{HolderName: "MARIA S", Number: "4444444444444444", ExpiryMonth: "12", ExpiryYear: "2030", CVV: "123"}
	request.Holder = &gateway.CardHolder{Name: "Maria", Email: "maria@example.com", Phone: "11999990000", TaxID: "52998224725", PostalCode: "13000000", AddressNumber: "10"}
	return request
}

func TestCreateCardChargeApproved(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("POST /payments", response{200, `{"id": "pay_3", "status": "CONFIRMED", "creditCard": {"creditCardNumber": "4444", "creditCardBrand": "VISA"}}`})

	charge, err := client.CreateCharge(context.Background(), cardRequest())

	if err != nil || charge.Status != gateway.StatusCaptured || charge.CardLast4 != "4444" || charge.CardBrand != "VISA" {
		t.Fatalf("CreateCharge = %+v, %v", charge, err)
	}
	body := api.calls[0].body
	card, _ := body["creditCard"].(map[string]any)
	holder, _ := body["creditCardHolderInfo"].(map[string]any)
	if body["billingType"] != "CREDIT_CARD" || body["remoteIp"] != "200.1.2.3" || card["number"] != "4444444444444444" || card["ccv"] != "123" ||
		card["expiryYear"] != "2030" || holder["cpfCnpj"] != "52998224725" || holder["postalCode"] != "13000000" || holder["addressNumber"] != "10" {
		t.Errorf("body = %v", body)
	}
}

func TestCreateCardChargeRefused(t *testing.T) {
	api, client := newFakeAPI(t)
	waits := noWait(client)
	api.on("POST /payments", response{400, `{"errors": [{"code": "invalid_creditCard", "description": "Transação não autorizada."}]}`})

	_, err := client.CreateCharge(context.Background(), cardRequest())

	refused, ok := errors.AsType[*gateway.CardRefusedError](err)
	if !ok || refused.Message != "Transação não autorizada." {
		t.Errorf("error = %v, want CardRefusedError with the description", err)
	}
	if api.count("POST /payments") != 1 || len(*waits) != 0 {
		t.Error("a refusal was retried")
	}
}

func TestClientErrorIsNotRetried(t *testing.T) {
	api, client := newFakeAPI(t)
	noWait(client)
	api.on("POST /payments", response{400, `{"errors": [{"code": "invalid_customer", "description": "Customer inválido"}]}`})

	_, err := client.CreateCharge(context.Background(), chargeRequest(gateway.MethodPix))

	apiErr, ok := errors.AsType[*APIError](err)
	if !ok || apiErr.StatusCode != 400 || apiErr.Errors[0].Code != "invalid_customer" || errors.Is(err, gateway.ErrUnavailable) {
		t.Errorf("error = %v, want the API error", err)
	}
	if api.count("POST /payments") != 1 {
		t.Errorf("requests = %d, want 1", api.count("POST /payments"))
	}
}

func TestRetriesServerErrorsWithBackoff(t *testing.T) {
	api, client := newFakeAPI(t)
	waits := noWait(client)
	api.on("GET /payments/pay_1", response{502, ""}, response{429, ""}, response{200, `{"id": "pay_1", "status": "RECEIVED"}`})

	charge, err := client.GetCharge(context.Background(), "pay_1")

	if err != nil || charge.Status != gateway.StatusCaptured {
		t.Fatalf("GetCharge = %+v, %v", charge, err)
	}
	if len(*waits) != 2 || (*waits)[0] != 500*time.Millisecond || (*waits)[1] != time.Second {
		t.Errorf("waits = %v, want 500ms then 1s", *waits)
	}
}

func TestGivesUpAfterThreeAttempts(t *testing.T) {
	api, client := newFakeAPI(t)
	noWait(client)
	api.on("GET /payments/pay_1", response{503, ""})

	_, err := client.GetCharge(context.Background(), "pay_1")

	if !errors.Is(err, gateway.ErrUnavailable) || api.count("GET /payments/pay_1") != 3 {
		t.Errorf("error = %v after %d attempts, want ErrUnavailable after 3", err, api.count("GET /payments/pay_1"))
	}
}

func TestRetriedChargeIsNotCreatedTwice(t *testing.T) {
	api, client := newFakeAPI(t)
	noWait(client)
	// The first POST timed out on our side but Asaas created the charge.
	api.on("POST /payments", response{500, ""})
	api.on("GET /payments", response{200, `{"data": [{"id": "pay_9", "status": "PENDING", "externalReference": "payment-1"}]}`})

	charge, err := client.CreateCharge(context.Background(), chargeRequest(gateway.MethodPix))

	if err != nil || charge.ID != "pay_9" {
		t.Fatalf("CreateCharge = %+v, %v, want the charge the first attempt made", charge, err)
	}
	if api.count("POST /payments") != 1 {
		t.Errorf("POSTs = %d, want 1", api.count("POST /payments"))
	}
	if q := api.calls[1].query; q != "externalReference=payment-1" {
		t.Errorf("lookup query = %q", q)
	}
}

func TestRetriedChargeIsCreatedWhenNotFound(t *testing.T) {
	api, client := newFakeAPI(t)
	noWait(client)
	api.on("POST /payments", response{500, ""}, response{200, `{"id": "pay_10", "status": "PENDING"}`})
	api.on("GET /payments", response{200, emptyList})

	charge, err := client.CreateCharge(context.Background(), chargeRequest(gateway.MethodPix))

	if err != nil || charge.ID != "pay_10" || api.count("POST /payments") != 2 {
		t.Errorf("CreateCharge = %+v, %v after %d POSTs", charge, err, api.count("POST /payments"))
	}
}

func TestRetryStopsWhenLookupFails(t *testing.T) {
	api, client := newFakeAPI(t)
	noWait(client)
	api.on("POST /payments", response{500, ""})
	api.on("GET /payments", response{500, ""})

	_, err := client.CreateCharge(context.Background(), chargeRequest(gateway.MethodPix))

	// Without knowing whether the first attempt made a charge, it is never sent again.
	if !errors.Is(err, gateway.ErrUnavailable) || api.count("POST /payments") != 1 {
		t.Errorf("error = %v after %d POSTs, want ErrUnavailable after 1", err, api.count("POST /payments"))
	}
}

func TestNetworkError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	client := New(server.URL, "test-key")
	noWait(client)

	if _, err := client.GetCharge(context.Background(), "pay_1"); !errors.Is(err, gateway.ErrUnavailable) {
		t.Errorf("error = %v, want ErrUnavailable", err)
	}
}

func TestCancelledContextStopsRetries(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("GET /payments/pay_1", response{503, ""})
	ctx, cancel := context.WithCancel(context.Background())
	client.sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	if _, err := client.GetCharge(ctx, "pay_1"); !errors.Is(err, context.Canceled) || api.count("GET /payments/pay_1") != 1 {
		t.Errorf("error = %v, want context.Canceled after one attempt", err)
	}
}

func TestGetChargeDeleted(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("GET /payments/pay_1", response{200, `{"id": "pay_1", "status": "PENDING", "deleted": true}`})

	charge, err := client.GetCharge(context.Background(), "pay_1")

	if err != nil || charge.Status != gateway.StatusCancelled {
		t.Errorf("GetCharge = %+v, %v, want cancelled", charge, err)
	}
}

func TestGetPixQRCode(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("GET /payments/pay_1/pixQrCode", response{200, `{"encodedImage": "iVBOR...", "payload": "00020101021226", "expirationDate": "2026-10-18 23:59:59"}`})

	qrCode, err := client.GetPixQRCode(context.Background(), "pay_1")

	if err != nil || qrCode.Payload != "00020101021226" {
		t.Fatalf("GetPixQRCode = %+v, %v", qrCode, err)
	}
	if want := time.Date(2026, 10, 19, 2, 59, 59, 0, time.UTC); qrCode.ExpiresAt == nil || !qrCode.ExpiresAt.Equal(want) {
		t.Errorf("expires at %v, want %v (23:59:59 in Brazil)", qrCode.ExpiresAt, want)
	}
}

func TestParseDateTime(t *testing.T) {
	if got, ok := parseDateTime("2026-10-18T12:00:00Z"); !ok || !got.Equal(time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("RFC 3339 = %v, %v", got, ok)
	}
	if _, ok := parseDateTime(""); ok {
		t.Error("an empty date was parsed")
	}
}

func TestCancelCharge(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("DELETE /payments/pay_1", response{200, `{"deleted": true, "id": "pay_1"}`})
	api.on("DELETE /payments/pay_2", response{200, `{"deleted": false, "id": "pay_2"}`})

	if err := client.CancelCharge(context.Background(), "pay_1"); err != nil {
		t.Errorf("CancelCharge: %v", err)
	}
	if err := client.CancelCharge(context.Background(), "pay_2"); err == nil {
		t.Error("a charge Asaas didn't delete was reported as cancelled")
	}
}

func TestRefund(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("POST /payments/pay_1/refund", response{200, `{"id": "pay_1", "status": "REFUNDED"}`})
	api.on("POST /payments/pay_2/refund", response{200, `{"id": "pay_2", "status": "REFUND_IN_PROGRESS"}`})

	charge, err := client.Refund(context.Background(), "pay_1", gateway.MethodPix)
	if err != nil || charge.Status != gateway.StatusRefunded || charge.RefundRequested {
		t.Errorf("Refund = %+v, %v", charge, err)
	}
	charge, err = client.Refund(context.Background(), "pay_2", gateway.MethodCreditCard)
	if err != nil || charge.Status != gateway.StatusCaptured || !charge.RefundRequested {
		t.Errorf("refund in progress = %+v, %v, want captured with the refund requested", charge, err)
	}
}

// A boleto's refund only opens a request: the payer gives a bank account at the URL Asaas returns.
func TestRefundBoleto(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("POST /payments/pay_1/bankSlip/refund", response{200, `{"requestUrl": "https://sandbox.asaas.com/r/abc"}`})
	api.on("GET /payments/pay_1", response{200, `{"id": "pay_1", "status": "RECEIVED"}`})

	charge, err := client.Refund(context.Background(), "pay_1", gateway.MethodBoleto)

	if err != nil || charge.Status != gateway.StatusCaptured || !charge.RefundRequested || charge.RefundURL != "https://sandbox.asaas.com/r/abc" {
		t.Errorf("Refund = %+v, %v", charge, err)
	}
	if api.count("POST /payments/pay_1/refund") != 0 {
		t.Error("a boleto was refunded through the card and Pix route")
	}
}

// A refund whose answer was lost is found by reading the charge, and is not asked again.
func TestRefundRetryFindsTheRefund(t *testing.T) {
	api, client := newFakeAPI(t)
	noWait(client)
	api.on("POST /payments/pay_1/refund", response{502, ``})
	api.on("GET /payments/pay_1", response{200, `{"id": "pay_1", "status": "REFUNDED"}`})

	charge, err := client.Refund(context.Background(), "pay_1", gateway.MethodPix)

	if err != nil || charge.Status != gateway.StatusRefunded {
		t.Errorf("Refund = %+v, %v", charge, err)
	}
	if n := api.count("POST /payments/pay_1/refund"); n != 1 {
		t.Errorf("refund asked %d times, want 1", n)
	}
}

func TestRefundRefused(t *testing.T) {
	api, client := newFakeAPI(t)
	api.on("POST /payments/pay_1/refund", response{400, `{"errors": [{"code": "invalid_action", "description": "Saldo insuficiente"}]}`})

	_, err := client.Refund(context.Background(), "pay_1", gateway.MethodPix)

	if apiErr, ok := errors.AsType[*APIError](err); !ok || apiErr.StatusCode != 400 || errors.Is(err, gateway.ErrUnavailable) {
		t.Errorf("err = %v, want the 400 without retries", err)
	}
}

func TestUnknownMethod(t *testing.T) {
	_, client := newFakeAPI(t)
	if _, err := client.CreateCharge(context.Background(), gateway.ChargeRequest{Method: "cash"}); err == nil {
		t.Error("an unknown method was sent")
	}
}

// Package asaas is the Asaas implementation of gateway.PaymentGateway (API v3).
package asaas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

const (
	SandboxURL = "https://api-sandbox.asaas.com/v3"

	maxAttempts    = 3
	firstBackoff   = 500 * time.Millisecond
	requestTimeout = 20 * time.Second
	userAgent      = "solado-certo"
)

// brazilTime is the zone of the dates Asaas returns without one.
var brazilTime = time.FixedZone("BRT", -3*60*60)

var billingTypes = map[gateway.Method]string{
	gateway.MethodPix:        "PIX",
	gateway.MethodBoleto:     "BOLETO",
	gateway.MethodCreditCard: "CREDIT_CARD",
}

// statuses maps Asaas payment statuses to the gateway's. A refund or chargeback in progress keeps the
// charge captured until it is final; statuses not listed are treated as pending.
var statuses = map[string]gateway.Status{
	"PENDING":                      gateway.StatusPending,
	"AWAITING_RISK_ANALYSIS":       gateway.StatusPending,
	"CONFIRMED":                    gateway.StatusCaptured,
	"RECEIVED":                     gateway.StatusCaptured,
	"RECEIVED_IN_CASH":             gateway.StatusCaptured,
	"DUNNING_RECEIVED":             gateway.StatusCaptured,
	"REFUND_REQUESTED":             gateway.StatusCaptured,
	"REFUND_IN_PROGRESS":           gateway.StatusCaptured,
	"CHARGEBACK_REQUESTED":         gateway.StatusCaptured,
	"CHARGEBACK_DISPUTE":           gateway.StatusCaptured,
	"AWAITING_CHARGEBACK_REVERSAL": gateway.StatusCaptured,
	"OVERDUE":                      gateway.StatusFailed,
	"DUNNING_REQUESTED":            gateway.StatusFailed,
	"REFUNDED":                     gateway.StatusRefunded,
}

func MapStatus(status string, deleted bool) gateway.Status {
	if deleted {
		return gateway.StatusCancelled
	}
	if mapped, ok := statuses[status]; ok {
		return mapped
	}
	return gateway.StatusPending
}

// APIError is an answer with a 4xx status; it is never retried.
type APIError struct {
	StatusCode int
	Errors     []struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"errors"`
}

func (e *APIError) Error() string {
	descriptions := make([]string, len(e.Errors))
	for i, item := range e.Errors {
		descriptions[i] = item.Code + ": " + item.Description
	}
	return fmt.Sprintf("asaas: status %d: %s", e.StatusCode, strings.Join(descriptions, "; "))
}

func (e *APIError) description() string {
	if len(e.Errors) == 0 {
		return ""
	}
	return e.Errors[0].Description
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	// sleep waits between attempts; tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
}

func New(baseURL string, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: requestTimeout},
		sleep:   sleep,
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) Name() string {
	return "asaas"
}

type customerBody struct {
	Name                 string `json:"name"`
	CPFCNPJ              string `json:"cpfCnpj"`
	Email                string `json:"email,omitempty"`
	MobilePhone          string `json:"mobilePhone,omitempty"`
	NotificationDisabled bool   `json:"notificationDisabled"`
}

type customerList struct {
	Data []struct {
		ID      string `json:"id"`
		Deleted bool   `json:"deleted"`
	} `json:"data"`
}

// CreateCustomer reuses the customer with that CPF if there is one, so a retry or a database reset
// doesn't create duplicates. Asaas's own e-mails are turned off: the app notifies the buyer.
func (c *Client) CreateCustomer(ctx context.Context, customer gateway.Customer) (string, error) {
	if id, err := c.findCustomer(ctx, customer.TaxID); err != nil || id != "" {
		return id, err
	}
	var created struct {
		ID string `json:"id"`
	}
	body := customerBody{Name: customer.Name, CPFCNPJ: customer.TaxID, Email: customer.Email, MobilePhone: customer.Phone, NotificationDisabled: true}
	err := c.do(ctx, http.MethodPost, "/customers", body, &created, "", func(ctx context.Context) (bool, error) {
		id, err := c.findCustomer(ctx, customer.TaxID)
		created.ID = id
		return id != "", err
	})
	return created.ID, err
}

func (c *Client) findCustomer(ctx context.Context, taxID string) (string, error) {
	var list customerList
	if err := c.do(ctx, http.MethodGet, "/customers?cpfCnpj="+url.QueryEscape(taxID), nil, &list, "", nil); err != nil {
		return "", err
	}
	for _, item := range list.Data {
		if !item.Deleted {
			return item.ID, nil
		}
	}
	return "", nil
}

type creditCard struct {
	HolderName  string `json:"holderName"`
	Number      string `json:"number"`
	ExpiryMonth string `json:"expiryMonth"`
	ExpiryYear  string `json:"expiryYear"`
	CCV         string `json:"ccv"`
}

type creditCardHolderInfo struct {
	Name          string `json:"name"`
	Email         string `json:"email"`
	CPFCNPJ       string `json:"cpfCnpj"`
	PostalCode    string `json:"postalCode"`
	AddressNumber string `json:"addressNumber"`
	Phone         string `json:"phone"`
}

type chargeBody struct {
	Customer             string                `json:"customer"`
	BillingType          string                `json:"billingType"`
	Value                json.Number           `json:"value"`
	DueDate              string                `json:"dueDate"`
	Description          string                `json:"description,omitempty"`
	ExternalReference    string                `json:"externalReference,omitempty"`
	CreditCard           *creditCard           `json:"creditCard,omitempty"`
	CreditCardHolderInfo *creditCardHolderInfo `json:"creditCardHolderInfo,omitempty"`
	RemoteIP             string                `json:"remoteIp,omitempty"`
}

type payment struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Deleted     bool   `json:"deleted"`
	InvoiceURL  string `json:"invoiceUrl"`
	BankSlipURL string `json:"bankSlipUrl"`
	CreditCard  *struct {
		Number string `json:"creditCardNumber"`
		Brand  string `json:"creditCardBrand"`
	} `json:"creditCard"`
}

func (p *payment) charge() *gateway.Charge {
	charge := &gateway.Charge{
		ID:            p.ID,
		Status:        MapStatus(p.Status, p.Deleted),
		GatewayStatus: p.Status,
		InvoiceURL:    p.InvoiceURL,
		BoletoURL:     p.BankSlipURL,
	}
	if p.CreditCard != nil {
		charge.CardLast4 = p.CreditCard.Number
		charge.CardBrand = p.CreditCard.Brand
	}
	return charge
}

// CreateCharge creates a payment. Asaas has no idempotency key, so before repeating a failed attempt
// the client looks for a charge with the same reference: a timeout may hide a charge that was made.
// A card the issuer refuses comes back as a 400 and is not saved by Asaas (gateway.CardRefusedError).
func (c *Client) CreateCharge(ctx context.Context, request gateway.ChargeRequest) (*gateway.Charge, error) {
	billingType, ok := billingTypes[request.Method]
	if !ok {
		return nil, fmt.Errorf("asaas: unknown method %q", request.Method)
	}
	body := chargeBody{
		Customer:          request.CustomerID,
		BillingType:       billingType,
		Value:             json.Number(request.Amount),
		DueDate:           request.DueDate.Format(time.DateOnly),
		Description:       request.Description,
		ExternalReference: request.Reference,
		RemoteIP:          request.RemoteIP,
	}
	if request.Card != nil {
		body.CreditCard = &creditCard{
			HolderName:  request.Card.HolderName,
			Number:      request.Card.Number,
			ExpiryMonth: request.Card.ExpiryMonth,
			ExpiryYear:  request.Card.ExpiryYear,
			CCV:         request.Card.CVV,
		}
	}
	if request.Holder != nil {
		body.CreditCardHolderInfo = &creditCardHolderInfo{
			Name:          request.Holder.Name,
			Email:         request.Holder.Email,
			CPFCNPJ:       request.Holder.TaxID,
			PostalCode:    request.Holder.PostalCode,
			AddressNumber: request.Holder.AddressNumber,
			Phone:         request.Holder.Phone,
		}
	}

	var created payment
	err := c.do(ctx, http.MethodPost, "/payments", body, &created, request.Reference, func(ctx context.Context) (bool, error) {
		found, err := c.findCharge(ctx, request.Reference)
		if found != nil {
			created = *found
		}
		return found != nil, err
	})
	if apiErr, ok := errors.AsType[*APIError](err); ok && request.Method == gateway.MethodCreditCard && apiErr.StatusCode == http.StatusBadRequest {
		return nil, &gateway.CardRefusedError{Message: apiErr.description()}
	}
	if err != nil {
		return nil, err
	}
	return created.charge(), nil
}

func (c *Client) findCharge(ctx context.Context, reference string) (*payment, error) {
	if reference == "" {
		return nil, nil
	}
	var list struct {
		Data []payment `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/payments?externalReference="+url.QueryEscape(reference), nil, &list, reference, nil); err != nil {
		return nil, err
	}
	for _, item := range list.Data {
		if !item.Deleted {
			return &item, nil
		}
	}
	return nil, nil
}

func (c *Client) GetCharge(ctx context.Context, id string) (*gateway.Charge, error) {
	var found payment
	if err := c.do(ctx, http.MethodGet, "/payments/"+url.PathEscape(id), nil, &found, id, nil); err != nil {
		return nil, err
	}
	return found.charge(), nil
}

func (c *Client) GetPixQRCode(ctx context.Context, chargeID string) (*gateway.PixQRCode, error) {
	var qrCode struct {
		Payload        string `json:"payload"`
		ExpirationDate string `json:"expirationDate"`
	}
	if err := c.do(ctx, http.MethodGet, "/payments/"+url.PathEscape(chargeID)+"/pixQrCode", nil, &qrCode, chargeID, nil); err != nil {
		return nil, err
	}
	result := &gateway.PixQRCode{Payload: qrCode.Payload}
	if expires, ok := parseDateTime(qrCode.ExpirationDate); ok {
		result.ExpiresAt = &expires
	}
	return result, nil
}

// parseDateTime reads "2026-10-18 23:59:59" (Brazil's time, as Asaas sends it) or RFC 3339.
func parseDateTime(value string) (time.Time, bool) {
	if t, err := time.ParseInLocation(time.DateTime, value, brazilTime); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

func (c *Client) CancelCharge(ctx context.Context, id string) error {
	var result struct {
		Deleted bool `json:"deleted"`
	}
	if err := c.do(ctx, http.MethodDelete, "/payments/"+url.PathEscape(id), nil, &result, id, nil); err != nil {
		return err
	}
	if !result.Deleted {
		return fmt.Errorf("asaas: payment %s was not deleted", id)
	}
	return nil
}

// Refund refunds the whole charge.
func (c *Client) Refund(ctx context.Context, id string) (*gateway.Charge, error) {
	var refunded payment
	if err := c.do(ctx, http.MethodPost, "/payments/"+url.PathEscape(id)+"/refund", struct{}{}, &refunded, id, nil); err != nil {
		return nil, err
	}
	return refunded.charge(), nil
}

// do sends a request, retrying network errors and 5xx answers up to maxAttempts with exponential
// backoff (RNF-013); 4xx answers are returned at once as *APIError. Before a POST is repeated,
// lookup is called to find what the failed attempt may have created; if it finds it, that is the
// result. Every failed attempt is logged with the reference (our payment id or the charge id). After
// the last attempt, the error wraps gateway.ErrUnavailable (RNF-023).
func (c *Client) do(ctx context.Context, method string, path string, body any, out any, reference string, lookup func(ctx context.Context) (bool, error)) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}

	backoff := firstBackoff
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			if err := c.sleep(ctx, backoff); err != nil {
				return err
			}
			backoff *= 2
			if lookup != nil {
				found, err := lookup(ctx)
				if found {
					return nil
				}
				if err != nil {
					lastErr = err
					slog.WarnContext(ctx, "asaas recovery lookup failed", "method", method, "path", logPath(path), "reference", reference, "attempt", attempt, "error", err)
					continue
				}
			}
		}

		retry, err := c.send(ctx, method, path, payload, out)
		if err == nil {
			return nil
		}
		if !retry {
			return err
		}
		lastErr = err
		slog.WarnContext(ctx, "asaas request failed", "method", method, "path", logPath(path), "reference", reference, "attempt", attempt, "error", err)
	}
	return fmt.Errorf("%w: %w", gateway.ErrUnavailable, lastErr)
}

// send makes one attempt and tells whether a failure can be retried.
func (c *Client) send(ctx context.Context, method string, path string, payload []byte, out any) (bool, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return false, err
	}
	req.Header.Set("access_token", c.apiKey)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		return true, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return true, err
	}

	switch {
	case res.StatusCode >= 500 || res.StatusCode == http.StatusTooManyRequests:
		return true, fmt.Errorf("asaas: status %d", res.StatusCode)
	case res.StatusCode >= 400:
		apiErr := &APIError{StatusCode: res.StatusCode}
		_ = json.Unmarshal(data, apiErr)
		return false, apiErr
	}
	if out == nil {
		return false, nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return false, fmt.Errorf("asaas: decode %s %s: %w", method, logPath(path), err)
	}
	return false, nil
}

// logPath drops the query, which may hold a CPF.
func logPath(path string) string {
	p, _, _ := strings.Cut(path, "?")
	return p
}

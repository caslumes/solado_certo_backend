package payments_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/pkg/gateway"
	"github.com/caslumes/solado_certo_backend/pkg/gateway/fake"
)

const (
	buyerID    = "buyer-1"
	otherID    = "buyer-2"
	orderID    = "00000000-0000-0000-0000-000000000701"
	paidID     = "00000000-0000-0000-0000-000000000702"
	unknownID  = "00000000-0000-0000-0000-000000000000"
	validCPF   = "529.982.247-25"
	cpfDigits  = "52998224725"
	visaNumber = "4111111111111111"
)

var orderCreated = time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)

type fakeRepository struct {
	orders    map[string]*payments.OrderEntity
	buyers    map[string]*payments.BuyerEntity
	customers map[string]string
	payments  []*payments.PaymentEntity
	keys      map[string]string
	changes   []payments.StatusChange
	paid      []string
	refunds   []payments.RefundRecord
	expired   []string
	err       error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		orders: map[string]*payments.OrderEntity{
			orderID: {ID: orderID, BuyerID: buyerID, Number: "SC-20261015-AAAAAA", Status: orders.OrderStatusAwaitingPayment,
				Subtotal: "100.00", ShippingTotal: "20.00", DiscountTotal: "0.00", Total: "120.00",
				PostalCode: "13000000", AddressNumber: "10", CreatedAt: orderCreated},
			paidID: {ID: paidID, BuyerID: buyerID, Number: "SC-20261015-BBBBBB", Status: orders.OrderStatusPaid,
				Subtotal: "50.00", ShippingTotal: "0.00", DiscountTotal: "0.00", Total: "50.00", CreatedAt: orderCreated},
		},
		buyers: map[string]*payments.BuyerEntity{
			buyerID: {ID: buyerID, Name: "Maria", Email: "maria@example.com", Phone: "11999990000"},
			otherID: {ID: otherID, Name: "João", Email: "joao@example.com", Phone: "11999990001"},
		},
		customers: map[string]string{},
		keys:      map[string]string{},
	}
}

func (f *fakeRepository) GetOrder(_ context.Context, id string) (*payments.OrderEntity, error) {
	if f.err != nil {
		return nil, f.err
	}
	order, ok := f.orders[id]
	if !ok {
		return nil, app.ErrOrderNotFound
	}
	copied := *order
	return &copied, nil
}

func (f *fakeRepository) GetBuyer(_ context.Context, id string) (*payments.BuyerEntity, error) {
	buyer := *f.buyers[id]
	return &buyer, nil
}

func (f *fakeRepository) SetTaxID(_ context.Context, userID string, taxID string) error {
	for id, buyer := range f.buyers {
		if id != userID && buyer.TaxID != nil && *buyer.TaxID == taxID {
			return app.ErrTaxIDInUse
		}
	}
	f.buyers[userID].TaxID = &taxID
	return nil
}

func (f *fakeRepository) GetCustomerID(_ context.Context, userID string, gateway string) (string, bool, error) {
	id, ok := f.customers[userID+"/"+gateway]
	return id, ok, nil
}

func (f *fakeRepository) SaveCustomerID(_ context.Context, userID string, gateway string, customerID string) error {
	f.customers[userID+"/"+gateway] = customerID
	return nil
}

func (f *fakeRepository) find(match func(*payments.PaymentEntity) bool) (*payments.PaymentEntity, error) {
	for _, p := range f.payments {
		if match(p) {
			copied := *p
			return &copied, nil
		}
	}
	return nil, app.ErrPaymentNotFound
}

func (f *fakeRepository) GetPaymentByKey(_ context.Context, key string) (*payments.PaymentEntity, error) {
	return f.find(func(p *payments.PaymentEntity) bool { return f.keys[p.ID] == key })
}

func (f *fakeRepository) GetPaymentByTransaction(_ context.Context, gateway string, transactionID string) (*payments.PaymentEntity, error) {
	return f.find(func(p *payments.PaymentEntity) bool {
		return p.Gateway == gateway && p.TransactionID != nil && *p.TransactionID == transactionID
	})
}

func (f *fakeRepository) GetPayment(_ context.Context, id string) (*payments.PaymentEntity, error) {
	return f.find(func(p *payments.PaymentEntity) bool { return p.ID == id })
}

func (f *fakeRepository) GetPayments(_ context.Context, orderID string) ([]*payments.PaymentEntity, error) {
	var found []*payments.PaymentEntity
	for _, p := range slices.Backward(f.payments) {
		if p.OrderID == orderID {
			copied := *p
			found = append(found, &copied)
		}
	}
	return found, nil
}

func (f *fakeRepository) StartPayment(_ context.Context, payment payments.NewPayment) (*payments.PaymentEntity, bool, error) {
	for _, p := range f.payments {
		if f.keys[p.ID] == payment.IdempotencyKey {
			copied := *p
			return &copied, false, nil
		}
		if p.OrderID == payment.OrderID && p.Status == payments.StatusPending {
			return nil, false, app.ErrPaymentInProgress
		}
	}
	order := f.orders[payment.OrderID]
	if order.Status != orders.OrderStatusAwaitingPayment {
		return nil, false, app.ErrInvalidStatusTransition
	}
	order.DiscountTotal = payment.DiscountTotal
	order.Total = (helpers.MustParseCents(order.Subtotal) + helpers.MustParseCents(order.ShippingTotal) - helpers.MustParseCents(payment.DiscountTotal)).String()

	due := payment.DueDate
	p := &payments.PaymentEntity{
		ID:      fmt.Sprintf("payment-%d", len(f.payments)+1),
		OrderID: payment.OrderID,
		Gateway: payment.Gateway,
		Method:  payment.Method,
		Status:  payments.StatusPending,
		Amount:  payment.Amount,
		DueDate: &due,
	}
	f.keys[p.ID] = payment.IdempotencyKey
	f.payments = append(f.payments, p)
	copied := *p
	return &copied, true, nil
}

func (f *fakeRepository) AttachCharge(_ context.Context, paymentID string, transactionID string, details payments.PaymentDetails) error {
	for _, p := range f.payments {
		if p.ID == paymentID {
			p.TransactionID = &transactionID
			p.Details = details
		}
	}
	return nil
}

func (f *fakeRepository) ChangePaymentStatus(_ context.Context, change payments.StatusChange) (bool, error) {
	for _, p := range f.payments {
		if p.ID != change.PaymentID {
			continue
		}
		if !p.Status.CanBecome(change.To) {
			return false, nil
		}
		p.Status = change.To
		if change.FailureReason != "" {
			p.FailureReason = &change.FailureReason
		}
		if change.Details.CardLast4 != "" {
			p.Details.CardLast4 = change.Details.CardLast4
		}
		if change.Details.GatewayError != "" {
			p.Details.GatewayError = change.Details.GatewayError
		}
		if change.Details.RefundRequestedAt != nil {
			p.Details.RefundRequestedAt = change.Details.RefundRequestedAt
		}
		f.changes = append(f.changes, change)
		if order := f.orders[p.OrderID]; change.To == payments.StatusCaptured && order.Status == orders.OrderStatusAwaitingPayment {
			order.Status = orders.OrderStatusPaid
			f.paid = append(f.paid, order.ID)
		}
		return true, nil
	}
	return false, app.ErrPaymentNotFound
}

func (f *fakeRepository) GetPendingPayments(_ context.Context) ([]*payments.PaymentEntity, error) {
	return f.filter(func(p *payments.PaymentEntity) bool {
		return p.Status == payments.StatusPending && p.TransactionID != nil
	})
}

func (f *fakeRepository) GetClosedOrderPayments(_ context.Context, startedBefore time.Time) ([]*payments.PaymentEntity, error) {
	return f.filter(func(p *payments.PaymentEntity) bool {
		return p.Status == payments.StatusPending && (p.TransactionID != nil || p.CreatedAt.Before(startedBefore)) &&
			f.orders[p.OrderID].Status != orders.OrderStatusAwaitingPayment
	})
}

func (f *fakeRepository) filter(match func(*payments.PaymentEntity) bool) ([]*payments.PaymentEntity, error) {
	if f.err != nil {
		return nil, f.err
	}
	var found []*payments.PaymentEntity
	for _, p := range f.payments {
		if match(p) {
			copied := *p
			found = append(found, &copied)
		}
	}
	return found, nil
}

func (f *fakeRepository) GetExpiredOrderIDs(_ context.Context, createdBefore time.Time) ([]string, error) {
	var ids []string
	for id, order := range f.orders {
		if order.Status == orders.OrderStatusAwaitingPayment && order.CreatedAt.Before(createdBefore) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (f *fakeRepository) ExpireOrder(_ context.Context, orderID string, createdBefore time.Time) (bool, error) {
	order := f.orders[orderID]
	if order.Status != orders.OrderStatusAwaitingPayment || !order.CreatedAt.Before(createdBefore) {
		return false, nil
	}
	order.Status = orders.OrderStatusCancelled
	f.expired = append(f.expired, orderID)
	return true, nil
}

type setup struct {
	r       *fakeRepository
	gateway *fake.Gateway
	s       interface {
		payments.CreatePaymentUseCase
		payments.ListPaymentsUseCase
		payments.ApplyPaymentStatusUseCase
		payments.CheckPaymentsUseCase
		SetNow(func() time.Time)
	}
	ctx context.Context
}

func newSetup(t *testing.T) setup {
	t.Helper()
	r := newFakeRepository()
	gateway := fake.New(0)
	s := payments.NewService(r, gateway)
	s.SetNow(func() time.Time { return orderCreated.Add(time.Hour) })
	return setup{r: r, gateway: gateway, s: s, ctx: context.Background()}
}

func (s setup) pay(t *testing.T, key string, input payments.CreatePaymentInput) *payments.PaymentEntity {
	t.Helper()
	if input.TaxID == "" {
		input.TaxID = validCPF
	}
	payment, created, err := s.s.CreatePayment(s.ctx, buyerID, orderID, key, input)
	if err != nil || !created {
		t.Fatalf("CreatePayment(%s) = %v, %v, %v", input.Method, payment, created, err)
	}
	return payment
}

func card(number string) *payments.CardInput {
	return &payments.CardInput{HolderName: "MARIA S", Number: number, ExpiryMonth: "12", ExpiryYear: "2030", CVV: "123"}
}

func TestCreatePixPayment(t *testing.T) {
	s := newSetup(t)

	payment := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix, TaxID: validCPF})

	if payment.Status != payments.StatusPending || payment.Method != payments.MethodPix || payment.Amount != "115.00" {
		t.Errorf("payment = %+v, want a pending Pix of 115.00 (5%% off the 100.00 subtotal)", payment)
	}
	if payment.TransactionID == nil || payment.Details.PixPayload == "" || payment.Details.PixExpiresAt == nil {
		t.Errorf("details = %+v, want the charge id and the Pix code", payment.Details)
	}
	if order := s.r.orders[orderID]; order.DiscountTotal != "5.00" || order.Total != "115.00" {
		t.Errorf("order totals = %s, %s, want the Pix discount", order.DiscountTotal, order.Total)
	}
	if got := *s.r.buyers[buyerID].TaxID; got != cpfDigits {
		t.Errorf("saved tax id = %q, want the CPF digits", got)
	}
	request := s.gateway.Requests[0]
	if request.Amount != "115.00" || request.Description != "Pedido SC-20261015-AAAAAA" || request.Reference != payment.ID ||
		!request.DueDate.Equal(time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("charge request = %+v", request)
	}
}

func TestCreateBoletoPaymentRemovesPixDiscount(t *testing.T) {
	s := newSetup(t)
	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})

	boleto := s.pay(t, "key-2", payments.CreatePaymentInput{Method: payments.MethodBoleto})

	if boleto.Amount != "120.00" || boleto.Details.BoletoURL == "" {
		t.Errorf("boleto = %+v, want 120.00 with a link", boleto)
	}
	if order := s.r.orders[orderID]; order.DiscountTotal != "0.00" || order.Total != "120.00" {
		t.Errorf("order totals = %s, %s, want no discount", order.DiscountTotal, order.Total)
	}
	replaced, _ := s.r.GetPayment(s.ctx, pix.ID)
	if replaced.Status != payments.StatusCancelled {
		t.Errorf("first payment = %s, want cancelled", replaced.Status)
	}
	charge, _ := s.gateway.GetCharge(s.ctx, *pix.TransactionID)
	if charge.Status != gateway.StatusCancelled {
		t.Errorf("first charge = %s, want cancelled at the gateway", charge.Status)
	}
	if len(s.gateway.Requests) != 2 || len(s.r.customers) != 1 {
		t.Errorf("%d charges for customers %v, want 2 charges for one customer", len(s.gateway.Requests), s.r.customers)
	}
}

func TestCreateCardPaymentApproved(t *testing.T) {
	s := newSetup(t)

	payment := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: card("4111 1111 1111 1111")})

	if payment.Status != payments.StatusCaptured || payment.Details.CardLast4 != "1111" || payment.Details.CardBrand == "" {
		t.Errorf("payment = %+v, want captured with the card's last digits", payment)
	}
	if s.r.orders[orderID].Status != orders.OrderStatusPaid {
		t.Errorf("order = %s, want paid", s.r.orders[orderID].Status)
	}
	request := s.gateway.Requests[0]
	if request.Holder == nil || request.Holder.TaxID != cpfDigits || request.Holder.PostalCode != "13000000" || request.Holder.AddressNumber != "10" {
		t.Errorf("holder = %+v, want the buyer's CPF and the order's address", request.Holder)
	}
	if s.r.changes[0].Source != payments.SourceCharge {
		t.Errorf("change source = %q, want charge", s.r.changes[0].Source)
	}
}

func TestCreateCardPaymentRefusedThenPix(t *testing.T) {
	s := newSetup(t)

	refused := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: card(fake.RefusedCardNumber)})

	if refused.Status != payments.StatusFailed || refused.FailureReason == nil || *refused.FailureReason != payments.FailureCardRefused {
		t.Fatalf("payment = %+v, want failed with card_refused", refused)
	}
	if refused.Details.GatewayError == "" || refused.Details.CardLast4 != "0002" {
		t.Errorf("details = %+v, want the gateway's message and the last digits", refused.Details)
	}
	if s.r.orders[orderID].Status != orders.OrderStatusAwaitingPayment {
		t.Errorf("order = %s, want still awaiting payment", s.r.orders[orderID].Status)
	}

	pix := s.pay(t, "key-2", payments.CreatePaymentInput{Method: payments.MethodPix})
	if pix.Status != payments.StatusPending {
		t.Errorf("Pix after refusal = %s, want pending", pix.Status)
	}
}

func TestCreatePaymentRepeatedKey(t *testing.T) {
	s := newSetup(t)
	first := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})

	again, created, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "key-1", payments.CreatePaymentInput{Method: payments.MethodBoleto})

	if err != nil || created || again.ID != first.ID || again.Method != payments.MethodPix {
		t.Errorf("repeat = %+v, %v, %v, want the first payment", again, created, err)
	}
	if len(s.gateway.Requests) != 1 {
		t.Errorf("charges = %d, want 1", len(s.gateway.Requests))
	}

	s.gateway.Confirm(*first.TransactionID)
	if err := s.s.ApplyPaymentStatus(s.ctx, *first.TransactionID, payments.SourceWebhook); err != nil {
		t.Fatalf("ApplyPaymentStatus: %v", err)
	}
	again, created, err = s.s.CreatePayment(s.ctx, buyerID, orderID, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	if err != nil || created || again.Status != payments.StatusCaptured {
		t.Errorf("repeat after paying = %+v, %v, %v, want the captured payment", again, created, err)
	}
}

func TestCreatePaymentKeyOfAnotherOrder(t *testing.T) {
	s := newSetup(t)
	s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.r.orders[paidID].Status = orders.OrderStatusAwaitingPayment

	_, _, err := s.s.CreatePayment(s.ctx, buyerID, paidID, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})

	if !errors.Is(err, app.ErrIdempotencyKeyReused) {
		t.Errorf("error = %v, want ErrIdempotencyKeyReused", err)
	}
}

func TestCreatePaymentTaxID(t *testing.T) {
	s := newSetup(t)
	pix := payments.CreatePaymentInput{Method: payments.MethodPix}

	if _, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "key-1", pix); !errors.Is(err, app.ErrTaxIDRequired) {
		t.Errorf("without CPF: error = %v, want ErrTaxIDRequired", err)
	}
	pix.TaxID = "529.982.247-24"
	if _, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "key-1", pix); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("invalid CPF: error = %v, want ErrInvalidInput", err)
	}

	cpf := cpfDigits
	s.r.buyers[otherID].TaxID = &cpf
	pix.TaxID = validCPF
	if _, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "key-1", pix); !errors.Is(err, app.ErrTaxIDInUse) {
		t.Errorf("CPF of another user: error = %v, want ErrTaxIDInUse", err)
	}
	if len(s.r.payments) != 0 || len(s.gateway.Requests) != 0 {
		t.Error("a payment was started without a CPF")
	}

	s.r.buyers[otherID].TaxID = nil
	stored := "11144477735"
	s.r.buyers[buyerID].TaxID = &stored
	pix.TaxID = ""
	if _, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "key-1", pix); err != nil {
		t.Errorf("with a saved CPF: error = %v", err)
	}
}

func TestCreatePaymentInvalidInput(t *testing.T) {
	s := newSetup(t)
	badCard := func(change func(*payments.CardInput)) *payments.CardInput {
		c := card(visaNumber)
		change(c)
		return c
	}
	cases := map[string]struct {
		key   string
		input payments.CreatePaymentInput
	}{
		"no key":         {"", payments.CreatePaymentInput{Method: payments.MethodPix}},
		"long key":       {string(make([]byte, 121)), payments.CreatePaymentInput{Method: payments.MethodPix}},
		"unknown method": {"k", payments.CreatePaymentInput{Method: "debit_card"}},
		"card with pix":  {"k", payments.CreatePaymentInput{Method: payments.MethodPix, Card: card(visaNumber)}},
		"card missing":   {"k", payments.CreatePaymentInput{Method: payments.MethodCreditCard}},
		"no holder":      {"k", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: badCard(func(c *payments.CardInput) { c.HolderName = " " })}},
		"short number":   {"k", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: badCard(func(c *payments.CardInput) { c.Number = "4111" })}},
		"month 13":       {"k", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: badCard(func(c *payments.CardInput) { c.ExpiryMonth = "13" })}},
		"two-digit year": {"k", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: badCard(func(c *payments.CardInput) { c.ExpiryYear = "30" })}},
		"five-digit cvv": {"k", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: badCard(func(c *payments.CardInput) { c.CVV = "12345" })}},
	}
	for name, c := range cases {
		c.input.TaxID = validCPF
		if _, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, c.key, c.input); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%s: error = %v, want ErrInvalidInput", name, err)
		}
	}
	if len(s.r.payments) != 0 {
		t.Error("an invalid request started a payment")
	}
}

func TestCreatePaymentOrderNotPayable(t *testing.T) {
	s := newSetup(t)
	pix := payments.CreatePaymentInput{Method: payments.MethodPix, TaxID: validCPF}

	for _, id := range []string{"not-a-uuid", unknownID} {
		if _, _, err := s.s.CreatePayment(s.ctx, buyerID, id, "k", pix); !errors.Is(err, app.ErrOrderNotFound) {
			t.Errorf("order %s: error = %v, want ErrOrderNotFound", id, err)
		}
	}
	if _, _, err := s.s.CreatePayment(s.ctx, otherID, orderID, "k", pix); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("another buyer's order: error = %v, want ErrOrderNotFound", err)
	}
	if _, _, err := s.s.CreatePayment(s.ctx, buyerID, paidID, "k", pix); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("paid order: error = %v, want ErrInvalidStatusTransition", err)
	}

	s.s.SetNow(func() time.Time { return afterWindow })
	if _, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "k", pix); !errors.Is(err, app.ErrOrderExpired) {
		t.Errorf("expired order: error = %v, want ErrOrderExpired", err)
	}
}

func TestCreatePaymentNeedsPrescriptions(t *testing.T) {
	s := newSetup(t)
	s.r.orders[orderID].MissingPrescriptions = []string{"item-1"}
	pix := payments.CreatePaymentInput{Method: payments.MethodPix, TaxID: validCPF}

	_, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "key-1", pix)

	var missing *app.PrescriptionRequiredError
	if !errors.As(err, &missing) || len(missing.ItemIDs) != 1 || missing.ItemIDs[0] != "item-1" {
		t.Fatalf("error = %v, want PrescriptionRequiredError for item-1", err)
	}
	if len(s.r.payments) != 0 {
		t.Error("a payment was started without the prescriptions")
	}
}

func TestCreatePaymentGatewayUnavailable(t *testing.T) {
	s := newSetup(t)
	pix := payments.CreatePaymentInput{Method: payments.MethodPix, TaxID: validCPF}

	s.gateway.Unavailable = true
	if _, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "key-1", pix); !errors.Is(err, app.ErrGatewayUnavailable) {
		t.Errorf("creating the customer: error = %v, want ErrGatewayUnavailable", err)
	}
	if len(s.r.payments) != 0 {
		t.Error("a payment was started without a customer")
	}

	s.r.customers[buyerID+"/fake"] = "cus_known"
	if _, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "key-2", pix); !errors.Is(err, app.ErrGatewayUnavailable) {
		t.Errorf("creating the charge: error = %v, want ErrGatewayUnavailable", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusFailed || *p.FailureReason != payments.FailureGatewayUnavailable {
		t.Errorf("payment = %+v, want failed with gateway_unavailable", p)
	}
}

func TestCreatePaymentWhenReplacedChargeWasPaid(t *testing.T) {
	s := newSetup(t)
	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.gateway.Confirm(*pix.TransactionID)

	_, _, err := s.s.CreatePayment(s.ctx, buyerID, orderID, "key-2", payments.CreatePaymentInput{Method: payments.MethodBoleto})

	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("error = %v, want ErrInvalidStatusTransition", err)
	}
	if s.r.orders[orderID].Status != orders.OrderStatusPaid || len(s.gateway.Requests) != 1 {
		t.Errorf("order = %s with %d charges, want paid by the first charge only", s.r.orders[orderID].Status, len(s.gateway.Requests))
	}
}

func TestCreatePaymentReplacesAttemptWithoutCharge(t *testing.T) {
	s := newSetup(t)
	s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.r.payments[0].TransactionID = nil

	s.pay(t, "key-2", payments.CreatePaymentInput{Method: payments.MethodPix})

	if p := s.r.payments[0]; p.Status != payments.StatusFailed || *p.FailureReason != payments.FailureGatewayUnavailable {
		t.Errorf("first payment = %+v, want failed", p)
	}
}

func TestListPaymentsChecksPendingCharges(t *testing.T) {
	s := newSetup(t)
	refused := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: card(fake.RefusedCardNumber)})
	pix := s.pay(t, "key-2", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.gateway.Confirm(*pix.TransactionID)

	list, err := s.s.ListPayments(s.ctx, buyerID, orderID)

	if err != nil || len(list) != 2 {
		t.Fatalf("ListPayments = %v, %v", list, err)
	}
	if list[0].ID != pix.ID || list[0].Status != payments.StatusCaptured || list[1].ID != refused.ID {
		t.Errorf("list = %+v, %+v, want the captured Pix first", list[0], list[1])
	}
	if s.r.orders[orderID].Status != orders.OrderStatusPaid {
		t.Error("the order was not marked as paid")
	}
	if s.r.changes[len(s.r.changes)-1].Source != payments.SourcePoll {
		t.Errorf("source = %q, want poll", s.r.changes[len(s.r.changes)-1].Source)
	}
}

func TestListPaymentsGatewayUnavailable(t *testing.T) {
	s := newSetup(t)
	s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.gateway.Unavailable = true

	list, err := s.s.ListPayments(s.ctx, buyerID, orderID)

	if err != nil || len(list) != 1 || list[0].Status != payments.StatusPending {
		t.Errorf("ListPayments = %v, %v, want the stored pending payment", list, err)
	}
}

func TestListPaymentsFetchesMissingPixCode(t *testing.T) {
	s := newSetup(t)
	s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.r.payments[0].Details.PixPayload = ""

	list, err := s.s.ListPayments(s.ctx, buyerID, orderID)

	if err != nil || list[0].Details.PixPayload == "" {
		t.Errorf("ListPayments = %+v, %v, want the Pix code fetched again", list[0], err)
	}
}

func TestListPaymentsOfAnotherBuyer(t *testing.T) {
	s := newSetup(t)
	if _, err := s.s.ListPayments(s.ctx, otherID, orderID); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("error = %v, want ErrOrderNotFound", err)
	}
}

func TestApplyPaymentStatus(t *testing.T) {
	s := newSetup(t)
	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.gateway.Confirm(*pix.TransactionID)

	for range 2 {
		if err := s.s.ApplyPaymentStatus(s.ctx, *pix.TransactionID, payments.SourceWebhook); err != nil {
			t.Fatalf("ApplyPaymentStatus: %v", err)
		}
	}

	if len(s.r.changes) != 1 || s.r.changes[0].To != payments.StatusCaptured || s.r.changes[0].Source != payments.SourceWebhook {
		t.Errorf("changes = %+v, want one capture from the webhook", s.r.changes)
	}
	if len(s.r.paid) != 1 {
		t.Errorf("order paid %d times, want once", len(s.r.paid))
	}
}

func TestApplyPaymentStatusOverdueThenPaidLate(t *testing.T) {
	s := newSetup(t)
	boleto := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodBoleto})

	s.gateway.Expire(*boleto.TransactionID)
	if err := s.s.ApplyPaymentStatus(s.ctx, *boleto.TransactionID, payments.SourcePoll); err != nil {
		t.Fatalf("ApplyPaymentStatus: %v", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusFailed || *p.FailureReason != payments.FailureOverdue {
		t.Errorf("payment = %+v, want failed as overdue", p)
	}

	s.gateway.Confirm(*boleto.TransactionID)
	if err := s.s.ApplyPaymentStatus(s.ctx, *boleto.TransactionID, payments.SourceWebhook); err != nil {
		t.Fatalf("ApplyPaymentStatus: %v", err)
	}
	if s.r.payments[0].Status != payments.StatusCaptured || s.r.orders[orderID].Status != orders.OrderStatusPaid {
		t.Error("a boleto paid late was not captured")
	}
}

func TestApplyPaymentStatusIgnoresOutOfOrderStatus(t *testing.T) {
	s := newSetup(t)
	payment := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: card(visaNumber)})
	changes := len(s.r.changes)

	s.gateway.Expire(*payment.TransactionID)
	if err := s.s.ApplyPaymentStatus(s.ctx, *payment.TransactionID, payments.SourceWebhook); err != nil {
		t.Fatalf("ApplyPaymentStatus: %v", err)
	}
	if len(s.r.changes) != changes || s.r.payments[0].Status != payments.StatusCaptured {
		t.Errorf("payment = %s after %d changes, want still captured", s.r.payments[0].Status, len(s.r.changes))
	}
}

func TestApplyPaymentStatusUnknownCharge(t *testing.T) {
	s := newSetup(t)
	if err := s.s.ApplyPaymentStatus(s.ctx, "pay_unknown", payments.SourceWebhook); !errors.Is(err, app.ErrPaymentNotFound) {
		t.Errorf("error = %v, want ErrPaymentNotFound", err)
	}

	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.gateway.Unavailable = true
	if err := s.s.ApplyPaymentStatus(s.ctx, *pix.TransactionID, payments.SourceWebhook); !errors.Is(err, app.ErrGatewayUnavailable) {
		t.Errorf("gateway down: error = %v, want ErrGatewayUnavailable", err)
	}
}

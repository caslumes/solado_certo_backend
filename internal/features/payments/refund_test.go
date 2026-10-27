package payments_test

import (
	"context"
	"errors"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

func (f *fakeRepository) GetPaymentsToRefund(_ context.Context) ([]*payments.PaymentEntity, error) {
	captured := map[string]bool{}
	return f.filter(func(p *payments.PaymentEntity) bool {
		earlier := captured[p.OrderID]
		if p.Status == payments.StatusCaptured || p.Status == payments.StatusRefunded {
			captured[p.OrderID] = true
		}
		return p.Status == payments.StatusCaptured && p.TransactionID != nil && p.Details.RefundAttempts < payments.MaxRefundAttempts &&
			(p.Details.RefundRequestedAt != nil || f.orders[p.OrderID].Status == orders.OrderStatusCancelled || earlier)
	})
}

func (f *fakeRepository) RecordRefund(_ context.Context, record payments.RefundRecord) error {
	for _, p := range f.payments {
		if p.ID != record.PaymentID || p.Status != payments.StatusCaptured {
			continue
		}
		if record.Error != "" {
			p.Details.RefundError = record.Error
			p.Details.RefundAttempts = record.Attempts
		} else {
			p.Details.RefundRequestedAt = record.Details.RefundRequestedAt
			p.Details.RefundURL = record.Details.RefundURL
		}
		f.refunds = append(f.refunds, record)
		return nil
	}
	return app.ErrPaymentNotFound
}

type refunder interface {
	RefundOrder(ctx context.Context, orderID string) error
}

func (s setup) paidThenCancelled(t *testing.T, input payments.CreatePaymentInput) *payments.PaymentEntity {
	t.Helper()
	payment := s.pay(t, "key-1", input)
	if payment.Status == payments.StatusPending {
		s.gateway.Confirm(*payment.TransactionID)
		if err := s.s.ApplyPaymentStatus(s.ctx, *payment.TransactionID, payments.SourceWebhook); err != nil {
			t.Fatalf("ApplyPaymentStatus: %v", err)
		}
	}
	if s.r.orders[orderID].Status != orders.OrderStatusPaid {
		t.Fatalf("order = %s, want paid", s.r.orders[orderID].Status)
	}
	s.r.orders[orderID].Status = orders.OrderStatusCancelled
	return payment
}

func (s setup) refundOrder(id string) error {
	return s.s.(refunder).RefundOrder(s.ctx, id)
}

func TestRefundOrderPaidByCard(t *testing.T) {
	s := newSetup(t)
	payment := s.paidThenCancelled(t, payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: card(visaNumber)})

	if err := s.refundOrder(orderID); err != nil {
		t.Fatalf("RefundOrder: %v", err)
	}

	refunded := s.r.payments[0]
	if refunded.Status != payments.StatusRefunded || refunded.Details.RefundRequestedAt == nil {
		t.Errorf("payment = %s, %+v, want refunded", refunded.Status, refunded.Details)
	}
	if last := s.r.changes[len(s.r.changes)-1]; last.To != payments.StatusRefunded || last.Source != payments.SourceRefund {
		t.Errorf("last change = %+v, want refunded by the system", last)
	}
	if s.chargeStatus(t, *payment.TransactionID) != gateway.StatusRefunded {
		t.Error("the gateway didn't refund the charge")
	}

	if err := s.refundOrder(orderID); err != nil {
		t.Errorf("RefundOrder again: %v", err)
	}
}

func TestRefundOrderPaidByBoleto(t *testing.T) {
	s := newSetup(t)
	payment := s.paidThenCancelled(t, payments.CreatePaymentInput{Method: payments.MethodBoleto})

	if err := s.refundOrder(orderID); err != nil {
		t.Fatalf("RefundOrder: %v", err)
	}
	requested := s.r.payments[0]
	if requested.Status != payments.StatusCaptured || requested.Details.RefundRequestedAt == nil || requested.Details.RefundURL == "" {
		t.Fatalf("payment = %s, %+v, want captured with the refund URL", requested.Status, requested.Details)
	}
	if output := requested.ToOutput(); output.RefundURL == nil || output.RefundRequestedAt == nil {
		t.Errorf("output = %+v, want the refund URL for the app", output)
	}
	if len(s.r.refunds) != 1 || s.r.refunds[0].Error != "" {
		t.Errorf("refund records = %+v, want one request", s.r.refunds)
	}

	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}
	if s.r.payments[0].Status != payments.StatusCaptured {
		t.Error("refunded before the buyer gave a bank account")
	}

	s.gateway.CompleteRefund(*payment.TransactionID)
	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}
	if s.r.payments[0].Status != payments.StatusRefunded {
		t.Errorf("payment = %s, want refunded", s.r.payments[0].Status)
	}
	if last := s.r.changes[len(s.r.changes)-1]; last.Source != payments.SourcePoll {
		t.Errorf("source = %q, want poll", last.Source)
	}
}

func TestRefundOrderWhenTheGatewayIsDown(t *testing.T) {
	s := newSetup(t)
	s.paidThenCancelled(t, payments.CreatePaymentInput{Method: payments.MethodPix})
	s.gateway.Unavailable = true

	if err := s.refundOrder(orderID); !errors.Is(err, app.ErrGatewayUnavailable) {
		t.Fatalf("err = %v, want gateway unavailable", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusCaptured || p.Details.RefundRequestedAt != nil || len(s.r.refunds) != 0 {
		t.Errorf("payment = %s, %+v, want unchanged", p.Status, p.Details)
	}

	s.gateway.Unavailable = false
	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusRefunded {
		t.Errorf("payment = %s, want refunded by the poller", p.Status)
	}
}

func TestRefundRefusedThenAccepted(t *testing.T) {
	s := newSetup(t)
	s.paidThenCancelled(t, payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: card(visaNumber)})
	s.gateway.RefuseRefunds = true

	if err := s.refundOrder(orderID); err == nil || errors.Is(err, app.ErrGatewayUnavailable) {
		t.Fatalf("err = %v, want the refusal", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusCaptured || p.Details.RefundError == "" || p.Details.RefundAttempts != 1 {
		t.Errorf("payment = %s, %+v, want captured with the first refusal", p.Status, p.Details)
	}

	s.gateway.RefuseRefunds = false
	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusRefunded {
		t.Errorf("payment = %s, want refunded on the next round", p.Status)
	}
}

func TestRefundRefusedUntilTheLimit(t *testing.T) {
	s := newSetup(t)
	s.paidThenCancelled(t, payments.CreatePaymentInput{Method: payments.MethodPix})
	s.gateway.RefuseRefunds = true

	_ = s.refundOrder(orderID)
	for range payments.MaxRefundAttempts + 2 {
		if err := s.s.CheckPayments(s.ctx); err != nil {
			t.Fatalf("CheckPayments: %v", err)
		}
	}

	if len(s.r.refunds) != payments.MaxRefundAttempts {
		t.Errorf("refusals recorded = %d, want %d", len(s.r.refunds), payments.MaxRefundAttempts)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusCaptured || p.Details.RefundAttempts != payments.MaxRefundAttempts {
		t.Errorf("payment = %s, %+v, want captured after the last attempt", p.Status, p.Details)
	}
	if err := s.refundOrder(orderID); err != nil {
		t.Errorf("RefundOrder after the limit = %v, want nothing asked", err)
	}
}

func TestRefundAlreadyMadeInTheGateway(t *testing.T) {
	s := newSetup(t)
	payment := s.paidThenCancelled(t, payments.CreatePaymentInput{Method: payments.MethodPix})
	if _, err := s.gateway.Refund(s.ctx, *payment.TransactionID, gateway.MethodPix); err != nil {
		t.Fatalf("Refund: %v", err)
	}
	s.gateway.RefuseRefunds = true

	if err := s.refundOrder(orderID); err != nil {
		t.Fatalf("RefundOrder: %v", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusRefunded || p.Details.RefundError != "" {
		t.Errorf("payment = %s, %+v, want refunded", p.Status, p.Details)
	}
}

func TestRefundOrderLeavesOrdersNotCancelled(t *testing.T) {
	s := newSetup(t)
	s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodCreditCard, Card: card(visaNumber)})

	if err := s.refundOrder(orderID); err != nil {
		t.Fatalf("RefundOrder: %v", err)
	}
	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusCaptured {
		t.Errorf("payment of a paid order = %s, want captured", p.Status)
	}

	if err := s.refundOrder(unknownID); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("unknown order: err = %v", err)
	}
}

func TestCheckPaymentsRefundsCaptureAfterClose(t *testing.T) {
	s := newSetup(t)
	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.r.orders[orderID].Status = orders.OrderStatusCancelled
	s.gateway.Confirm(*pix.TransactionID)
	if err := s.s.ApplyPaymentStatus(s.ctx, *pix.TransactionID, payments.SourceWebhook); err != nil {
		t.Fatalf("ApplyPaymentStatus: %v", err)
	}

	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusRefunded {
		t.Errorf("payment = %s, want refunded", p.Status)
	}
}

func TestCheckPaymentsRefundsSecondCapture(t *testing.T) {
	s := newSetup(t)
	first := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodBoleto})
	s.gateway.Confirm(*first.TransactionID)
	if err := s.s.ApplyPaymentStatus(s.ctx, *first.TransactionID, payments.SourceWebhook); err != nil {
		t.Fatalf("ApplyPaymentStatus: %v", err)
	}
	charge, err := s.gateway.CreateCharge(s.ctx, gateway.ChargeRequest{CustomerID: "cus_1", Method: gateway.MethodPix, Amount: "115.00"})
	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}
	s.gateway.Confirm(charge.ID)
	second := *first
	second.ID, second.TransactionID, second.Method = "payment-2", &charge.ID, payments.MethodPix
	s.r.payments = append(s.r.payments, &second)

	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}
	if s.r.payments[0].Status != payments.StatusCaptured || s.r.payments[1].Status != payments.StatusRefunded {
		t.Errorf("payments = %s, %s, want the first kept and the second refunded", s.r.payments[0].Status, s.r.payments[1].Status)
	}
}

func TestRefundWebhook(t *testing.T) {
	s := newSetup(t)
	payment := s.paidThenCancelled(t, payments.CreatePaymentInput{Method: payments.MethodBoleto})
	if err := s.refundOrder(orderID); err != nil {
		t.Fatalf("RefundOrder: %v", err)
	}

	s.gateway.CompleteRefund(*payment.TransactionID)
	if err := s.s.ApplyPaymentStatus(s.ctx, *payment.TransactionID, payments.SourceWebhook); err != nil {
		t.Fatalf("ApplyPaymentStatus: %v", err)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusRefunded || p.Details.RefundURL == "" {
		t.Errorf("payment = %s, %+v, want refunded, keeping the refund URL", p.Status, p.Details)
	}
}

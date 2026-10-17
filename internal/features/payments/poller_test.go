package payments_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

// The order was placed on Oct 15 (Brazil); it is due on Oct 18 and expires when that day ends.
var afterWindow = time.Date(2026, 10, 19, 0, 0, 0, 0, time.FixedZone("BRT", -3*60*60))

func (s setup) chargeStatus(t *testing.T, id string) gateway.Status {
	t.Helper()
	charge, err := s.gateway.GetCharge(s.ctx, id)
	if err != nil {
		t.Fatalf("GetCharge: %v", err)
	}
	return charge.Status
}

func TestCheckPaymentsAppliesPendingCharges(t *testing.T) {
	s := newSetup(t)
	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.gateway.Confirm(*pix.TransactionID)

	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}

	if s.r.payments[0].Status != payments.StatusCaptured || s.r.orders[orderID].Status != orders.OrderStatusPaid {
		t.Errorf("payment %s, order %s, want captured and paid", s.r.payments[0].Status, s.r.orders[orderID].Status)
	}
	if last := s.r.changes[len(s.r.changes)-1]; last.Source != payments.SourcePoll {
		t.Errorf("source = %q, want poll", last.Source)
	}
	if len(s.r.expired) != 0 {
		t.Errorf("expired = %v, want none", s.r.expired)
	}
}

func TestCheckPaymentsExpiresOrderAndCancelsItsCharge(t *testing.T) {
	s := newSetup(t)
	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.s.SetNow(func() time.Time { return afterWindow })

	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}

	if len(s.r.expired) != 1 || s.r.expired[0] != orderID {
		t.Errorf("expired = %v, want the order awaiting payment", s.r.expired)
	}
	if p := s.r.payments[0]; p.Status != payments.StatusCancelled {
		t.Errorf("payment = %s, want cancelled", p.Status)
	}
	if last := s.r.changes[len(s.r.changes)-1]; last.Source != payments.SourceOrderClosed {
		t.Errorf("source = %q, want order_closed", last.Source)
	}
	if status := s.chargeStatus(t, *pix.TransactionID); status != gateway.StatusCancelled {
		t.Errorf("charge = %s, want cancelled at the gateway", status)
	}
}

func TestCheckPaymentsKeepsOrdersInsideTheWindow(t *testing.T) {
	s := newSetup(t)
	s.s.SetNow(func() time.Time { return afterWindow.Add(-time.Second) })

	if err := s.s.CheckPayments(s.ctx); err != nil || len(s.r.expired) != 0 {
		t.Errorf("CheckPayments = %v, expired = %v, want none", err, s.r.expired)
	}
}

func TestCheckPaymentsAppliesAPaymentMadeBeforeTheDeadline(t *testing.T) {
	s := newSetup(t)
	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.gateway.Confirm(*pix.TransactionID)
	s.s.SetNow(func() time.Time { return afterWindow })

	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}

	if s.r.orders[orderID].Status != orders.OrderStatusPaid || len(s.r.expired) != 0 {
		t.Errorf("order = %s, expired = %v, want paid and not expired", s.r.orders[orderID].Status, s.r.expired)
	}
}

func TestCheckPaymentsCancelsChargesOfCancelledOrders(t *testing.T) {
	s := newSetup(t)
	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.r.orders[orderID].Status = orders.OrderStatusCancelled
	// Attempts whose gateway call never returned: an old one is abandoned, a young one may still get its charge.
	abandoned := &payments.PaymentEntity{ID: "payment-old", OrderID: orderID, Status: payments.StatusPending, CreatedAt: orderCreated}
	young := &payments.PaymentEntity{ID: "payment-young", OrderID: orderID, Status: payments.StatusPending, CreatedAt: orderCreated.Add(time.Hour - time.Minute)}
	s.r.payments = append(s.r.payments, abandoned, young)

	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}

	if s.r.payments[0].Status != payments.StatusCancelled || s.chargeStatus(t, *pix.TransactionID) != gateway.StatusCancelled {
		t.Error("the Pix of the cancelled order was not cancelled")
	}
	if abandoned.Status != payments.StatusCancelled || young.Status != payments.StatusPending {
		t.Errorf("abandoned = %s, young = %s, want cancelled and pending", abandoned.Status, young.Status)
	}
}

func TestCheckPaymentsRetriesCancellingWhenTheGatewayIsDown(t *testing.T) {
	s := newSetup(t)
	pix := s.pay(t, "key-1", payments.CreatePaymentInput{Method: payments.MethodPix})
	s.s.SetNow(func() time.Time { return afterWindow })
	s.gateway.Unavailable = true

	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}
	if len(s.r.expired) != 1 || s.r.payments[0].Status != payments.StatusPending {
		t.Fatalf("expired = %v, payment = %s, want the order expired and the payment still pending", s.r.expired, s.r.payments[0].Status)
	}

	s.gateway.Unavailable = false
	if err := s.s.CheckPayments(s.ctx); err != nil {
		t.Fatalf("CheckPayments: %v", err)
	}
	if s.r.payments[0].Status != payments.StatusCancelled || s.chargeStatus(t, *pix.TransactionID) != gateway.StatusCancelled {
		t.Error("the charge was not cancelled in the next round")
	}
}

func TestCheckPaymentsRepositoryError(t *testing.T) {
	s := newSetup(t)
	s.r.err = errors.New("database down")

	if err := s.s.CheckPayments(s.ctx); !errors.Is(err, s.r.err) {
		t.Errorf("error = %v, want the repository's", err)
	}
}

type countingChecker struct {
	calls  int
	cancel context.CancelFunc
}

func (c *countingChecker) CheckPayments(context.Context) error {
	c.calls++
	if c.calls == 3 {
		c.cancel()
	}
	return errors.New("ignored")
}

func TestRunPollerRunsUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	checker := &countingChecker{cancel: cancel}

	done := make(chan struct{})
	go func() {
		payments.RunPoller(ctx, checker, time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunPoller didn't stop")
	}
	if checker.calls != 3 {
		t.Errorf("calls = %d, want 3", checker.calls)
	}
}

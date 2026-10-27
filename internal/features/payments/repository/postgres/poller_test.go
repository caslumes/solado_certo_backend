package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
)

func ids(list []*payments.PaymentEntity) []string {
	out := make([]string, len(list))
	for i, p := range list {
		out[i] = p.ID
	}
	return out
}

func TestGetPendingPayments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	withoutCharge := f.start(t, "key-1", payments.MethodPix, "115.00", "5.00")
	f.exec(t, "UPDATE payments SET status = 'failed' WHERE id = $1", withoutCharge.ID)
	charged := f.start(t, "key-2", payments.MethodBoleto, "120.00", "0.00")
	if err := f.r.AttachCharge(ctx, charged.ID, "pay_1", payments.PaymentDetails{}); err != nil {
		t.Fatalf("AttachCharge: %v", err)
	}

	pending, err := f.r.GetPendingPayments(ctx)

	if err != nil || len(pending) != 1 || pending[0].ID != charged.ID || *pending[0].TransactionID != "pay_1" {
		t.Errorf("GetPendingPayments = %v, %v, want only the pending payment with a charge", ids(pending), err)
	}
}

func (f fixture) insertPending(t *testing.T, key string, transactionID string, age time.Duration) string {
	t.Helper()
	var transaction *string
	if transactionID != "" {
		transaction = &transactionID
	}
	var id string
	err := f.pool.QueryRow(
		context.Background(),
		"INSERT INTO payments (order_id, gateway, idempotency_key, status, method, amount, gateway_transaction_id, gateway_payload, created_at, updated_at) "+
			"VALUES ($1, 'fake', $2, 'pending', 'pix', 115.00, $3, '{}', $4, $4) RETURNING id",
		f.orderID, key, transaction, time.Now().UTC().Add(-age),
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert payment: %v", err)
	}
	return id
}

func TestGetClosedOrderPayments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	charged := f.insertPending(t, "key-1", "pay_1", time.Hour)
	abandoned := f.insertPending(t, "key-2", "", 10*time.Minute)
	f.insertPending(t, "key-3", "", time.Minute)
	startedBefore := time.Now().Add(-5 * time.Minute)

	open, err := f.r.GetClosedOrderPayments(ctx, startedBefore)
	if err != nil || len(open) != 0 {
		t.Fatalf("order awaiting payment: GetClosedOrderPayments = %v, %v, want none", ids(open), err)
	}

	f.exec(t, "UPDATE orders SET status = 'cancelled' WHERE id = $1", f.orderID)
	closed, err := f.r.GetClosedOrderPayments(ctx, startedBefore)
	if err != nil || len(closed) != 2 || closed[0].ID != charged || closed[1].ID != abandoned {
		t.Errorf("GetClosedOrderPayments = %v, %v, want the charged attempt and the abandoned one", ids(closed), err)
	}
}

func TestExpireOrder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	seller := testutil.InsertUser(t, f.pool)
	category := testutil.InsertCategory(t, f.pool, nil)
	ad := testutil.InsertAd(t, f.pool, testutil.Ad{SellerID: seller.ID, CategoryID: category.ID, Stock: 1})
	f.exec(t, "INSERT INTO order_items (order_id, advertisement_id, seller_id, quantity, unit_price, subtotal, product_snapshot, created_at) "+
		"VALUES ($1, $2, $3, 2, 50, 100, '{}', now())", f.orderID, ad.ID, seller.ID)
	f.exec(t, "UPDATE orders SET created_at = now() - interval '3 days 1 minute' WHERE id = $1", f.orderID)
	recent := f.insertOrder(t, "SC-20261015-BBBBBB")
	paid := f.insertOrder(t, "SC-20261015-CCCCCC")
	f.exec(t, "UPDATE orders SET status = 'paid', created_at = now() - interval '4 days' WHERE id = $1", paid)
	cutoff := time.Now().Add(-3 * 24 * time.Hour)

	expired, err := f.r.GetExpiredOrderIDs(ctx, cutoff)
	if err != nil || len(expired) != 1 || expired[0] != f.orderID {
		t.Fatalf("GetExpiredOrderIDs = %v, %v, want the old order awaiting payment", expired, err)
	}

	changed, err := f.r.ExpireOrder(ctx, f.orderID, cutoff)
	if err != nil || !changed {
		t.Fatalf("ExpireOrder = %v, %v", changed, err)
	}
	if status, _, _, _ := f.orderTotals(t); status != "cancelled" {
		t.Errorf("status = %s, want cancelled", status)
	}
	if n := f.count(t, "SELECT stock FROM advertisements WHERE id = $1", ad.ID); n != 3 {
		t.Errorf("stock = %d, want the 2 units back", n)
	}
	history := "SELECT count(*) FROM order_status_history WHERE order_id = $1 AND previous_status = 'awaiting_payment' " +
		"AND new_status = 'cancelled' AND reason = 'payment_expired' AND changed_by IS NULL"
	if n := f.count(t, history, f.orderID); n != 1 {
		t.Errorf("history rows = %d, want one system change", n)
	}
	audit := "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'order.status_changed' AND user_id IS NULL " +
		"AND context->>'reason' = 'payment_expired' AND (context->>'stock_released')::boolean"
	if n := f.count(t, audit, f.orderID); n != 1 {
		t.Errorf("audit rows = %d, want one", n)
	}

	for _, id := range []string{f.orderID, recent, paid} {
		if changed, err := f.r.ExpireOrder(ctx, id, cutoff); err != nil || changed {
			t.Errorf("ExpireOrder(%s) = %v, %v, want no change", id, changed, err)
		}
	}
	if n := f.count(t, "SELECT stock FROM advertisements WHERE id = $1", ad.ID); n != 3 {
		t.Errorf("stock = %d, want it released once", n)
	}
}

func (f fixture) capturedPayment(t *testing.T, key string, age time.Duration) string {
	t.Helper()
	id := f.insertPending(t, key, "pay_"+key, age)
	f.exec(t, "UPDATE payments SET status = 'captured', captured_at = $2 WHERE id = $1", id, time.Now().UTC().Add(-age))
	return id
}

func TestGetPaymentsToRefund(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first := f.capturedPayment(t, "key-1", time.Hour)
	second := f.capturedPayment(t, "key-2", time.Minute)
	f.exec(t, "UPDATE orders SET status = 'paid' WHERE id = $1", f.orderID)

	found, err := f.r.GetPaymentsToRefund(ctx)
	if err != nil || len(found) != 1 || found[0].ID != second {
		t.Fatalf("paid order: GetPaymentsToRefund = %v, %v, want the second capture only", ids(found), err)
	}

	f.exec(t, "UPDATE orders SET status = 'cancelled' WHERE id = $1", f.orderID)
	found, err = f.r.GetPaymentsToRefund(ctx)
	if err != nil || len(found) != 2 || found[0].ID != first || found[1].ID != second {
		t.Fatalf("cancelled order: GetPaymentsToRefund = %v, %v, want both, oldest first", ids(found), err)
	}

	if err := f.r.RecordRefund(ctx, payments.RefundRecord{PaymentID: first, Error: "saldo insuficiente", Attempts: 1}); err != nil {
		t.Fatalf("RecordRefund: %v", err)
	}
	f.exec(t, "UPDATE payments SET status = 'refunded' WHERE id = $1", second)
	found, err = f.r.GetPaymentsToRefund(ctx)
	if err != nil || len(found) != 1 || found[0].ID != first {
		t.Errorf("refused once: GetPaymentsToRefund = %v, %v, want it asked again", ids(found), err)
	}

	if err := f.r.RecordRefund(ctx, payments.RefundRecord{PaymentID: first, Error: "saldo insuficiente", Attempts: payments.MaxRefundAttempts}); err != nil {
		t.Fatalf("RecordRefund: %v", err)
	}
	found, err = f.r.GetPaymentsToRefund(ctx)
	if err != nil || len(found) != 0 {
		t.Errorf("refused until the limit: GetPaymentsToRefund = %v, %v, want none", ids(found), err)
	}
}

func TestGetPaymentsToRefundRequested(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	payment := f.capturedPayment(t, "key-1", time.Hour)
	f.exec(t, "UPDATE orders SET status = 'paid' WHERE id = $1", f.orderID)
	requestedAt := time.Now().UTC()

	if err := f.r.RecordRefund(ctx, payments.RefundRecord{PaymentID: payment, Details: payments.PaymentDetails{RefundRequestedAt: &requestedAt, RefundURL: "https://r"}}); err != nil {
		t.Fatalf("RecordRefund: %v", err)
	}

	found, err := f.r.GetPaymentsToRefund(ctx)
	if err != nil || len(found) != 1 || found[0].Details.RefundRequestedAt == nil || found[0].Details.RefundURL != "https://r" {
		t.Errorf("GetPaymentsToRefund = %+v, %v, want the payment with its refund", found, err)
	}
}

func TestRecordRefund(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	payment := f.capturedPayment(t, "key-1", time.Hour)
	f.exec(t, "UPDATE payments SET gateway_payload = '{\"boleto_url\": \"https://b\"}' WHERE id = $1", payment)
	requestedAt := time.Now().UTC()

	err := f.r.RecordRefund(ctx, payments.RefundRecord{PaymentID: payment, Details: payments.PaymentDetails{RefundRequestedAt: &requestedAt, RefundURL: "https://r"}})
	if err != nil {
		t.Fatalf("RecordRefund: %v", err)
	}
	got, _ := f.r.GetPayment(ctx, payment)
	if got.Status != payments.StatusCaptured || got.Details.RefundURL != "https://r" || got.Details.BoletoURL != "https://b" {
		t.Errorf("payment = %s, %+v, want captured with the boleto and refund links", got.Status, got.Details)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'payment.refund_requested' AND context->>'order_id' = $2", payment, f.orderID); n != 1 {
		t.Errorf("request audit rows = %d, want 1", n)
	}

	if err := f.r.RecordRefund(ctx, payments.RefundRecord{PaymentID: payment, Error: "saldo insuficiente", Attempts: payments.MaxRefundAttempts}); err != nil {
		t.Fatalf("RecordRefund refusal: %v", err)
	}
	got, _ = f.r.GetPayment(ctx, payment)
	if got.Details.RefundError != "saldo insuficiente" || got.Details.RefundAttempts != payments.MaxRefundAttempts || got.Details.RefundURL != "https://r" {
		t.Errorf("details = %+v, want the refusal kept with the earlier fields", got.Details)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'payment.refund_refused' AND context->>'error' = 'saldo insuficiente' AND (context->>'given_up')::boolean", payment); n != 1 {
		t.Errorf("refusal audit rows = %d, want 1", n)
	}

	f.exec(t, "UPDATE payments SET status = 'refunded' WHERE id = $1", payment)
	if err := f.r.RecordRefund(ctx, payments.RefundRecord{PaymentID: payment}); !errors.Is(err, app.ErrPaymentNotFound) {
		t.Errorf("refunded payment: err = %v, want not found", err)
	}
}

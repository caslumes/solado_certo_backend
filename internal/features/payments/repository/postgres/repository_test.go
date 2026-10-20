package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool    *pgxpool.Pool
	r       *postgresRepository
	buyer   testutil.User
	orderID string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testutil.DB(t)
	f := fixture{pool: pool, r: NewPostgresRepository(pool), buyer: testutil.InsertUser(t, pool)}
	f.orderID = f.insertOrder(t, "SC-20261015-AAAAAA")
	return f
}

// insertOrder adds an order of 100.00 plus 20.00 shipping, awaiting payment.
func (f fixture) insertOrder(t *testing.T, number string) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	var checkoutID, orderID string
	if err := f.pool.QueryRow(ctx, "INSERT INTO checkouts (buyer_id, idempotency_key, created_at) VALUES ($1, $2, $3) RETURNING id", f.buyer.ID, number, now).Scan(&checkoutID); err != nil {
		t.Fatalf("insert checkout: %v", err)
	}
	err := f.pool.QueryRow(
		ctx,
		"INSERT INTO orders (buyer_id, checkout_id, number, status, subtotal, shipping_total, discount_total, total, shipping_address, created_at, updated_at) "+
			"VALUES ($1, $2, $3, 'awaiting_payment', 100.00, 20.00, 0, 120.00, $4, $5, $5) RETURNING id",
		f.buyer.ID, checkoutID, number, `{"receiver": "Maria", "zip_code": "13000000", "number": "10"}`, now,
	).Scan(&orderID)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	return orderID
}

func (f fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (f fixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func (f fixture) start(t *testing.T, key string, method payments.Method, amount string, discount string) *payments.PaymentEntity {
	t.Helper()
	payment, created, err := f.r.StartPayment(context.Background(), payments.NewPayment{
		OrderID:        f.orderID,
		CreatedBy:      f.buyer.ID,
		IdempotencyKey: key,
		Gateway:        "fake",
		Method:         method,
		Amount:         amount,
		DiscountTotal:  discount,
		DueDate:        time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || !created {
		t.Fatalf("StartPayment = %v, %v, %v", payment, created, err)
	}
	return payment
}

func (f fixture) orderTotals(t *testing.T) (status string, discount string, total string, paidAt *time.Time) {
	t.Helper()
	err := f.pool.QueryRow(context.Background(), "SELECT status, discount_total::text, total::text, paid_at FROM orders WHERE id = $1", f.orderID).
		Scan(&status, &discount, &total, &paidAt)
	if err != nil {
		t.Fatalf("read order: %v", err)
	}
	return status, discount, total, paidAt
}

func TestGetOrderAndBuyer(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	order, err := f.r.GetOrder(ctx, f.orderID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if order.BuyerID != f.buyer.ID || order.Status != orders.OrderStatusAwaitingPayment || order.Subtotal != "100.00" ||
		order.ShippingTotal != "20.00" || order.Total != "120.00" || order.PostalCode != "13000000" || order.AddressNumber != "10" {
		t.Errorf("order = %+v", order)
	}
	if _, err := f.r.GetOrder(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("unknown order: error = %v", err)
	}

	buyer, err := f.r.GetBuyer(ctx, f.buyer.ID)
	if err != nil || buyer.Email != f.buyer.Email || buyer.TaxID != nil {
		t.Errorf("GetBuyer = %+v, %v", buyer, err)
	}
	if _, err := f.r.GetBuyer(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, app.ErrUserNotFound) {
		t.Errorf("unknown buyer: error = %v", err)
	}
}

func TestSetTaxID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other := testutil.InsertUser(t, f.pool)

	if err := f.r.SetTaxID(ctx, f.buyer.ID, "52998224725"); err != nil {
		t.Fatalf("SetTaxID: %v", err)
	}
	if err := f.r.SetTaxID(ctx, other.ID, "52998224725"); !errors.Is(err, app.ErrTaxIDInUse) {
		t.Errorf("another user's CPF: error = %v, want ErrTaxIDInUse", err)
	}
	// A saved CPF is not replaced.
	if err := f.r.SetTaxID(ctx, f.buyer.ID, "11144477735"); err != nil {
		t.Fatalf("SetTaxID: %v", err)
	}
	buyer, _ := f.r.GetBuyer(ctx, f.buyer.ID)
	if buyer.TaxID == nil || *buyer.TaxID != "52998224725" {
		t.Errorf("tax id = %v, want the first one", buyer.TaxID)
	}
}

func TestCustomerID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, found, err := f.r.GetCustomerID(ctx, f.buyer.ID, "fake"); found || err != nil {
		t.Fatalf("before saving: found = %v, error = %v", found, err)
	}
	if err := f.r.SaveCustomerID(ctx, f.buyer.ID, "fake", "cus_1"); err != nil {
		t.Fatalf("SaveCustomerID: %v", err)
	}
	if err := f.r.SaveCustomerID(ctx, f.buyer.ID, "fake", "cus_2"); err != nil {
		t.Fatalf("SaveCustomerID again: %v", err)
	}
	if id, found, err := f.r.GetCustomerID(ctx, f.buyer.ID, "fake"); id != "cus_1" || !found || err != nil {
		t.Errorf("GetCustomerID = %q, %v, %v, want the first customer", id, found, err)
	}
	if _, found, _ := f.r.GetCustomerID(ctx, f.buyer.ID, "asaas"); found {
		t.Error("a customer of one gateway was found for another")
	}
}

func TestStartPayment(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	payment := f.start(t, "key-1", payments.MethodPix, "115.00", "5.00")

	if payment.OrderID != f.orderID || payment.Status != payments.StatusPending || payment.Method != payments.MethodPix ||
		payment.Amount != "115.00" || payment.Gateway != "fake" || payment.TransactionID != nil || payment.DueDate == nil {
		t.Errorf("payment = %+v", payment)
	}
	if _, discount, total, _ := f.orderTotals(t); discount != "5.00" || total != "115.00" {
		t.Errorf("order totals = %s, %s, want the Pix discount", discount, total)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity = 'order' AND action = 'order.discount_changed' AND user_id = $1", f.buyer.ID); n != 1 {
		t.Errorf("discount audit rows = %d, want 1", n)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'payment.created' AND user_id = $2", payment.ID, f.buyer.ID); n != 1 {
		t.Errorf("payment audit rows = %d, want 1", n)
	}

	// The same key returns the payment without changing anything.
	again, created, err := f.r.StartPayment(ctx, payments.NewPayment{OrderID: f.orderID, IdempotencyKey: "key-1", Gateway: "fake", Method: payments.MethodBoleto, Amount: "120.00", DiscountTotal: "0.00"})
	if err != nil || created || again.ID != payment.ID {
		t.Errorf("repeat = %+v, %v, %v", again, created, err)
	}
	other := f.insertOrder(t, "SC-20261015-BBBBBB")
	if _, _, err := f.r.StartPayment(ctx, payments.NewPayment{OrderID: other, IdempotencyKey: "key-1", Gateway: "fake", Method: payments.MethodPix, Amount: "1.00", DiscountTotal: "0.00"}); !errors.Is(err, app.ErrIdempotencyKeyReused) {
		t.Errorf("key of another order: error = %v", err)
	}

	// While a payment is pending, another attempt is refused.
	if _, _, err := f.r.StartPayment(ctx, payments.NewPayment{OrderID: f.orderID, IdempotencyKey: "key-2", Gateway: "fake", Method: payments.MethodBoleto, Amount: "120.00", DiscountTotal: "0.00"}); !errors.Is(err, app.ErrPaymentInProgress) {
		t.Errorf("second pending attempt: error = %v, want ErrPaymentInProgress", err)
	}

	// Once it is cancelled, a boleto removes the discount.
	if _, err := f.r.ChangePaymentStatus(ctx, payments.StatusChange{PaymentID: payment.ID, To: payments.StatusCancelled, Source: payments.SourceReplaced}); err != nil {
		t.Fatalf("ChangePaymentStatus: %v", err)
	}
	f.start(t, "key-2", payments.MethodBoleto, "120.00", "0.00")
	if _, discount, total, _ := f.orderTotals(t); discount != "0.00" || total != "120.00" {
		t.Errorf("order totals = %s, %s, want no discount", discount, total)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE action = 'order.discount_changed'"); n != 2 {
		t.Errorf("discount audit rows = %d, want 2", n)
	}
}

func TestStartPaymentOrderNotAwaitingPayment(t *testing.T) {
	f := newFixture(t)
	f.exec(t, "UPDATE orders SET status = 'cancelled' WHERE id = $1", f.orderID)

	_, _, err := f.r.StartPayment(context.Background(), payments.NewPayment{OrderID: f.orderID, IdempotencyKey: "key-1", Gateway: "fake", Method: payments.MethodPix, Amount: "115.00", DiscountTotal: "5.00"})

	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("error = %v, want ErrInvalidStatusTransition", err)
	}
	if n := f.count(t, "SELECT count(*) FROM payments"); n != 0 {
		t.Errorf("payments = %d, want none", n)
	}
	if _, _, err := f.r.StartPayment(context.Background(), payments.NewPayment{OrderID: "00000000-0000-0000-0000-000000000000", IdempotencyKey: "key-2", Gateway: "fake", Method: payments.MethodPix, Amount: "1.00", DiscountTotal: "0.00"}); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("unknown order: error = %v", err)
	}
}

func TestAttachChargeAndLookups(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	payment := f.start(t, "key-1", payments.MethodPix, "115.00", "5.00")
	expires := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	if err := f.r.AttachCharge(ctx, payment.ID, "pay_1", payments.PaymentDetails{GatewayStatus: "PENDING", InvoiceURL: "https://i"}); err != nil {
		t.Fatalf("AttachCharge: %v", err)
	}
	// Details are merged, so a later Pix code keeps what was stored.
	if err := f.r.AttachCharge(ctx, payment.ID, "pay_1", payments.PaymentDetails{PixPayload: "000201", PixExpiresAt: &expires}); err != nil {
		t.Fatalf("AttachCharge: %v", err)
	}

	got, err := f.r.GetPaymentByTransaction(ctx, "fake", "pay_1")
	if err != nil || got.ID != payment.ID || *got.TransactionID != "pay_1" {
		t.Fatalf("GetPaymentByTransaction = %+v, %v", got, err)
	}
	if got.Details.GatewayStatus != "PENDING" || got.Details.InvoiceURL != "https://i" || got.Details.PixPayload != "000201" || !got.Details.PixExpiresAt.Equal(expires) {
		t.Errorf("details = %+v", got.Details)
	}
	if _, err := f.r.GetPaymentByTransaction(ctx, "asaas", "pay_1"); !errors.Is(err, app.ErrPaymentNotFound) {
		t.Errorf("other gateway: error = %v", err)
	}
	if got, err := f.r.GetPaymentByKey(ctx, "key-1"); err != nil || got.ID != payment.ID {
		t.Errorf("GetPaymentByKey = %+v, %v", got, err)
	}
	if _, err := f.r.GetPaymentByKey(ctx, "key-2"); !errors.Is(err, app.ErrPaymentNotFound) {
		t.Errorf("unknown key: error = %v", err)
	}
	if _, err := f.r.GetPayment(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, app.ErrPaymentNotFound) {
		t.Errorf("unknown payment: error = %v", err)
	}
}

func TestGetPaymentsNewestFirst(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first := f.start(t, "key-1", payments.MethodCreditCard, "120.00", "0.00")
	if _, err := f.r.ChangePaymentStatus(ctx, payments.StatusChange{PaymentID: first.ID, To: payments.StatusFailed, FailureReason: payments.FailureCardRefused, Source: payments.SourceCharge}); err != nil {
		t.Fatalf("ChangePaymentStatus: %v", err)
	}
	second := f.start(t, "key-2", payments.MethodPix, "115.00", "5.00")

	list, err := f.r.GetPayments(ctx, f.orderID)

	if err != nil || len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("GetPayments = %v, %v, want the newest first", list, err)
	}
	if list[1].FailureReason == nil || *list[1].FailureReason != payments.FailureCardRefused {
		t.Errorf("failure reason = %v", list[1].FailureReason)
	}
}

func TestChangePaymentStatusCapture(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	payment := f.start(t, "key-1", payments.MethodPix, "115.00", "5.00")
	capture := payments.StatusChange{PaymentID: payment.ID, To: payments.StatusCaptured, Source: payments.SourceWebhook, Details: payments.PaymentDetails{GatewayStatus: "RECEIVED"}}

	changed, err := f.r.ChangePaymentStatus(ctx, capture)

	if err != nil || !changed {
		t.Fatalf("ChangePaymentStatus = %v, %v", changed, err)
	}
	got, _ := f.r.GetPayment(ctx, payment.ID)
	if got.Status != payments.StatusCaptured || got.CapturedAt == nil || got.Details.GatewayStatus != "RECEIVED" {
		t.Errorf("payment = %+v", got)
	}
	if status, _, _, paidAt := f.orderTotals(t); status != "paid" || paidAt == nil {
		t.Errorf("order = %s (paid at %v), want paid", status, paidAt)
	}
	if n := f.count(t, "SELECT count(*) FROM order_status_history WHERE order_id = $1 AND previous_status = 'awaiting_payment' AND new_status = 'paid' AND reason = 'payment_captured' AND changed_by IS NULL", f.orderID); n != 1 {
		t.Errorf("history rows = %d, want 1", n)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity = 'payment' AND entity_id = $1 AND action = 'payment.status_changed' AND context->>'source' = 'webhook'", payment.ID); n != 1 {
		t.Errorf("payment audit rows = %d, want 1", n)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity = 'order' AND action = 'order.status_changed' AND context->>'payment_id' = $1", payment.ID); n != 1 {
		t.Errorf("order audit rows = %d, want 1", n)
	}

	// The same webhook again changes nothing.
	changed, err = f.r.ChangePaymentStatus(ctx, capture)
	if err != nil || changed {
		t.Errorf("repeat = %v, %v, want no change", changed, err)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'payment.status_changed'", payment.ID); n != 1 {
		t.Errorf("payment audit rows after the repeat = %d, want 1", n)
	}

	// A refund keeps the order as it is: the order was cancelled before it.
	if changed, err := f.r.ChangePaymentStatus(ctx, payments.StatusChange{PaymentID: payment.ID, To: payments.StatusRefunded, Source: payments.SourceWebhook}); err != nil || !changed {
		t.Errorf("refund = %v, %v", changed, err)
	}
	got, _ = f.r.GetPayment(ctx, payment.ID)
	if got.RefundedAt == nil || got.CapturedAt == nil {
		t.Errorf("payment = %+v, want both timestamps", got)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'order_refunded' AND data->>'order_id' = $2", f.buyer.ID, f.orderID); n != 1 {
		t.Errorf("refund notifications = %d, want 1", n)
	}
}

// addItem adds an item of the seller to the fixture's order.
func (f fixture) addItem(t *testing.T, sellerID string) {
	t.Helper()
	category := testutil.InsertCategory(t, f.pool, nil)
	ad := testutil.InsertAd(t, f.pool, testutil.Ad{SellerID: sellerID, CategoryID: category.ID, Stock: 1})
	f.exec(
		t,
		"INSERT INTO order_items (order_id, advertisement_id, seller_id, quantity, unit_price, subtotal, product_snapshot, created_at) "+
			"VALUES ($1, $2, $3, 1, 50.00, 50.00, '{}', now())",
		f.orderID, ad.ID, sellerID,
	)
}

func TestChangePaymentStatusCaptureNotifiesSellers(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first := testutil.InsertUser(t, f.pool)
	second := testutil.InsertUser(t, f.pool)
	f.addItem(t, first.ID)
	f.addItem(t, first.ID)
	f.addItem(t, second.ID)
	payment := f.start(t, "key-1", payments.MethodPix, "120.00", "0.00")
	capture := payments.StatusChange{PaymentID: payment.ID, To: payments.StatusCaptured, Source: payments.SourceWebhook}

	if changed, err := f.r.ChangePaymentStatus(ctx, capture); err != nil || !changed {
		t.Fatalf("ChangePaymentStatus = %v, %v", changed, err)
	}
	if _, err := f.r.ChangePaymentStatus(ctx, capture); err != nil {
		t.Fatalf("repeat: %v", err)
	}

	for _, seller := range []testutil.User{first, second} {
		if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'order_paid' AND data->>'order_id' = $2 AND read_at IS NULL", seller.ID, f.orderID); n != 1 {
			t.Errorf("seller %s: notifications = %d, want 1", seller.Name, n)
		}
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1", f.buyer.ID); n != 0 {
		t.Errorf("buyer notifications = %d, want 0", n)
	}
}

func TestChangePaymentStatusConcurrentCaptures(t *testing.T) {
	f := newFixture(t)
	payment := f.start(t, "key-1", payments.MethodPix, "115.00", "5.00")
	capture := payments.StatusChange{PaymentID: payment.ID, To: payments.StatusCaptured, Source: payments.SourceWebhook}

	// A webhook and the poller apply the same capture at once.
	var wg sync.WaitGroup
	results := make([]bool, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Go(func() {
			results[i], errs[i] = f.r.ChangePaymentStatus(context.Background(), capture)
		})
	}
	wg.Wait()

	if errs[0] != nil || errs[1] != nil || results[0] == results[1] {
		t.Errorf("results = %v, errors = %v, want exactly one change", results, errs)
	}
	if n := f.count(t, "SELECT count(*) FROM order_status_history WHERE order_id = $1 AND new_status = 'paid'", f.orderID); n != 1 {
		t.Errorf("history rows = %d, want 1", n)
	}
}

func TestChangePaymentStatusCaptureOfClosedOrder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	payment := f.start(t, "key-1", payments.MethodPix, "115.00", "5.00")
	f.exec(t, "UPDATE orders SET status = 'cancelled' WHERE id = $1", f.orderID)

	changed, err := f.r.ChangePaymentStatus(ctx, payments.StatusChange{PaymentID: payment.ID, To: payments.StatusCaptured, Source: payments.SourceWebhook})

	if err != nil || !changed {
		t.Fatalf("ChangePaymentStatus = %v, %v", changed, err)
	}
	if status, _, _, paidAt := f.orderTotals(t); status != "cancelled" || paidAt != nil {
		t.Errorf("order = %s, want still cancelled", status)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'order.payment_captured_after_close' AND context->>'payment_id' = $2", f.orderID, payment.ID); n != 1 {
		t.Errorf("audit rows = %d, want 1 so the money can be refunded", n)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications"); n != 0 {
		t.Errorf("notifications = %d, want none for an order that wasn't paid", n)
	}
}

func TestChangePaymentStatusNotAllowed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	payment := f.start(t, "key-1", payments.MethodBoleto, "120.00", "0.00")
	if _, err := f.r.ChangePaymentStatus(ctx, payments.StatusChange{PaymentID: payment.ID, To: payments.StatusCancelled, Source: payments.SourceReplaced}); err != nil {
		t.Fatalf("ChangePaymentStatus: %v", err)
	}

	changed, err := f.r.ChangePaymentStatus(ctx, payments.StatusChange{PaymentID: payment.ID, To: payments.StatusCaptured, Source: payments.SourceWebhook})

	if err != nil || changed {
		t.Errorf("capture of a cancelled payment = %v, %v, want no change", changed, err)
	}
	if status, _, _, _ := f.orderTotals(t); status != "awaiting_payment" {
		t.Errorf("order = %s, want still awaiting payment", status)
	}
	if _, err := f.r.ChangePaymentStatus(ctx, payments.StatusChange{PaymentID: "00000000-0000-0000-0000-000000000000", To: payments.StatusCaptured}); !errors.Is(err, app.ErrPaymentNotFound) {
		t.Errorf("unknown payment: error = %v", err)
	}
}

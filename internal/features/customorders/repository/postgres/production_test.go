package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/customorders"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	ordersdb "github.com/caslumes/solado_certo_backend/internal/features/orders/repository/postgres"
)

func (f fixture) approved(t *testing.T) string {
	t.Helper()
	id := f.create(t, f.buyer.ID)
	price := "189.90"
	accept := f.change(id, customorders.StatusRequested, customorders.ActionAccept, customorders.StatusApproved)
	accept.QuotedPrice = &price
	if err := f.r.ChangeStatus(context.Background(), accept); err != nil {
		t.Fatalf("accept: %v", err)
	}
	return id
}

func (f fixture) checkout(t *testing.T, requestID string, key string) (bool, error) {
	t.Helper()
	ctx := context.Background()
	var addressID, optionID string
	err := f.pool.QueryRow(ctx,
		"INSERT INTO addresses (user_id, label, receiver, zip_code, street, number, district, city, state, created_at, updated_at) "+
			"VALUES ($1, 'Casa', 'Maria Souza', '13000000', 'Rua A', '10', 'Centro', 'Campinas', 'SP', now(), now()) RETURNING id",
		f.buyer.ID).Scan(&addressID)
	if err != nil {
		t.Fatalf("insert address: %v", err)
	}
	err = f.pool.QueryRow(ctx,
		"INSERT INTO shipping_options (advertisement_id, name, carrier, price, delivery_days) VALUES ($1, 'PAC', 'Correios', 20.00, 5) RETURNING id",
		f.ad.ID).Scan(&optionID)
	if err != nil {
		t.Fatalf("insert shipping option: %v", err)
	}

	plan, err := orders.NewService(ordersdb.NewPostgresRepository(f.pool)).PlanCustomOrder(ctx, f.buyer.ID, orders.CustomItem{
		RequestID: requestID, AdvertisementID: f.ad.ID, AddressID: addressID, ShippingOptionID: optionID, Price: "189.90",
	})
	if err != nil {
		t.Fatalf("PlanCustomOrder: %v", err)
	}
	return f.r.Checkout(ctx, customorders.Checkout{RequestID: requestID, BuyerID: f.buyer.ID, IdempotencyKey: key, Plan: plan})
}

func (f fixture) orderOf(t *testing.T, requestID string) string {
	t.Helper()
	request, err := f.r.GetRequest(context.Background(), requestID)
	if err != nil || request.OrderID == nil {
		t.Fatalf("request %s has no order: %v", requestID, err)
	}
	return *request.OrderID
}

func (f fixture) moveOrder(t *testing.T, orderID string, userID string, from orders.OrderStatus, to orders.OrderStatus) error {
	t.Helper()
	change := orders.StatusChange{OrderID: orderID, ChangedBy: userID, From: from, To: to, Reason: "test", ReleaseStock: to == orders.OrderStatusCancelled}
	if to == orders.OrderStatusShipped {
		change.Shipment = &orders.NewShipment{Carrier: "Correios", TrackingCode: "AA123456789BR"}
	}
	return ordersdb.NewPostgresRepository(f.pool).ChangeOrderStatus(context.Background(), change)
}

func (f fixture) status(t *testing.T, requestID string) customorders.Status {
	t.Helper()
	request, err := f.r.GetRequest(context.Background(), requestID)
	if err != nil {
		t.Fatalf("GetRequest: %v", err)
	}
	return request.Status
}

func TestCustomOrderFromCheckoutToReport(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.approved(t)

	created, err := f.checkout(t, id, "key-1")

	if err != nil || !created {
		t.Fatalf("Checkout: %v, created = %v", err, created)
	}
	orderID := f.orderOf(t, id)
	if n := f.count(t, "SELECT count(*) FROM orders WHERE id = $1 AND status = 'awaiting_payment' AND subtotal = 189.90 AND total = 209.90", orderID); n != 1 {
		t.Errorf("order with the quoted price = %d rows", n)
	}
	if n := f.count(t, "SELECT count(*) FROM order_items WHERE order_id = $1 AND custom_order_request_id = $2 AND quantity = 1 AND unit_price = 189.90", orderID, id); n != 1 {
		t.Errorf("custom order item = %d rows", n)
	}
	if n := f.count(t, "SELECT stock FROM advertisements WHERE id = $1", f.ad.ID); n != 1 {
		t.Errorf("stock = %d, want 1: a custom item is made to order", n)
	}
	if f.status(t, id) != customorders.StatusApproved {
		t.Errorf("status = %s, want approved until payment", f.status(t, id))
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'custom_order.checkout' AND context->>'order_id' = $2", id, orderID); n != 1 {
		t.Errorf("checkout audit rows = %d", n)
	}

	if created, err := f.checkout(t, id, "key-1"); err != nil || created {
		t.Errorf("same key: %v, created = %v, want nothing new", err, created)
	}
	if _, err := f.checkout(t, id, "key-2"); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("second order: err = %v, want ErrInvalidStatusTransition", err)
	}
	if n := f.count(t, "SELECT count(*) FROM orders WHERE buyer_id = $1", f.buyer.ID); n != 1 {
		t.Errorf("orders = %d, want 1", n)
	}

	if err := f.moveOrder(t, orderID, "", orders.OrderStatusAwaitingPayment, orders.OrderStatusPaid); err != nil {
		t.Fatalf("pay: %v", err)
	}
	request, _ := f.r.GetRequest(ctx, id)
	if last := request.History[len(request.History)-1]; request.Status != customorders.StatusInProduction ||
		*last.PreviousStatus != customorders.StatusApproved || last.ChangedBy != nil {
		t.Errorf("after payment: status = %s, last history = %+v, want in_production by the system", request.Status, last)
	}

	if err := f.moveOrder(t, orderID, f.seller.ID, orders.OrderStatusPaid, orders.OrderStatusProcessing); err != nil {
		t.Fatalf("processing: %v", err)
	}
	err = f.moveOrder(t, orderID, f.seller.ID, orders.OrderStatusProcessing, orders.OrderStatusShipped)
	if reports, ok := errors.AsType[*app.FittingReportRequiredError](err); !ok || len(reports.ItemIDs) != 1 {
		t.Fatalf("ship before the report: err = %v, want FittingReportRequiredError", err)
	}

	key, instructions := "fitting-reports/report.pdf", "Use 2 horas no primeiro dia."
	report := f.change(id, customorders.StatusInProduction, customorders.ActionReport, customorders.StatusCompleted)
	report.ReportKey, report.AdaptationInstructions = &key, &instructions
	report.Notification = notifications.CustomOrderCompleted(f.buyer.ID, id, f.ad.Title)
	if err := f.r.ChangeStatus(ctx, report); err != nil {
		t.Fatalf("report: %v", err)
	}
	request, _ = f.r.GetRequest(ctx, id)
	if request.Status != customorders.StatusCompleted || request.FittingReport == nil || *request.AdaptationInstructions != instructions {
		t.Errorf("after the report: %+v", request.RequestEntity)
	}
	if n := f.count(t, "SELECT count(*) FROM order_documents d JOIN order_items oi ON oi.id = d.order_item_id "+
		"WHERE d.id = $1 AND d.order_id = $2 AND d.custom_order_request_id = $3 AND d.document_type = 'technical_report' AND d.file_url = $4 AND d.uploaded_by = $5",
		request.FittingReport.ID, orderID, id, key, f.seller.ID); n != 1 {
		t.Errorf("report document rows = %d", n)
	}
	if err := f.moveOrder(t, orderID, f.seller.ID, orders.OrderStatusProcessing, orders.OrderStatusShipped); err != nil {
		t.Errorf("ship after the report: %v", err)
	}
}

func TestCancellingTheOrderBeforePaymentFreesTheRequest(t *testing.T) {
	f := newFixture(t)
	id := f.approved(t)
	if _, err := f.checkout(t, id, "key-1"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	orderID := f.orderOf(t, id)

	if err := f.moveOrder(t, orderID, f.buyer.ID, orders.OrderStatusAwaitingPayment, orders.OrderStatusCancelled); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	request, _ := f.r.GetRequest(context.Background(), id)
	if request.Status != customorders.StatusApproved || request.OrderID != nil {
		t.Errorf("request = %s, order %v, want approved without an order", request.Status, request.OrderID)
	}
	if n := f.count(t, "SELECT stock FROM advertisements WHERE id = $1", f.ad.ID); n != 1 {
		t.Errorf("stock = %d, want 1: nothing was taken, so nothing comes back", n)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'custom_order.order_released'", id); n != 1 {
		t.Errorf("release audit rows = %d", n)
	}
	if created, err := f.checkout(t, id, "key-2"); err != nil || !created {
		t.Errorf("checkout again: %v, created = %v", err, created)
	}
}

func TestCancellingAPaidOrderClosesTheRequest(t *testing.T) {
	f := newFixture(t)
	id := f.approved(t)
	if _, err := f.checkout(t, id, "key-1"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	orderID := f.orderOf(t, id)
	if err := f.moveOrder(t, orderID, "", orders.OrderStatusAwaitingPayment, orders.OrderStatusPaid); err != nil {
		t.Fatalf("pay: %v", err)
	}

	if err := f.moveOrder(t, orderID, f.buyer.ID, orders.OrderStatusPaid, orders.OrderStatusCancelled); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	request, _ := f.r.GetRequest(context.Background(), id)
	last := request.History[len(request.History)-1]
	if request.Status != customorders.StatusCancelled || *last.PreviousStatus != customorders.StatusInProduction || *last.ChangedBy != f.buyer.ID {
		t.Errorf("request = %s, last history = %+v, want cancelled by the buyer", request.Status, last)
	}
}

func TestFootPhoto(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.create(t, f.buyer.ID)

	previous, err := f.r.SetFootPhoto(ctx, customorders.PhotoChange{RequestID: id, BuyerID: f.buyer.ID, Key: "foot-photos/a.png"})
	if err != nil || previous != nil {
		t.Fatalf("first photo: %v, previous = %v", err, previous)
	}
	previous, err = f.r.SetFootPhoto(ctx, customorders.PhotoChange{RequestID: id, BuyerID: f.buyer.ID, Key: "foot-photos/b.png"})
	if err != nil || previous == nil || *previous != "foot-photos/a.png" {
		t.Fatalf("second photo: %v, previous = %v", err, previous)
	}
	request, _ := f.r.GetRequest(ctx, id)
	if request.FootPhotoKey == nil || *request.FootPhotoKey != "foot-photos/b.png" {
		t.Errorf("photo key = %v", request.FootPhotoKey)
	}
	if err := f.r.RecordPhotoView(ctx, id, f.seller.ID); err != nil {
		t.Fatalf("RecordPhotoView: %v", err)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action IN ('custom_order.photo_sent', 'custom_order.photo_viewed')", id); n != 3 {
		t.Errorf("photo audit rows = %d, want 3", n)
	}

	reason := "Não fazemos esse modelo"
	reject := f.change(id, customorders.StatusRequested, customorders.ActionReject, customorders.StatusRejected)
	reject.Reason, reject.RejectionReason = &reason, &reason
	if err := f.r.ChangeStatus(ctx, reject); err != nil {
		t.Fatalf("reject: %v", err)
	}
	if _, err := f.r.SetFootPhoto(ctx, customorders.PhotoChange{RequestID: id, BuyerID: f.buyer.ID, Key: "foot-photos/c.png"}); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("after the decision: err = %v, want ErrInvalidStatusTransition", err)
	}
	if _, err := f.r.SetFootPhoto(ctx, customorders.PhotoChange{RequestID: unknownID, BuyerID: f.buyer.ID, Key: "x"}); !errors.Is(err, app.ErrCustomOrderNotFound) {
		t.Errorf("unknown request: err = %v", err)
	}
}

func TestCheckoutExists(t *testing.T) {
	f := newFixture(t)
	id := f.approved(t)
	if _, err := f.checkout(t, id, "key-1"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	for key, want := range map[string]bool{"key-1": true, "key-2": false} {
		if found, err := f.r.CheckoutExists(context.Background(), f.buyer.ID, key); err != nil || found != want {
			t.Errorf("%s: found = %v, %v, want %v", key, found, err, want)
		}
	}
}

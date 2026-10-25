package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/documents"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	ordersdb "github.com/caslumes/solado_certo_backend/internal/features/orders/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

const unknownID = "00000000-0000-0000-0000-000000000000"

type fixture struct {
	pool   *pgxpool.Pool
	r      *postgresRepository
	buyer  testutil.User
	seller testutil.User
	order  string
	itemID string
}

// newFixture has an order of buyer, in status, with one item that needs a prescription.
func newFixture(t *testing.T, status orders.OrderStatus) fixture {
	t.Helper()
	ctx := context.Background()
	pool := testutil.DB(t)
	f := fixture{pool: pool, r: NewPostgresRepository(pool), buyer: testutil.InsertUser(t, pool), seller: testutil.InsertUser(t, pool)}
	category := testutil.InsertCategory(t, pool, nil)
	ad := testutil.InsertAd(t, pool, testutil.Ad{SellerID: f.seller.ID, CategoryID: category.ID, Stock: 5, PrescriptionRequired: true})

	now := time.Now().UTC()
	var checkoutID string
	if err := pool.QueryRow(ctx, "INSERT INTO checkouts (buyer_id, idempotency_key, created_at) VALUES ($1, 'k', $2) RETURNING id", f.buyer.ID, now).Scan(&checkoutID); err != nil {
		t.Fatalf("insert checkout: %v", err)
	}
	var paidAt *time.Time
	if status != orders.OrderStatusAwaitingPayment {
		paidAt = &now
	}
	err := pool.QueryRow(
		ctx,
		"INSERT INTO orders (buyer_id, checkout_id, number, status, subtotal, shipping_total, discount_total, total, shipping_address, paid_at, created_at, updated_at) "+
			"VALUES ($1, $2, 'SC-20261024-AAAAAA', $3, 100.00, 0, 0, 100.00, '{}', $4, $5, $5) RETURNING id",
		f.buyer.ID, checkoutID, status, paidAt, now,
	).Scan(&f.order)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	err = pool.QueryRow(
		ctx,
		"INSERT INTO order_items (order_id, advertisement_id, seller_id, quantity, unit_price, subtotal, product_snapshot, prescription_required, created_at) "+
			"VALUES ($1, $2, $3, 1, 100.00, 100.00, '{\"title\": \"Órtese corretora\"}', true, $4) RETURNING id",
		f.order, ad.ID, f.seller.ID, now,
	).Scan(&f.itemID)
	if err != nil {
		t.Fatalf("insert order item: %v", err)
	}
	return f
}

func (f fixture) upload(t *testing.T, notification *notifications.NewNotification) *documents.DocumentEntity {
	t.Helper()
	created, err := f.r.CreateDocument(context.Background(), documents.NewDocument{
		OrderID: f.order, OrderItemID: f.itemID, UploadedBy: f.buyer.ID, Type: documents.DocumentTypePrescription,
		Key: "prescriptions/0123456789abcdef0123456789abcdef.pdf", Status: orders.DocumentStatusApproved, Notification: notification,
	})
	if err != nil {
		t.Fatalf("CreateDocument: %v", err)
	}
	return created
}

func (f fixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestGetPrescriptionItem(t *testing.T) {
	f := newFixture(t, orders.OrderStatusAwaitingPayment)
	ctx := context.Background()

	item, err := f.r.GetPrescriptionItem(ctx, f.order, f.itemID)
	if err != nil || item.ID != f.itemID || item.Title != "Órtese corretora" || item.BuyerID != f.buyer.ID || item.SellerID != f.seller.ID ||
		item.OrderNumber != "SC-20261024-AAAAAA" || item.OrderStatus != orders.OrderStatusAwaitingPayment || !item.PrescriptionRequired || item.Latest != nil {
		t.Fatalf("GetPrescriptionItem = %+v, %v", item, err)
	}

	document := f.upload(t, nil)
	item, err = f.r.GetPrescriptionItem(ctx, f.order, f.itemID)
	if err != nil || item.Latest == nil || item.Latest.ID != document.ID || item.Latest.Status != orders.DocumentStatusApproved {
		t.Errorf("after upload: latest = %+v, %v", item.Latest, err)
	}

	item, err = f.r.GetPrescriptionItem(ctx, f.order, unknownID)
	if err != nil || item.ID != "" || item.BuyerID != f.buyer.ID {
		t.Errorf("unknown item = %+v, %v, want the order without an item", item, err)
	}
	if _, err := f.r.GetPrescriptionItem(ctx, unknownID, f.itemID); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("unknown order: error = %v", err)
	}
}

func TestCreateDocument(t *testing.T) {
	f := newFixture(t, orders.OrderStatusProcessing)
	notification := notifications.PrescriptionSent(f.seller.ID, f.order, "SC-20261024-AAAAAA", "Órtese corretora")

	created := f.upload(t, &notification)

	if created.ID == "" || created.Status != orders.DocumentStatusApproved || *created.OrderItemID != f.itemID {
		t.Errorf("created = %+v", created)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity = 'document' AND entity_id = $1 AND action = 'document.uploaded' AND user_id = $2", created.ID, f.buyer.ID); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'prescription_sent'", f.seller.ID); n != 1 {
		t.Errorf("notifications = %d, want 1", n)
	}

	stored, err := f.r.GetDocument(context.Background(), created.ID)
	if err != nil || stored.Key != "prescriptions/0123456789abcdef0123456789abcdef.pdf" || stored.UploadedBy != f.buyer.ID ||
		stored.BuyerID != f.buyer.ID || stored.SellerID != f.seller.ID || !stored.OrderPaid || stored.ItemTitle != "Órtese corretora" ||
		stored.OrderNumber != "SC-20261024-AAAAAA" || stored.Type != documents.DocumentTypePrescription {
		t.Errorf("GetDocument = %+v, %v", stored, err)
	}
	if _, err := f.r.GetDocument(context.Background(), unknownID); !errors.Is(err, app.ErrDocumentNotFound) {
		t.Errorf("unknown document: error = %v", err)
	}
}

func TestIsAdminAndRecordView(t *testing.T) {
	f := newFixture(t, orders.OrderStatusPaid)
	ctx := context.Background()
	document := f.upload(t, nil)

	if admin, err := f.r.IsAdmin(ctx, f.seller.ID); err != nil || admin {
		t.Errorf("seller: admin = %v, %v", admin, err)
	}
	if _, err := f.pool.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = 'admin'", f.seller.ID); err != nil {
		t.Fatal(err)
	}
	if admin, err := f.r.IsAdmin(ctx, f.seller.ID); err != nil || !admin {
		t.Errorf("admin: admin = %v, %v", admin, err)
	}

	if err := f.r.RecordView(ctx, document.ID, f.seller.ID); err != nil {
		t.Fatalf("RecordView: %v", err)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'document.viewed' AND user_id = $2", document.ID, f.seller.ID); n != 1 {
		t.Errorf("view rows = %d, want 1", n)
	}
}

func (f fixture) rejection(documentID string) documents.Rejection {
	return documents.Rejection{
		DocumentID: documentID, OrderID: f.order, OrderItemID: f.itemID, SellerID: f.seller.ID, Reason: "Receita sem assinatura",
		Notification: notifications.PrescriptionRejected(f.buyer.ID, f.order, "SC-20261024-AAAAAA", "Órtese corretora", "Receita sem assinatura"),
	}
}

func TestRejectPrescription(t *testing.T) {
	f := newFixture(t, orders.OrderStatusProcessing)
	ctx := context.Background()
	document := f.upload(t, nil)

	rejected, err := f.r.RejectPrescription(ctx, f.rejection(document.ID))

	if err != nil {
		t.Fatalf("RejectPrescription: %v", err)
	}
	if rejected.Status != orders.DocumentStatusRejected || *rejected.RejectionReason != "Receita sem assinatura" || rejected.OrderID != f.order {
		t.Errorf("rejected = %+v", rejected)
	}
	if n := f.count(t, "SELECT count(*) FROM order_documents WHERE id = $1 AND validation_status = 'rejected' AND validated_by = $2", document.ID, f.seller.ID); n != 1 {
		t.Errorf("rejected rows = %d, want 1", n)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action = 'document.rejected'", document.ID); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'prescription_rejected'", f.buyer.ID); n != 1 {
		t.Errorf("notifications = %d, want 1", n)
	}

	if _, err := f.r.RejectPrescription(ctx, f.rejection(document.ID)); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("rejected twice: error = %v", err)
	}
}

func TestRejectPrescriptionOnlyTheLatestBeforeShipping(t *testing.T) {
	f := newFixture(t, orders.OrderStatusPaid)
	ctx := context.Background()
	older := f.upload(t, nil)
	newer := f.upload(t, nil)

	if _, err := f.r.RejectPrescription(ctx, f.rejection(older.ID)); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("older document: error = %v", err)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE orders SET status = 'shipped' WHERE id = $1", f.order); err != nil {
		t.Fatal(err)
	}
	if _, err := f.r.RejectPrescription(ctx, f.rejection(newer.ID)); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("shipped order: error = %v", err)
	}
	if n := f.count(t, "SELECT count(*) FROM order_documents WHERE validation_status = 'rejected'"); n != 0 {
		t.Errorf("rejected rows = %d, want 0", n)
	}
}

func TestPrescriptionsInOrders(t *testing.T) {
	f := newFixture(t, orders.OrderStatusProcessing)
	ctx := context.Background()
	o := ordersdb.NewPostgresRepository(f.pool)

	order, err := o.GetOrder(ctx, f.order)
	if err != nil || len(order.Items) != 1 || !order.Items[0].PrescriptionRequired || order.Items[0].Prescription != nil {
		t.Fatalf("before upload: GetOrder = %+v, %v", order, err)
	}
	if missing := order.MissingPrescriptions(); len(missing) != 1 || missing[0] != f.itemID {
		t.Errorf("before upload: missing = %v", missing)
	}

	document := f.upload(t, nil)
	order, _ = o.GetOrder(ctx, f.order)
	if p := order.Items[0].Prescription; p == nil || p.ID != document.ID || p.Status != orders.DocumentStatusApproved || len(order.MissingPrescriptions()) != 0 {
		t.Errorf("after upload: prescription = %+v", p)
	}

	if _, err := f.r.RejectPrescription(ctx, f.rejection(document.ID)); err != nil {
		t.Fatal(err)
	}
	order, _ = o.GetOrder(ctx, f.order)
	if p := order.Items[0].Prescription; p == nil || p.Status != orders.DocumentStatusRejected || *p.RejectionReason != "Receita sem assinatura" {
		t.Errorf("after rejection: prescription = %+v", p)
	}

	err = o.ChangeOrderStatus(ctx, orders.StatusChange{
		OrderID: f.order, ChangedBy: f.seller.ID, From: orders.OrderStatusProcessing, To: orders.OrderStatusShipped,
		Reason: orders.ReasonSellerShipped, Shipment: &orders.NewShipment{Carrier: "Correios", TrackingCode: "AA123456789BR"},
	})
	var missing *app.PrescriptionRequiredError
	if !errors.As(err, &missing) || len(missing.ItemIDs) != 1 {
		t.Fatalf("shipping with a rejected prescription: error = %v", err)
	}
	if n := f.count(t, "SELECT count(*) FROM shipments WHERE order_id = $1", f.order); n != 0 {
		t.Errorf("shipments = %d, want 0", n)
	}
}

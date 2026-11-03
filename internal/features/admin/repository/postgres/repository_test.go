package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/admin"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool    *pgxpool.Pool
	r       *postgresRepository
	admin   testutil.User
	buyer   testutil.User
	seller  testutil.User
	adID    string
	orderID string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testutil.DB(t)
	f := fixture{pool: pool, r: NewPostgresRepository(pool), admin: testutil.InsertUser(t, pool), buyer: testutil.InsertUser(t, pool), seller: testutil.InsertUser(t, pool)}
	ctx := context.Background()

	for user, role := range map[string]string{f.admin.ID: "admin", f.buyer.ID: "buyer", f.seller.ID: "seller"} {
		f.exec(t, "INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = $2", user, role)
	}

	category := testutil.InsertCategory(t, pool, nil)
	f.adID = testutil.InsertAd(t, pool, testutil.Ad{SellerID: f.seller.ID, CategoryID: category.ID, Title: "Palmilha Conforto", Stock: 3}).ID

	now := time.Now().UTC()
	var checkoutID string
	if err := pool.QueryRow(ctx, "INSERT INTO checkouts (buyer_id, idempotency_key, created_at) VALUES ($1, 'key-1', $2) RETURNING id", f.buyer.ID, now).Scan(&checkoutID); err != nil {
		t.Fatalf("insert checkout: %v", err)
	}
	err := pool.QueryRow(
		ctx,
		"INSERT INTO orders (buyer_id, checkout_id, number, status, subtotal, shipping_total, discount_total, total, shipping_address, created_at, updated_at, paid_at) "+
			"VALUES ($1, $2, 'SC-20261102-AAAAAA', 'paid', 120.50, 0, 0, 120.50, '{}', $3, $3, $3) RETURNING id",
		f.buyer.ID, checkoutID, now,
	).Scan(&f.orderID)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	f.exec(t, "INSERT INTO order_items (order_id, advertisement_id, seller_id, quantity, unit_price, subtotal, product_snapshot, created_at) "+
		"VALUES ($1, $2, $3, 1, 120.50, 120.50, '{}', $4)", f.orderID, f.adID, f.seller.ID, now)
	return f
}

func (f fixture) exec(t *testing.T, query string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func (f fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func (f fixture) insertPayment(t *testing.T, status string, amount string, payload string) string {
	t.Helper()
	var id string
	err := f.pool.QueryRow(
		context.Background(),
		"INSERT INTO payments (order_id, gateway, idempotency_key, status, method, amount, gateway_payload, created_at, updated_at) "+
			"VALUES ($1, 'fake', gen_random_uuid()::text, $2, 'pix', $3, $4, now(), now()) RETURNING id",
		f.orderID, status, amount, payload,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert payment: %v", err)
	}
	return id
}

func ids[T any](items []T, id func(T) string) []string {
	var list []string
	for _, item := range items {
		list = append(list, id(item))
	}
	slices.Sort(list)
	return list
}

func sorted(values ...string) []string {
	slices.Sort(values)
	return values
}

func TestListUsersFilters(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	f.exec(t, "UPDATE users SET suspended_until = $2 WHERE id = $1", f.buyer.ID, now.Add(time.Hour))
	f.exec(t, "UPDATE users SET status = 'blocked' WHERE id = $1", f.seller.ID)
	userID := func(u *admin.UserEntity) string { return u.ID }

	tests := map[string]struct {
		filter admin.UserFilter
		want   []string
	}{
		"everyone":  {admin.UserFilter{Now: now}, sorted(f.admin.ID, f.buyer.ID, f.seller.ID)},
		"search":    {admin.UserFilter{Search: f.buyer.Email[:6], Now: now}, []string{f.buyer.ID}},
		"suspended": {admin.UserFilter{Status: enum.ProfileStatusSuspended, Now: now}, []string{f.buyer.ID}},
		"blocked":   {admin.UserFilter{Status: enum.ProfileStatusBlocked, Now: now}, []string{f.seller.ID}},
		"active":    {admin.UserFilter{Status: enum.ProfileStatusActive, Now: now}, []string{f.admin.ID}},
		"later":     {admin.UserFilter{Status: enum.ProfileStatusActive, Now: now.Add(2 * time.Hour)}, sorted(f.admin.ID, f.buyer.ID)},
		"role":      {admin.UserFilter{Role: "seller", Now: now}, []string{f.seller.ID}},
		"wildcard":  {admin.UserFilter{Search: "%", Now: now}, nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			users, total, err := f.r.ListUsers(ctx, tt.filter, 10, 0)
			if err != nil {
				t.Fatalf("ListUsers: %v", err)
			}
			if got := ids(users, userID); int(total) != len(tt.want) || !slices.Equal(got, tt.want) {
				t.Fatalf("total = %d, ids = %v, want %v", total, got, tt.want)
			}
		})
	}

	user, err := f.r.GetUser(ctx, f.admin.ID)
	if err != nil || !slices.Equal(user.Roles, []string{"admin"}) || user.Email != f.admin.Email {
		t.Fatalf("GetUser = %+v, %v", user, err)
	}
	f.exec(t, "UPDATE users SET deleted_at = now() WHERE id = $1", f.admin.ID)
	if _, err := f.r.GetUser(ctx, f.admin.ID); !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("deleted: err = %v", err)
	}
}

func TestModerateUserRevokesSessionsAndAudits(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.exec(t, "INSERT INTO refresh_tokens (user_id, token_hash, expires_at, created_at) VALUES ($1, 'hash-1', now() + interval '1 day', now())", f.buyer.ID)

	if status, err := f.r.StatusBeforeBlock(ctx, f.buyer.ID); err != nil || status != "" {
		t.Fatalf("StatusBeforeBlock before any block = %q, %v", status, err)
	}

	err := f.r.ModerateUser(ctx, admin.UserModeration{
		UserID: f.buyer.ID, AdminID: f.admin.ID, FromStatus: "active", ToStatus: enum.ProfileStatusBlocked, RevokeSessions: true,
		Action: admin.ActionBlockUser, Details: map[string]any{"reason": "Fraude", "previous_status": "active"},
	})
	if err != nil {
		t.Fatalf("ModerateUser: %v", err)
	}

	user, _ := f.r.GetUser(ctx, f.buyer.ID)
	if user.Status != enum.ProfileStatusBlocked {
		t.Fatalf("status = %s", user.Status)
	}
	if n := f.count(t, "SELECT count(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NULL", f.buyer.ID); n != 0 {
		t.Fatalf("active sessions = %d", n)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity = 'user' AND entity_id = $1 AND user_id = $2 AND action = $3 AND context->>'reason' = 'Fraude'",
		f.buyer.ID, f.admin.ID, admin.ActionBlockUser); n != 1 {
		t.Fatalf("audit rows = %d", n)
	}
	if status, err := f.r.StatusBeforeBlock(ctx, f.buyer.ID); err != nil || status != "active" {
		t.Fatalf("StatusBeforeBlock = %q, %v", status, err)
	}

	err = f.r.ModerateUser(ctx, admin.UserModeration{UserID: f.buyer.ID, AdminID: f.admin.ID, FromStatus: "active", ToStatus: "active", Action: "x"})
	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("stale: err = %v", err)
	}
	err = f.r.ModerateUser(ctx, admin.UserModeration{UserID: "33333333-3333-3333-3333-333333333333", AdminID: f.admin.ID, Action: "x"})
	if !errors.Is(err, app.ErrUserNotFound) {
		t.Fatalf("unknown: err = %v", err)
	}
}

func TestRemoveAdTakesItOutOfTheCatalog(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	ads, total, err := f.r.ListAds(ctx, admin.AdFilter{Search: "conforto", SellerID: f.seller.ID}, 10, 0)
	if err != nil || total != 1 || ads[0].SellerName != f.seller.Name || ads[0].Price != "100.00" {
		t.Fatalf("ListAds = %+v, %d, %v", ads, total, err)
	}

	err = f.r.RemoveAd(ctx, admin.AdRemoval{
		AdID: f.adID, AdminID: f.admin.ID, Reason: "Promete cura", FromStatus: "published",
		Notification: notifications.AdvertisementRemoved(f.seller.ID, f.adID, "Palmilha Conforto", "Promete cura"),
	})
	if err != nil {
		t.Fatalf("RemoveAd: %v", err)
	}

	ad, err := f.r.GetAd(ctx, f.adID)
	if err != nil || ad.Status != "removed" || ad.RemovedAt == nil {
		t.Fatalf("GetAd = %+v, %v", ad, err)
	}
	if n := f.count(t, "SELECT count(*) FROM advertisements WHERE status = 'published'"); n != 0 {
		t.Fatalf("published = %d", n)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'advertisement_removed'", f.seller.ID); n != 1 {
		t.Fatalf("notifications = %d", n)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity = 'advertisement' AND entity_id = $1 AND action = $2 AND context->>'previous_status' = 'published'",
		f.adID, admin.ActionRemoveAd); n != 1 {
		t.Fatalf("audit rows = %d", n)
	}

	err = f.r.RemoveAd(ctx, admin.AdRemoval{AdID: f.adID, AdminID: f.admin.ID, FromStatus: "published"})
	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("removed twice: err = %v", err)
	}
	if _, err := f.r.GetAd(ctx, "33333333-3333-3333-3333-333333333333"); !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Fatalf("unknown: err = %v", err)
	}
}

func TestListOrdersAndPayments(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	orders, total, err := f.r.ListOrders(ctx, admin.OrderFilter{Search: "aaaaaa", Status: "paid"}, 10, 0)
	if err != nil || total != 1 {
		t.Fatalf("ListOrders = %+v, %d, %v", orders, total, err)
	}
	order := orders[0]
	if order.Total != "120.50" || order.BuyerName != f.buyer.Name || order.SellerID == nil || *order.SellerID != f.seller.ID || *order.SellerName != f.seller.Name {
		t.Fatalf("order = %+v", order)
	}
	tomorrow := time.Now().Add(24 * time.Hour)
	if _, total, _ := f.r.ListOrders(ctx, admin.OrderFilter{Since: &tomorrow}, 10, 0); total != 0 {
		t.Fatalf("since tomorrow: total = %d", total)
	}

	f.insertPayment(t, "failed", "120.50", "{}")
	givenUp := f.insertPayment(t, "captured", "120.50", `{"refund_error":"saldo insuficiente","refund_attempts":10}`)
	f.insertPayment(t, "captured", "120.50", `{"refund_error":"saldo insuficiente","refund_attempts":3}`)

	list, total, err := f.r.ListPayments(ctx, admin.PaymentFilter{RefundGivenUp: true}, 10, 0)
	if err != nil || total != 1 || list[0].ID != givenUp || !list[0].RefundGivenUp || *list[0].RefundError != "saldo insuficiente" || list[0].OrderNumber != "SC-20261102-AAAAAA" {
		t.Fatalf("given up = %+v, %d, %v", list, total, err)
	}
	if _, total, _ := f.r.ListPayments(ctx, admin.PaymentFilter{Status: "captured", Method: "pix"}, 10, 0); total != 2 {
		t.Fatalf("captured: total = %d", total)
	}
	page, total, _ := f.r.ListPayments(ctx, admin.PaymentFilter{}, 2, 2)
	if total != 3 || len(page) != 1 {
		t.Fatalf("second page: total = %d, items = %d", total, len(page))
	}
}

func TestListAuditLogs(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.exec(t, "INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'user', $2, 'admin.user_blocked', '{\"reason\":\"Fraude\"}', now())", f.admin.ID, f.buyer.ID)
	f.exec(t, "INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES (NULL, 'payment', $1, 'payment.captured', NULL, now())", f.orderID)
	f.exec(t, "INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'order', $2, 'admin_like', '{}', now())", f.admin.ID, f.orderID)

	logs, total, err := f.r.ListAuditLogs(ctx, admin.AuditFilter{Action: "admin."}, 10, 0)
	if err != nil || total != 1 {
		t.Fatalf("by action = %+v, %d, %v", logs, total, err)
	}
	var details map[string]string
	if err := json.Unmarshal(logs[0].Context, &details); err != nil || details["reason"] != "Fraude" || *logs[0].UserName != f.admin.Name {
		t.Fatalf("log = %+v, details = %v, err = %v", logs[0], details, err)
	}

	logs, total, _ = f.r.ListAuditLogs(ctx, admin.AuditFilter{Entity: "payment"}, 10, 0)
	if total != 1 || logs[0].UserID != nil || logs[0].ToOutput().User != nil || string(logs[0].ToOutput().Context) != "null" {
		t.Fatalf("system entry = %+v", logs)
	}
	if _, total, _ := f.r.ListAuditLogs(ctx, admin.AuditFilter{UserID: f.admin.ID, EntityID: f.orderID}, 10, 0); total != 1 {
		t.Fatalf("by user and entity: total = %d", total)
	}
}

func TestGetIndicators(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	f.exec(t, "UPDATE users SET status = 'blocked' WHERE id = $1", f.seller.ID)
	f.exec(t, "UPDATE users SET suspended_until = $2 WHERE id = $1", f.buyer.ID, now.Add(time.Hour))
	f.insertPayment(t, "captured", "120.50", `{"refund_attempts":10}`)
	f.insertPayment(t, "failed", "120.50", "{}")
	f.exec(t, "INSERT INTO support_tickets (opened_by, category, status, subject, description, opened_at, updated_at) VALUES ($1, 'other', 'open', 'Dúvida', 'Uma dúvida qualquer aqui.', now(), now())", f.buyer.ID)
	f.exec(t, "INSERT INTO support_tickets (opened_by, category, status, subject, description, opened_at, updated_at, closed_at) VALUES ($1, 'other', 'resolved', 'Dúvida', 'Uma dúvida qualquer aqui.', now(), now(), now())", f.buyer.ID)

	period := admin.Period{From: now.Add(-24 * time.Hour), Until: now.Add(time.Hour)}
	i, err := f.r.GetIndicators(ctx, period, now)
	if err != nil {
		t.Fatalf("GetIndicators: %v", err)
	}
	if i.NewUsers != 3 || i.BlockedUsers != 1 || i.SuspendedUsers != 1 || i.NewAds != 1 || i.PublishedAds != 1 {
		t.Fatalf("users and ads = %+v", i)
	}
	if i.Orders != 1 || i.OrdersByStatus["paid"] != 1 || i.PaidOrders != 1 || i.PaidAmount != "120.50" {
		t.Fatalf("orders = %+v", i)
	}
	if i.PaymentsByStatus["captured"] != (admin.Amount{Count: 1, Amount: "120.50"}) || i.PaymentsByStatus["failed"].Count != 1 || i.RefundsGivenUp != 1 {
		t.Fatalf("payments = %+v", i.PaymentsByStatus)
	}
	if i.TicketsOpened != 2 || i.TicketsResolved != 1 || i.OpenTickets != 1 {
		t.Fatalf("tickets = %+v", i)
	}

	empty, err := f.r.GetIndicators(ctx, admin.Period{From: now.Add(48 * time.Hour), Until: now.Add(72 * time.Hour)}, now)
	if err != nil || empty.NewUsers != 0 || empty.PaidAmount != "0.00" || empty.Orders != 0 || empty.OpenTickets != 1 {
		t.Fatalf("future period = %+v, %v", empty, err)
	}
}

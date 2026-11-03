package postgres

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/support"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fixture struct {
	pool    *pgxpool.Pool
	r       *postgresRepository
	buyer   testutil.User
	seller  testutil.User
	admin   testutil.User
	orderID string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testutil.DB(t)
	f := fixture{pool: pool, r: NewPostgresRepository(pool), buyer: testutil.InsertUser(t, pool), seller: testutil.InsertUser(t, pool), admin: testutil.InsertUser(t, pool)}
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = 'admin'", f.admin.ID); err != nil {
		t.Fatalf("grant admin: %v", err)
	}

	category := testutil.InsertCategory(t, pool, nil)
	ad := testutil.InsertAd(t, pool, testutil.Ad{SellerID: f.seller.ID, CategoryID: category.ID, Stock: 5})
	now := time.Now().UTC()
	var checkoutID string
	if err := pool.QueryRow(ctx, "INSERT INTO checkouts (buyer_id, idempotency_key, created_at) VALUES ($1, 'key-1', $2) RETURNING id", f.buyer.ID, now).Scan(&checkoutID); err != nil {
		t.Fatalf("insert checkout: %v", err)
	}
	err := pool.QueryRow(
		ctx,
		"INSERT INTO orders (buyer_id, checkout_id, number, status, subtotal, shipping_total, discount_total, total, shipping_address, created_at, updated_at) "+
			"VALUES ($1, $2, 'SC-20261101-AAAAAA', 'paid', 100.00, 0, 0, 100.00, '{}', $3, $3) RETURNING id",
		f.buyer.ID, checkoutID, now,
	).Scan(&f.orderID)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	_, err = pool.Exec(
		ctx,
		"INSERT INTO order_items (order_id, advertisement_id, seller_id, quantity, unit_price, subtotal, product_snapshot, created_at) "+
			"VALUES ($1, $2, $3, 1, 100.00, 100.00, '{}', $4)",
		f.orderID, ad.ID, f.seller.ID, now,
	)
	if err != nil {
		t.Fatalf("insert order item: %v", err)
	}
	return f
}

func (f fixture) createTicket(t *testing.T, openedBy string, orderID *string, note []notifications.NewNotification) string {
	t.Helper()
	id, err := f.r.CreateTicket(context.Background(), support.NewTicket{
		OpenedBy: openedBy, Category: support.CategoryOrder, Subject: "Produto errado", Description: "Recebi o tamanho errado do pedido.", OrderID: orderID,
		Notifications: func(string) []notifications.NewNotification { return note },
	})
	if err != nil {
		t.Fatalf("CreateTicket: %v", err)
	}
	return id
}

func (f fixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestGetOrderParties(t *testing.T) {
	f := newFixture(t)

	order, err := f.r.GetOrderParties(context.Background(), f.orderID)
	if err != nil {
		t.Fatalf("GetOrderParties: %v", err)
	}
	if order.Number != "SC-20261101-AAAAAA" || order.BuyerID != f.buyer.ID || order.SellerID != f.seller.ID {
		t.Fatalf("order = %+v", order)
	}

	if _, err := f.r.GetOrderParties(context.Background(), "33333333-3333-3333-3333-333333333333"); !errors.Is(err, app.ErrOrderNotFound) {
		t.Fatalf("unknown: err = %v", err)
	}
}

func TestListAdminIDs(t *testing.T) {
	f := newFixture(t)

	ids, err := f.r.ListAdminIDs(context.Background())
	if err != nil {
		t.Fatalf("ListAdminIDs: %v", err)
	}
	if !slices.Equal(ids, []string{f.admin.ID}) {
		t.Fatalf("ids = %v", ids)
	}
}

func TestCreateTicketStoresNotifiesAndAudits(t *testing.T) {
	f := newFixture(t)

	id := f.createTicket(t, f.buyer.ID, &f.orderID, []notifications.NewNotification{
		notifications.SupportTicketOpenedForSupport(f.admin.ID, "x", "Produto errado"),
	})

	ticket, err := f.r.GetTicket(context.Background(), id)
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if ticket.Status != support.StatusOpen || ticket.OpenedBy != f.buyer.ID || ticket.Order == nil || ticket.Order.SellerID != f.seller.ID || len(ticket.Messages) != 0 {
		t.Fatalf("ticket = %+v", ticket)
	}
	if !ticket.OpenedAt.Equal(ticket.UpdatedAt) {
		t.Fatalf("opened %v, updated %v", ticket.OpenedAt, ticket.UpdatedAt)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'support_ticket_opened'", f.admin.ID); n != 1 {
		t.Fatalf("notifications = %d", n)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity = 'support_ticket' AND entity_id = $1 AND action = 'support.ticket_opened' AND user_id = $2", id, f.buyer.ID); n != 1 {
		t.Fatalf("audit rows = %d", n)
	}

	if _, err := f.r.GetTicket(context.Background(), "33333333-3333-3333-3333-333333333333"); !errors.Is(err, app.ErrTicketNotFound) {
		t.Fatalf("unknown: err = %v", err)
	}
}

func TestGetTicketsFindsParticipants(t *testing.T) {
	f := newFixture(t)
	other := testutil.InsertUser(t, f.pool)
	aboutOrder := f.createTicket(t, f.buyer.ID, &f.orderID, nil)
	general := f.createTicket(t, other.ID, nil, nil)
	ctx := context.Background()

	tests := map[string]struct {
		filter support.Filter
		want   []string
	}{
		"buyer":         {support.Filter{ParticipantID: f.buyer.ID}, []string{aboutOrder}},
		"seller":        {support.Filter{ParticipantID: f.seller.ID}, []string{aboutOrder}},
		"other":         {support.Filter{ParticipantID: other.ID}, []string{general}},
		"admin":         {support.Filter{ParticipantID: f.admin.ID}, nil},
		"everyone":      {support.Filter{}, []string{general, aboutOrder}},
		"status filter": {support.Filter{ParticipantID: f.buyer.ID, Status: support.StatusResolved}, nil},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			total, err := f.r.CountTickets(ctx, tt.filter)
			if err != nil {
				t.Fatalf("CountTickets: %v", err)
			}
			tickets, err := f.r.GetTickets(ctx, tt.filter, 10, 0)
			if err != nil {
				t.Fatalf("GetTickets: %v", err)
			}
			var ids []string
			for _, ticket := range tickets {
				ids = append(ids, ticket.ID)
			}
			if int(total) != len(tt.want) || !slices.Equal(ids, tt.want) {
				t.Fatalf("total = %d, ids = %v, want %v", total, ids, tt.want)
			}
		})
	}
}

func TestAddMessageChangesStatusAndKeepsOrder(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.createTicket(t, f.buyer.ID, &f.orderID, nil)
	key := "support/0123456789abcdef0123456789abcdef.png"

	err := f.r.AddMessage(ctx, support.NewMessage{
		TicketID: id, SenderID: f.admin.ID, Message: "Envie uma foto.", From: support.StatusOpen, To: support.StatusAwaitingCustomer,
		Notifications: []notifications.NewNotification{notifications.SupportTicketReplied(f.buyer.ID, id, "Produto errado", true)},
	})
	if err != nil {
		t.Fatalf("admin message: %v", err)
	}
	err = f.r.AddMessage(ctx, support.NewMessage{
		TicketID: id, SenderID: f.buyer.ID, Message: "Segue.", AttachmentKey: &key, From: support.StatusAwaitingCustomer, To: support.StatusOpen,
	})
	if err != nil {
		t.Fatalf("buyer message: %v", err)
	}

	ticket, err := f.r.GetTicket(ctx, id)
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if ticket.Status != support.StatusOpen || len(ticket.Messages) != 2 || !ticket.UpdatedAt.After(ticket.OpenedAt) {
		t.Fatalf("ticket = %+v", ticket)
	}
	first, second := ticket.Messages[0], ticket.Messages[1]
	if first.SenderID != f.admin.ID || !first.SenderIsAdmin || first.AttachmentKey != nil || first.SenderName != f.admin.Name {
		t.Fatalf("first = %+v", first)
	}
	if second.SenderID != f.buyer.ID || second.SenderIsAdmin || second.AttachmentKey == nil || *second.AttachmentKey != key {
		t.Fatalf("second = %+v", second)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'support_ticket_updated'", f.buyer.ID); n != 1 {
		t.Fatalf("notifications = %d", n)
	}
}

func TestAddMessageRefusesStaleStatus(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.createTicket(t, f.buyer.ID, &f.orderID, nil)

	err := f.r.AddMessage(ctx, support.NewMessage{TicketID: id, SenderID: f.buyer.ID, Message: "Oi", From: support.StatusAwaitingCustomer, To: support.StatusOpen})
	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("err = %v, want %v", err, app.ErrInvalidStatusTransition)
	}
	if n := f.count(t, "SELECT count(*) FROM support_messages WHERE ticket_id = $1", id); n != 0 {
		t.Fatalf("messages = %d", n)
	}

	err = f.r.AddMessage(ctx, support.NewMessage{TicketID: "33333333-3333-3333-3333-333333333333", SenderID: f.buyer.ID, Message: "Oi"})
	if !errors.Is(err, app.ErrTicketNotFound) {
		t.Fatalf("unknown: err = %v", err)
	}
}

func TestRecordAttachmentView(t *testing.T) {
	f := newFixture(t)
	id := f.createTicket(t, f.buyer.ID, &f.orderID, nil)

	if err := f.r.RecordAttachmentView(context.Background(), id, "message-1", f.admin.ID); err != nil {
		t.Fatalf("RecordAttachmentView: %v", err)
	}
	n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND user_id = $2 AND action = 'support.attachment_viewed' AND context->>'message_id' = 'message-1'", id, f.admin.ID)
	if n != 1 {
		t.Fatalf("audit rows = %d", n)
	}
}

func TestChangeTicketAssignsResolvesAndAudits(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	id := f.createTicket(t, f.buyer.ID, &f.orderID, nil)

	err := f.r.ChangeTicket(ctx, support.TicketChange{
		TicketID: id, ActorID: f.admin.ID, From: support.StatusOpen, To: support.StatusInProgress, AssignTo: &f.admin.ID,
		Action: "support.ticket_assigned", Details: map[string]any{"status": support.StatusInProgress},
	})
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	queue, err := f.r.GetTickets(ctx, support.Filter{AssignedTo: f.admin.ID, Active: true}, 10, 0)
	if err != nil {
		t.Fatalf("GetTickets: %v", err)
	}
	if len(queue) != 1 || queue[0].Assignee == nil || *queue[0].Assignee != f.admin.Name || queue[0].OpenerName != f.buyer.Name || queue[0].ClosedAt != nil {
		t.Fatalf("queue = %+v", queue)
	}

	resolution := "Troca autorizada pelo vendedor."
	err = f.r.ChangeTicket(ctx, support.TicketChange{
		TicketID: id, ActorID: f.admin.ID, From: support.StatusInProgress, To: support.StatusResolved, Resolution: &resolution,
		Action: "support.ticket_resolved", Details: map[string]any{"resolution": resolution},
		Notifications: []notifications.NewNotification{notifications.SupportTicketResolved(f.buyer.ID, id, "Produto errado", resolution)},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	ticket, err := f.r.GetTicket(ctx, id)
	if err != nil {
		t.Fatalf("GetTicket: %v", err)
	}
	if ticket.Status != support.StatusResolved || ticket.Resolution == nil || *ticket.Resolution != resolution || ticket.ClosedAt == nil ||
		ticket.AssignedTo == nil || *ticket.AssignedTo != f.admin.ID {
		t.Fatalf("ticket = %+v", ticket.TicketEntity)
	}
	if n := f.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND user_id = $2 AND action IN ('support.ticket_assigned', 'support.ticket_resolved')", id, f.admin.ID); n != 2 {
		t.Fatalf("audit rows = %d", n)
	}
	if n := f.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND title = 'Chamado resolvido'", f.buyer.ID); n != 1 {
		t.Fatalf("notifications = %d", n)
	}
	if active, _ := f.r.CountTickets(ctx, support.Filter{Active: true}); active != 0 {
		t.Fatalf("active = %d", active)
	}
	if unassigned, _ := f.r.CountTickets(ctx, support.Filter{Unassigned: true}); unassigned != 0 {
		t.Fatalf("unassigned = %d", unassigned)
	}

	err = f.r.ChangeTicket(ctx, support.TicketChange{TicketID: id, ActorID: f.admin.ID, From: support.StatusInProgress, To: support.StatusResolved, Action: "x"})
	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("stale: err = %v", err)
	}
	err = f.r.ChangeTicket(ctx, support.TicketChange{TicketID: "33333333-3333-3333-3333-333333333333", ActorID: f.admin.ID, Action: "x"})
	if !errors.Is(err, app.ErrTicketNotFound) {
		t.Fatalf("unknown: err = %v", err)
	}
}

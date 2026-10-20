package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func create(t *testing.T, pool *pgxpool.Pool, notification notifications.NewNotification, at time.Time) {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	if err := Create(ctx, tx, notification, at); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestCreateAndList(t *testing.T) {
	pool := testutil.DB(t)
	r := NewPostgresRepository(pool)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	other := testutil.InsertUser(t, pool)
	base := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

	create(t, pool, notifications.OrderPaid(user.ID, "order-1", "SC-1"), base)
	create(t, pool, notifications.OrderRefunded(user.ID, "order-2", "SC-2"), base.Add(time.Minute))
	create(t, pool, notifications.NewNotification{UserID: user.ID, Type: "custom", Title: "Sem dados", Body: "Corpo"}, base.Add(2*time.Minute))
	create(t, pool, notifications.OrderPaid(other.ID, "order-3", "SC-3"), base)

	total, unread, err := r.CountNotifications(ctx, user.ID)
	if err != nil || total != 3 || unread != 3 {
		t.Fatalf("CountNotifications = %d, %d, %v, want 3, 3", total, unread, err)
	}

	list, err := r.GetNotifications(ctx, user.ID, 10, 0)
	if err != nil || len(list) != 3 {
		t.Fatalf("GetNotifications = %d items, %v", len(list), err)
	}
	if list[0].Title != "Sem dados" || list[0].Data != nil {
		t.Errorf("newest = %+v, want the one without data", list[0])
	}
	if list[1].Type != notifications.TypeOrderRefunded || list[1].Data["order_id"] != "order-2" || list[1].ReadAt != nil || !list[1].CreatedAt.Equal(base.Add(time.Minute)) {
		t.Errorf("second = %+v", list[1])
	}

	page, err := r.GetNotifications(ctx, user.ID, 1, 2)
	if err != nil || len(page) != 1 || page[0].Type != notifications.TypeOrderPaid {
		t.Errorf("last page = %+v, %v", page, err)
	}
}

func TestMarkRead(t *testing.T) {
	pool := testutil.DB(t)
	r := NewPostgresRepository(pool)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	other := testutil.InsertUser(t, pool)
	create(t, pool, notifications.OrderPaid(user.ID, "order-1", "SC-1"), time.Now().UTC())
	create(t, pool, notifications.OrderPaid(user.ID, "order-2", "SC-2"), time.Now().UTC())
	list, _ := r.GetNotifications(ctx, user.ID, 10, 0)
	id := list[0].ID

	if err := r.MarkRead(ctx, other.ID, id); !errors.Is(err, app.ErrNotificationNotFound) {
		t.Errorf("another user's notification: error = %v", err)
	}
	if err := r.MarkRead(ctx, user.ID, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, app.ErrNotificationNotFound) {
		t.Errorf("unknown notification: error = %v", err)
	}

	if err := r.MarkRead(ctx, user.ID, id); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	list, _ = r.GetNotifications(ctx, user.ID, 10, 0)
	firstRead := list[0].ReadAt
	if firstRead == nil || list[1].ReadAt != nil {
		t.Fatalf("read_at = %v, %v, want only the first set", firstRead, list[1].ReadAt)
	}

	// Reading again keeps the first time.
	if err := r.MarkRead(ctx, user.ID, id); err != nil {
		t.Fatalf("MarkRead again: %v", err)
	}
	list, _ = r.GetNotifications(ctx, user.ID, 10, 0)
	if !list[0].ReadAt.Equal(*firstRead) {
		t.Errorf("read_at changed from %v to %v", firstRead, list[0].ReadAt)
	}
	if _, unread, _ := r.CountNotifications(ctx, user.ID); unread != 1 {
		t.Errorf("unread = %d, want 1", unread)
	}
}

func TestMarkAllRead(t *testing.T) {
	pool := testutil.DB(t)
	r := NewPostgresRepository(pool)
	ctx := context.Background()
	user := testutil.InsertUser(t, pool)
	other := testutil.InsertUser(t, pool)
	create(t, pool, notifications.OrderPaid(user.ID, "order-1", "SC-1"), time.Now().UTC())
	create(t, pool, notifications.OrderPaid(user.ID, "order-2", "SC-2"), time.Now().UTC())
	create(t, pool, notifications.OrderPaid(other.ID, "order-3", "SC-3"), time.Now().UTC())

	if err := r.MarkAllRead(ctx, user.ID); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
	if total, unread, _ := r.CountNotifications(ctx, user.ID); total != 2 || unread != 0 {
		t.Errorf("user: total, unread = %d, %d, want 2, 0", total, unread)
	}
	if _, unread, _ := r.CountNotifications(ctx, other.ID); unread != 1 {
		t.Errorf("other user: unread = %d, want 1", unread)
	}
}

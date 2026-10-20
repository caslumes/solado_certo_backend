package notifications

import (
	"context"
	"errors"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const (
	testUserID         = "user-1"
	testNotificationID = "00000000-0000-0000-0000-000000000901"
)

type fakeRepository struct {
	total, unread uint
	items         []*NotificationEntity
	gotLimit      uint
	gotOffset     uint
	read          []string
	readAll       []string
	err           error
}

func (f *fakeRepository) CountNotifications(_ context.Context, _ string) (uint, uint, error) {
	return f.total, f.unread, f.err
}

func (f *fakeRepository) GetNotifications(_ context.Context, _ string, limit uint, offset uint) ([]*NotificationEntity, error) {
	f.gotLimit, f.gotOffset = limit, offset
	return f.items, f.err
}

func (f *fakeRepository) MarkRead(_ context.Context, userID string, id string) error {
	if f.err != nil {
		return f.err
	}
	f.read = append(f.read, userID+":"+id)
	return nil
}

func (f *fakeRepository) MarkAllRead(_ context.Context, userID string) error {
	f.readAll = append(f.readAll, userID)
	return f.err
}

func TestListNotifications(t *testing.T) {
	r := &fakeRepository{total: 45, unread: 3, items: []*NotificationEntity{{ID: testNotificationID, Type: TypeOrderPaid}}}

	page, err := NewService(r).ListNotifications(context.Background(), testUserID, 3, 20)

	if err != nil {
		t.Fatalf("ListNotifications: %v", err)
	}
	if r.gotLimit != 20 || r.gotOffset != 40 {
		t.Errorf("limit, offset = %d, %d, want 20, 40", r.gotLimit, r.gotOffset)
	}
	if page.TotalPages != 3 || page.TotalItems != 45 || page.UnreadCount != 3 || len(page.Items) != 1 {
		t.Errorf("page = %+v", page)
	}
	output := page.ToOutput()
	if !output.HasPrevious || output.HasNext || output.UnreadCount != 3 {
		t.Errorf("output = %+v", output)
	}
}

func TestListNotificationsDefaultsAndLimits(t *testing.T) {
	r := &fakeRepository{}
	s := NewService(r)

	page, err := s.ListNotifications(context.Background(), testUserID, 0, 0)
	if err != nil || page.CurrentPage != 1 || r.gotLimit != defaultNotificationsPerPage || r.gotOffset != 0 {
		t.Errorf("defaults: page = %+v, limit = %d, offset = %d, error = %v", page, r.gotLimit, r.gotOffset, err)
	}
	if _, err := s.ListNotifications(context.Background(), testUserID, 1, maxNotificationsPerPage+1); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("limit too big: error = %v", err)
	}

	r.err = errors.New("database down")
	if _, err := s.ListNotifications(context.Background(), testUserID, 1, 10); !errors.Is(err, r.err) {
		t.Errorf("repository failure: error = %v", err)
	}
}

func TestMarkRead(t *testing.T) {
	r := &fakeRepository{}
	s := NewService(r)

	if err := s.MarkRead(context.Background(), testUserID, testNotificationID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if len(r.read) != 1 || r.read[0] != testUserID+":"+testNotificationID {
		t.Errorf("read = %v", r.read)
	}

	if err := s.MarkRead(context.Background(), testUserID, "not-a-uuid"); !errors.Is(err, app.ErrNotificationNotFound) {
		t.Errorf("bad id: error = %v", err)
	}
	if len(r.read) != 1 {
		t.Errorf("a bad id reached the repository: %v", r.read)
	}

	r.err = app.ErrNotificationNotFound
	if err := s.MarkRead(context.Background(), testUserID, testNotificationID); !errors.Is(err, app.ErrNotificationNotFound) {
		t.Errorf("unknown notification: error = %v", err)
	}
}

func TestMarkAllRead(t *testing.T) {
	r := &fakeRepository{}
	s := NewService(r)

	if err := s.MarkAllRead(context.Background(), testUserID); err != nil || len(r.readAll) != 1 {
		t.Errorf("MarkAllRead = %v, calls = %v", err, r.readAll)
	}

	r.err = errors.New("database down")
	if err := s.MarkAllRead(context.Background(), testUserID); !errors.Is(err, r.err) {
		t.Errorf("repository failure: error = %v", err)
	}
}

func TestNotificationTexts(t *testing.T) {
	paid := OrderPaid("seller-1", "order-1", "SC-20261019-ABC123")
	if paid.UserID != "seller-1" || paid.Type != TypeOrderPaid || paid.Data["order_id"] != "order-1" || paid.Title == "" {
		t.Errorf("OrderPaid = %+v", paid)
	}
	refunded := OrderRefunded("buyer-1", "order-1", "SC-20261019-ABC123")
	if refunded.UserID != "buyer-1" || refunded.Type != TypeOrderRefunded || refunded.Data["order_id"] != "order-1" {
		t.Errorf("OrderRefunded = %+v", refunded)
	}
}

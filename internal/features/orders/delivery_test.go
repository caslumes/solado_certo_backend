package orders

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
)

func TestDeliverOverdueOrders(t *testing.T) {
	r, s := fulfillmentFixture()
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	overdue := now.Add(-DeliveryDeadline - time.Minute)
	recent := now.Add(-DeliveryDeadline + time.Minute)
	r.placed[0].Status = OrderStatusShipped
	r.placed[2].Status = OrderStatusShipped
	r.shipments = map[string]*ShipmentEntity{
		r.placed[0].ID: {Carrier: "Correios", TrackingCode: "AA123456789BR", PostedAt: &overdue},
		r.placed[2].ID: {Carrier: "Correios", TrackingCode: "AA987654321BR", PostedAt: &recent},
	}

	if err := s.DeliverOverdueOrders(context.Background()); err != nil {
		t.Fatalf("DeliverOverdueOrders: %v", err)
	}

	if !r.postedBefore.Equal(now.Add(-DeliveryDeadline)) {
		t.Errorf("postedBefore = %v", r.postedBefore)
	}
	if len(r.changes) != 1 || r.placed[0].Status != OrderStatusDelivered || r.placed[2].Status != OrderStatusShipped {
		t.Fatalf("changes = %+v", r.changes)
	}
	change := r.changes[0]
	if change.ChangedBy != "" || change.From != OrderStatusShipped || change.Reason != ReasonDeliveryDeadline {
		t.Errorf("change = %+v, want a system change", change)
	}
	if n := change.Notifications; len(n) != 2 ||
		n[0].UserID != testBuyerID || n[0].Type != notifications.TypeDeliveryDeadline ||
		n[1].UserID != testSellerID || n[1].Type != notifications.TypeOrderDelivered {
		t.Errorf("notifications = %+v, want the buyer's and the seller's", n)
	}
}

func TestDeliverOverdueOrdersSkipsOrdersConfirmedMeanwhile(t *testing.T) {
	r, s := fulfillmentFixture()
	posted := time.Now().Add(-DeliveryDeadline - time.Hour)
	r.placed[0].Status = OrderStatusShipped
	r.shipments = map[string]*ShipmentEntity{r.placed[0].ID: {PostedAt: &posted}}
	r.changeErr = app.ErrInvalidStatusTransition

	if err := s.DeliverOverdueOrders(context.Background()); err != nil {
		t.Errorf("error = %v, want nil", err)
	}

	r.err = errors.New("database down")
	if err := s.DeliverOverdueOrders(context.Background()); !errors.Is(err, r.err) {
		t.Errorf("error = %v, want the repository's", err)
	}
}

type countingDeliveryJob struct {
	calls  int
	cancel context.CancelFunc
}

func (j *countingDeliveryJob) DeliverOverdueOrders(context.Context) error {
	j.calls++
	if j.calls == 3 {
		j.cancel()
	}
	return errors.New("ignored")
}

func TestRunDeliveryJobRunsUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	job := &countingDeliveryJob{cancel: cancel}

	done := make(chan struct{})
	go func() {
		RunDeliveryJob(ctx, job, time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("RunDeliveryJob didn't stop")
	}
	if job.calls != 3 {
		t.Errorf("calls = %d, want 3", job.calls)
	}
}

package orders

import (
	"context"
	"log/slog"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
)

type DeliverOverdueOrdersUseCase interface {
	DeliverOverdueOrders(ctx context.Context) error
}

const (
	DeliveryDeadline = 30 * 24 * time.Hour

	DeliveryJobInterval = time.Hour
)

// DeliverOverdueOrders marks as delivered the orders shipped more than DeliveryDeadline ago, so they
// can be reviewed and don't stay open forever (RF-023). Both sides are notified. Failures of a single
// order are logged and retried in the next round.
func (s *service) DeliverOverdueOrders(ctx context.Context) error {
	overdue, err := s.r.GetOverdueShippedOrders(ctx, s.now().Add(-DeliveryDeadline))
	if err != nil {
		slog.ErrorContext(ctx, "GetOverdueShippedOrders failed", "error", err)
		return err
	}
	for _, order := range overdue {
		err := s.changeStatus(ctx, StatusChange{
			OrderID: order.ID,
			From:    OrderStatusShipped,
			To:      OrderStatusDelivered,
			Reason:  ReasonDeliveryDeadline,
			Notifications: []notifications.NewNotification{
				notifications.BuyerDeliveryDeadline(order.BuyerID, order.ID, order.Number),
				notifications.SellerDeliveryDeadline(order.SellerID, order.ID, order.Number),
			},
		})
		// changeStatus logs the failures; a refused transition means the buyer confirmed it meanwhile.
		if err == nil {
			slog.InfoContext(ctx, "order delivered by deadline", "order_id", order.ID)
		}
	}
	return nil
}

// RunDeliveryJob runs DeliverOverdueOrders now and then every interval, until ctx is done.
func RunDeliveryJob(ctx context.Context, job DeliverOverdueOrdersUseCase, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = job.DeliverOverdueOrders(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

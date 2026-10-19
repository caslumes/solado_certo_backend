package payments

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type CheckPaymentsUseCase interface {
	CheckPayments(ctx context.Context) error
}

const (
	// PollInterval is how often the poller runs, as the fallback for webhooks that don't arrive.
	PollInterval = 2 * time.Minute

	// abandonedAttempt is how old an attempt without a charge must be before the poller closes it;
	// a younger one may still be waiting for the gateway's answer.
	abandonedAttempt = 5 * time.Minute
)

// CheckPayments is one round of the poller. It applies the gateway's status to pending charges,
// cancels the orders whose payment window is over (their units go back on sale), and cancels the
// charges left on closed orders, so an expired or cancelled order can't be paid, and refunds what must
// not be kept. Failures of a single payment or order are logged and retried in the next round.
func (s *service) CheckPayments(ctx context.Context) error {
	pending, err := s.r.GetPendingPayments(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "GetPendingPayments failed", "error", err)
		return err
	}
	// Pending charges come first, so a payment made just before the deadline is applied before
	// its order expires.
	for _, payment := range pending {
		_, _ = s.sync(ctx, payment, SourcePoll)
	}

	expireErr := s.expireOrders(ctx)

	closed, err := s.r.GetClosedOrderPayments(ctx, s.now().Add(-abandonedAttempt))
	if err != nil {
		slog.ErrorContext(ctx, "GetClosedOrderPayments failed", "error", err)
		return errors.Join(expireErr, err)
	}
	for _, payment := range closed {
		if payment.TransactionID == nil {
			_ = s.changeStatus(ctx, StatusChange{PaymentID: payment.ID, To: StatusCancelled, Source: SourceOrderClosed})
			continue
		}
		_, _ = s.cancelCharge(ctx, payment, SourceOrderClosed)
	}
	return errors.Join(expireErr, s.refunds(ctx))
}

func (s *service) expireOrders(ctx context.Context) error {
	createdBefore := expiredBefore(s.now())
	ids, err := s.r.GetExpiredOrderIDs(ctx, createdBefore)
	if err != nil {
		slog.ErrorContext(ctx, "GetExpiredOrderIDs failed", "error", err)
		return err
	}
	for _, id := range ids {
		expired, err := s.r.ExpireOrder(ctx, id, createdBefore)
		if err != nil {
			slog.ErrorContext(ctx, "ExpireOrder failed", "order_id", id, "error", err)
			continue
		}
		if expired {
			slog.InfoContext(ctx, "order expired without payment", "order_id", id)
		}
	}
	return nil
}

// RunPoller runs CheckPayments now and then every interval, until ctx is done.
func RunPoller(ctx context.Context, checker CheckPaymentsUseCase, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		// Errors are logged where they happen; the next round tries again.
		_ = checker.CheckPayments(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

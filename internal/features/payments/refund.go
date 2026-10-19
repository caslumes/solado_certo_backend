package payments

import (
	"context"
	"errors"
	"log/slog"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

// RefundRecord stores what the gateway answered to a refund that is not final: requested (a boleto
// waiting for the payer's bank account, or a refund in progress) or refused (Error, for the
// Attempts-th time).
type RefundRecord struct {
	PaymentID string
	Details   PaymentDetails
	Error     string
	Attempts  int
}

// RefundOrder returns the money of a cancelled order's captured payments (RF-017). The order service
// calls it right after the buyer cancels a paid order; when the gateway can't be reached, the poller
// tries again, so the cancellation itself never waits for the gateway.
func (s *service) RefundOrder(ctx context.Context, orderID string) error {
	order, err := s.r.GetOrder(ctx, orderID)
	if err != nil {
		slog.ErrorContext(ctx, "GetOrder failed", "order_id", orderID, "error", err)
		return err
	}
	if order.Status != orders.OrderStatusCancelled {
		return nil
	}
	payments, err := s.r.GetPayments(ctx, order.ID)
	if err != nil {
		slog.ErrorContext(ctx, "GetPayments failed", "order_id", order.ID, "error", err)
		return err
	}

	var errs []error
	for _, payment := range payments {
		if payment.Status != StatusCaptured || payment.TransactionID == nil || payment.Details.RefundRequestedAt != nil || payment.Details.RefundAttempts >= MaxRefundAttempts {
			continue
		}
		errs = append(errs, s.refund(ctx, payment))
	}
	return errors.Join(errs...)
}

// refunds is the poller's part for refunds: it asks back the money of captured payments that must
// not be kept (the order was cancelled, or another attempt paid it), and checks the refunds still
// on their way, in case their webhook doesn't arrive.
func (s *service) refunds(ctx context.Context) error {
	payments, err := s.r.GetPaymentsToRefund(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "GetPaymentsToRefund failed", "error", err)
		return err
	}
	for _, payment := range payments {
		if payment.Details.RefundRequestedAt != nil {
			_, _ = s.sync(ctx, payment, SourcePoll)
			continue
		}
		_ = s.refund(ctx, payment)
	}
	return nil
}

// refund asks the gateway for a captured payment's money. The charge is read first: a refund already
// asked for (an answer lost on the way back, or one made in the gateway's dashboard) is not asked
// again, so the money is never returned twice. A refusal is recorded and asked again by the poller
// until MaxRefundAttempts, then left to support (Slice 9); an unreachable gateway changes nothing and
// is tried again by the poller (RNF-013).
func (s *service) refund(ctx context.Context, payment *PaymentEntity) error {
	charge, err := s.provider.GetCharge(ctx, *payment.TransactionID)
	if err != nil {
		slog.ErrorContext(ctx, "GetCharge failed", "payment_id", payment.ID, "error", err)
		return app.ErrGatewayUnavailable
	}
	if charge.Status == gateway.StatusCaptured && !charge.RefundRequested {
		charge, err = s.provider.Refund(ctx, *payment.TransactionID, gateway.Method(payment.Method))
		if err != nil {
			if errors.Is(err, gateway.ErrUnavailable) {
				slog.ErrorContext(ctx, "Refund failed", "payment_id", payment.ID, "order_id", payment.OrderID, "error", err)
				return app.ErrGatewayUnavailable
			}
			attempts := payment.Details.RefundAttempts + 1
			level := slog.LevelWarn
			if attempts >= MaxRefundAttempts {
				level = slog.LevelError
			}
			slog.Log(ctx, level, "refund refused", "payment_id", payment.ID, "order_id", payment.OrderID, "attempt", attempts, "error", err)
			if recordErr := s.recordRefund(ctx, RefundRecord{PaymentID: payment.ID, Error: err.Error(), Attempts: attempts}); recordErr != nil {
				return recordErr
			}
			return err
		}
	}

	now := s.now().UTC()
	details := chargeDetails(charge)
	details.RefundRequestedAt = &now
	details.RefundURL = charge.RefundURL
	switch {
	case Status(charge.Status) == StatusRefunded:
		return s.changeStatus(ctx, StatusChange{PaymentID: payment.ID, To: StatusRefunded, Source: SourceRefund, Details: details})
	case charge.RefundRequested:
		return s.recordRefund(ctx, RefundRecord{PaymentID: payment.ID, Details: details})
	default:
		// Not captured any more (a chargeback, for example): there is nothing to refund.
		slog.WarnContext(ctx, "payment not refundable", "payment_id", payment.ID, "gateway_status", charge.GatewayStatus)
		return nil
	}
}

func (s *service) recordRefund(ctx context.Context, record RefundRecord) error {
	if err := s.r.RecordRefund(ctx, record); err != nil {
		slog.ErrorContext(ctx, "RecordRefund failed", "payment_id", record.PaymentID, "error", err)
		return err
	}
	return nil
}

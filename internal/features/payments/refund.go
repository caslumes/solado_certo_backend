package payments

import (
	"context"
	"errors"
	"log/slog"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

type RefundRecord struct {
	PaymentID string
	Details   PaymentDetails
	Error     string
	Attempts  int
}

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

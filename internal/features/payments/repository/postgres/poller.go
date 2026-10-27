package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	ordersdb "github.com/caslumes/solado_certo_backend/internal/features/orders/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	"github.com/jackc/pgx/v5"
)

const pollBatch = 200

func (r *postgresRepository) GetPendingPayments(ctx context.Context) ([]*payments.PaymentEntity, error) {
	return r.getPayments(ctx, "status = 'pending' AND gateway_transaction_id IS NOT NULL ORDER BY created_at, id LIMIT $1", pollBatch)
}

func (r *postgresRepository) GetClosedOrderPayments(ctx context.Context, startedBefore time.Time) ([]*payments.PaymentEntity, error) {
	return r.getPayments(
		ctx,
		"status = 'pending' AND (gateway_transaction_id IS NOT NULL OR created_at < $1) "+
			"AND order_id IN (SELECT id FROM orders WHERE status <> 'awaiting_payment') ORDER BY created_at, id LIMIT $2",
		startedBefore, pollBatch,
	)
}

func (r *postgresRepository) getPayments(ctx context.Context, where string, args ...any) ([]*payments.PaymentEntity, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+paymentColumns+" FROM payments WHERE "+where, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*payments.PaymentEntity, error) {
		return scanPayment(row)
	})
}

func (r *postgresRepository) GetExpiredOrderIDs(ctx context.Context, createdBefore time.Time) ([]string, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT id FROM orders WHERE status = 'awaiting_payment' AND created_at < $1 ORDER BY created_at, id LIMIT $2",
		createdBefore, pollBatch,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (r *postgresRepository) ExpireOrder(ctx context.Context, orderID string, createdBefore time.Time) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var status orders.OrderStatus
	var createdAt time.Time
	if err := tx.QueryRow(ctx, "SELECT status, created_at FROM orders WHERE id = $1 FOR UPDATE", orderID).Scan(&status, &createdAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, app.ErrOrderNotFound
		}
		return false, err
	}
	if status != orders.OrderStatusAwaitingPayment || !createdAt.Before(createdBefore) {
		return false, nil
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, "UPDATE orders SET status = 'cancelled', updated_at = $2 WHERE id = $1", orderID, now); err != nil {
		return false, err
	}
	if err := ordersdb.ReleaseStock(ctx, tx, orderID); err != nil {
		return false, err
	}
	err = ordersdb.RecordStatusChange(ctx, tx, orderID, "", &status, orders.OrderStatusCancelled, payments.ReasonPaymentExpired, map[string]any{
		"stock_released": true,
	}, now)
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (r *postgresRepository) GetPaymentsToRefund(ctx context.Context) ([]*payments.PaymentEntity, error) {
	return r.getPayments(
		ctx,
		"status = 'captured' AND gateway_transaction_id IS NOT NULL AND COALESCE((gateway_payload->>'refund_attempts')::int, 0) < $2 AND ("+
			"COALESCE(gateway_payload, '{}') ? 'refund_requested_at' "+
			"OR order_id IN (SELECT id FROM orders WHERE status = 'cancelled') "+
			"OR EXISTS (SELECT 1 FROM payments earlier WHERE earlier.order_id = payments.order_id "+
			"AND earlier.status IN ('captured', 'refunded') AND (earlier.captured_at, earlier.id) < (payments.captured_at, payments.id))"+
			") ORDER BY captured_at, id LIMIT $1",
		pollBatch, payments.MaxRefundAttempts,
	)
}

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

// pollBatch caps how many rows one round of the poller handles; the rest wait for the next round.
const pollBatch = 200

// GetPendingPayments returns the pending payments that have a charge, oldest first.
func (r *postgresRepository) GetPendingPayments(ctx context.Context) ([]*payments.PaymentEntity, error) {
	return r.getPayments(ctx, "status = 'pending' AND gateway_transaction_id IS NOT NULL ORDER BY created_at, id LIMIT $1", pollBatch)
}

// GetClosedOrderPayments returns the pending payments of orders no longer awaiting payment. An
// attempt without a charge is only returned once it is older than startedBefore, so a charge being
// created right now isn't taken for an abandoned one.
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

// GetExpiredOrderIDs returns the orders still awaiting payment that were placed before createdBefore.
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

// ExpireOrder cancels an order whose payment window is over and puts its units back on sale, as a
// system change. It returns false when the order was paid or cancelled meanwhile.
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

package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/jackc/pgx/v5"
)

func (r *postgresRepository) CountOrdersByBuyer(ctx context.Context, buyerID string) (uint, error) {
	var total uint
	err := r.pool.QueryRow(ctx, "SELECT count(*) FROM orders WHERE buyer_id = $1", buyerID).Scan(&total)
	return total, err
}

func (r *postgresRepository) GetOrdersByBuyer(ctx context.Context, buyerID string, limit uint, offset uint) ([]*orders.OrderEntity, error) {
	return r.getOrders(ctx, "o.buyer_id = $1 ORDER BY o.created_at DESC, o.id DESC LIMIT $2 OFFSET $3", buyerID, limit, offset)
}

func (r *postgresRepository) GetOrder(ctx context.Context, id string) (*orders.OrderEntity, error) {
	found, err := r.getOrders(ctx, "o.id = $1", id)
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, app.ErrOrderNotFound
	}
	return found[0], nil
}

func (r *postgresRepository) GetStatusHistory(ctx context.Context, orderID string) ([]orders.StatusChangeEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT previous_status, new_status, reason, created_at FROM order_status_history WHERE order_id = $1 ORDER BY created_at, id",
		orderID,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (orders.StatusChangeEntity, error) {
		var change orders.StatusChangeEntity
		err := row.Scan(&change.From, &change.To, &change.Reason, &change.CreatedAt)
		return change, err
	})
}

// ChangeOrderStatus locks the order, so two changes can't both start from the same status.
func (r *postgresRepository) ChangeOrderStatus(ctx context.Context, change orders.StatusChange) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status orders.OrderStatus
	if err := tx.QueryRow(ctx, "SELECT status FROM orders WHERE id = $1 FOR UPDATE", change.OrderID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrOrderNotFound
		}
		return err
	}
	if status != change.From {
		return app.ErrInvalidStatusTransition
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, "UPDATE orders SET status = $2, updated_at = $3 WHERE id = $1", change.OrderID, change.To, now); err != nil {
		return err
	}
	if change.ReleaseStock {
		_, err := tx.Exec(
			ctx,
			"UPDATE advertisements a SET stock = a.stock + oi.quantity FROM order_items oi WHERE oi.order_id = $1 AND a.id = oi.advertisement_id",
			change.OrderID,
		)
		if err != nil {
			return err
		}
	}

	from := change.From
	err = recordStatusChange(ctx, tx, change.OrderID, change.ChangedBy, &from, change.To, change.Reason, map[string]any{
		"stock_released": change.ReleaseStock,
	}, now)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

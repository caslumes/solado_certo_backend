package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/jackc/pgx/v5"
)

func syncCustomOrder(ctx context.Context, tx pgx.Tx, orderID string, changedBy *string, from *orders.OrderStatus, to orders.OrderStatus, now time.Time) error {
	switch {
	case to == orders.OrderStatusPaid:
		return moveCustomOrder(ctx, tx, orderID, changedBy, "approved", "in_production", "custom_order.order_paid", now)
	case to == orders.OrderStatusCancelled && from != nil && *from == orders.OrderStatusPaid:
		return moveCustomOrder(ctx, tx, orderID, changedBy, "in_production", "cancelled", "custom_order.order_cancelled", now)
	case to == orders.OrderStatusCancelled:
		rows, err := tx.Query(
			ctx,
			"UPDATE custom_order_requests SET order_id = NULL, updated_at = $2 WHERE order_id = $1 AND status = 'approved' RETURNING id",
			orderID, now,
		)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		for _, id := range ids {
			if err := auditCustomOrder(ctx, tx, changedBy, id, "custom_order.order_released", orderID, now); err != nil {
				return err
			}
		}
	}
	return nil
}

func moveCustomOrder(ctx context.Context, tx pgx.Tx, orderID string, changedBy *string, from string, to string, action string, now time.Time) error {
	rows, err := tx.Query(
		ctx,
		"UPDATE custom_order_requests SET status = $3, updated_at = $4 WHERE order_id = $1 AND status = $2 RETURNING id",
		orderID, from, to, now,
	)
	if err != nil {
		return err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, id := range ids {
		_, err := tx.Exec(
			ctx,
			"INSERT INTO custom_order_request_history (request_id, changed_by, previous_status, new_status, created_at) VALUES ($1, $2, $3, $4, $5)",
			id, changedBy, from, to, now,
		)
		if err != nil {
			return err
		}
		if err := auditCustomOrder(ctx, tx, changedBy, id, action, orderID, now); err != nil {
			return err
		}
	}
	return nil
}

func auditCustomOrder(ctx context.Context, tx pgx.Tx, changedBy *string, requestID string, action string, orderID string, now time.Time) error {
	details, err := json.Marshal(map[string]any{"order_id": orderID})
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'custom_order', $2, $3, $4, $5)",
		changedBy, requestID, action, details, now,
	)
	return err
}

func PendingFittingReports(ctx context.Context, db Querier, orderID string) ([]string, error) {
	rows, err := db.Query(
		ctx,
		"SELECT oi.id FROM order_items oi JOIN custom_order_requests cr ON cr.id = oi.custom_order_request_id "+
			"WHERE oi.order_id = $1 AND cr.status <> 'completed' ORDER BY oi.created_at, oi.id",
		orderID,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

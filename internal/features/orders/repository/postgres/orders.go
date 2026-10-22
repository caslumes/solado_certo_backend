package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	notificationsdb "github.com/caslumes/solado_certo_backend/internal/features/notifications/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const uniqueViolation = "23505"

// historyFilter is the query's status ($2) and range ($3, $4) on the given column.
func historyFilter(column string) string {
	return " AND ($2 = '' OR o.status = $2) AND ($3::timestamptz IS NULL OR " + column + " >= $3) AND ($4::timestamptz IS NULL OR " + column + " < $4)"
}

func historyArgs(id string, query orders.OrderQuery) []any {
	return []any{id, string(query.Status), query.Since, query.Until}
}

// buyerOrders are the buyer's orders, filtered by order date.
var buyerOrders = "o.buyer_id = $1" + historyFilter("o.created_at")

func (r *postgresRepository) CountOrdersByBuyer(ctx context.Context, buyerID string, query orders.OrderQuery) (uint, error) {
	var total uint
	err := r.pool.QueryRow(ctx, "SELECT count(*) FROM orders o WHERE "+buyerOrders, historyArgs(buyerID, query)...).Scan(&total)
	return total, err
}

func (r *postgresRepository) GetOrdersByBuyer(ctx context.Context, buyerID string, query orders.OrderQuery, limit uint, offset uint) ([]*orders.OrderEntity, error) {
	return r.getOrders(ctx, buyerOrders+" ORDER BY o.created_at DESC, o.id DESC LIMIT $5 OFFSET $6", append(historyArgs(buyerID, query), limit, offset)...)
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

// ChangeOrderStatus locks the order, so two changes can't both start from the same status. The
// shipment (when shipped), the delivery time (when delivered) and the notifications are written in
// the same transaction.
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
		if err := ReleaseStock(ctx, tx, change.OrderID); err != nil {
			return err
		}
	}
	auditContext := map[string]any{"stock_released": change.ReleaseStock}
	if change.Shipment != nil {
		_, err := tx.Exec(
			ctx,
			"INSERT INTO shipments (order_id, carrier, tracking_code, status, posted_at, created_at, updated_at) VALUES ($1, $2, $3, 'posted', $4, $4, $4)",
			change.OrderID, change.Shipment.Carrier, change.Shipment.TrackingCode, now,
		)
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == uniqueViolation {
			return app.ErrTrackingCodeInUse
		}
		if err != nil {
			return err
		}
		auditContext["carrier"] = change.Shipment.Carrier
		auditContext["tracking_code"] = change.Shipment.TrackingCode
	}
	if change.To == orders.OrderStatusDelivered {
		if _, err := tx.Exec(ctx, "UPDATE orders SET delivered_at = $2 WHERE id = $1", change.OrderID, now); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "UPDATE shipments SET status = 'delivered', delivered_at = $2, updated_at = $2 WHERE order_id = $1", change.OrderID, now); err != nil {
			return err
		}
	}

	from := change.From
	if err := RecordStatusChange(ctx, tx, change.OrderID, change.ChangedBy, &from, change.To, change.Reason, auditContext, now); err != nil {
		return err
	}
	for _, notification := range change.Notifications {
		if err := notificationsdb.Create(ctx, tx, notification, now); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// paidOrders are the orders once paid, filtered by payment date.
var paidOrders = "o.paid_at IS NOT NULL" + historyFilter("o.paid_at")

func (r *postgresRepository) CountOrdersBySeller(ctx context.Context, sellerID string, query orders.OrderQuery) (uint, error) {
	var total uint
	err := r.pool.QueryRow(
		ctx,
		"SELECT count(*) FROM orders o WHERE "+paidOrders+
			" AND EXISTS (SELECT 1 FROM order_items oi WHERE oi.order_id = o.id AND oi.seller_id = $1)",
		historyArgs(sellerID, query)...,
	).Scan(&total)
	return total, err
}

func (r *postgresRepository) GetOrdersBySeller(ctx context.Context, sellerID string, query orders.OrderQuery, limit uint, offset uint) ([]*orders.OrderEntity, error) {
	return r.getOrders(ctx, "s.id = $1 AND "+paidOrders+" ORDER BY o.paid_at DESC, o.id DESC LIMIT $5 OFFSET $6", append(historyArgs(sellerID, query), limit, offset)...)
}

// overdueBatch caps how many orders one round of the delivery job handles; the rest wait for the next round.
const overdueBatch = 200

// GetOverdueShippedOrders returns the orders still shipped whose shipment was posted before postedBefore.
func (r *postgresRepository) GetOverdueShippedOrders(ctx context.Context, postedBefore time.Time) ([]*orders.OrderEntity, error) {
	return r.getOrders(
		ctx,
		"o.status = 'shipped' AND EXISTS (SELECT 1 FROM shipments sh WHERE sh.order_id = o.id AND sh.posted_at < $1) ORDER BY o.updated_at, o.id LIMIT $2",
		postedBefore, overdueBatch,
	)
}

// GetShipment returns the order's shipment, or nil when it wasn't shipped.
func (r *postgresRepository) GetShipment(ctx context.Context, orderID string) (*orders.ShipmentEntity, error) {
	var s orders.ShipmentEntity
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, carrier, tracking_code, status, posted_at, delivered_at FROM shipments WHERE order_id = $1 ORDER BY created_at DESC LIMIT 1",
		orderID,
	).Scan(&s.ID, &s.Carrier, &s.TrackingCode, &s.Status, &s.PostedAt, &s.DeliveredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ReleaseStock puts the order's units back on sale.
func ReleaseStock(ctx context.Context, tx pgx.Tx, orderID string) error {
	_, err := tx.Exec(
		ctx,
		"UPDATE advertisements a SET stock = a.stock + oi.quantity FROM order_items oi WHERE oi.order_id = $1 AND a.id = oi.advertisement_id",
		orderID,
	)
	return err
}

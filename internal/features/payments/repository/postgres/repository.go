package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	ordersdb "github.com/caslumes/solado_certo_backend/internal/features/orders/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolation = "23505"

const paymentColumns = "id, order_id, gateway, gateway_transaction_id, method, status, amount::text, due_date, " +
	"failure_reason, COALESCE(gateway_payload, '{}'), captured_at, refunded_at, created_at, updated_at"

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) GetOrder(ctx context.Context, id string) (*payments.OrderEntity, error) {
	var order payments.OrderEntity
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, buyer_id, number, status, subtotal::text, shipping_total::text, discount_total::text, total::text, "+
			"shipping_address->>'zip_code', shipping_address->>'number', created_at FROM orders WHERE id = $1",
		id,
	).Scan(
		&order.ID, &order.BuyerID, &order.Number, &order.Status, &order.Subtotal, &order.ShippingTotal,
		&order.DiscountTotal, &order.Total, &order.PostalCode, &order.AddressNumber, &order.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrOrderNotFound
		}
		return nil, err
	}
	return &order, nil
}

func (r *postgresRepository) GetBuyer(ctx context.Context, id string) (*payments.BuyerEntity, error) {
	var buyer payments.BuyerEntity
	err := r.pool.QueryRow(ctx, "SELECT id, name, email, phone, tax_id FROM users WHERE id = $1", id).
		Scan(&buyer.ID, &buyer.Name, &buyer.Email, &buyer.Phone, &buyer.TaxID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrUserNotFound
		}
		return nil, err
	}
	return &buyer, nil
}

// SetTaxID saves the CPF only if the user has none yet.
func (r *postgresRepository) SetTaxID(ctx context.Context, userID string, taxID string) error {
	_, err := r.pool.Exec(ctx, "UPDATE users SET tax_id = $2, updated_at = $3 WHERE id = $1 AND tax_id IS NULL", userID, taxID, time.Now().UTC())
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == uniqueViolation {
		return app.ErrTaxIDInUse
	}
	return err
}

func (r *postgresRepository) GetCustomerID(ctx context.Context, userID string, gateway string) (string, bool, error) {
	var id string
	err := r.pool.QueryRow(ctx, "SELECT customer_id FROM payment_customers WHERE user_id = $1 AND gateway = $2", userID, gateway).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	return id, err == nil, err
}

func (r *postgresRepository) SaveCustomerID(ctx context.Context, userID string, gateway string, customerID string) error {
	_, err := r.pool.Exec(
		ctx,
		"INSERT INTO payment_customers (user_id, gateway, customer_id, created_at) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING",
		userID, gateway, customerID, time.Now().UTC(),
	)
	return err
}

func (r *postgresRepository) GetPaymentByKey(ctx context.Context, idempotencyKey string) (*payments.PaymentEntity, error) {
	return r.getPayment(ctx, r.pool, "idempotency_key = $1", idempotencyKey)
}

func (r *postgresRepository) GetPaymentByTransaction(ctx context.Context, gateway string, transactionID string) (*payments.PaymentEntity, error) {
	return r.getPayment(ctx, r.pool, "gateway = $1 AND gateway_transaction_id = $2", gateway, transactionID)
}

func (r *postgresRepository) GetPayment(ctx context.Context, id string) (*payments.PaymentEntity, error) {
	return r.getPayment(ctx, r.pool, "id = $1", id)
}

func (r *postgresRepository) GetPayments(ctx context.Context, orderID string) ([]*payments.PaymentEntity, error) {
	rows, err := r.pool.Query(ctx, "SELECT "+paymentColumns+" FROM payments WHERE order_id = $1 ORDER BY created_at DESC, id DESC", orderID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*payments.PaymentEntity, error) {
		return scanPayment(row)
	})
}

func (r *postgresRepository) getPayment(ctx context.Context, db querier, where string, args ...any) (*payments.PaymentEntity, error) {
	payment, err := scanPayment(db.QueryRow(ctx, "SELECT "+paymentColumns+" FROM payments WHERE "+where, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, app.ErrPaymentNotFound
	}
	return payment, err
}

func scanPayment(row pgx.Row) (*payments.PaymentEntity, error) {
	var p payments.PaymentEntity
	var details []byte
	err := row.Scan(
		&p.ID, &p.OrderID, &p.Gateway, &p.TransactionID, &p.Method, &p.Status, &p.Amount, &p.DueDate,
		&p.FailureReason, &details, &p.CapturedAt, &p.RefundedAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(details, &p.Details); err != nil {
		return nil, err
	}
	return &p, nil
}

// StartPayment records a pending attempt and sets the order's discount and total for its method,
// with the order locked so it can't be paid or cancelled meanwhile. A key already used returns
// that payment; another attempt still pending means a concurrent request (ErrPaymentInProgress).
func (r *postgresRepository) StartPayment(ctx context.Context, payment payments.NewPayment) (*payments.PaymentEntity, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	var status orders.OrderStatus
	var discount string
	err = tx.QueryRow(ctx, "SELECT status, discount_total::text FROM orders WHERE id = $1 FOR UPDATE", payment.OrderID).Scan(&status, &discount)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, app.ErrOrderNotFound
		}
		return nil, false, err
	}

	if existing, err := r.getPayment(ctx, tx, "idempotency_key = $1", payment.IdempotencyKey); err == nil {
		if existing.OrderID != payment.OrderID {
			return nil, false, app.ErrIdempotencyKeyReused
		}
		return existing, false, nil
	} else if !errors.Is(err, app.ErrPaymentNotFound) {
		return nil, false, err
	}
	if status != orders.OrderStatusAwaitingPayment {
		return nil, false, app.ErrInvalidStatusTransition
	}
	var pending bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM payments WHERE order_id = $1 AND status = 'pending')", payment.OrderID).Scan(&pending); err != nil {
		return nil, false, err
	}
	if pending {
		return nil, false, app.ErrPaymentInProgress
	}

	now := time.Now().UTC()
	var id string
	err = tx.QueryRow(
		ctx,
		"INSERT INTO payments (order_id, gateway, idempotency_key, status, method, amount, due_date, gateway_payload, created_at, updated_at) "+
			"VALUES ($1, $2, $3, 'pending', $4, $5, $6, '{}', $7, $7) RETURNING id",
		payment.OrderID, payment.Gateway, payment.IdempotencyKey, payment.Method, payment.Amount, payment.DueDate, now,
	).Scan(&id)
	if err != nil {
		return nil, false, err
	}
	if err := auditBy(ctx, tx, payment.CreatedBy, "payment", id, "payment.created", map[string]any{
		"order_id": payment.OrderID, "gateway": payment.Gateway, "method": payment.Method, "amount": payment.Amount,
	}, now); err != nil {
		return nil, false, err
	}

	if discount != payment.DiscountTotal {
		_, err := tx.Exec(
			ctx,
			"UPDATE orders SET discount_total = $2, total = subtotal + shipping_total - $2, updated_at = $3 WHERE id = $1",
			payment.OrderID, payment.DiscountTotal, now,
		)
		if err != nil {
			return nil, false, err
		}
		if err := auditBy(ctx, tx, payment.CreatedBy, "order", payment.OrderID, "order.discount_changed", map[string]any{
			"from": discount, "to": payment.DiscountTotal, "method": payment.Method, "payment_id": id,
		}, now); err != nil {
			return nil, false, err
		}
	}

	created, err := r.getPayment(ctx, tx, "id = $1", id)
	if err != nil {
		return nil, false, err
	}
	return created, true, tx.Commit(ctx)
}

func (r *postgresRepository) AttachCharge(ctx context.Context, paymentID string, transactionID string, details payments.PaymentDetails) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(
		ctx,
		"UPDATE payments SET gateway_transaction_id = $2, gateway_payload = COALESCE(gateway_payload, '{}') || $3, updated_at = $4 WHERE id = $1",
		paymentID, transactionID, detailsJSON, time.Now().UTC(),
	)
	return err
}

// ChangePaymentStatus applies a status in one transaction (RNF-011): the payment, its audit row and,
// for a capture, the order (paid, with history). The order is locked before the payment, as
// StartPayment does, so the two can't deadlock. A change the status machine doesn't allow, or to the
// status the payment already has, changes nothing and returns false (RNF-012).
func (r *postgresRepository) ChangePaymentStatus(ctx context.Context, change payments.StatusChange) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var orderID string
	if err := tx.QueryRow(ctx, "SELECT order_id FROM payments WHERE id = $1", change.PaymentID).Scan(&orderID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, app.ErrPaymentNotFound
		}
		return false, err
	}
	var orderStatus orders.OrderStatus
	if err := tx.QueryRow(ctx, "SELECT status FROM orders WHERE id = $1 FOR UPDATE", orderID).Scan(&orderStatus); err != nil {
		return false, err
	}
	var from payments.Status
	if err := tx.QueryRow(ctx, "SELECT status FROM payments WHERE id = $1 FOR UPDATE", change.PaymentID).Scan(&from); err != nil {
		return false, err
	}
	if !from.CanBecome(change.To) {
		return false, nil
	}

	now := time.Now().UTC()
	detailsJSON, err := json.Marshal(change.Details)
	if err != nil {
		return false, err
	}
	var failureReason *string
	if change.FailureReason != "" {
		failureReason = &change.FailureReason
	}
	_, err = tx.Exec(
		ctx,
		"UPDATE payments SET status = $2::varchar, failure_reason = COALESCE($3, failure_reason), "+
			"gateway_payload = COALESCE(gateway_payload, '{}') || $4, updated_at = $5, "+
			"captured_at = CASE WHEN $2::varchar = 'captured' THEN $5 ELSE captured_at END, "+
			"refunded_at = CASE WHEN $2::varchar = 'refunded' THEN $5 ELSE refunded_at END WHERE id = $1",
		change.PaymentID, change.To, failureReason, detailsJSON, now,
	)
	if err != nil {
		return false, err
	}
	if err := audit(ctx, tx, "payment", change.PaymentID, "payment.status_changed", map[string]any{
		"order_id": orderID, "from": from, "to": change.To, "source": change.Source, "failure_reason": change.FailureReason,
	}, now); err != nil {
		return false, err
	}

	if change.To == payments.StatusCaptured {
		if err := markPaid(ctx, tx, orderID, orderStatus, change.PaymentID, now); err != nil {
			return false, err
		}
	}
	return true, tx.Commit(ctx)
}

// markPaid moves the order to paid. An order no longer awaiting payment (cancelled because it
// expired, or already paid by another attempt) keeps its status, and the capture is audited so the
// money can be refunded.
func markPaid(ctx context.Context, tx pgx.Tx, orderID string, status orders.OrderStatus, paymentID string, now time.Time) error {
	if !status.CanBecome(orders.OrderStatusPaid) {
		return audit(ctx, tx, "order", orderID, "order.payment_captured_after_close", map[string]any{
			"status": status, "payment_id": paymentID,
		}, now)
	}
	if _, err := tx.Exec(ctx, "UPDATE orders SET status = 'paid', paid_at = $2, updated_at = $2 WHERE id = $1", orderID, now); err != nil {
		return err
	}
	return ordersdb.RecordStatusChange(ctx, tx, orderID, "", &status, orders.OrderStatusPaid, payments.ReasonPaymentCaptured, map[string]any{
		"payment_id": paymentID,
	}, now)
}

// audit records a change made by the system (a webhook, the poller or the gateway's answer).
func audit(ctx context.Context, tx pgx.Tx, entity string, entityID string, action string, details map[string]any, now time.Time) error {
	return auditBy(ctx, tx, "", entity, entityID, action, details, now)
}

func auditBy(ctx context.Context, tx pgx.Tx, userID string, entity string, entityID string, action string, details map[string]any, now time.Time) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}
	var user *string
	if userID != "" {
		user = &userID
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
		user, entity, entityID, action, detailsJSON, now,
	)
	return err
}

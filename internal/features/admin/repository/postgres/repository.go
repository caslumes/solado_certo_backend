package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/admin"
	notificationsdb "github.com/caslumes/solado_certo_backend/internal/features/notifications/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/features/payments"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

type conditions struct {
	clauses []string
	args    []any
}

func (c *conditions) add(clause string, args ...any) {
	for _, arg := range args {
		c.args = append(c.args, arg)
		clause = strings.Replace(clause, "?", fmt.Sprintf("$%d", len(c.args)), 1)
	}
	c.clauses = append(c.clauses, clause)
}

func (c *conditions) where() string {
	if len(c.clauses) == 0 {
		return " WHERE TRUE"
	}
	return " WHERE " + strings.Join(c.clauses, " AND ")
}

func (c *conditions) page(limit uint, offset uint) (string, []any) {
	n := len(c.args)
	return fmt.Sprintf(" LIMIT $%d OFFSET $%d", n+1, n+2), append(append([]any{}, c.args...), limit, offset)
}

func contains(search string) string {
	return "%" + escapeLike(search) + "%"
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(value)
}

func list[T any](ctx context.Context, pool *pgxpool.Pool, from string, columns string, order string, c conditions, limit uint, offset uint, scan func(pgx.Row) (T, error)) ([]T, uint, error) {
	var total uint
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM "+from+c.where(), c.args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	paging, args := c.page(limit, offset)
	rows, err := pool.Query(ctx, "SELECT "+columns+" FROM "+from+c.where()+" ORDER BY "+order+paging, args...)
	if err != nil {
		return nil, 0, err
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (T, error) {
		return scan(row)
	})
	return items, total, err
}

func audit(ctx context.Context, tx pgx.Tx, userID string, entity string, entityID string, action string, details map[string]any, now time.Time) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
		userID, entity, entityID, action, detailsJSON, now,
	)
	return err
}

const userColumns = "u.id, u.name, u.email, u.phone, u.status, u.suspended_until, u.created_at, " +
	"ARRAY(SELECT ro.code FROM user_roles ur JOIN roles ro ON ro.id = ur.role_id WHERE ur.user_id = u.id ORDER BY ro.code)"

func scanUser(row pgx.Row) (*admin.UserEntity, error) {
	var u admin.UserEntity
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.Status, &u.SuspendedUntil, &u.CreatedAt, &u.Roles)
	return &u, err
}

func (r *postgresRepository) ListUsers(ctx context.Context, filter admin.UserFilter, limit uint, offset uint) ([]*admin.UserEntity, uint, error) {
	var c conditions
	c.add("u.deleted_at IS NULL")
	if filter.Search != "" {
		search := contains(filter.Search)
		c.add("(u.name ILIKE ? OR u.email ILIKE ? OR u.phone LIKE ?)", search, search, search)
	}
	switch filter.Status {
	case "":
	case enum.ProfileStatusSuspended:
		c.add("u.status <> 'blocked' AND u.suspended_until > ?", filter.Now)
	case enum.ProfileStatusBlocked:
		c.add("u.status = 'blocked'")
	default:
		c.add("u.status = ? AND (u.suspended_until IS NULL OR u.suspended_until <= ?)", filter.Status, filter.Now)
	}
	if filter.Role != "" {
		c.add("EXISTS (SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id = ur.role_id WHERE ur.user_id = u.id AND ro.code = ?)", filter.Role)
	}
	return list(ctx, r.pool, "users u", userColumns, "u.created_at DESC, u.id", c, limit, offset, scanUser)
}

func (r *postgresRepository) GetUser(ctx context.Context, id string) (*admin.UserEntity, error) {
	user, err := scanUser(r.pool.QueryRow(ctx, "SELECT "+userColumns+" FROM users u WHERE u.id = $1 AND u.deleted_at IS NULL", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func (r *postgresRepository) StatusBeforeBlock(ctx context.Context, userID string) (enum.ProfileStatus, error) {
	var status enum.ProfileStatus
	err := r.pool.QueryRow(
		ctx,
		"SELECT COALESCE(context->>'previous_status', '') FROM audit_logs WHERE entity = 'user' AND entity_id = $1 AND action = $2 "+
			"ORDER BY created_at DESC LIMIT 1",
		userID, admin.ActionBlockUser,
	).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return status, err
}

func (r *postgresRepository) ModerateUser(ctx context.Context, moderation admin.UserModeration) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status enum.ProfileStatus
	err = tx.QueryRow(ctx, "SELECT status FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE", moderation.UserID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrUserNotFound
		}
		return err
	}
	if status != moderation.FromStatus {
		return fmt.Errorf("%w: the account is now %s", app.ErrInvalidStatusTransition, status)
	}

	now := time.Now().UTC()
	_, err = tx.Exec(
		ctx,
		"UPDATE users SET status = $2, suspended_until = $3, updated_at = $4 WHERE id = $1",
		moderation.UserID, moderation.ToStatus, moderation.SuspendedUntil, now,
	)
	if err != nil {
		return err
	}

	if moderation.RevokeSessions {
		_, err = tx.Exec(ctx, "UPDATE refresh_tokens SET revoked_at = $2 WHERE user_id = $1 AND revoked_at IS NULL", moderation.UserID, now)
		if err != nil {
			return err
		}
	}

	if err := audit(ctx, tx, moderation.AdminID, "user", moderation.UserID, moderation.Action, moderation.Details, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const adColumns = "a.id, a.title, a.status, a.price::text, a.stock, a.seller_id, u.name, a.created_at, a.updated_at, a.removed_at"

func scanAd(row pgx.Row) (*admin.AdEntity, error) {
	var a admin.AdEntity
	err := row.Scan(&a.ID, &a.Title, &a.Status, &a.Price, &a.Stock, &a.SellerID, &a.SellerName, &a.CreatedAt, &a.UpdatedAt, &a.RemovedAt)
	return &a, err
}

func (r *postgresRepository) ListAds(ctx context.Context, filter admin.AdFilter, limit uint, offset uint) ([]*admin.AdEntity, uint, error) {
	var c conditions
	if filter.Search != "" {
		c.add("a.title ILIKE ?", contains(filter.Search))
	}
	if filter.Status != "" {
		c.add("a.status = ?", filter.Status)
	}
	if filter.SellerID != "" {
		c.add("a.seller_id = ?", filter.SellerID)
	}
	return list(ctx, r.pool, "advertisements a JOIN users u ON u.id = a.seller_id", adColumns, "a.created_at DESC, a.id", c, limit, offset, scanAd)
}

func (r *postgresRepository) GetAd(ctx context.Context, id string) (*admin.AdEntity, error) {
	ad, err := scanAd(r.pool.QueryRow(ctx, "SELECT "+adColumns+" FROM advertisements a JOIN users u ON u.id = a.seller_id WHERE a.id = $1", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrAdvertisementNotFound
		}
		return nil, err
	}
	return ad, nil
}

func (r *postgresRepository) RemoveAd(ctx context.Context, removal admin.AdRemoval) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	var removedAt *time.Time
	err = tx.QueryRow(ctx, "SELECT status, removed_at FROM advertisements WHERE id = $1 FOR UPDATE", removal.AdID).Scan(&status, &removedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrAdvertisementNotFound
		}
		return err
	}
	if removedAt != nil || status != removal.FromStatus {
		return fmt.Errorf("%w: the advertisement is now %s", app.ErrInvalidStatusTransition, status)
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, "UPDATE advertisements SET status = 'removed', removed_at = $2, updated_at = $2 WHERE id = $1", removal.AdID, now)
	if err != nil {
		return err
	}

	if err := notificationsdb.Create(ctx, tx, removal.Notification, now); err != nil {
		return err
	}

	details := map[string]any{"reason": removal.Reason, "previous_status": removal.FromStatus}
	if err := audit(ctx, tx, removal.AdminID, "advertisement", removal.AdID, admin.ActionRemoveAd, details, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

const orderFrom = "orders o JOIN users b ON b.id = o.buyer_id " +
	"LEFT JOIN LATERAL (SELECT oi.seller_id FROM order_items oi WHERE oi.order_id = o.id LIMIT 1) os ON TRUE " +
	"LEFT JOIN users s ON s.id = os.seller_id"

const orderColumns = "o.id, o.number, o.status, o.total::text, o.buyer_id, b.name, b.email, s.id, s.name, o.created_at, o.paid_at, o.delivered_at"

func (r *postgresRepository) ListOrders(ctx context.Context, filter admin.OrderFilter, limit uint, offset uint) ([]*admin.OrderEntity, uint, error) {
	var c conditions
	if filter.Search != "" {
		search := contains(filter.Search)
		c.add("(o.number ILIKE ? OR b.name ILIKE ? OR b.email ILIKE ?)", search, search, search)
	}
	if filter.Status != "" {
		c.add("o.status = ?", filter.Status)
	}
	if filter.Since != nil {
		c.add("o.created_at >= ?", *filter.Since)
	}
	if filter.Until != nil {
		c.add("o.created_at < ?", *filter.Until)
	}
	return list(ctx, r.pool, orderFrom, orderColumns, "o.created_at DESC, o.id", c, limit, offset, func(row pgx.Row) (*admin.OrderEntity, error) {
		var o admin.OrderEntity
		err := row.Scan(&o.ID, &o.Number, &o.Status, &o.Total, &o.BuyerID, &o.BuyerName, &o.BuyerEmail, &o.SellerID, &o.SellerName, &o.CreatedAt, &o.PaidAt, &o.DeliveredAt)
		return &o, err
	})
}

var refundGivenUp = fmt.Sprintf("(p.status = 'captured' AND COALESCE((p.gateway_payload->>'refund_attempts')::int, 0) >= %d)", payments.MaxRefundAttempts)

var paymentColumns = "p.id, p.order_id, o.number, p.gateway, p.method, p.status, p.amount::text, p.failure_reason, " +
	"p.gateway_payload->>'refund_error', " + refundGivenUp + ", p.created_at, p.captured_at, p.refunded_at"

func (r *postgresRepository) ListPayments(ctx context.Context, filter admin.PaymentFilter, limit uint, offset uint) ([]*admin.PaymentEntity, uint, error) {
	var c conditions
	if filter.Status != "" {
		c.add("p.status = ?", filter.Status)
	}
	if filter.Method != "" {
		c.add("p.method = ?", filter.Method)
	}
	if filter.RefundGivenUp {
		c.add(refundGivenUp)
	}
	if filter.Since != nil {
		c.add("p.created_at >= ?", *filter.Since)
	}
	if filter.Until != nil {
		c.add("p.created_at < ?", *filter.Until)
	}
	from := "payments p JOIN orders o ON o.id = p.order_id"
	return list(ctx, r.pool, from, paymentColumns, "p.created_at DESC, p.id", c, limit, offset, func(row pgx.Row) (*admin.PaymentEntity, error) {
		var p admin.PaymentEntity
		err := row.Scan(&p.ID, &p.OrderID, &p.OrderNumber, &p.Gateway, &p.Method, &p.Status, &p.Amount, &p.FailureReason,
			&p.RefundError, &p.RefundGivenUp, &p.CreatedAt, &p.CapturedAt, &p.RefundedAt)
		return &p, err
	})
}

func (r *postgresRepository) ListAuditLogs(ctx context.Context, filter admin.AuditFilter, limit uint, offset uint) ([]*admin.AuditEntity, uint, error) {
	var c conditions
	if filter.Entity != "" {
		c.add("a.entity = ?", filter.Entity)
	}
	if filter.EntityID != "" {
		c.add("a.entity_id = ?", filter.EntityID)
	}
	if filter.UserID != "" {
		c.add("a.user_id = ?", filter.UserID)
	}
	if filter.Action != "" {
		c.add("a.action LIKE ?", escapeLike(filter.Action)+"%")
	}
	if filter.Since != nil {
		c.add("a.created_at >= ?", *filter.Since)
	}
	if filter.Until != nil {
		c.add("a.created_at < ?", *filter.Until)
	}
	from := "audit_logs a LEFT JOIN users u ON u.id = a.user_id"
	columns := "a.id, a.user_id, u.name, a.entity, a.entity_id, a.action, a.context, a.created_at"
	return list(ctx, r.pool, from, columns, "a.created_at DESC, a.id", c, limit, offset, func(row pgx.Row) (*admin.AuditEntity, error) {
		var a admin.AuditEntity
		var raw []byte
		err := row.Scan(&a.ID, &a.UserID, &a.UserName, &a.Entity, &a.EntityID, &a.Action, &raw, &a.CreatedAt)
		a.Context = raw
		return &a, err
	})
}

func (r *postgresRepository) GetIndicators(ctx context.Context, period admin.Period, now time.Time) (*admin.Indicators, error) {
	i := admin.Indicators{OrdersByStatus: map[string]uint{}, PaymentsByStatus: map[string]admin.Amount{}}
	err := r.pool.QueryRow(
		ctx,
		"SELECT "+
			"(SELECT count(*) FROM users WHERE deleted_at IS NULL AND created_at >= $1 AND created_at < $2), "+
			"(SELECT count(*) FROM users WHERE deleted_at IS NULL AND status = 'blocked'), "+
			"(SELECT count(*) FROM users WHERE deleted_at IS NULL AND status <> 'blocked' AND suspended_until > $3), "+
			"(SELECT count(*) FROM advertisements WHERE created_at >= $1 AND created_at < $2), "+
			"(SELECT count(*) FROM advertisements WHERE status = 'published'), "+
			"(SELECT count(*) FROM orders WHERE paid_at >= $1 AND paid_at < $2), "+
			"(SELECT COALESCE(sum(total), 0)::numeric(12,2)::text FROM orders WHERE paid_at >= $1 AND paid_at < $2), "+
			"(SELECT count(*) FROM payments p WHERE "+refundGivenUp+"), "+
			"(SELECT count(*) FROM support_tickets WHERE opened_at >= $1 AND opened_at < $2), "+
			"(SELECT count(*) FROM support_tickets WHERE status IN ('resolved', 'closed') AND closed_at >= $1 AND closed_at < $2), "+
			"(SELECT count(*) FROM support_tickets WHERE status NOT IN ('resolved', 'closed'))",
		period.From, period.Until, now,
	).Scan(
		&i.NewUsers, &i.BlockedUsers, &i.SuspendedUsers, &i.NewAds, &i.PublishedAds, &i.PaidOrders, &i.PaidAmount,
		&i.RefundsGivenUp, &i.TicketsOpened, &i.TicketsResolved, &i.OpenTickets,
	)
	if err != nil {
		return nil, err
	}

	rows, err := r.pool.Query(ctx, "SELECT status, count(*) FROM orders WHERE created_at >= $1 AND created_at < $2 GROUP BY status", period.From, period.Until)
	if err != nil {
		return nil, err
	}
	var status string
	var count uint
	_, err = pgx.ForEachRow(rows, []any{&status, &count}, func() error {
		i.OrdersByStatus[status] = count
		i.Orders += count
		return nil
	})
	if err != nil {
		return nil, err
	}

	rows, err = r.pool.Query(
		ctx,
		"SELECT status, count(*), sum(amount)::numeric(12,2)::text FROM payments WHERE created_at >= $1 AND created_at < $2 GROUP BY status",
		period.From, period.Until,
	)
	if err != nil {
		return nil, err
	}
	var amount string
	_, err = pgx.ForEachRow(rows, []any{&status, &count, &amount}, func() error {
		i.PaymentsByStatus[status] = admin.Amount{Count: count, Amount: amount}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &i, nil
}

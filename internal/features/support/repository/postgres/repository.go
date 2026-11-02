package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	notificationsdb "github.com/caslumes/solado_certo_backend/internal/features/notifications/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/features/support"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

const orderSeller = "(SELECT oi.seller_id FROM order_items oi WHERE oi.order_id = o.id LIMIT 1)"

const selectTickets = "SELECT t.id, t.category, t.status, t.subject, t.description, t.opened_by, t.assigned_to, t.resolution, " +
	"t.opened_at, t.updated_at, t.closed_at, o.id, o.number, o.buyer_id, " + orderSeller + " " +
	"FROM support_tickets t LEFT JOIN orders o ON o.id = t.order_id "

func scanTicket(row pgx.Row) (*support.TicketEntity, error) {
	var t support.TicketEntity
	var orderID, orderNumber, buyerID, sellerID *string
	err := row.Scan(
		&t.ID, &t.Category, &t.Status, &t.Subject, &t.Description, &t.OpenedBy, &t.AssignedTo, &t.Resolution,
		&t.OpenedAt, &t.UpdatedAt, &t.ClosedAt, &orderID, &orderNumber, &buyerID, &sellerID,
	)
	if err != nil {
		return nil, err
	}
	if orderID != nil {
		t.Order = &support.OrderParties{ID: *orderID, Number: *orderNumber, BuyerID: *buyerID}
		if sellerID != nil {
			t.Order.SellerID = *sellerID
		}
	}
	return &t, nil
}

func (r *postgresRepository) GetOrderParties(ctx context.Context, orderID string) (*support.OrderParties, error) {
	var order support.OrderParties
	var sellerID *string
	err := r.pool.QueryRow(
		ctx,
		"SELECT o.id, o.number, o.buyer_id, "+orderSeller+" FROM orders o WHERE o.id = $1",
		orderID,
	).Scan(&order.ID, &order.Number, &order.BuyerID, &sellerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrOrderNotFound
		}
		return nil, err
	}
	if sellerID != nil {
		order.SellerID = *sellerID
	}
	return &order, nil
}

func (r *postgresRepository) ListAdminIDs(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT ur.user_id FROM user_roles ur JOIN roles ro ON ro.id = ur.role_id JOIN users u ON u.id = ur.user_id "+
			"WHERE ro.code = $1 AND u.deleted_at IS NULL ORDER BY ur.user_id",
		enum.RoleAdmin,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func (r *postgresRepository) CreateTicket(ctx context.Context, ticket support.NewTicket) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	var id string
	err = tx.QueryRow(
		ctx,
		"INSERT INTO support_tickets (order_id, opened_by, category, status, subject, description, opened_at, updated_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $7) RETURNING id",
		ticket.OrderID, ticket.OpenedBy, ticket.Category, support.StatusOpen, ticket.Subject, ticket.Description, now,
	).Scan(&id)
	if err != nil {
		return "", err
	}

	for _, notification := range ticket.Notifications(id) {
		if err := notificationsdb.Create(ctx, tx, notification, now); err != nil {
			return "", err
		}
	}

	details := map[string]any{"category": ticket.Category}
	if ticket.OrderID != nil {
		details["order_id"] = *ticket.OrderID
	}
	if err := audit(ctx, tx, ticket.OpenedBy, id, "support.ticket_opened", details, now); err != nil {
		return "", err
	}

	return id, tx.Commit(ctx)
}

func (r *postgresRepository) GetTicket(ctx context.Context, id string) (*support.TicketDetailEntity, error) {
	ticket, err := scanTicket(r.pool.QueryRow(ctx, selectTickets+"WHERE t.id = $1", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrTicketNotFound
		}
		return nil, err
	}

	rows, err := r.pool.Query(
		ctx,
		"SELECT m.id, m.sender_id, u.name, "+
			"EXISTS (SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id = ur.role_id WHERE ur.user_id = m.sender_id AND ro.code = $2), "+
			"m.message, m.attachment_url, m.created_at "+
			"FROM support_messages m JOIN users u ON u.id = m.sender_id WHERE m.ticket_id = $1 ORDER BY m.created_at, m.id",
		id, enum.RoleAdmin,
	)
	if err != nil {
		return nil, err
	}
	messages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (support.MessageEntity, error) {
		var m support.MessageEntity
		err := row.Scan(&m.ID, &m.SenderID, &m.SenderName, &m.SenderIsAdmin, &m.Message, &m.AttachmentKey, &m.CreatedAt)
		return m, err
	})
	if err != nil {
		return nil, err
	}

	return &support.TicketDetailEntity{TicketEntity: *ticket, Messages: messages}, nil
}

func filter(f support.Filter) (string, []any) {
	where, args := "TRUE", []any{}
	if f.ParticipantID != "" {
		args = append(args, f.ParticipantID)
		where = fmt.Sprintf("(t.opened_by = $%[1]d OR o.buyer_id = $%[1]d OR EXISTS "+
			"(SELECT 1 FROM order_items oi WHERE oi.order_id = t.order_id AND oi.seller_id = $%[1]d))", len(args))
	}
	if f.Status != "" {
		args = append(args, f.Status)
		where += fmt.Sprintf(" AND t.status = $%d", len(args))
	}
	return where, args
}

func (r *postgresRepository) CountTickets(ctx context.Context, f support.Filter) (uint, error) {
	where, args := filter(f)
	var total uint
	err := r.pool.QueryRow(
		ctx,
		"SELECT count(*) FROM support_tickets t LEFT JOIN orders o ON o.id = t.order_id WHERE "+where,
		args...,
	).Scan(&total)

	return total, err
}

func (r *postgresRepository) GetTickets(ctx context.Context, f support.Filter, limit uint, offset uint) ([]*support.TicketEntity, error) {
	where, args := filter(f)
	n := len(args)

	rows, err := r.pool.Query(
		ctx,
		selectTickets+"WHERE "+where+fmt.Sprintf(" ORDER BY t.updated_at DESC, t.id LIMIT $%d OFFSET $%d", n+1, n+2),
		append(args, limit, offset)...,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*support.TicketEntity, error) {
		return scanTicket(row)
	})
}

func (r *postgresRepository) AddMessage(ctx context.Context, message support.NewMessage) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status support.Status
	err = tx.QueryRow(ctx, "SELECT status FROM support_tickets WHERE id = $1 FOR UPDATE", message.TicketID).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrTicketNotFound
		}
		return err
	}
	if status != message.From {
		return fmt.Errorf("%w: the ticket is now %s", app.ErrInvalidStatusTransition, status)
	}

	now := time.Now().UTC()
	_, err = tx.Exec(
		ctx,
		"INSERT INTO support_messages (ticket_id, sender_id, message, attachment_url, created_at) VALUES ($1, $2, $3, $4, $5)",
		message.TicketID, message.SenderID, message.Message, message.AttachmentKey, now,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, "UPDATE support_tickets SET status = $1, updated_at = $2 WHERE id = $3", message.To, now, message.TicketID)
	if err != nil {
		return err
	}

	for _, notification := range message.Notifications {
		if err := notificationsdb.Create(ctx, tx, notification, now); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *postgresRepository) RecordAttachmentView(ctx context.Context, ticketID string, messageID string, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := audit(ctx, tx, userID, ticketID, "support.attachment_viewed", map[string]any{"message_id": messageID}, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func audit(ctx context.Context, tx pgx.Tx, userID string, ticketID string, action string, details map[string]any, now time.Time) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'support_ticket', $2, $3, $4, $5)",
		userID, ticketID, action, detailsJSON, now,
	)
	return err
}

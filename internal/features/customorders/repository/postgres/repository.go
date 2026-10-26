package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/customorders"
	notificationsdb "github.com/caslumes/solado_certo_backend/internal/features/notifications/repository/postgres"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) GetAdvertisement(ctx context.Context, id string) (*customorders.CustomizableAd, error) {
	var ad customorders.CustomizableAd
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, seller_id, title, status = 'published' AND removed_at IS NULL, custom_made_allowed FROM advertisements WHERE id = $1",
		id,
	).Scan(&ad.ID, &ad.SellerID, &ad.Title, &ad.Published, &ad.CustomMadeAllowed)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrAdvertisementNotFound
		}
		return nil, err
	}

	return &ad, nil
}

// CreateRequest stores the request with its first history entry, its audit row and the seller's notification.
func (r *postgresRepository) CreateRequest(ctx context.Context, request customorders.NewRequest) (string, error) {
	measurements, err := json.Marshal(request.Measurements)
	if err != nil {
		return "", err
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	var id string
	err = tx.QueryRow(
		ctx,
		"INSERT INTO custom_order_requests (buyer_id, advertisement_id, status, measurements, technical_notes, created_at, updated_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $6) RETURNING id",
		request.BuyerID, request.AdvertisementID, customorders.StatusRequested, measurements, request.TechnicalNotes, now,
	).Scan(&id)
	if err != nil {
		return "", err
	}
	if err := recordHistory(ctx, tx, id, &request.BuyerID, nil, customorders.StatusRequested, nil, now); err != nil {
		return "", err
	}
	if err := audit(ctx, tx, request.BuyerID, id, "custom_order.requested", map[string]any{"advertisement_id": request.AdvertisementID}, now); err != nil {
		return "", err
	}
	if err := notificationsdb.Create(ctx, tx, request.Notification(id), now); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	return id, nil
}

const selectRequests = "SELECT r.id, r.buyer_id, b.name, a.seller_id, s.name, a.id, a.title, " +
	"(SELECT i.url FROM advertisement_images i WHERE i.advertisement_id = a.id AND i.is_cover LIMIT 1), " +
	"r.order_id, r.status, r.measurements, r.technical_notes, r.quoted_price::text, r.rejection_reason, r.adaptation_instructions, " +
	"r.created_at, r.updated_at " +
	"FROM custom_order_requests r JOIN advertisements a ON a.id = r.advertisement_id " +
	"JOIN users b ON b.id = r.buyer_id JOIN users s ON s.id = a.seller_id "

func scanRequest(row pgx.Row) (*customorders.RequestEntity, error) {
	var request customorders.RequestEntity
	var measurements []byte
	err := row.Scan(&request.ID, &request.BuyerID, &request.BuyerName, &request.SellerID, &request.SellerName,
		&request.AdvertisementID, &request.AdvertisementTitle, &request.CoverURL, &request.OrderID, &request.Status,
		&measurements, &request.TechnicalNotes, &request.QuotedPrice, &request.RejectionReason, &request.AdaptationInstructions,
		&request.CreatedAt, &request.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(measurements, &request.Measurements); err != nil {
		return nil, err
	}

	return &request, nil
}

func (r *postgresRepository) GetRequest(ctx context.Context, id string) (*customorders.RequestDetailEntity, error) {
	request, err := scanRequest(r.pool.QueryRow(ctx, selectRequests+"WHERE r.id = $1", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrCustomOrderNotFound
		}
		return nil, err
	}

	rows, err := r.pool.Query(
		ctx,
		"SELECT previous_status, new_status, changed_by, reason, created_at FROM custom_order_request_history "+
			"WHERE request_id = $1 ORDER BY created_at, id",
		id,
	)
	if err != nil {
		return nil, err
	}

	history, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (customorders.HistoryEntity, error) {
		var h customorders.HistoryEntity
		err := row.Scan(&h.PreviousStatus, &h.Status, &h.ChangedBy, &h.Reason, &h.CreatedAt)
		return h, err
	})
	if err != nil {
		return nil, err
	}

	return &customorders.RequestDetailEntity{RequestEntity: *request, History: history}, nil
}

// filter turns a Filter into a WHERE clause and its arguments, numbered from $1.
func filter(f customorders.Filter) (string, []any) {
	where, args := "a.seller_id = $1", []any{f.SellerID}
	if f.BuyerID != "" {
		where, args = "r.buyer_id = $1", []any{f.BuyerID}
	}
	if f.Status != "" {
		where += " AND r.status = $2"
		args = append(args, f.Status)
	}

	return where, args
}

func (r *postgresRepository) CountRequests(ctx context.Context, f customorders.Filter) (uint, error) {
	where, args := filter(f)
	var total uint
	err := r.pool.QueryRow(
		ctx,
		"SELECT count(*) FROM custom_order_requests r JOIN advertisements a ON a.id = r.advertisement_id WHERE "+where,
		args...,
	).Scan(&total)

	return total, err
}

func (r *postgresRepository) GetRequests(ctx context.Context, f customorders.Filter, limit uint, offset uint) ([]*customorders.RequestEntity, error) {
	where, args := filter(f)
	n := len(args)

	rows, err := r.pool.Query(
		ctx,
		selectRequests+"WHERE "+where+" ORDER BY r.updated_at DESC, r.id DESC LIMIT $"+strconv.Itoa(n+1)+" OFFSET $"+strconv.Itoa(n+2),
		append(args, limit, offset)...,
	)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*customorders.RequestEntity, error) {
		return scanRequest(row)
	})
}

// ChangeStatus locks the request and applies the change only if it is still in change.From, so two
// decisions made at the same time can't both apply. History, audit and notification go in the same
// transaction.
func (r *postgresRepository) ChangeStatus(ctx context.Context, change customorders.StatusChange) error {
	var measurements []byte
	if change.Measurements != nil {
		var err error
		if measurements, err = json.Marshal(change.Measurements); err != nil {
			return err
		}
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status customorders.Status
	if err := tx.QueryRow(ctx, "SELECT status FROM custom_order_requests WHERE id = $1 FOR UPDATE", change.RequestID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrCustomOrderNotFound
		}
		return err
	}
	if status != change.From {
		return app.ErrInvalidStatusTransition
	}

	now := time.Now().UTC()
	_, err = tx.Exec(
		ctx,
		"UPDATE custom_order_requests SET status = $2, updated_at = $3, "+
			"quoted_price = COALESCE($4::numeric, quoted_price), rejection_reason = COALESCE($5, rejection_reason), "+
			"measurements = COALESCE($6::jsonb, measurements), technical_notes = COALESCE($7, technical_notes) "+
			"WHERE id = $1",
		change.RequestID, change.To, now, change.QuotedPrice, change.RejectionReason, measurements, change.TechnicalNotes,
	)
	if err != nil {
		return err
	}
	from := change.From
	if err := recordHistory(ctx, tx, change.RequestID, &change.ChangedBy, &from, change.To, change.Reason, now); err != nil {
		return err
	}
	details := map[string]any{"from": change.From, "to": change.To}
	if change.QuotedPrice != nil {
		details["quoted_price"] = *change.QuotedPrice
	}
	if change.Measurements != nil {
		details["measurements_changed"] = true
	}
	if err := audit(ctx, tx, change.ChangedBy, change.RequestID, "custom_order."+string(change.Action), details, now); err != nil {
		return err
	}
	if err := notificationsdb.Create(ctx, tx, change.Notification, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func recordHistory(ctx context.Context, tx pgx.Tx, requestID string, changedBy *string, from *customorders.Status, to customorders.Status, reason *string, now time.Time) error {
	_, err := tx.Exec(
		ctx,
		"INSERT INTO custom_order_request_history (request_id, changed_by, previous_status, new_status, reason, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
		requestID, changedBy, from, to, reason, now,
	)

	return err
}

func audit(ctx context.Context, tx pgx.Tx, userID string, requestID string, action string, details map[string]any, now time.Time) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		ctx,
		"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'custom_order', $2, $3, $4, $5)",
		userID, requestID, action, detailsJSON, now,
	)

	return err
}

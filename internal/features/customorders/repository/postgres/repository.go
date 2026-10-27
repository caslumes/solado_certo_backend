package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/customorders"
	"github.com/caslumes/solado_certo_backend/internal/features/documents"
	notificationsdb "github.com/caslumes/solado_certo_backend/internal/features/notifications/repository/postgres"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	ordersdb "github.com/caslumes/solado_certo_backend/internal/features/orders/repository/postgres"
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
	"r.foot_photo_key, fr.id, fr.created_at, r.created_at, r.updated_at " +
	"FROM custom_order_requests r JOIN advertisements a ON a.id = r.advertisement_id " +
	"JOIN users b ON b.id = r.buyer_id JOIN users s ON s.id = a.seller_id " +
	"LEFT JOIN LATERAL (SELECT d.id, d.created_at FROM order_documents d WHERE d.custom_order_request_id = r.id " +
	"AND d.document_type = 'technical_report' ORDER BY d.created_at DESC, d.id DESC LIMIT 1) fr ON true "

func scanRequest(row pgx.Row) (*customorders.RequestEntity, error) {
	var request customorders.RequestEntity
	var measurements []byte
	var reportID *string
	var reportedAt *time.Time
	err := row.Scan(&request.ID, &request.BuyerID, &request.BuyerName, &request.SellerID, &request.SellerName,
		&request.AdvertisementID, &request.AdvertisementTitle, &request.CoverURL, &request.OrderID, &request.Status,
		&measurements, &request.TechnicalNotes, &request.QuotedPrice, &request.RejectionReason, &request.AdaptationInstructions,
		&request.FootPhotoKey, &reportID, &reportedAt, &request.CreatedAt, &request.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if reportID != nil {
		request.FittingReport = &customorders.FittingReportEntity{ID: *reportID, CreatedAt: *reportedAt}
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
	var orderID *string
	err = tx.QueryRow(ctx, "SELECT status, order_id FROM custom_order_requests WHERE id = $1 FOR UPDATE", change.RequestID).Scan(&status, &orderID)
	if err != nil {
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
			"measurements = COALESCE($6::jsonb, measurements), technical_notes = COALESCE($7, technical_notes), "+
			"adaptation_instructions = COALESCE($8, adaptation_instructions) "+
			"WHERE id = $1",
		change.RequestID, change.To, now, change.QuotedPrice, change.RejectionReason, measurements, change.TechnicalNotes,
		change.AdaptationInstructions,
	)
	if err != nil {
		return err
	}
	if change.ReportKey != nil {
		if orderID == nil {
			return app.ErrInvalidStatusTransition
		}
		_, err := tx.Exec(
			ctx,
			"INSERT INTO order_documents (order_id, order_item_id, custom_order_request_id, uploaded_by, document_type, file_url, validation_status, validated_at, created_at) "+
				"VALUES ($1, (SELECT id FROM order_items WHERE order_id = $1 AND custom_order_request_id = $2 LIMIT 1), $2, $3, $4, $5, $6, $7, $7)",
			*orderID, change.RequestID, change.ChangedBy, documents.DocumentTypeTechnicalReport, *change.ReportKey, orders.DocumentStatusApproved, now,
		)
		if err != nil {
			return err
		}
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
	if change.ReportKey != nil {
		details["order_id"] = *orderID
	}
	if err := audit(ctx, tx, change.ChangedBy, change.RequestID, "custom_order."+string(change.Action), details, now); err != nil {
		return err
	}
	if err := notificationsdb.Create(ctx, tx, change.Notification, now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *postgresRepository) CheckoutExists(ctx context.Context, buyerID string, idempotencyKey string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(
		ctx,
		"SELECT EXISTS (SELECT 1 FROM checkouts WHERE buyer_id = $1 AND idempotency_key = $2)",
		buyerID, idempotencyKey,
	).Scan(&exists)
	return exists, err
}

func (r *postgresRepository) Checkout(ctx context.Context, checkout customorders.Checkout) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	checkoutID, created, err := ordersdb.InsertCheckout(ctx, tx, checkout.BuyerID, checkout.IdempotencyKey, now)
	if err != nil || !created {
		return false, err
	}

	var status customorders.Status
	var orderID *string
	err = tx.QueryRow(ctx, "SELECT status, order_id FROM custom_order_requests WHERE id = $1 FOR UPDATE", checkout.RequestID).Scan(&status, &orderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, app.ErrCustomOrderNotFound
		}
		return false, err
	}
	if status != customorders.StatusApproved || orderID != nil {
		return false, app.ErrInvalidStatusTransition
	}
	if err := ordersdb.CheckShippingOptions(ctx, tx, checkout.Plan); err != nil {
		return false, err
	}

	newOrderID, err := ordersdb.InsertOrder(ctx, tx, checkout.BuyerID, checkoutID, checkout.Plan, checkout.Plan.Orders[0], now)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, "UPDATE custom_order_requests SET order_id = $2, updated_at = $3 WHERE id = $1", checkout.RequestID, newOrderID, now); err != nil {
		return false, err
	}
	if err := audit(ctx, tx, checkout.BuyerID, checkout.RequestID, "custom_order.checkout", map[string]any{"order_id": newOrderID}, now); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func (r *postgresRepository) SetFootPhoto(ctx context.Context, change customorders.PhotoChange) (*string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status customorders.Status
	var previous *string
	err = tx.QueryRow(ctx, "SELECT status, foot_photo_key FROM custom_order_requests WHERE id = $1 FOR UPDATE", change.RequestID).Scan(&status, &previous)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrCustomOrderNotFound
		}
		return nil, err
	}
	if !status.TakesPhoto() {
		return nil, app.ErrInvalidStatusTransition
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, "UPDATE custom_order_requests SET foot_photo_key = $2, updated_at = $3 WHERE id = $1", change.RequestID, change.Key, now); err != nil {
		return nil, err
	}
	if err := audit(ctx, tx, change.BuyerID, change.RequestID, "custom_order.photo_sent", map[string]any{"replaced": previous != nil}, now); err != nil {
		return nil, err
	}
	return previous, tx.Commit(ctx)
}

func (r *postgresRepository) RecordPhotoView(ctx context.Context, requestID string, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := audit(ctx, tx, userID, requestID, "custom_order.photo_viewed", map[string]any{}, time.Now().UTC()); err != nil {
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

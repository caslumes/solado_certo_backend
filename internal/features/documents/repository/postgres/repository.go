package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
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

func (r *postgresRepository) GetPrescriptionItem(ctx context.Context, orderID string, itemID string) (*documents.PrescriptionItem, error) {
	var item documents.PrescriptionItem
	var id, title, sellerID, latestID *string
	var required *bool
	var latestStatus *orders.DocumentStatus
	var latest orders.PrescriptionEntity
	var latestAt *time.Time
	err := r.pool.QueryRow(
		ctx,
		"SELECT o.id, o.number, o.status, o.buyer_id, oi.id, oi.product_snapshot->>'title', oi.seller_id, oi.prescription_required, "+
			"pd.id, pd.validation_status, pd.rejection_reason, pd.created_at "+
			"FROM orders o LEFT JOIN order_items oi ON oi.order_id = o.id AND oi.id = $2 "+
			"LEFT JOIN LATERAL ("+ordersdb.LatestPrescription+") pd ON true WHERE o.id = $1",
		orderID, itemID,
	).Scan(&item.OrderID, &item.OrderNumber, &item.OrderStatus, &item.BuyerID, &id, &title, &sellerID, &required,
		&latestID, &latestStatus, &latest.RejectionReason, &latestAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrOrderNotFound
		}
		return nil, err
	}
	if id != nil {
		item.ID, item.Title, item.SellerID, item.PrescriptionRequired = *id, *title, *sellerID, *required
	}
	if latestID != nil {
		latest.ID, latest.Status, latest.CreatedAt = *latestID, *latestStatus, *latestAt
		item.Latest = &latest
	}
	return &item, nil
}

// CreateDocument stores the document with its audit row, and its notification if there is one.
func (r *postgresRepository) CreateDocument(ctx context.Context, document documents.NewDocument) (*documents.DocumentEntity, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	created := documents.DocumentEntity{
		OrderID:     document.OrderID,
		OrderItemID: &document.OrderItemID,
		Type:        document.Type,
		Status:      document.Status,
		CreatedAt:   now,
	}
	err = tx.QueryRow(
		ctx,
		"INSERT INTO order_documents (order_id, order_item_id, uploaded_by, document_type, file_url, validation_status, validated_at, created_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $7) RETURNING id",
		document.OrderID, document.OrderItemID, document.UploadedBy, document.Type, document.Key, document.Status, now,
	).Scan(&created.ID)
	if err != nil {
		return nil, err
	}

	details := map[string]any{"order_id": document.OrderID, "order_item_id": document.OrderItemID, "type": document.Type, "status": document.Status}
	if err := audit(ctx, tx, document.UploadedBy, created.ID, "document.uploaded", details, now); err != nil {
		return nil, err
	}
	if document.Notification != nil {
		if err := notificationsdb.Create(ctx, tx, *document.Notification, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &created, nil
}

func (r *postgresRepository) GetDocument(ctx context.Context, id string) (*documents.StoredDocument, error) {
	var d documents.StoredDocument
	err := r.pool.QueryRow(
		ctx,
		"SELECT d.id, d.order_id, d.order_item_id, d.document_type, d.validation_status, d.rejection_reason, d.created_at, "+
			"d.file_url, d.uploaded_by, o.number, COALESCE(oi.product_snapshot->>'title', ''), o.buyer_id, "+
			"(SELECT s.seller_id FROM order_items s WHERE s.order_id = o.id LIMIT 1), o.paid_at IS NOT NULL "+
			"FROM order_documents d JOIN orders o ON o.id = d.order_id LEFT JOIN order_items oi ON oi.id = d.order_item_id "+
			"WHERE d.id = $1",
		id,
	).Scan(&d.ID, &d.OrderID, &d.OrderItemID, &d.Type, &d.Status, &d.RejectionReason, &d.CreatedAt,
		&d.Key, &d.UploadedBy, &d.OrderNumber, &d.ItemTitle, &d.BuyerID, &d.SellerID, &d.OrderPaid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrDocumentNotFound
		}
		return nil, err
	}
	return &d, nil
}

func (r *postgresRepository) IsAdmin(ctx context.Context, userID string) (bool, error) {
	var admin bool
	err := r.pool.QueryRow(
		ctx,
		"SELECT EXISTS (SELECT 1 FROM user_roles ur JOIN roles ro ON ro.id = ur.role_id WHERE ur.user_id = $1 AND ro.code = 'admin')",
		userID,
	).Scan(&admin)
	return admin, err
}

func (r *postgresRepository) RecordView(ctx context.Context, documentID string, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := audit(ctx, tx, userID, documentID, "document.viewed", nil, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RejectPrescription locks the order, so the prescription can't be rejected while the order is being
// shipped, and checks that the document is still the item's latest, approved one.
func (r *postgresRepository) RejectPrescription(ctx context.Context, rejection documents.Rejection) (*documents.DocumentEntity, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var status orders.OrderStatus
	if err := tx.QueryRow(ctx, "SELECT status FROM orders WHERE id = $1 FOR UPDATE", rejection.OrderID).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrOrderNotFound
		}
		return nil, err
	}
	var latest orders.PrescriptionEntity
	err = tx.QueryRow(
		ctx,
		"SELECT pd.id, pd.validation_status, pd.rejection_reason, pd.created_at FROM order_items oi "+
			"JOIN LATERAL ("+ordersdb.LatestPrescription+") pd ON true WHERE oi.id = $1",
		rejection.OrderItemID,
	).Scan(&latest.ID, &latest.Status, &latest.RejectionReason, &latest.CreatedAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if latest.ID != rejection.DocumentID || !orders.CanRejectPrescription(status, &latest) {
		return nil, fmt.Errorf("%w: only the latest approved prescription of an order not shipped yet can be rejected", app.ErrInvalidStatusTransition)
	}

	now := time.Now().UTC()
	rejected := documents.DocumentEntity{
		ID:              rejection.DocumentID,
		OrderItemID:     &rejection.OrderItemID,
		Type:            documents.DocumentTypePrescription,
		Status:          orders.DocumentStatusRejected,
		RejectionReason: &rejection.Reason,
	}
	err = tx.QueryRow(
		ctx,
		"UPDATE order_documents SET validation_status = $2, rejection_reason = $3, validated_by = $4, validated_at = $5 "+
			"WHERE id = $1 RETURNING order_id, created_at",
		rejection.DocumentID, orders.DocumentStatusRejected, rejection.Reason, rejection.SellerID, now,
	).Scan(&rejected.OrderID, &rejected.CreatedAt)
	if err != nil {
		return nil, err
	}

	details := map[string]any{"order_id": rejection.OrderID, "order_item_id": rejection.OrderItemID, "reason": rejection.Reason}
	if err := audit(ctx, tx, rejection.SellerID, rejection.DocumentID, "document.rejected", details, now); err != nil {
		return nil, err
	}
	if err := notificationsdb.Create(ctx, tx, rejection.Notification, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &rejected, nil
}

func audit(ctx context.Context, tx pgx.Tx, userID string, documentID string, action string, details map[string]any, now time.Time) error {
	var detailsJSON []byte
	if details != nil {
		var err error
		if detailsJSON, err = json.Marshal(details); err != nil {
			return err
		}
	}
	_, err := tx.Exec(
		ctx,
		"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'document', $2, $3, $4, $5)",
		userID, documentID, action, detailsJSON, now,
	)
	return err
}

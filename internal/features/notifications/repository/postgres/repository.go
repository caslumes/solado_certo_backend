package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

// Create writes a notification inside the transaction of the change it tells about, so it exists
// exactly when the change does.
func Create(ctx context.Context, tx pgx.Tx, notification notifications.NewNotification, now time.Time) error {
	var data []byte
	if notification.Data != nil {
		var err error
		if data, err = json.Marshal(notification.Data); err != nil {
			return err
		}
	}
	_, err := tx.Exec(
		ctx,
		"INSERT INTO notifications (user_id, type, title, body, data, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
		notification.UserID, notification.Type, notification.Title, notification.Body, data, now,
	)
	return err
}

func (r *postgresRepository) CountNotifications(ctx context.Context, userID string) (uint, uint, error) {
	var total, unread uint
	err := r.pool.QueryRow(
		ctx,
		"SELECT count(*), count(*) FILTER (WHERE read_at IS NULL) FROM notifications WHERE user_id = $1",
		userID,
	).Scan(&total, &unread)
	return total, unread, err
}

func (r *postgresRepository) GetNotifications(ctx context.Context, userID string, limit uint, offset uint) ([]*notifications.NotificationEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT id, user_id, type, title, body, data, read_at, created_at FROM notifications "+
			"WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2 OFFSET $3",
		userID, limit, offset,
	)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*notifications.NotificationEntity, error) {
		var n notifications.NotificationEntity
		err := row.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.Data, &n.ReadAt, &n.CreatedAt)
		return &n, err
	})
}

func (r *postgresRepository) MarkRead(ctx context.Context, userID string, id string) error {
	tag, err := r.pool.Exec(
		ctx,
		"UPDATE notifications SET read_at = COALESCE(read_at, $3) WHERE id = $1 AND user_id = $2",
		id, userID, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return app.ErrNotificationNotFound
	}
	return nil
}

func (r *postgresRepository) MarkAllRead(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, "UPDATE notifications SET read_at = $2 WHERE user_id = $1 AND read_at IS NULL", userID, time.Now().UTC())
	return err
}

package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	model "github.com/caslumes/solado_certo_backend/internal/features/auth/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolationCode = "23505"

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) CreateUser(ctx context.Context, ue *auth.UserEntity) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	var userID string
	err = tx.QueryRow(
		ctx,
		"INSERT INTO users (name, email, phone, password_hash, status, created_at, updated_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id",
		ue.Name, ue.Email, ue.Phone, ue.PasswordHash, enum.ProfileStatusPending, now, now,
	).Scan(&userID)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return app.ErrUserAlreadyExists
		}

		return err
	}

	result, err := tx.Exec(
		ctx,
		"INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = $2",
		userID, enum.RoleBuyer,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("role %q not found", enum.RoleBuyer)
	}

	return tx.Commit(ctx)
}

func (r *postgresRepository) GetUserByEmail(ctx context.Context, email string) (*auth.UserEntity, error) {
	var um model.UserModel
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, name, email, phone, password_hash, status, created_at, updated_at FROM users WHERE email = $1",
		email,
	).Scan(&um.ID, &um.Name, &um.Email, &um.Phone, &um.PasswordHash, &um.Status, &um.CreatedAt, &um.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrUserNotFound
		}

		return nil, err
	}

	ue := um.ToEntity()

	return &ue, nil
}

func (r *postgresRepository) SaveRefreshToken(ctx context.Context, rt *auth.RefreshTokenEntity) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(
		ctx,
		"INSERT INTO refresh_tokens (user_id, token_hash, device_name, expires_at, created_at) "+
			"VALUES ($1, $2, $3, $4, $5)",
		rt.UserID, rt.TokenHash, rt.DeviceName, rt.ExpiresAt, now,
	)

	if err != nil {
		return err
	}

	return nil
}

func (r *postgresRepository) GetTokenByHash(ctx context.Context, tokenHash string) (*auth.RefreshTokenEntity, error) {
	var rtm model.RefreshTokenModel
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, user_id, token_hash, device_name, expires_at, revoked_at, created_at FROM refresh_tokens WHERE token_hash = $1",
		tokenHash,
	).Scan(&rtm.ID, &rtm.UserID, &rtm.TokenHash, &rtm.DeviceName, &rtm.ExpiresAt, &rtm.RevokedAt, &rtm.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrTokenNotFound
		}

		return nil, err
	}

	rt := rtm.ToEntity()

	return &rt, nil
}

func (r *postgresRepository) RevokeToken(ctx context.Context, tokenHash string) error {
	now := time.Now().UTC()
	result, err := r.pool.Exec(
		ctx,
		"UPDATE refresh_tokens SET revoked_at = $1 WHERE token_hash = $2 AND revoked_at IS NULL",
		now, tokenHash,
	)

	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return app.ErrTokenNotFound
	}

	return nil
}

func (r *postgresRepository) RotateRefreshToken(ctx context.Context, oldTokenHash string, newToken *auth.RefreshTokenEntity) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	result, err := tx.Exec(
		ctx,
		"UPDATE refresh_tokens SET revoked_at = $1 WHERE token_hash = $2 AND revoked_at IS NULL",
		now, oldTokenHash,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return app.ErrTokenRevoked
	}

	_, err = tx.Exec(
		ctx,
		"INSERT INTO refresh_tokens (user_id, token_hash, device_name, expires_at, created_at) "+
			"VALUES ($1, $2, $3, $4, $5)",
		newToken.UserID, newToken.TokenHash, newToken.DeviceName, newToken.ExpiresAt, now,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *postgresRepository) RevokeUserTokens(ctx context.Context, userID string) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(
		ctx,
		"UPDATE refresh_tokens SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL",
		now, userID,
	)

	return err
}

func (r *postgresRepository) LastPasswordResetAt(ctx context.Context, userID string) (*time.Time, error) {
	var createdAt *time.Time
	err := r.pool.QueryRow(
		ctx,
		"SELECT max(created_at) FROM password_reset_tokens WHERE user_id = $1",
		userID,
	).Scan(&createdAt)

	return createdAt, err
}

func (r *postgresRepository) CreatePasswordReset(ctx context.Context, reset *auth.PasswordResetEntity) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	_, err = tx.Exec(
		ctx,
		"UPDATE password_reset_tokens SET expires_at = $1 WHERE user_id = $2 AND used_at IS NULL AND expires_at > $1",
		now, reset.UserID,
	)
	if err != nil {
		return err
	}

	err = tx.QueryRow(
		ctx,
		"INSERT INTO password_reset_tokens (user_id, token_hash, expires_at, created_at) VALUES ($1, $2, $3, $4) RETURNING id",
		reset.UserID, reset.CodeHash, reset.ExpiresAt, now,
	).Scan(&reset.ID)
	if err != nil {
		return err
	}
	reset.CreatedAt = now

	return tx.Commit(ctx)
}

func (r *postgresRepository) ClaimPasswordResetAttempt(ctx context.Context, userID string, maxAttempts int) (*auth.PasswordResetEntity, error) {
	now := time.Now().UTC()
	var reset auth.PasswordResetEntity
	err := r.pool.QueryRow(
		ctx,
		"UPDATE password_reset_tokens SET attempts = attempts + 1 "+
			"WHERE id = (SELECT id FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL AND expires_at > $2 ORDER BY created_at DESC LIMIT 1) "+
			"AND used_at IS NULL AND expires_at > $2 AND attempts < $3 "+
			"RETURNING id, user_id, token_hash, attempts, expires_at, created_at",
		userID, now, maxAttempts,
	).Scan(&reset.ID, &reset.UserID, &reset.CodeHash, &reset.Attempts, &reset.ExpiresAt, &reset.CreatedAt)
	if err == nil {
		return &reset, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}

	var exhausted bool
	err = r.pool.QueryRow(
		ctx,
		"SELECT EXISTS (SELECT 1 FROM password_reset_tokens WHERE user_id = $1 AND used_at IS NULL AND expires_at > $2 AND attempts >= $3)",
		userID, now, maxAttempts,
	).Scan(&exhausted)
	if err != nil {
		return nil, err
	}
	if exhausted {
		return nil, app.ErrResetAttemptsExceeded
	}

	return nil, app.ErrInvalidResetCode
}

func (r *postgresRepository) CompletePasswordReset(ctx context.Context, resetID string, userID string, passwordHash string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	result, err := tx.Exec(
		ctx,
		"UPDATE password_reset_tokens SET used_at = $1 WHERE id = $2 AND user_id = $3 AND used_at IS NULL AND expires_at > $1",
		now, resetID, userID,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return app.ErrInvalidResetCode
	}

	_, err = tx.Exec(ctx, "UPDATE users SET password_hash = $1, updated_at = $2 WHERE id = $3", passwordHash, now, userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, "UPDATE refresh_tokens SET revoked_at = $1 WHERE user_id = $2 AND revoked_at IS NULL", now, userID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		ctx,
		"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'user', $1, 'user.password_reset', '{}', $2)",
		userID, now,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

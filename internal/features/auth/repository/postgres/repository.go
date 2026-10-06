package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/auth"
	model "github.com/caslumes/solado_certo_backend/internal/features/auth/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) CreateUser(ctx context.Context, ue *auth.UserEntity) error {
	now := time.Now().UTC()
	_, err := r.pool.Exec(
		ctx,
		"INSERT INTO usuarios (nome, email, telefone, senha_hash, status, created_at, updated_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7)",
		ue.Nome, ue.Email, ue.Telefone, ue.SenhaHash, enum.ProfileStatusPending, now, now,
	)

	if err != nil {
		return err
	}

	return nil
}

func (r *postgresRepository) GetUserByEmail(ctx context.Context, email string) (*auth.UserEntity, error) {
	var um model.UserModel
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, nome, email, telefone, senha_hash, status, created_at, updated_at FROM usuarios WHERE email = $1",
		email,
	).Scan(&um.ID, &um.Nome, &um.Email, &um.Telefone, &um.SenhaHash, &um.Status, &um.CreatedAt, &um.UpdatedAt)

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
		"INSERT INTO refresh_tokens (usuario_id, token_hash, device_name, expires_at, created_at) "+
			"VALUES ($1, $2, $3, $4, $5)",
		rt.UsuarioID, rt.TokenHash, rt.DeviceName, rt.ExpiresAt, now,
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
		"SELECT id, usuario_id, token_hash, device_name, expires_at, revoked_at, created_at FROM refresh_tokens WHERE token_hash = $1",
		tokenHash,
	).Scan(&rtm.ID, &rtm.UsuarioID, &rtm.TokenHash, &rtm.DeviceName, &rtm.ExpiresAt, &rtm.RevokedAt, &rtm.CreatedAt)

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

// RotateRefreshToken revokes the old refresh token and stores the new one in a
// single transaction. It returns app.ErrTokenRevoked if the old token was
// already revoked, so two concurrent refreshes cannot both succeed.
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
		"INSERT INTO refresh_tokens (usuario_id, token_hash, device_name, expires_at, created_at) "+
			"VALUES ($1, $2, $3, $4, $5)",
		newToken.UsuarioID, newToken.TokenHash, newToken.DeviceName, newToken.ExpiresAt, now,
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
		"UPDATE refresh_tokens SET revoked_at = $1 WHERE usuario_id = $2 AND revoked_at IS NULL",
		now, userID,
	)

	return err
}

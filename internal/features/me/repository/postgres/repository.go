package postgres

import (
	"context"

	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/features/me/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) GetProfileByUserID(ctx context.Context, userID string) (*me.ProfileEntity, error) {
	var m repository.ProfileModel
	err := r.pool.QueryRow(
		ctx,
		"SELECT nome, email, telefone, avatar_url FROM usuarios WHERE id = $1",
		userID,
	).Scan(&m.Name, &m.Email, &m.Phone, &m.AvatarURL)
	if err != nil {
		return nil, err
	}

	e := m.ToEntity()
	return e, nil
}

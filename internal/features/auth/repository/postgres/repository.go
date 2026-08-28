package repository

import (
	"context"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/features/auth"
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
		"INSERT INTO usuarios (nome, email, telefone, senha_hash, status, created_at, updated_at) " + 
		"VALUES ($1, $2, $3, $4, $5, $6, $7)",
		ue.Nome, ue.Email, ue.Telefone, ue.SenhaHash, "pendente", now, now,
	)

	if err != nil {
		return err
	}

	return nil
}

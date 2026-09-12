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

func (r *postgresRepository) UpdateProfile(ctx context.Context, profile *me.ProfileEntity) error {
	_, err := r.pool.Exec(
		ctx,
		"UPDATE usuarios SET nome = $1, email = $2, telefone = $3, avatar_url = $4 WHERE id = $5",
		profile.Name,
		profile.Email,
		profile.Phone,
		profile.AvatarURL,
		profile.ID,
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *postgresRepository) GetAddressesByUserID(ctx context.Context, userID string) ([]*me.AddressEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT id, usuario_id, apelido, destinatario, cep, logradouro, numero, complemento, bairro, cidade, estado, pais, is_principal FROM enderecos WHERE usuario_id = $1",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var addresses []*me.AddressEntity
	for rows.Next() {
		var m repository.AddressModel
		err := rows.Scan(
			&m.ID,
			&m.UserID,
			&m.Label,
			&m.Receiver,
			&m.ZIPCode,
			&m.Street,
			&m.Number,
			&m.Complement,
			&m.District,
			&m.City,
			&m.State,
			&m.Country,
			&m.IsDefault,
		)
		if err != nil {
			return nil, err
		}
		addresses = append(addresses, m.ToEntity())
	}

	return addresses, nil
}

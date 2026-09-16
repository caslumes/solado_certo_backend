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

func (r *postgresRepository) GetAddressByID(ctx context.Context, addressID string) (*me.AddressEntity, error) {
	var m repository.AddressModel
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, usuario_id, apelido, destinatario, cep, logradouro, numero, complemento, bairro, cidade, estado, pais, is_principal FROM enderecos WHERE id = $1",
		addressID,
	).Scan(
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

	e := m.ToEntity()
	return e, nil
}

func (r *postgresRepository) CreateAddress(ctx context.Context, userID string, address *me.AddressEntity) (*me.AddressEntity, error) {
	var m repository.AddressModel
	err := r.pool.QueryRow(
		ctx,
		"INSERT INTO enderecos (usuario_id, apelido, destinatario, cep, logradouro, numero, complemento, bairro, cidade, estado, pais, is_principal, created_at, updated_at)"+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW(), NOW()) RETURNING id",
		userID,
		address.Label,
		address.Receiver,
		address.ZIPCode,
		address.Street,
		address.Number,
		address.Complement,
		address.District,
		address.City,
		address.State,
		address.Country,
		address.IsDefault,
	).Scan(&m.ID)
	if err != nil {
		return nil, err
	}

	address.ID = m.ID

	return address, nil
}

func (r *postgresRepository) UpdateAddress(ctx context.Context, address *me.AddressEntity) error {
	_, err := r.pool.Exec(
		ctx,
		"UPDATE enderecos SET apelido = $1, destinatario = $2, cep = $3, logradouro = $4, numero = $5, complemento = $6, bairro = $7, cidade = $8, estado = $9, pais = $10, is_principal = $11, updated_at = NOW() WHERE id = $12",
		address.Label,
		address.Receiver,
		address.ZIPCode,
		address.Street,
		address.Number,
		address.Complement,
		address.District,
		address.City,
		address.State,
		address.Country,
		address.IsDefault,
		address.ID,
	)
	if err != nil {
		return err
	}

	return nil
}

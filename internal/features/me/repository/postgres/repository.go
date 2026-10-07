package postgres

import (
	"context"
	"errors"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/features/me/repository"
	"github.com/jackc/pgx/v5"
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
		"SELECT name, email, phone, avatar_url, status FROM users WHERE id = $1",
		userID,
	).Scan(&m.Name, &m.Email, &m.Phone, &m.AvatarURL, &m.Status)
	if err != nil {
		return nil, err
	}

	m.ID = userID
	e := m.ToEntity()
	return e, nil
}

func (r *postgresRepository) UpdateProfile(ctx context.Context, profile *me.ProfileEntity) error {
	_, err := r.pool.Exec(
		ctx,
		"UPDATE users SET name = $1, email = $2, phone = $3, avatar_url = $4, status = $5 WHERE id = $6",
		profile.Name,
		profile.Email,
		profile.Phone,
		profile.AvatarURL,
		profile.Status,
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
		"SELECT id, user_id, label, receiver, zip_code, street, number, complement, district, city, state, country, is_default FROM addresses WHERE user_id = $1",
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
		"SELECT id, user_id, label, receiver, zip_code, street, number, complement, district, city, state, country, is_default FROM addresses WHERE id = $1",
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
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrAddressNotFound
		}

		return nil, err
	}

	e := m.ToEntity()
	return e, nil
}

func (r *postgresRepository) CreateAddress(ctx context.Context, userID string, address *me.AddressEntity) (*me.AddressEntity, error) {
	var m repository.AddressModel
	err := r.pool.QueryRow(
		ctx,
		"INSERT INTO addresses (user_id, label, receiver, zip_code, street, number, complement, district, city, state, country, is_default, created_at, updated_at)"+
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
		"UPDATE addresses SET label = $1, receiver = $2, zip_code = $3, street = $4, number = $5, complement = $6, district = $7, city = $8, state = $9, country = $10, is_default = $11, updated_at = NOW() WHERE id = $12",
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

func (r *postgresRepository) DeleteAddress(ctx context.Context, addressID string) error {
	_, err := r.pool.Exec(
		ctx,
		"DELETE FROM addresses WHERE id = $1",
		addressID,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrAddressNotFound
		}

		return err
	}

	return nil
}

func (r *postgresRepository) GetPodologicalProfileByUserID(ctx context.Context, userID string) (*me.PodologicalProfileEntity, error) {
	var m repository.PodologicalProfileModel
	err := r.pool.QueryRow(
		ctx,
		"SELECT user_id, footstrike_type, clinical_condition, notes, updated_at FROM podological_profiles WHERE user_id = $1",
		userID,
	).Scan(
		&m.UserID,
		&m.FootstrikeType,
		&m.ClinicalCondition,
		&m.Obs,
		&m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrPodologicalProfileNotFound
		}

		return nil, err
	}

	e := m.ToEntity()
	return e, nil
}

func (r *postgresRepository) UpsertPodologicalProfile(ctx context.Context, profile *me.PodologicalProfileEntity) error {
	_, err := r.pool.Exec(
		ctx,
		"INSERT INTO podological_profiles (user_id, footstrike_type, clinical_condition, notes, updated_at)"+
			"VALUES ($1, $2, $3, $4, NOW()) ON CONFLICT (user_id) DO UPDATE SET footstrike_type = $2, clinical_condition = $3, notes = $4, updated_at = NOW()",
		profile.UserID,
		profile.FootstrikeType,
		profile.ClinicalCondition,
		profile.Obs,
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *postgresRepository) CreateOnboarding(ctx context.Context, onboarding *me.OnboardingEntity) error {
	_, err := r.pool.Exec(
		ctx,
		"INSERT INTO onboardings (user_id, status, current_step_id, completed_at, dismissed_at, created_at, updated_at) "+
			"VALUES ($1, $2, (SELECT id FROM onboarding_steps WHERE position = 1), $3, $4, NOW(), NOW())",
		onboarding.UserID,
		onboarding.Status,
		onboarding.CompletedAt,
		onboarding.DispensedAt,
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *postgresRepository) GetOnboardingByUserID(ctx context.Context, userID string) (*me.OnboardingEntity, error) {
	var osm repository.OnboardingStepModel
	var om repository.OnboardingModel
	err := r.pool.QueryRow(
		ctx,
		"SELECT o.user_id, o.status, o.current_step_id, o.completed_at, o.created_at, o.updated_at, e.id, e.tag, e.name, e.position, e.required FROM onboardings o LEFT JOIN onboarding_steps e ON o.current_step_id = e.id WHERE o.user_id = $1",
		userID,
	).Scan(
		&om.UserID,
		&om.Status,
		&osm.ID,
		&om.CompletedAt,
		&om.CreatedAt,
		&om.UpdatedAt,
		&osm.ID,
		&osm.Tag,
		&osm.Name,
		&osm.Order,
		&osm.IsRequired,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrOnboardingNotFound
		}

		return nil, err
	}

	e := om.ToEntity()
	e.CurrentStep = me.OnboardingStepEntity{
		ID:         osm.ID,
		Tag:        osm.Tag,
		Name:       osm.Name,
		Order:      osm.Order,
		IsRequired: osm.IsRequired,
	}
	return e, nil
}

func (r *postgresRepository) GetOnboardingStepByTag(ctx context.Context, tag string) (*me.OnboardingStepEntity, error) {
	var m repository.OnboardingStepModel
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, tag, name, position, required FROM onboarding_steps WHERE tag = $1",
		tag,
	).Scan(
		&m.ID,
		&m.Tag,
		&m.Name,
		&m.Order,
		&m.IsRequired,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrOnboardingStepNotFound
		}

		return nil, err
	}

	e := m.ToEntity()
	return e, nil
}

func (r *postgresRepository) GetOnboardingStepByOrder(ctx context.Context, order int) (*me.OnboardingStepEntity, error) {
	var m repository.OnboardingStepModel
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, tag, name, position, required FROM onboarding_steps WHERE position = $1",
		order,
	).Scan(
		&m.ID,
		&m.Tag,
		&m.Name,
		&m.Order,
		&m.IsRequired,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrOnboardingStepNotFound
		}

		return nil, err
	}

	e := m.ToEntity()
	return e, nil
}

func (r *postgresRepository) UpdateOnboarding(ctx context.Context, userID string, onboarding *me.OnboardingEntity) error {
	_, err := r.pool.Exec(
		ctx,
		"UPDATE onboardings SET status = $1, current_step_id = $2, completed_at = $3, dismissed_at = $4, updated_at = NOW() WHERE user_id = $5",
		onboarding.Status,
		onboarding.CurrentStep.ID,
		onboarding.CompletedAt,
		onboarding.DispensedAt,
		userID,
	)
	if err != nil {
		return err
	}

	return nil
}

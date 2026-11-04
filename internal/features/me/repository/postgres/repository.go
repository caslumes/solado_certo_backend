package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/features/me/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	uniqueViolationCode     = "23505"
	foreignKeyViolationCode = "23503"
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
		"SELECT name, email, phone, avatar_url, status, tax_id IS NOT NULL FROM users WHERE id = $1",
		userID,
	).Scan(&m.Name, &m.Email, &m.Phone, &m.AvatarURL, &m.Status, &m.HasTaxID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrUserNotFound
		}

		return nil, err
	}

	m.ID = userID
	e := m.ToEntity()
	return e, nil
}

func (r *postgresRepository) UpdateProfile(ctx context.Context, profile *me.ProfileEntity) error {
	_, err := r.pool.Exec(
		ctx,
		"UPDATE users SET name = $1, email = $2, phone = $3, status = $4 WHERE id = $5",
		profile.Name,
		profile.Email,
		profile.Phone,
		profile.Status,
		profile.ID,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode {
			return app.ErrUserAlreadyExists
		}

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

	rows, err := r.pool.Query(
		ctx,
		"SELECT p.id, p.name, p.description FROM podological_profile_pain_points pp "+
			"JOIN pain_points p ON p.id = pp.pain_point_id WHERE pp.profile_user_id = $1 ORDER BY p.name",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var pm repository.PainPointModel
		if err := rows.Scan(&pm.ID, &pm.Name, &pm.Description); err != nil {
			return nil, err
		}
		e.PainPoints = append(e.PainPoints, *pm.ToEntity())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return e, nil
}

func (r *postgresRepository) SavePodologicalProfile(ctx context.Context, profile *me.PodologicalProfileEntity, painPointIDs []string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(
		ctx,
		"INSERT INTO podological_profiles (user_id, footstrike_type, clinical_condition, notes, updated_at) "+
			"VALUES ($1, $2, $3, $4, NOW()) ON CONFLICT (user_id) DO UPDATE SET footstrike_type = $2, clinical_condition = $3, notes = $4, updated_at = NOW()",
		profile.UserID,
		profile.FootstrikeType,
		profile.ClinicalCondition,
		profile.Obs,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, "DELETE FROM podological_profile_pain_points WHERE profile_user_id = $1", profile.UserID)
	if err != nil {
		return err
	}

	if len(painPointIDs) > 0 {
		_, err = tx.Exec(
			ctx,
			"INSERT INTO podological_profile_pain_points (profile_user_id, pain_point_id) SELECT $1, unnest($2::uuid[])",
			profile.UserID,
			painPointIDs,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *postgresRepository) GetPainPoints(ctx context.Context) ([]*me.PainPointEntity, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, name, description FROM pain_points ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	painPoints := []*me.PainPointEntity{}
	for rows.Next() {
		var m repository.PainPointModel
		if err := rows.Scan(&m.ID, &m.Name, &m.Description); err != nil {
			return nil, err
		}
		painPoints = append(painPoints, m.ToEntity())
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return painPoints, nil
}

func (r *postgresRepository) CountPainPoints(ctx context.Context, ids []string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT count(*) FROM pain_points WHERE id = ANY($1::uuid[])", ids).Scan(&count)
	if err != nil {
		return 0, err
	}

	return count, nil
}

const consentColumns = "id, user_id, purpose, terms_version, granted_at, revoked_at"

func scanConsent(row pgx.Row) (*me.ConsentEntity, error) {
	var m repository.ConsentModel
	err := row.Scan(&m.ID, &m.UserID, &m.Purpose, &m.TermsVersion, &m.GrantedAt, &m.RevokedAt)
	if err != nil {
		return nil, err
	}

	return m.ToEntity(), nil
}

func (r *postgresRepository) GetActiveConsents(ctx context.Context, userID string) ([]*me.ConsentEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT "+consentColumns+" FROM consents WHERE user_id = $1 AND revoked_at IS NULL ORDER BY granted_at",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	consents := []*me.ConsentEntity{}
	for rows.Next() {
		consent, err := scanConsent(rows)
		if err != nil {
			return nil, err
		}
		consents = append(consents, consent)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return consents, nil
}

func (r *postgresRepository) GetActiveConsent(ctx context.Context, userID string, purpose me.ConsentPurpose) (*me.ConsentEntity, error) {
	consent, err := scanConsent(r.pool.QueryRow(
		ctx,
		"SELECT "+consentColumns+" FROM consents WHERE user_id = $1 AND purpose = $2 AND revoked_at IS NULL",
		userID,
		purpose,
	))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrConsentNotFound
		}

		return nil, err
	}

	return consent, nil
}

func (r *postgresRepository) GrantConsent(ctx context.Context, consent *me.ConsentEntity) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(
		ctx,
		"UPDATE consents SET revoked_at = NOW() WHERE user_id = $1 AND purpose = $2 AND revoked_at IS NULL AND terms_version <> $3",
		consent.UserID,
		consent.Purpose,
		consent.TermsVersion,
	)
	if err != nil {
		return false, err
	}

	result, err := tx.Exec(
		ctx,
		"INSERT INTO consents (user_id, purpose, terms_version, granted_at) VALUES ($1, $2, $3, NOW()) "+
			"ON CONFLICT (user_id, purpose) WHERE revoked_at IS NULL DO NOTHING",
		consent.UserID,
		consent.Purpose,
		consent.TermsVersion,
	)
	if err != nil {
		return false, err
	}
	created := result.RowsAffected() == 1

	saved, err := scanConsent(tx.QueryRow(
		ctx,
		"SELECT "+consentColumns+" FROM consents WHERE user_id = $1 AND purpose = $2 AND revoked_at IS NULL",
		consent.UserID,
		consent.Purpose,
	))
	if err != nil {
		return false, err
	}
	*consent = *saved

	return created, tx.Commit(ctx)
}

func (r *postgresRepository) RevokeConsent(ctx context.Context, userID string, purpose me.ConsentPurpose) error {
	_, err := r.pool.Exec(
		ctx,
		"UPDATE consents SET revoked_at = NOW() WHERE user_id = $1 AND purpose = $2 AND revoked_at IS NULL",
		userID,
		purpose,
	)
	return err
}

func (r *postgresRepository) RevokeConsentAndDeletePodologicalProfile(ctx context.Context, userID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	statements := []string{
		"UPDATE consents SET revoked_at = NOW() WHERE user_id = $1 AND purpose = 'podological_profile' AND revoked_at IS NULL",
		"DELETE FROM podological_profile_pain_points WHERE profile_user_id = $1",
		"DELETE FROM podological_profiles WHERE user_id = $1",
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement, userID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *postgresRepository) CreateOnboarding(ctx context.Context, onboarding *me.OnboardingEntity) error {
	_, err := r.pool.Exec(
		ctx,
		"INSERT INTO onboardings (user_id, status, current_step_id, completed_at, dismissed_at, created_at, updated_at) "+
			"VALUES ($1, $2, (SELECT id FROM onboarding_steps WHERE position = 1), $3, $4, NOW(), NOW())",
		onboarding.UserID,
		onboarding.Status,
		onboarding.CompletedAt,
		onboarding.DismissedAt,
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
		"SELECT o.user_id, o.status, o.current_step_id, o.completed_at, o.dismissed_at, o.created_at, o.updated_at, e.id, e.tag, e.name, e.position, e.required FROM onboardings o LEFT JOIN onboarding_steps e ON o.current_step_id = e.id WHERE o.user_id = $1",
		userID,
	).Scan(
		&om.UserID,
		&om.Status,
		&osm.ID,
		&om.CompletedAt,
		&om.DismissedAt,
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
		onboarding.DismissedAt,
		userID,
	)
	if err != nil {
		return err
	}

	return nil
}

func (r *postgresRepository) GetRolesByUserID(ctx context.Context, userID string) ([]enum.Role, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT r.code FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1 ORDER BY r.code",
		userID,
	)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, pgx.RowTo[enum.Role])
}

func (r *postgresRepository) AddRole(ctx context.Context, userID string, role enum.Role) (bool, error) {
	result, err := r.pool.Exec(
		ctx,
		"INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = $2 ON CONFLICT DO NOTHING",
		userID,
		role,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode {
			return false, app.ErrUserNotFound
		}

		return false, err
	}

	return result.RowsAffected() == 1, nil
}

func (r *postgresRepository) UpdateAvatar(ctx context.Context, userID string, avatarURL *string) error {
	result, err := r.pool.Exec(ctx, "UPDATE users SET avatar_url = $1, updated_at = NOW() WHERE id = $2", avatarURL, userID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return app.ErrUserNotFound
	}
	return nil
}

const personalDataProfileQuery = "SELECT id, name, email, phone, tax_id, avatar_url, status, created_at, updated_at FROM users WHERE id = $1 AND deleted_at IS NULL"

var personalDataSections = []struct{ name, query string }{
	{"roles", "SELECT r.code FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE ur.user_id = $1 ORDER BY r.code"},
	{"addresses", "SELECT label, receiver, zip_code, street, number, complement, district, city, state, country, is_default, created_at " +
		"FROM addresses WHERE user_id = $1 ORDER BY created_at"},
	{"podological_profile", "SELECT footstrike_type, clinical_condition, notes, updated_at FROM podological_profiles WHERE user_id = $1"},
	{"podological_pain_points", "SELECT pp.name FROM podological_profile_pain_points ppp JOIN pain_points pp ON pp.id = ppp.pain_point_id " +
		"WHERE ppp.profile_user_id = $1 ORDER BY pp.name"},
	{"consents", "SELECT purpose, terms_version, granted_at, revoked_at FROM consents WHERE user_id = $1 ORDER BY granted_at"},
	{"orders", "SELECT number, status, subtotal, shipping_total, discount_total, total, shipping_address, created_at, paid_at, delivered_at " +
		"FROM orders WHERE buyer_id = $1 ORDER BY created_at"},
	{"order_items", "SELECT o.number AS order_number, oi.product_snapshot->>'title' AS title, oi.product_snapshot->>'seller_name' AS seller_name, " +
		"oi.quantity, oi.unit_price, oi.subtotal FROM order_items oi JOIN orders o ON o.id = oi.order_id WHERE o.buyer_id = $1 ORDER BY o.created_at, oi.created_at"},
	{"payments", "SELECT o.number AS order_number, p.method, p.status, p.amount, p.created_at " +
		"FROM payments p JOIN orders o ON o.id = p.order_id WHERE o.buyer_id = $1 ORDER BY p.created_at"},
	{"custom_orders", "SELECT a.title AS advertisement, c.status, c.measurements, c.technical_notes, c.quoted_price, c.created_at " +
		"FROM custom_order_requests c JOIN advertisements a ON a.id = c.advertisement_id WHERE c.buyer_id = $1 ORDER BY c.created_at"},
	{"documents", "SELECT o.number AS order_number, d.document_type, d.validation_status, d.created_at " +
		"FROM order_documents d JOIN orders o ON o.id = d.order_id WHERE d.uploaded_by = $1 ORDER BY d.created_at"},
	{"reviews", "SELECT oi.product_snapshot->>'title' AS product, r.product_rating, r.seller_rating, r.comfort_rating, r.comment, r.adaptation_feedback, r.created_at " +
		"FROM reviews r JOIN order_items oi ON oi.id = r.order_item_id WHERE r.reviewer_id = $1 ORDER BY r.created_at"},
	{"support_tickets", "SELECT category, status, subject, description, resolution, opened_at, closed_at FROM support_tickets WHERE opened_by = $1 ORDER BY opened_at"},
	{"support_messages", "SELECT t.subject AS ticket, m.message, m.attachment_url IS NOT NULL AS has_attachment, m.created_at " +
		"FROM support_messages m JOIN support_tickets t ON t.id = m.ticket_id WHERE m.sender_id = $1 ORDER BY m.created_at"},
	{"notifications", "SELECT type, title, body, read_at, created_at FROM notifications WHERE user_id = $1 ORDER BY created_at"},
	{"advertisements", "SELECT title, status, price, stock, created_at FROM advertisements WHERE seller_id = $1 ORDER BY created_at"},
}

func (r *postgresRepository) ExportPersonalData(ctx context.Context, userID string) (json.RawMessage, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	export := map[string]any{"generated_at": time.Now().UTC()}

	var profile json.RawMessage
	if err := tx.QueryRow(ctx, "SELECT row_to_json(t) FROM ("+personalDataProfileQuery+") t", userID).Scan(&profile); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrUserNotFound
		}
		return nil, err
	}
	export["profile"] = profile

	for _, section := range personalDataSections {
		var rows json.RawMessage
		if err := tx.QueryRow(ctx, "SELECT COALESCE(json_agg(t), '[]') FROM ("+section.query+") t", userID).Scan(&rows); err != nil {
			return nil, err
		}
		export[section.name] = rows
	}

	_, err = tx.Exec(
		ctx,
		"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'user', $1, 'user.data_exported', '{}', NOW())",
		userID,
	)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return json.Marshal(export)
}

func (r *postgresRepository) GetPasswordHash(ctx context.Context, userID string) (string, error) {
	var hash string
	err := r.pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE id = $1 AND deleted_at IS NULL", userID).Scan(&hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", app.ErrUserNotFound
	}
	return hash, err
}

const openOrdersQuery = "SELECT EXISTS (SELECT 1 FROM orders WHERE buyer_id = $1 AND status IN ('awaiting_payment', 'paid', 'processing', 'shipped')) " +
	"OR EXISTS (SELECT 1 FROM order_items oi JOIN orders o ON o.id = oi.order_id " +
	"WHERE oi.seller_id = $1 AND o.status IN ('awaiting_payment', 'paid', 'processing', 'shipped')) " +
	"OR EXISTS (SELECT 1 FROM custom_order_requests cr JOIN advertisements a ON a.id = cr.advertisement_id " +
	"WHERE (cr.buyer_id = $1 OR a.seller_id = $1) AND cr.status IN ('requested', 'under_review', 'needs_info', 'approved', 'in_production'))"

var accountDeletionStatements = []string{
	"UPDATE users SET name = 'Usuário removido', email = id::text || '@removido.invalid', phone = replace(id::text, '-', ''), " +
		"password_hash = '', avatar_url = NULL, tax_id = NULL, status = 'inactive', suspended_until = NULL, deleted_at = NOW(), updated_at = NOW() WHERE id = $1",
	"UPDATE refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL",
	"DELETE FROM password_reset_tokens WHERE user_id = $1",
	"UPDATE consents SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL",
	"DELETE FROM podological_profile_pain_points WHERE profile_user_id = $1",
	"DELETE FROM podological_profiles WHERE user_id = $1",
	"DELETE FROM addresses WHERE user_id = $1",
	"DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id = $1)",
	"UPDATE carts SET status = 'abandoned', updated_at = NOW() WHERE user_id = $1 AND status = 'active'",
	"DELETE FROM notifications WHERE user_id = $1",
	"DELETE FROM payment_customers WHERE user_id = $1",
	"UPDATE advertisements SET status = 'archived', updated_at = NOW() WHERE seller_id = $1 AND status IN ('draft', 'published', 'paused')",
	"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'user', $1, 'user.account_deleted', '{}', NOW())",
}

func (r *postgresRepository) DeleteAccount(ctx context.Context, userID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var avatarURL *string
	err = tx.QueryRow(ctx, "SELECT avatar_url FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE", userID).Scan(&avatarURL)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", app.ErrUserNotFound
		}
		return "", err
	}

	var hasOpenOrders bool
	if err := tx.QueryRow(ctx, openOrdersQuery, userID).Scan(&hasOpenOrders); err != nil {
		return "", err
	}
	if hasOpenOrders {
		return "", app.ErrAccountHasOpenOrders
	}

	for _, statement := range accountDeletionStatements {
		if _, err := tx.Exec(ctx, statement, userID); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	if avatarURL == nil {
		return "", nil
	}
	return *avatarURL, nil
}

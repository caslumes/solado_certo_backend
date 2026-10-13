package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/seller"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

const advertisementColumns = "a.id, a.seller_id, a.category_id, a.title, a.description, a.price::text, a.stock, a.status, " +
	"a.prescription_required, a.custom_made_allowed, a.side, a.size, a.material, a.rigidity, a.usage_indication, " +
	"a.safety_alerts, a.contraindications, a.shipping_notes, c.required_attributes, a.created_at, a.updated_at " +
	"FROM advertisements a JOIN categories c ON c.id = a.category_id"

func scanAdvertisement(row pgx.Row) (*seller.AdvertisementEntity, error) {
	var ad seller.AdvertisementEntity
	err := row.Scan(
		&ad.ID,
		&ad.SellerID,
		&ad.CategoryID,
		&ad.Title,
		&ad.Description,
		&ad.Price,
		&ad.Stock,
		&ad.Status,
		&ad.PrescriptionRequired,
		&ad.CustomMadeAllowed,
		&ad.Side,
		&ad.Size,
		&ad.Material,
		&ad.Rigidity,
		&ad.UsageIndication,
		&ad.SafetyAlerts,
		&ad.Contraindications,
		&ad.ShippingNotes,
		&ad.RequiredAttributes,
		&ad.CreatedAt,
		&ad.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &ad, nil
}

func (r *postgresRepository) GetAdvertisementsBySeller(ctx context.Context, sellerID string, status *seller.AdvertisementStatus) ([]*seller.AdvertisementEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT "+advertisementColumns+" WHERE a.seller_id = $1 AND a.removed_at IS NULL "+
			"AND ($2::text IS NULL OR a.status = $2) ORDER BY a.updated_at DESC, a.id DESC",
		sellerID, status,
	)
	if err != nil {
		return nil, err
	}

	ads := []*seller.AdvertisementEntity{}
	for rows.Next() {
		ad, err := scanAdvertisement(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		ads = append(ads, ad)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := r.loadRelations(ctx, ads); err != nil {
		return nil, err
	}
	return ads, nil
}

func (r *postgresRepository) GetAdvertisementByID(ctx context.Context, id string) (*seller.AdvertisementEntity, error) {
	ad, err := scanAdvertisement(r.pool.QueryRow(ctx, "SELECT "+advertisementColumns+" WHERE a.id = $1 AND a.removed_at IS NULL", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrAdvertisementNotFound
		}
		return nil, err
	}

	if err := r.loadRelations(ctx, []*seller.AdvertisementEntity{ad}); err != nil {
		return nil, err
	}
	return ad, nil
}

// loadRelations fills purposes, conditions, active shipping options and images with one query each.
func (r *postgresRepository) loadRelations(ctx context.Context, ads []*seller.AdvertisementEntity) error {
	if len(ads) == 0 {
		return nil
	}
	ids := make([]string, len(ads))
	byID := make(map[string]*seller.AdvertisementEntity, len(ads))
	for i, ad := range ads {
		ids[i] = ad.ID
		ad.PurposeCodes = []string{}
		ad.ConditionIDs = []string{}
		ad.ShippingOptions = []seller.ShippingOptionEntity{}
		ad.Images = []seller.ImageEntity{}
		byID[ad.ID] = ad
	}

	err := r.each(ctx,
		"SELECT ap.advertisement_id, p.code FROM advertisement_purposes ap JOIN podological_purposes p ON p.id = ap.purpose_id "+
			"WHERE ap.advertisement_id = ANY($1::uuid[]) ORDER BY p.code",
		ids,
		func(rows pgx.Rows) error {
			var adID, code string
			if err := rows.Scan(&adID, &code); err != nil {
				return err
			}
			byID[adID].PurposeCodes = append(byID[adID].PurposeCodes, code)
			return nil
		},
	)
	if err != nil {
		return err
	}

	err = r.each(ctx,
		"SELECT advertisement_id, condition_id FROM advertisement_conditions WHERE advertisement_id = ANY($1::uuid[]) ORDER BY condition_id",
		ids,
		func(rows pgx.Rows) error {
			var adID, conditionID string
			if err := rows.Scan(&adID, &conditionID); err != nil {
				return err
			}
			byID[adID].ConditionIDs = append(byID[adID].ConditionIDs, conditionID)
			return nil
		},
	)
	if err != nil {
		return err
	}

	err = r.each(ctx,
		"SELECT advertisement_id, id, name, carrier, price::text, delivery_days FROM shipping_options "+
			"WHERE advertisement_id = ANY($1::uuid[]) AND active ORDER BY price, delivery_days, id",
		ids,
		func(rows pgx.Rows) error {
			var adID string
			var o seller.ShippingOptionEntity
			if err := rows.Scan(&adID, &o.ID, &o.Name, &o.Carrier, &o.Price, &o.DeliveryDays); err != nil {
				return err
			}
			byID[adID].ShippingOptions = append(byID[adID].ShippingOptions, o)
			return nil
		},
	)
	if err != nil {
		return err
	}

	return r.each(ctx,
		"SELECT advertisement_id, id, url, COALESCE(alt_text, ''), position, is_cover FROM advertisement_images "+
			"WHERE advertisement_id = ANY($1::uuid[]) ORDER BY position, id",
		ids,
		func(rows pgx.Rows) error {
			var adID string
			var i seller.ImageEntity
			if err := rows.Scan(&adID, &i.ID, &i.URL, &i.AltText, &i.Position, &i.IsCover); err != nil {
				return err
			}
			byID[adID].Images = append(byID[adID].Images, i)
			return nil
		},
	)
}

func (r *postgresRepository) each(ctx context.Context, query string, ids []string, scan func(pgx.Rows) error) error {
	rows, err := r.pool.Query(ctx, query, ids)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (r *postgresRepository) GetCategoryRequiredAttributes(ctx context.Context, categoryID string) ([]string, error) {
	var required []string
	err := r.pool.QueryRow(ctx, "SELECT required_attributes FROM categories WHERE id = $1 AND active", categoryID).Scan(&required)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrCategoryNotFound
		}
		return nil, err
	}
	return required, nil
}

func (r *postgresRepository) CountPurposes(ctx context.Context, codes []string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM podological_purposes WHERE code = ANY($1)", codes).Scan(&count)
	return count, err
}

func (r *postgresRepository) CountActiveConditions(ctx context.Context, ids []string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM podological_conditions WHERE id = ANY($1::uuid[]) AND active", ids).Scan(&count)
	return count, err
}

func (r *postgresRepository) CreateAdvertisement(ctx context.Context, ad *seller.AdvertisementEntity) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	var id string
	err = tx.QueryRow(
		ctx,
		"INSERT INTO advertisements (seller_id, category_id, title, description, price, stock, status, prescription_required, "+
			"custom_made_allowed, side, size, material, rigidity, usage_indication, safety_alerts, contraindications, shipping_notes, "+
			"created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $18) RETURNING id",
		ad.SellerID, ad.CategoryID, ad.Title, ad.Description, ad.Price, ad.Stock, ad.Status, ad.PrescriptionRequired,
		ad.CustomMadeAllowed, ad.Side, ad.Size, ad.Material, ad.Rigidity, ad.UsageIndication, ad.SafetyAlerts,
		ad.Contraindications, ad.ShippingNotes, now,
	).Scan(&id)
	if err != nil {
		return "", err
	}

	if err := saveLinks(ctx, tx, id, ad); err != nil {
		return "", err
	}
	if err := saveShippingOptions(ctx, tx, id, ad.ShippingOptions); err != nil {
		return "", err
	}

	return id, tx.Commit(ctx)
}

// UpdateAdvertisement saves the ad only if its status is still expectedStatus, so an edit validated
// for a draft can't land on an ad that was published meanwhile (app.ErrInvalidStatusTransition).
func (r *postgresRepository) UpdateAdvertisement(ctx context.Context, ad *seller.AdvertisementEntity, expectedStatus seller.AdvertisementStatus) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(
		ctx,
		"UPDATE advertisements SET category_id = $2, title = $3, description = $4, price = $5, stock = $6, "+
			"prescription_required = $7, custom_made_allowed = $8, side = $9, size = $10, material = $11, rigidity = $12, "+
			"usage_indication = $13, safety_alerts = $14, contraindications = $15, shipping_notes = $16, updated_at = $17 "+
			"WHERE id = $1 AND status = $18 AND removed_at IS NULL",
		ad.ID, ad.CategoryID, ad.Title, ad.Description, ad.Price, ad.Stock, ad.PrescriptionRequired, ad.CustomMadeAllowed,
		ad.Side, ad.Size, ad.Material, ad.Rigidity, ad.UsageIndication, ad.SafetyAlerts, ad.Contraindications,
		ad.ShippingNotes, time.Now().UTC(), expectedStatus,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return app.ErrInvalidStatusTransition
	}

	if _, err := tx.Exec(ctx, "DELETE FROM advertisement_purposes WHERE advertisement_id = $1", ad.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "DELETE FROM advertisement_conditions WHERE advertisement_id = $1", ad.ID); err != nil {
		return err
	}
	if err := saveLinks(ctx, tx, ad.ID, ad); err != nil {
		return err
	}
	if err := saveShippingOptions(ctx, tx, ad.ID, ad.ShippingOptions); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func saveLinks(ctx context.Context, tx pgx.Tx, id string, ad *seller.AdvertisementEntity) error {
	if len(ad.PurposeCodes) > 0 {
		_, err := tx.Exec(
			ctx,
			"INSERT INTO advertisement_purposes (advertisement_id, purpose_id) SELECT $1, id FROM podological_purposes WHERE code = ANY($2)",
			id, ad.PurposeCodes,
		)
		if err != nil {
			return err
		}
	}
	if len(ad.ConditionIDs) > 0 {
		_, err := tx.Exec(
			ctx,
			"INSERT INTO advertisement_conditions (advertisement_id, condition_id) SELECT $1, unnest($2::uuid[])",
			id, ad.ConditionIDs,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

// saveShippingOptions updates the options that have an ID, inserts the new ones and deactivates the rest.
// Options are never deleted, because shipments reference them.
func saveShippingOptions(ctx context.Context, tx pgx.Tx, advertisementID string, options []seller.ShippingOptionEntity) error {
	keep := []string{}
	for _, o := range options {
		if o.ID != "" {
			_, err := tx.Exec(
				ctx,
				"UPDATE shipping_options SET name = $3, carrier = $4, price = $5, delivery_days = $6, active = true "+
					"WHERE id = $1 AND advertisement_id = $2",
				o.ID, advertisementID, o.Name, o.Carrier, o.Price, o.DeliveryDays,
			)
			if err != nil {
				return err
			}
			keep = append(keep, o.ID)
			continue
		}

		var id string
		err := tx.QueryRow(
			ctx,
			"INSERT INTO shipping_options (advertisement_id, name, carrier, price, delivery_days) VALUES ($1, $2, $3, $4, $5) RETURNING id",
			advertisementID, o.Name, o.Carrier, o.Price, o.DeliveryDays,
		).Scan(&id)
		if err != nil {
			return err
		}
		keep = append(keep, id)
	}

	_, err := tx.Exec(
		ctx,
		"UPDATE shipping_options SET active = false WHERE advertisement_id = $1 AND active AND NOT (id = ANY($2::uuid[]))",
		advertisementID, keep,
	)
	return err
}

func (r *postgresRepository) ChangeStatus(ctx context.Context, id string, from seller.AdvertisementStatus, to seller.AdvertisementStatus) error {
	result, err := r.pool.Exec(
		ctx,
		"UPDATE advertisements SET status = $3, updated_at = $4 WHERE id = $1 AND status = $2 AND removed_at IS NULL",
		id, from, to, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return app.ErrInvalidStatusTransition
	}
	return nil
}

func (r *postgresRepository) RemoveAdvertisement(ctx context.Context, id string) error {
	now := time.Now().UTC()
	result, err := r.pool.Exec(
		ctx,
		"UPDATE advertisements SET status = $2, removed_at = $3, updated_at = $3 WHERE id = $1 AND removed_at IS NULL",
		id, seller.AdvertisementStatusRemoved, now,
	)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return app.ErrAdvertisementNotFound
	}
	return nil
}

// AddImage locks the ad row so concurrent uploads can't go past maxImages. The first image becomes the cover.
func (r *postgresRepository) AddImage(ctx context.Context, advertisementID string, url string, altText *string, maxImages int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var locked string
	err = tx.QueryRow(ctx, "SELECT id FROM advertisements WHERE id = $1 AND removed_at IS NULL FOR UPDATE", advertisementID).Scan(&locked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrAdvertisementNotFound
		}
		return err
	}

	var count, lastPosition int
	err = tx.QueryRow(
		ctx,
		"SELECT COUNT(*), COALESCE(MAX(position), 0) FROM advertisement_images WHERE advertisement_id = $1",
		advertisementID,
	).Scan(&count, &lastPosition)
	if err != nil {
		return err
	}
	if count >= maxImages {
		return app.ErrTooManyImages
	}

	_, err = tx.Exec(
		ctx,
		"INSERT INTO advertisement_images (advertisement_id, url, alt_text, position, is_cover) VALUES ($1, $2, $3, $4, $5)",
		advertisementID, url, altText, lastPosition+1, count == 0,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RemoveImage deletes the image, renumbers the others from 1 and, if the cover was removed,
// makes the first remaining image the cover.
func (r *postgresRepository) RemoveImage(ctx context.Context, advertisementID string, imageID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	result, err := tx.Exec(ctx, "DELETE FROM advertisement_images WHERE id = $1 AND advertisement_id = $2", imageID, advertisementID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return app.ErrImageNotFound
	}

	_, err = tx.Exec(
		ctx,
		"UPDATE advertisement_images ai SET position = o.position, "+
			"is_cover = ai.is_cover OR (o.position = 1 AND NOT EXISTS "+
			"(SELECT 1 FROM advertisement_images c WHERE c.advertisement_id = $1 AND c.is_cover)) "+
			"FROM (SELECT id, ROW_NUMBER() OVER (ORDER BY position, id) AS position FROM advertisement_images "+
			"WHERE advertisement_id = $1) o WHERE ai.id = o.id",
		advertisementID,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *postgresRepository) ReorderImages(ctx context.Context, advertisementID string, imageIDs []string, coverImageID string) error {
	_, err := r.pool.Exec(
		ctx,
		"UPDATE advertisement_images ai SET position = o.position, is_cover = (ai.id = $3) "+
			"FROM unnest($2::uuid[]) WITH ORDINALITY AS o(id, position) "+
			"WHERE ai.id = o.id AND ai.advertisement_id = $1",
		advertisementID, imageIDs, coverImageID,
	)
	return err
}

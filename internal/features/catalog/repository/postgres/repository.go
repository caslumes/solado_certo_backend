package repository

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog/repository"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

const advertisementColumns = "id, seller_id, category_id, title, description, price::text, stock, status, prescription_required, " +
	"custom_made_allowed, side, size, material, rigidity, usage_indication, safety_alerts, contraindications, shipping_notes, " +
	"(SELECT ai.url FROM advertisement_images ai WHERE ai.advertisement_id = advertisements.id ORDER BY ai.is_cover DESC, ai.position LIMIT 1), " +
	"(SELECT AVG(rv.product_rating)::float8 FROM reviews rv JOIN order_items oi ON oi.id = rv.order_item_id WHERE oi.advertisement_id = advertisements.id), " +
	"(SELECT COUNT(*) FROM reviews rv JOIN order_items oi ON oi.id = rv.order_item_id WHERE oi.advertisement_id = advertisements.id), " +
	"(SELECT addr.state FROM addresses addr WHERE addr.user_id = advertisements.seller_id AND addr.is_default LIMIT 1), " +
	"created_at, updated_at, removed_at"

func scanAdvertisement(row pgx.Row) (*repository.AdvertisementModel, error) {
	var m repository.AdvertisementModel
	err := row.Scan(
		&m.ID,
		&m.SellerID,
		&m.CategoryID,
		&m.Title,
		&m.Description,
		&m.Price,
		&m.Stock,
		&m.Status,
		&m.PrescriptionRequired,
		&m.CustomMadeAllowed,
		&m.Side,
		&m.Size,
		&m.Material,
		&m.Rigidity,
		&m.UsageIndication,
		&m.SafetyAlerts,
		&m.Contraindications,
		&m.ShippingNotes,
		&m.CoverImageURL,
		&m.AverageRating,
		&m.ReviewCount,
		&m.SellerState,
		&m.CreatedAt,
		&m.UpdatedAt,
		&m.RemovedAt,
	)
	if err != nil {
		return nil, err
	}

	return &m, nil
}

func (r *postgresRepository) getImages(ctx context.Context, advertisementID string) ([]*repository.AdvertisementImageModel, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT url, COALESCE(alt_text, ''), position, is_cover FROM advertisement_images WHERE advertisement_id = $1 ORDER BY position",
		advertisementID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var images []*repository.AdvertisementImageModel
	for rows.Next() {
		image := &repository.AdvertisementImageModel{}
		if err := rows.Scan(&image.URL, &image.AltText, &image.Order, &image.IsCover); err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return images, nil
}

var orderBy = map[catalog.AdvertisementSort]string{
	catalog.AdvertisementSortRelevance: "advertisements.created_at DESC, advertisements.id DESC",
	catalog.AdvertisementSortNewest:    "advertisements.created_at DESC, advertisements.id DESC",
	catalog.AdvertisementSortPriceAsc:  "advertisements.price ASC, advertisements.created_at DESC, advertisements.id DESC",
	catalog.AdvertisementSortPriceDesc: "advertisements.price DESC, advertisements.created_at DESC, advertisements.id DESC",
}

func (r *postgresRepository) GetAdvertisements(ctx context.Context, limit uint, offset uint, filters catalog.AdvertisementFilters, sort catalog.AdvertisementSort) ([]*catalog.AdvertisementEntity, error) {
	order, ok := orderBy[sort]
	if !ok {
		return nil, fmt.Errorf("%w: invalid sort", app.ErrInvalidInput)
	}

	whereClause, args := whereStringFromFilters(filters)
	argsCount := len(args)
	rows, err := r.pool.Query(
		ctx,
		"SELECT "+advertisementColumns+" FROM advertisements "+whereClause+
			" ORDER BY "+order+" LIMIT $"+strconv.Itoa(argsCount+1)+" OFFSET $"+strconv.Itoa(argsCount+2),
		append(args, limit, offset)...,
	)
	if err != nil {
		return nil, err
	}

	var models []*repository.AdvertisementModel
	for rows.Next() {
		m, err := scanAdvertisement(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		models = append(models, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	ads := make([]*catalog.AdvertisementEntity, 0, len(models))
	for _, m := range models {
		m.Images, err = r.getImages(ctx, m.ID)
		if err != nil {
			return nil, err
		}
		ads = append(ads, m.ToEntity())
	}

	return ads, nil
}

func (r *postgresRepository) GetAdvertisementByID(ctx context.Context, id string) (*catalog.AdvertisementEntity, error) {
	m, err := scanAdvertisement(r.pool.QueryRow(ctx, "SELECT "+advertisementColumns+" FROM advertisements WHERE id = $1", id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrAdvertisementNotFound
		}

		return nil, err
	}

	m.Images, err = r.getImages(ctx, m.ID)
	if err != nil {
		return nil, err
	}

	return m.ToEntity(), nil
}

func (r *postgresRepository) CountAdvertisements(ctx context.Context, filters catalog.AdvertisementFilters) (uint, error) {
	whereClause, args := whereStringFromFilters(filters)
	var total int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM advertisements "+whereClause, args...).Scan(&total)
	if err != nil {
		return 0, err
	}

	return uint(total), nil
}

func (r *postgresRepository) GetCategories(ctx context.Context) ([]*catalog.CategoryEntity, error) {
	var categories []*catalog.CategoryEntity
	rows, err := r.pool.Query(
		ctx,
		"SELECT id, parent_category_id, name, slug, active FROM categories",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var c catalog.CategoryEntity
		err := rows.Scan(
			&c.ID,
			&c.ParentCategoryID,
			&c.Name,
			&c.Slug,
			&c.Active,
		)
		if err != nil {
			return nil, err
		}
		categories = append(categories, &c)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return categories, nil
}

func (r *postgresRepository) GetPurposes(ctx context.Context) ([]*catalog.PurposeEntity, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, code, name, description FROM podological_purposes ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	purposes := []*catalog.PurposeEntity{}
	for rows.Next() {
		var p catalog.PurposeEntity
		if err := rows.Scan(&p.ID, &p.Code, &p.Name, &p.Description); err != nil {
			return nil, err
		}
		purposes = append(purposes, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return purposes, nil
}

func (r *postgresRepository) GetConditions(ctx context.Context) ([]*catalog.ConditionEntity, error) {
	rows, err := r.pool.Query(ctx, "SELECT id, name, description FROM podological_conditions WHERE active ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	conditions := []*catalog.ConditionEntity{}
	for rows.Next() {
		var c catalog.ConditionEntity
		if err := rows.Scan(&c.ID, &c.Name, &c.Description); err != nil {
			return nil, err
		}
		conditions = append(conditions, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return conditions, nil
}

func whereStringFromFilters(filters catalog.AdvertisementFilters) (string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filters.Query != nil {
		conditions = append(conditions, "(title ILIKE $"+strconv.Itoa(argIndex)+" OR description ILIKE $"+strconv.Itoa(argIndex)+")")
		args = append(args, "%"+*filters.Query+"%")
		argIndex++
	}
	if filters.Category != nil {
		conditions = append(conditions, "category_id = $"+strconv.Itoa(argIndex))
		args = append(args, *filters.Category)
		argIndex++
	}
	if filters.PriceMin != nil {
		conditions = append(conditions, "price >= $"+strconv.Itoa(argIndex))
		args = append(args, *filters.PriceMin)
		argIndex++
	}
	if filters.PriceMax != nil {
		conditions = append(conditions, "price <= $"+strconv.Itoa(argIndex))
		args = append(args, *filters.PriceMax)
		argIndex++
	}
	if filters.PrescriptionRequired != nil {
		conditions = append(conditions, "prescription_required = $"+strconv.Itoa(argIndex))
		args = append(args, *filters.PrescriptionRequired)
		argIndex++
	}
	if filters.CustomMadeAllowed != nil {
		conditions = append(conditions, "custom_made_allowed = $"+strconv.Itoa(argIndex))
		args = append(args, *filters.CustomMadeAllowed)
		argIndex++
	}
	if filters.Side != nil {
		conditions = append(conditions, "side = $"+strconv.Itoa(argIndex))
		args = append(args, *filters.Side)
		argIndex++
	}
	if filters.Size != nil {
		conditions = append(conditions, "size = $"+strconv.Itoa(argIndex))
		args = append(args, *filters.Size)
		argIndex++
	}
	if filters.Material != nil {
		conditions = append(conditions, "material = $"+strconv.Itoa(argIndex))
		args = append(args, *filters.Material)
		argIndex++
	}
	if filters.Rigidity != nil {
		conditions = append(conditions, "rigidity = $"+strconv.Itoa(argIndex))
		args = append(args, *filters.Rigidity)
		argIndex++
	}
	if filters.UsageIndication != nil {
		conditions = append(conditions, "usage_indication = $"+strconv.Itoa(argIndex))
		args = append(args, *filters.UsageIndication)
		argIndex++
	}
	if len(filters.Purposes) > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM advertisement_purposes ap JOIN podological_purposes p ON p.id = ap.purpose_id "+
			"WHERE ap.advertisement_id = advertisements.id AND p.code = ANY($"+strconv.Itoa(argIndex)+"))")
		args = append(args, filters.Purposes)
		argIndex++
	}
	if len(filters.Conditions) > 0 {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM advertisement_conditions ac "+
			"WHERE ac.advertisement_id = advertisements.id AND ac.condition_id = ANY($"+strconv.Itoa(argIndex)+"::uuid[]))")
		args = append(args, filters.Conditions)
		argIndex++
	}
	if filters.RatingMin != nil {
		conditions = append(conditions, "(SELECT AVG(rv.product_rating) FROM reviews rv JOIN order_items oi ON oi.id = rv.order_item_id "+
			"WHERE oi.advertisement_id = advertisements.id) >= $"+strconv.Itoa(argIndex))
		args = append(args, *filters.RatingMin)
		argIndex++
	}
	if filters.State != nil {
		conditions = append(conditions, "EXISTS (SELECT 1 FROM addresses addr WHERE addr.user_id = advertisements.seller_id "+
			"AND addr.is_default AND addr.state = $"+strconv.Itoa(argIndex)+")")
		args = append(args, *filters.State)
		argIndex++
	}

	conditions = append(conditions, fmt.Sprintf("status = '%s'", catalog.AdvertisementStatusPublished))

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause, args
}

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
	if sort == catalog.AdvertisementSortRelevance && searchQuery(filters) != "" {
		order = "ts_rank(advertisements.search_vector, to_tsquery('" + textSearchConfig + "', $1)) DESC, " + order
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

func (r *postgresRepository) GetAdvertisementByID(ctx context.Context, id string) (*catalog.AdvertisementDetailEntity, error) {
	m, err := scanAdvertisement(r.pool.QueryRow(
		ctx,
		"SELECT "+advertisementColumns+" FROM advertisements WHERE id = $1 AND status = $2",
		id, catalog.AdvertisementStatusPublished,
	))
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
	ad := m.ToEntity()

	purposes, err := r.getAdvertisementPurposes(ctx, ad.ID)
	if err != nil {
		return nil, err
	}
	conditions, err := r.getAdvertisementConditions(ctx, ad.ID)
	if err != nil {
		return nil, err
	}
	seller, err := r.getSeller(ctx, ad.SellerID)
	if err != nil {
		return nil, err
	}
	seller.State = ad.SellerState
	shippingOptions, err := r.getShippingOptions(ctx, ad.ID)
	if err != nil {
		return nil, err
	}

	return &catalog.AdvertisementDetailEntity{
		Advertisement:   ad,
		Purposes:        purposes,
		Conditions:      conditions,
		Seller:          seller,
		ShippingOptions: shippingOptions,
	}, nil
}

func (r *postgresRepository) getAdvertisementPurposes(ctx context.Context, advertisementID string) ([]*catalog.PurposeEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT p.id, p.code, p.name, p.description FROM podological_purposes p "+
			"JOIN advertisement_purposes ap ON ap.purpose_id = p.id WHERE ap.advertisement_id = $1 ORDER BY p.name",
		advertisementID,
	)
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

func (r *postgresRepository) getAdvertisementConditions(ctx context.Context, advertisementID string) ([]*catalog.ConditionEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT c.id, c.name, c.description FROM podological_conditions c "+
			"JOIN advertisement_conditions ac ON ac.condition_id = c.id WHERE ac.advertisement_id = $1 AND c.active ORDER BY c.name",
		advertisementID,
	)
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

func (r *postgresRepository) getSeller(ctx context.Context, sellerID string) (*catalog.SellerEntity, error) {
	var s catalog.SellerEntity
	err := r.pool.QueryRow(
		ctx,
		"SELECT u.id, u.name, "+
			"(SELECT AVG(rv.seller_rating)::float8 FROM reviews rv JOIN order_items oi ON oi.id = rv.order_item_id WHERE oi.seller_id = u.id), "+
			"u.created_at FROM users u WHERE u.id = $1",
		sellerID,
	).Scan(&s.ID, &s.Name, &s.AverageRating, &s.MemberSince)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

func (r *postgresRepository) getShippingOptions(ctx context.Context, advertisementID string) ([]*catalog.ShippingOptionEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT id, name, carrier, price::text, delivery_days FROM shipping_options "+
			"WHERE advertisement_id = $1 AND active ORDER BY price, delivery_days, id",
		advertisementID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	options := []*catalog.ShippingOptionEntity{}
	for rows.Next() {
		var o catalog.ShippingOptionEntity
		if err := rows.Scan(&o.ID, &o.Name, &o.Carrier, &o.Price, &o.DeliveryDays); err != nil {
			return nil, err
		}
		options = append(options, &o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return options, nil
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

	if query := searchQuery(filters); query != "" {
		conditions = append(conditions, "search_vector @@ to_tsquery('"+textSearchConfig+"', $"+strconv.Itoa(argIndex)+")")
		args = append(args, query)
		argIndex++
	}
	if filters.Category != nil {
		conditions = append(conditions, `category_id IN (
			WITH RECURSIVE subtree AS (
				SELECT id FROM categories WHERE id = $`+strconv.Itoa(argIndex)+`
				UNION ALL
				SELECT c.id FROM categories c JOIN subtree s ON c.parent_category_id = s.id
			)
			SELECT id FROM subtree
		)`)
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
	if filters.Profile != nil {
		var matches []string
		if len(filters.Profile.PainPointIDs) > 0 {
			matches = append(matches, "ac.condition_id IN (SELECT ppc.condition_id FROM pain_point_conditions ppc "+
				"WHERE ppc.pain_point_id = ANY($"+strconv.Itoa(argIndex)+"::uuid[]))")
			args = append(args, filters.Profile.PainPointIDs)
			argIndex++
		}
		if filters.Profile.ClinicalCondition != nil {
			if query := anyWordQuery(*filters.Profile.ClinicalCondition); query != "" {
				matches = append(matches, "ac.condition_id IN (SELECT pc.id FROM podological_conditions pc "+
					"WHERE to_tsvector('"+textSearchConfig+"', pc.name) @@ to_tsquery('"+textSearchConfig+"', $"+strconv.Itoa(argIndex)+"))")
				args = append(args, query)
				argIndex++
			}
		}

		if len(matches) == 0 {
			conditions = append(conditions, "FALSE")
		} else {
			conditions = append(conditions, "EXISTS (SELECT 1 FROM advertisement_conditions ac "+
				"WHERE ac.advertisement_id = advertisements.id AND ("+strings.Join(matches, " OR ")+"))")
		}
	}

	conditions = append(conditions, fmt.Sprintf("status = '%s'", catalog.AdvertisementStatusPublished))

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause, args
}

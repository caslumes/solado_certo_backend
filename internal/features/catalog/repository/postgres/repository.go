package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/caslumes/solado_certo_backend/internal/features/catalog"
	"github.com/caslumes/solado_certo_backend/internal/features/catalog/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) GetAdvertisements(ctx context.Context, limit uint, offset uint, filters catalog.AdvertisementFilters) ([]*catalog.AdvertisementEntity, error) {
	var ads []*catalog.AdvertisementEntity
	whereClause, args := whereStringFromFilters(filters)
	argsCount := len(args)
	rows, err := r.pool.Query(
		ctx,
		"SELECT id, vendedor_id, titulo, descricao, preco, estoque, status, prescription_required, custom_made_allowed, "+
			"side, size, material, rigidity, usage_indication, safety_alerts, contraindications, shipping_notes, created_at, "+
			"updated_at, removed_at FROM anuncios "+whereClause+" ORDER BY created_at DESC, id DESC LIMIT $"+(strconv.Itoa(argsCount+1))+" OFFSET $"+(strconv.Itoa(argsCount+2)),
		append(args, limit, offset)...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var m repository.AdvertisementModel
		var images []*repository.AdvertisementImageModel
		err := rows.Scan(
			&m.ID,
			&m.SellerID,
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
			&m.CreatedAt,
			&m.UpdatedAt,
			&m.RemovedAt,
		)
		if err != nil {
			return nil, err
		}

		imageRows, err := r.pool.Query(
			ctx,
			"SELECT url, alt_text, ordem, is_cover FROM anuncio_imagens WHERE anuncio_id = $1 ORDER BY ordem",
			m.ID,
		)
		if err != nil {
			return nil, err
		}

		for imageRows.Next() {
			image := &repository.AdvertisementImageModel{}
			if err := imageRows.Scan(
				&image.URL,
				&image.AltText,
				&image.Order,
				&image.IsCover,
			); err != nil {
				imageRows.Close()
				return nil, err
			}
			images = append(images, image)
		}
		imageRows.Close()

		if err := imageRows.Err(); err != nil {
			return nil, err
		}

		m.Images = images
		ads = append(ads, m.ToEntity())
	}

	return ads, nil
}

func (r *postgresRepository) CountAdvertisements(ctx context.Context) (uint, error) {
	var total int64
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM anuncios").Scan(&total)
	if err != nil {
		return 0, err
	}

	return uint(total), nil
}

func (r *postgresRepository) GetCategories(ctx context.Context) ([]*catalog.CategoryEntity, error) {
	var categories []*catalog.CategoryEntity
	rows, err := r.pool.Query(
		ctx,
		"SELECT id, categoria_pai_id, nome, slug, ativo FROM categorias",
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

func whereStringFromFilters(filters catalog.AdvertisementFilters) (string, []interface{}) {
	var conditions []string
	var args []interface{}
	argIndex := 1

	if filters.Category != nil {
		conditions = append(conditions, "categoria_id = $"+strconv.Itoa(argIndex))
		args = append(args, *filters.Category)
		argIndex++
	}
	if filters.PriceMin != nil {
		conditions = append(conditions, "preco >= $"+strconv.Itoa(argIndex))
		args = append(args, *filters.PriceMin)
		argIndex++
	}
	if filters.PriceMax != nil {
		conditions = append(conditions, "preco <= $"+strconv.Itoa(argIndex))
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

	conditions = append(conditions, fmt.Sprintf("status = '%s'", catalog.AdvertisementStatusPublished))

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause, args
}

package repository

import (
	"context"

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

func (r *postgresRepository) GetAdvertisements(ctx context.Context, limit uint, cursor *catalog.AdvertisementCursor) ([]*catalog.Advertisement, error) {
	var ads []*catalog.Advertisement
	rows, err := r.pool.Query(
		ctx,
		"SELECT id, vendedor_id, titulo, descricao, preco, estoque, status, prescription_required, custom_made_allowed, " + 
		"side, size, material, rigidity, usage_indication, safety_alerts, contraindications, shipping_notes, created_at, " + 
		"updated_at, removed_at FROM anuncios ORDER BY created_at DESC LIMIT $1",
		limit,
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
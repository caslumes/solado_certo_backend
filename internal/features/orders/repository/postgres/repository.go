package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{pool: pool}
}

func (r *postgresRepository) GetAdvertisement(ctx context.Context, id string) (*orders.AdvertisementEntity, error) {
	var ad orders.AdvertisementEntity
	err := r.pool.QueryRow(
		ctx,
		"SELECT id, seller_id, price::text, stock, status FROM advertisements WHERE id = $1 AND removed_at IS NULL",
		id,
	).Scan(&ad.ID, &ad.SellerID, &ad.Price, &ad.Stock, &ad.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrAdvertisementNotFound
		}
		return nil, err
	}
	return &ad, nil
}

func (r *postgresRepository) GetAdvertisementItem(ctx context.Context, id string) (*orders.CartItemEntity, error) {
	item := orders.CartItemEntity{Quantity: 1}
	err := r.pool.QueryRow(
		ctx,
		"SELECT a.id, a.seller_id, u.name, a.title, "+
			"(SELECT ai.url FROM advertisement_images ai WHERE ai.advertisement_id = a.id ORDER BY ai.is_cover DESC, ai.position LIMIT 1), "+
			"a.price::text, a.price::text, a.stock, a.status "+
			"FROM advertisements a JOIN users u ON u.id = a.seller_id WHERE a.id = $1 AND a.removed_at IS NULL",
		id,
	).Scan(&item.AdvertisementID, &item.SellerID, &item.SellerName, &item.Title, &item.CoverImageURL,
		&item.AddedPrice, &item.Price, &item.Stock, &item.AdStatus)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrAdvertisementNotFound
		}
		return nil, err
	}
	item.ShippingOptions = []orders.ShippingOptionEntity{}
	items := []orders.CartItemEntity{item}
	if err := r.loadShippingOptions(ctx, items); err != nil {
		return nil, err
	}
	return &items[0], nil
}

func (r *postgresRepository) GetActiveCart(ctx context.Context, userID string) (*orders.CartEntity, error) {
	cart := &orders.CartEntity{Items: []orders.CartItemEntity{}}
	err := r.pool.QueryRow(ctx, "SELECT id FROM carts WHERE user_id = $1 AND status = 'active'", userID).Scan(&cart.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return cart, nil
		}
		return nil, err
	}

	rows, err := r.pool.Query(
		ctx,
		"SELECT ci.advertisement_id, a.seller_id, u.name, a.title, "+
			"(SELECT ai.url FROM advertisement_images ai WHERE ai.advertisement_id = a.id ORDER BY ai.is_cover DESC, ai.position LIMIT 1), "+
			"ci.quantity, ci.unit_price::text, a.price::text, a.stock, "+
			"CASE WHEN a.removed_at IS NULL THEN a.status ELSE 'removed' END "+
			"FROM cart_items ci JOIN advertisements a ON a.id = ci.advertisement_id JOIN users u ON u.id = a.seller_id "+
			"WHERE ci.cart_id = $1 ORDER BY u.name, a.seller_id, a.title, a.id",
		cart.ID,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var item orders.CartItemEntity
		err := rows.Scan(
			&item.AdvertisementID,
			&item.SellerID,
			&item.SellerName,
			&item.Title,
			&item.CoverImageURL,
			&item.Quantity,
			&item.AddedPrice,
			&item.Price,
			&item.Stock,
			&item.AdStatus,
		)
		if err != nil {
			rows.Close()
			return nil, err
		}
		item.ShippingOptions = []orders.ShippingOptionEntity{}
		cart.Items = append(cart.Items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := r.loadShippingOptions(ctx, cart.Items); err != nil {
		return nil, err
	}
	return cart, nil
}

func (r *postgresRepository) loadShippingOptions(ctx context.Context, items []orders.CartItemEntity) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, len(items))
	byID := make(map[string]*orders.CartItemEntity, len(items))
	for i := range items {
		ids[i] = items[i].AdvertisementID
		byID[items[i].AdvertisementID] = &items[i]
	}

	rows, err := r.pool.Query(
		ctx,
		"SELECT advertisement_id, id, name, carrier, price::text, delivery_days FROM shipping_options "+
			"WHERE advertisement_id = ANY($1::uuid[]) AND active ORDER BY price, delivery_days, id",
		ids,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var adID string
		var option orders.ShippingOptionEntity
		if err := rows.Scan(&adID, &option.ID, &option.Name, &option.Carrier, &option.Price, &option.DeliveryDays); err != nil {
			return err
		}
		byID[adID].ShippingOptions = append(byID[adID].ShippingOptions, option)
	}
	return rows.Err()
}

func (r *postgresRepository) AddCartItem(ctx context.Context, userID string, advertisementID string, quantity int, unitPrice string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	var cartID string
	err = tx.QueryRow(
		ctx,
		"INSERT INTO carts (user_id, status, created_at, updated_at) VALUES ($1, 'active', $2, $2) "+
			"ON CONFLICT (user_id) WHERE status = 'active' DO UPDATE SET updated_at = EXCLUDED.updated_at RETURNING id",
		userID, now,
	).Scan(&cartID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(
		ctx,
		"INSERT INTO cart_items (cart_id, advertisement_id, quantity, unit_price, subtotal) "+
			"VALUES ($1, $2, $3::integer, $4::numeric, $3::integer * $4::numeric) "+
			"ON CONFLICT (cart_id, advertisement_id) DO UPDATE SET quantity = cart_items.quantity + EXCLUDED.quantity, "+
			"unit_price = EXCLUDED.unit_price, subtotal = (cart_items.quantity + EXCLUDED.quantity) * EXCLUDED.unit_price",
		cartID, advertisementID, quantity, unitPrice,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *postgresRepository) SetCartItemQuantity(ctx context.Context, userID string, advertisementID string, quantity int) error {
	return r.changeCartItem(
		ctx,
		"UPDATE cart_items ci SET quantity = $3::integer, subtotal = $3::integer * ci.unit_price FROM carts c "+
			"WHERE c.id = ci.cart_id AND c.user_id = $1 AND c.status = 'active' AND ci.advertisement_id = $2 RETURNING c.id",
		userID, advertisementID, quantity,
	)
}

func (r *postgresRepository) RemoveCartItem(ctx context.Context, userID string, advertisementID string) error {
	return r.changeCartItem(
		ctx,
		"DELETE FROM cart_items ci USING carts c "+
			"WHERE c.id = ci.cart_id AND c.user_id = $1 AND c.status = 'active' AND ci.advertisement_id = $2 RETURNING c.id",
		userID, advertisementID,
	)
}

func (r *postgresRepository) changeCartItem(ctx context.Context, sql string, args ...any) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var cartID string
	if err := tx.QueryRow(ctx, sql, args...).Scan(&cartID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrCartItemNotFound
		}
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE carts SET updated_at = $2 WHERE id = $1", cartID, time.Now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/pkg/money"
	"github.com/jackc/pgx/v5"
)

func (r *postgresRepository) RefreshCartPrices(ctx context.Context, cartID string) error {
	_, err := r.pool.Exec(
		ctx,
		"UPDATE cart_items ci SET unit_price = a.price, subtotal = ci.quantity * a.price FROM advertisements a "+
			"WHERE a.id = ci.advertisement_id AND ci.cart_id = $1",
		cartID,
	)
	return err
}

func (r *postgresRepository) GetAddress(ctx context.Context, userID string, addressID string) (*orders.AddressSnapshot, error) {
	var a orders.AddressSnapshot
	err := r.pool.QueryRow(
		ctx,
		"SELECT label, receiver, zip_code, street, number, complement, district, city, state, country "+
			"FROM addresses WHERE id = $1 AND user_id = $2",
		addressID, userID,
	).Scan(&a.Label, &a.Receiver, &a.ZIPCode, &a.Street, &a.Number, &a.Complement, &a.District, &a.City, &a.State, &a.Country)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, app.ErrAddressNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *postgresRepository) GetCheckoutID(ctx context.Context, buyerID string, idempotencyKey string) (string, bool, error) {
	var id string
	err := r.pool.QueryRow(ctx, "SELECT id FROM checkouts WHERE buyer_id = $1 AND idempotency_key = $2", buyerID, idempotencyKey).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	return id, true, nil
}

func (r *postgresRepository) CreateCheckout(ctx context.Context, buyerID string, idempotencyKey string, plan *orders.CheckoutPlan) (string, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()

	// A concurrent request with the same key waits here until the first one commits, then sees its row.
	var checkoutID string
	err = tx.QueryRow(
		ctx,
		"INSERT INTO checkouts (buyer_id, idempotency_key, created_at) VALUES ($1, $2, $3) "+
			"ON CONFLICT (buyer_id, idempotency_key) DO NOTHING RETURNING id",
		buyerID, idempotencyKey, now,
	).Scan(&checkoutID)
	if errors.Is(err, pgx.ErrNoRows) {
		id, _, err := r.GetCheckoutID(ctx, buyerID, idempotencyKey)
		return id, false, err
	}
	if err != nil {
		return "", false, err
	}

	if err := lockCart(ctx, tx, buyerID, plan); err != nil {
		return "", false, err
	}
	if err := lockAndCheckAdvertisements(ctx, tx, plan); err != nil {
		return "", false, err
	}
	if err := checkShippingOptions(ctx, tx, plan); err != nil {
		return "", false, err
	}

	for _, order := range plan.Orders {
		if err := insertOrder(ctx, tx, buyerID, checkoutID, plan, order, now); err != nil {
			return "", false, err
		}
	}

	if _, err := tx.Exec(ctx, "UPDATE carts SET status = 'converted', updated_at = $2 WHERE id = $1", plan.CartID, now); err != nil {
		return "", false, err
	}
	return checkoutID, true, tx.Commit(ctx)
}

// lockCart blocks cart changes until the checkout ends and checks the cart still holds what was planned.
func lockCart(ctx context.Context, tx pgx.Tx, buyerID string, plan *orders.CheckoutPlan) error {
	var locked string
	err := tx.QueryRow(ctx, "SELECT id FROM carts WHERE id = $1 AND user_id = $2 AND status = 'active' FOR UPDATE", plan.CartID, buyerID).Scan(&locked)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return app.ErrCartChanged
		}
		return err
	}

	planned := map[string]int{}
	for _, order := range plan.Orders {
		for _, item := range order.Items {
			planned[item.AdvertisementID] = item.Quantity
		}
	}
	rows, err := tx.Query(ctx, "SELECT advertisement_id, quantity FROM cart_items WHERE cart_id = $1", plan.CartID)
	if err != nil {
		return err
	}
	defer rows.Close()
	count := 0
	changed := false
	for rows.Next() {
		var id string
		var quantity int
		if err := rows.Scan(&id, &quantity); err != nil {
			return err
		}
		count++
		changed = changed || planned[id] != quantity
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if changed || count != len(planned) {
		return app.ErrCartChanged
	}
	return nil
}

// lockAndCheckAdvertisements locks the ads in ID order, so concurrent checkouts can't deadlock,
// then checks stock and prices (RF-015) and takes the units.
func lockAndCheckAdvertisements(ctx context.Context, tx pgx.Tx, plan *orders.CheckoutPlan) error {
	items := map[string]orders.PlannedItem{}
	for _, order := range plan.Orders {
		for _, item := range order.Items {
			items[item.AdvertisementID] = item
		}
	}
	ids := slices.Sorted(maps.Keys(items))

	rows, err := tx.Query(
		ctx,
		"SELECT id, price::text, stock, CASE WHEN removed_at IS NULL THEN status ELSE 'removed' END "+
			"FROM advertisements WHERE id = ANY($1::uuid[]) ORDER BY id FOR UPDATE",
		ids,
	)
	if err != nil {
		return err
	}
	shortages := []app.StockShortage{}
	pricesChanged := false
	for rows.Next() {
		var id, price, status string
		var stock int
		if err := rows.Scan(&id, &price, &stock, &status); err != nil {
			rows.Close()
			return err
		}
		item := items[id]
		if status != orders.AdvertisementStatusPublished {
			stock = 0
		}
		if stock < item.Quantity {
			shortages = append(shortages, app.StockShortage{AdvertisementID: id, Requested: item.Quantity, Available: stock})
		} else if money.MustParse(price) != money.MustParse(item.Price) {
			pricesChanged = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(shortages) > 0 {
		return &app.OutOfStockError{Items: shortages}
	}
	if pricesChanged {
		return app.ErrPricesChanged
	}

	for _, id := range ids {
		if _, err := tx.Exec(ctx, "UPDATE advertisements SET stock = stock - $2 WHERE id = $1", id, items[id].Quantity); err != nil {
			return err
		}
	}
	return nil
}

// checkShippingOptions makes sure the chosen options are still offered at the planned price.
func checkShippingOptions(ctx context.Context, tx pgx.Tx, plan *orders.CheckoutPlan) error {
	planned := map[string]orders.PlannedItem{}
	ids := []string{}
	for _, order := range plan.Orders {
		for _, item := range order.Items {
			planned[item.ShippingOption.ID] = item
			ids = append(ids, item.ShippingOption.ID)
		}
	}

	rows, err := tx.Query(ctx, "SELECT id, advertisement_id, price::text FROM shipping_options WHERE id = ANY($1::uuid[]) AND active", ids)
	if err != nil {
		return err
	}
	found := map[string]bool{}
	pricesChanged := false
	for rows.Next() {
		var id, advertisementID, price string
		if err := rows.Scan(&id, &advertisementID, &price); err != nil {
			rows.Close()
			return err
		}
		found[id] = true
		if money.MustParse(price) != money.MustParse(planned[id].ShippingOption.Price) {
			pricesChanged = true
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	missing := []string{}
	for _, order := range plan.Orders {
		for _, item := range order.Items {
			if !found[item.ShippingOption.ID] {
				missing = append(missing, item.AdvertisementID)
			}
		}
	}
	if len(missing) > 0 {
		return &app.ShippingRequiredError{AdvertisementIDs: missing}
	}
	if pricesChanged {
		return app.ErrPricesChanged
	}
	return nil
}

func insertOrder(ctx context.Context, tx pgx.Tx, buyerID string, checkoutID string, plan *orders.CheckoutPlan, order orders.PlannedOrder, now time.Time) error {
	address, err := json.Marshal(plan.Address)
	if err != nil {
		return err
	}

	var orderID string
	number := orders.NewOrderNumber(now)
	err = tx.QueryRow(
		ctx,
		"INSERT INTO orders (buyer_id, checkout_id, shipping_address_id, shipping_address, number, status, "+
			"subtotal, shipping_total, discount_total, total, created_at, updated_at) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11) RETURNING id",
		buyerID, checkoutID, plan.AddressID, address, number, orders.OrderStatusAwaitingPayment,
		order.Subtotal.String(), order.ShippingTotal.String(), order.DiscountTotal.String(), order.Total().String(), now,
	).Scan(&orderID)
	if err != nil {
		return err
	}

	for _, item := range order.Items {
		snapshot, err := json.Marshal(orders.ProductSnapshot{
			Title:          item.Title,
			CoverImageURL:  item.CoverImageURL,
			SellerName:     item.SellerName,
			ShippingOption: item.ShippingOption.ToOutput(),
		})
		if err != nil {
			return err
		}
		_, err = tx.Exec(
			ctx,
			"INSERT INTO order_items (order_id, advertisement_id, seller_id, shipping_option_id, quantity, unit_price, subtotal, product_snapshot, created_at) "+
				"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
			orderID, item.AdvertisementID, item.SellerID, item.ShippingOption.ID, item.Quantity, item.Price, item.Subtotal().String(), snapshot, now,
		)
		if err != nil {
			return err
		}
	}

	return recordStatusChange(ctx, tx, orderID, buyerID, nil, orders.OrderStatusAwaitingPayment, "checkout", map[string]any{
		"checkout_id": checkoutID,
		"number":      number,
	}, now)
}

// recordStatusChange writes the order's history and the audit log (RNF-010). Every status change goes through it.
func recordStatusChange(ctx context.Context, tx pgx.Tx, orderID string, userID string, from *orders.OrderStatus, to orders.OrderStatus, reason string, auditContext map[string]any, now time.Time) error {
	_, err := tx.Exec(
		ctx,
		"INSERT INTO order_status_history (order_id, changed_by, previous_status, new_status, reason, created_at) VALUES ($1, $2, $3, $4, $5, $6)",
		orderID, userID, from, to, reason, now,
	)
	if err != nil {
		return err
	}

	details := map[string]any{"from": from, "to": to, "reason": reason}
	maps.Copy(details, auditContext)
	auditJSON, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO audit_logs (user_id, entity, entity_id, action, context, created_at) VALUES ($1, 'order', $2, 'order.status_changed', $3, $4)",
		userID, orderID, auditJSON, now,
	)
	return err
}

func (r *postgresRepository) GetOrdersByCheckout(ctx context.Context, checkoutID string) ([]*orders.OrderEntity, error) {
	return r.getOrders(ctx, "o.checkout_id = $1 ORDER BY o.created_at, seller_name, o.id", checkoutID)
}

// getOrders reads orders with their items; the seller comes from the items, since an order has one seller.
func (r *postgresRepository) getOrders(ctx context.Context, where string, args ...any) ([]*orders.OrderEntity, error) {
	rows, err := r.pool.Query(
		ctx,
		"SELECT o.id, o.checkout_id, o.buyer_id, s.id, s.name AS seller_name, o.number, o.status, o.shipping_address, "+
			"o.subtotal::text, o.shipping_total::text, o.discount_total::text, o.total::text, o.created_at, o.updated_at "+
			"FROM orders o JOIN users s ON s.id = (SELECT oi.seller_id FROM order_items oi WHERE oi.order_id = o.id LIMIT 1) "+
			"WHERE "+where,
		args...,
	)
	if err != nil {
		return nil, err
	}
	result := []*orders.OrderEntity{}
	byID := map[string]*orders.OrderEntity{}
	ids := []string{}
	for rows.Next() {
		var o orders.OrderEntity
		err := rows.Scan(&o.ID, &o.CheckoutID, &o.BuyerID, &o.SellerID, &o.SellerName, &o.Number, &o.Status, &o.Address,
			&o.Subtotal, &o.ShippingTotal, &o.DiscountTotal, &o.Total, &o.CreatedAt, &o.UpdatedAt)
		if err != nil {
			rows.Close()
			return nil, err
		}
		o.Items = []orders.OrderItemEntity{}
		result = append(result, &o)
		byID[o.ID] = &o
		ids = append(ids, o.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return result, nil
	}

	rows, err = r.pool.Query(
		ctx,
		"SELECT order_id, id, advertisement_id, quantity, unit_price::text, subtotal::text, product_snapshot "+
			"FROM order_items WHERE order_id = ANY($1::uuid[]) ORDER BY created_at, product_snapshot->>'title', id",
		ids,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var orderID string
		var item orders.OrderItemEntity
		if err := rows.Scan(&orderID, &item.ID, &item.AdvertisementID, &item.Quantity, &item.UnitPrice, &item.Subtotal, &item.Snapshot); err != nil {
			return nil, err
		}
		byID[orderID].Items = append(byID[orderID].Items, item)
	}
	return result, rows.Err()
}

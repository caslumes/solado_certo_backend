package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/reviews"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

const unknownID = "00000000-0000-0000-0000-000000000000"

type fixture struct {
	pool   *pgxpool.Pool
	r      *postgresRepository
	buyer  testutil.User
	ad     testutil.Ad
	order  string
	itemID string
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testutil.DB(t)
	f := fixture{pool: pool, r: NewPostgresRepository(pool), buyer: testutil.InsertUser(t, pool)}
	seller := testutil.InsertUser(t, pool)
	category := testutil.InsertCategory(t, pool, nil)
	f.ad = testutil.InsertAd(t, pool, testutil.Ad{SellerID: seller.ID, CategoryID: category.ID, Stock: 5})
	f.order, f.itemID = f.insertOrder(t, f.buyer.ID, "SC-20261020-AAAAAA")
	return f
}

// insertOrder adds a delivered order of the buyer with one item of the fixture's ad.
func (f fixture) insertOrder(t *testing.T, buyerID string, number string) (orderID string, itemID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	var checkoutID string
	if err := f.pool.QueryRow(ctx, "INSERT INTO checkouts (buyer_id, idempotency_key, created_at) VALUES ($1, $2, $3) RETURNING id", buyerID, number, now).Scan(&checkoutID); err != nil {
		t.Fatalf("insert checkout: %v", err)
	}
	err := f.pool.QueryRow(
		ctx,
		"INSERT INTO orders (buyer_id, checkout_id, number, status, subtotal, shipping_total, discount_total, total, shipping_address, created_at, updated_at) "+
			"VALUES ($1, $2, $3, 'delivered', 100.00, 0, 0, 100.00, '{}', $4, $4) RETURNING id",
		buyerID, checkoutID, number, now,
	).Scan(&orderID)
	if err != nil {
		t.Fatalf("insert order: %v", err)
	}
	err = f.pool.QueryRow(
		ctx,
		"INSERT INTO order_items (order_id, advertisement_id, seller_id, quantity, unit_price, subtotal, product_snapshot, created_at) "+
			"VALUES ($1, $2, $3, 1, 100.00, 100.00, '{}', $4) RETURNING id",
		orderID, f.ad.ID, f.ad.SellerID, now,
	).Scan(&itemID)
	if err != nil {
		t.Fatalf("insert order item: %v", err)
	}
	return orderID, itemID
}

func TestGetOrderItem(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	item, err := f.r.GetOrderItem(ctx, f.order, f.itemID)
	if err != nil || item.ID != f.itemID || item.OrderID != f.order || item.BuyerID != f.buyer.ID || item.OrderStatus != "delivered" {
		t.Errorf("GetOrderItem = %+v, %v", item, err)
	}

	// An item of another order is not found in this one, but the order still is.
	_, otherItem := f.insertOrder(t, f.buyer.ID, "SC-20261020-BBBBBB")
	item, err = f.r.GetOrderItem(ctx, f.order, otherItem)
	if err != nil || item.ID != "" || item.BuyerID != f.buyer.ID {
		t.Errorf("item of another order = %+v, %v, want the order without an item", item, err)
	}

	if _, err := f.r.GetOrderItem(ctx, unknownID, f.itemID); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("unknown order: error = %v", err)
	}
}

func TestCreateReview(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	comfort, comment := 2, "Apertou um pouco"
	review := reviews.NewReview{OrderItemID: f.itemID, ReviewerID: f.buyer.ID, ProductRating: 4, SellerRating: 5, ComfortRating: &comfort, Comment: &comment}

	created, err := f.r.CreateReview(ctx, review)

	if err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if created.ID == "" || created.ProductRating != 4 || *created.ComfortRating != 2 || created.AdaptationFeedback != nil {
		t.Errorf("created = %+v", created)
	}
	if _, err := f.r.CreateReview(ctx, review); !errors.Is(err, app.ErrAlreadyReviewed) {
		t.Errorf("second review: error = %v, want ErrAlreadyReviewed", err)
	}
}

func TestListReviews(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	other := testutil.InsertUser(t, f.pool)
	_, otherItem := f.insertOrder(t, other.ID, "SC-20261020-CCCCCC")
	feedback := "Usei por uma semana"
	if _, err := f.r.CreateReview(ctx, reviews.NewReview{OrderItemID: f.itemID, ReviewerID: f.buyer.ID, ProductRating: 5, SellerRating: 5, AdaptationFeedback: &feedback}); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if _, err := f.r.CreateReview(ctx, reviews.NewReview{OrderItemID: otherItem, ReviewerID: other.ID, ProductRating: 3, SellerRating: 4}); err != nil {
		t.Fatalf("CreateReview: %v", err)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE reviews SET created_at = created_at - interval '1 day' WHERE order_item_id = $1", f.itemID); err != nil {
		t.Fatalf("age review: %v", err)
	}

	total, err := f.r.CountReviews(ctx, f.ad.ID)
	list, listErr := f.r.GetReviews(ctx, f.ad.ID, 10, 0)

	if err != nil || listErr != nil || total != 2 || len(list) != 2 {
		t.Fatalf("reviews = %d, %+v, %v, %v", total, list, err, listErr)
	}
	// Newest first, with the reviewer's first name only.
	if list[0].ProductRating != 3 || list[0].ReviewerName != "Usuário" || list[1].ProductRating != 5 {
		t.Errorf("list = %+v, %+v", list[0], list[1])
	}
	if page, _ := f.r.GetReviews(ctx, f.ad.ID, 1, 1); len(page) != 1 || page[0].ProductRating != 5 {
		t.Errorf("second page = %+v", page)
	}
	if total, _ := f.r.CountReviews(ctx, unknownID); total != 0 {
		t.Errorf("unknown ad: total = %d", total)
	}
}

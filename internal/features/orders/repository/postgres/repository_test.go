package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

const unknownID = "00000000-0000-0000-0000-000000000000"

type fixture struct {
	r        *postgresRepository
	pool     *pgxpool.Pool
	buyer    testutil.User
	seller   testutil.User
	category testutil.Category
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	pool := testutil.DB(t)
	return fixture{
		r:        NewPostgresRepository(pool),
		pool:     pool,
		buyer:    testutil.InsertUser(t, pool),
		seller:   testutil.InsertUser(t, pool),
		category: testutil.InsertCategory(t, pool, nil),
	}
}

func (f fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("exec %q: %v", sql, err)
	}
}

func (f fixture) ad(t *testing.T, sellerID string, title string, price string, stock int) testutil.Ad {
	t.Helper()
	return testutil.InsertAd(t, f.pool, testutil.Ad{SellerID: sellerID, CategoryID: f.category.ID, Title: title, Price: price, Stock: stock})
}

func (f fixture) add(t *testing.T, adID string, quantity int, price string) {
	t.Helper()
	if err := f.r.AddCartItem(context.Background(), f.buyer.ID, adID, quantity, price); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}
}

func (f fixture) cart(t *testing.T) *orders.CartEntity {
	t.Helper()
	cart, err := f.r.GetActiveCart(context.Background(), f.buyer.ID)
	if err != nil {
		t.Fatalf("GetActiveCart: %v", err)
	}
	return cart
}

func TestGetAdvertisement(t *testing.T) {
	f := newFixture(t)
	ad := f.ad(t, f.seller.ID, "Palmilha", "89.90", 4)

	got, err := f.r.GetAdvertisement(context.Background(), ad.ID)
	if err != nil {
		t.Fatalf("GetAdvertisement: %v", err)
	}
	want := orders.AdvertisementEntity{ID: ad.ID, SellerID: f.seller.ID, Price: "89.90", Stock: 4, Status: "published"}
	if *got != want {
		t.Errorf("ad = %+v, want %+v", *got, want)
	}

	f.exec(t, "UPDATE advertisements SET status = 'removed', removed_at = now() WHERE id = $1", ad.ID)
	if _, err := f.r.GetAdvertisement(context.Background(), ad.ID); !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Errorf("removed ad: error = %v, want ErrAdvertisementNotFound", err)
	}
	if _, err := f.r.GetAdvertisement(context.Background(), unknownID); !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Errorf("unknown ad: error = %v, want ErrAdvertisementNotFound", err)
	}
}

func TestGetActiveCartWithoutCart(t *testing.T) {
	f := newFixture(t)

	cart := f.cart(t)

	if cart.ID != "" || cart.Items == nil || len(cart.Items) != 0 {
		t.Errorf("cart = %+v, want empty with no ID", cart)
	}
}

func TestAddCartItemCreatesOneCartAndAddsQuantities(t *testing.T) {
	f := newFixture(t)
	ad := f.ad(t, f.seller.ID, "Palmilha", "89.90", 10)

	f.add(t, ad.ID, 2, "89.90")
	f.exec(t, "UPDATE advertisements SET price = 79.90 WHERE id = $1", ad.ID)
	f.add(t, ad.ID, 1, "79.90")

	var carts int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM carts WHERE user_id = $1", f.buyer.ID).Scan(&carts); err != nil {
		t.Fatalf("count carts: %v", err)
	}
	if carts != 1 {
		t.Errorf("carts = %d, want 1", carts)
	}

	cart := f.cart(t)
	if len(cart.Items) != 1 {
		t.Fatalf("items = %+v, want 1", cart.Items)
	}
	item := cart.Items[0]
	if item.Quantity != 3 || item.AddedPrice != "79.90" || item.Price != "79.90" {
		t.Errorf("item = %+v, want 3 units at 79.90", item)
	}

	var subtotal string
	if err := f.pool.QueryRow(context.Background(), "SELECT subtotal::text FROM cart_items WHERE advertisement_id = $1", ad.ID).Scan(&subtotal); err != nil {
		t.Fatalf("read subtotal: %v", err)
	}
	if subtotal != "239.70" {
		t.Errorf("stored subtotal = %s, want 239.70", subtotal)
	}
}

func TestGetActiveCartReadsItems(t *testing.T) {
	f := newFixture(t)
	otherSeller := testutil.InsertUser(t, f.pool)
	insoles := f.ad(t, f.seller.ID, "Palmilha", "89.90", 10)
	socks := f.ad(t, otherSeller.ID, "Meia", "29.90", 10)
	f.exec(t, "INSERT INTO advertisement_images (advertisement_id, url, position, is_cover) VALUES ($1, '/files/public/ads/a.png', 1, false), ($1, '/files/public/ads/b.png', 2, true)", insoles.ID)
	f.exec(t, "INSERT INTO shipping_options (advertisement_id, name, price, delivery_days) VALUES ($1, 'Sedex', 30.00, 2), ($1, 'PAC', 15.00, 7)", insoles.ID)
	f.exec(t, "INSERT INTO shipping_options (advertisement_id, name, price, delivery_days, active) VALUES ($1, 'Antigo', 1.00, 1, false)", insoles.ID)
	f.add(t, insoles.ID, 1, "89.90")
	f.add(t, socks.ID, 2, "29.90")
	f.exec(t, "UPDATE advertisements SET price = 99.90, stock = 3 WHERE id = $1", insoles.ID)
	f.exec(t, "UPDATE advertisements SET status = 'removed', removed_at = now() WHERE id = $1", socks.ID)

	cart := f.cart(t)

	if cart.ID == "" || len(cart.Items) != 2 {
		t.Fatalf("cart = %+v", cart)
	}
	var insolesItem, socksItem *orders.CartItemEntity
	for i := range cart.Items {
		switch cart.Items[i].AdvertisementID {
		case insoles.ID:
			insolesItem = &cart.Items[i]
		case socks.ID:
			socksItem = &cart.Items[i]
		}
	}
	if insolesItem == nil || socksItem == nil {
		t.Fatalf("items = %+v", cart.Items)
	}

	if insolesItem.SellerID != f.seller.ID || insolesItem.SellerName != f.seller.Name || insolesItem.Title != "Palmilha" {
		t.Errorf("insoles = %+v", insolesItem)
	}
	if insolesItem.AddedPrice != "89.90" || insolesItem.Price != "99.90" || insolesItem.Stock != 3 || insolesItem.AdStatus != "published" {
		t.Errorf("insoles prices and stock = %+v", insolesItem)
	}
	if insolesItem.CoverImageURL == nil || *insolesItem.CoverImageURL != "/files/public/ads/b.png" {
		t.Errorf("cover = %v, want b.png", insolesItem.CoverImageURL)
	}
	if len(insolesItem.ShippingOptions) != 2 || insolesItem.ShippingOptions[0].Name != "PAC" || insolesItem.ShippingOptions[0].Price != "15.00" {
		t.Errorf("shipping options = %+v, want PAC then Sedex (inactive left out)", insolesItem.ShippingOptions)
	}
	if socksItem.AdStatus != "removed" || socksItem.CoverImageURL != nil || len(socksItem.ShippingOptions) != 0 {
		t.Errorf("socks = %+v, want removed without cover or options", socksItem)
	}
}

func TestGetActiveCartIgnoresConvertedCarts(t *testing.T) {
	f := newFixture(t)
	ad := f.ad(t, f.seller.ID, "Palmilha", "89.90", 10)
	f.add(t, ad.ID, 1, "89.90")
	f.exec(t, "UPDATE carts SET status = 'converted' WHERE user_id = $1", f.buyer.ID)

	if cart := f.cart(t); cart.ID != "" {
		t.Fatalf("cart = %+v, want none", cart)
	}

	f.add(t, ad.ID, 2, "89.90")
	cart := f.cart(t)
	if cart.ID == "" || len(cart.Items) != 1 || cart.Items[0].Quantity != 2 {
		t.Errorf("new cart = %+v, want a fresh cart with 2 units", cart)
	}
}

func TestSetCartItemQuantity(t *testing.T) {
	f := newFixture(t)
	ad := f.ad(t, f.seller.ID, "Palmilha", "10.50", 10)
	f.add(t, ad.ID, 1, "10.50")

	if err := f.r.SetCartItemQuantity(context.Background(), f.buyer.ID, ad.ID, 4); err != nil {
		t.Fatalf("SetCartItemQuantity: %v", err)
	}
	var quantity int
	var subtotal string
	if err := f.pool.QueryRow(context.Background(), "SELECT quantity, subtotal::text FROM cart_items WHERE advertisement_id = $1", ad.ID).Scan(&quantity, &subtotal); err != nil {
		t.Fatalf("read item: %v", err)
	}
	if quantity != 4 || subtotal != "42.00" {
		t.Errorf("quantity = %d, subtotal = %s, want 4 and 42.00", quantity, subtotal)
	}

	other := testutil.InsertUser(t, f.pool)
	if err := f.r.SetCartItemQuantity(context.Background(), other.ID, ad.ID, 2); !errors.Is(err, app.ErrCartItemNotFound) {
		t.Errorf("other buyer: error = %v, want ErrCartItemNotFound", err)
	}
	if err := f.r.SetCartItemQuantity(context.Background(), f.buyer.ID, unknownID, 2); !errors.Is(err, app.ErrCartItemNotFound) {
		t.Errorf("unknown item: error = %v, want ErrCartItemNotFound", err)
	}
}

func TestRemoveCartItem(t *testing.T) {
	f := newFixture(t)
	insoles := f.ad(t, f.seller.ID, "Palmilha", "89.90", 10)
	socks := f.ad(t, f.seller.ID, "Meia", "29.90", 10)
	f.add(t, insoles.ID, 1, "89.90")
	f.add(t, socks.ID, 1, "29.90")

	if err := f.r.RemoveCartItem(context.Background(), f.buyer.ID, insoles.ID); err != nil {
		t.Fatalf("RemoveCartItem: %v", err)
	}
	cart := f.cart(t)
	if len(cart.Items) != 1 || cart.Items[0].AdvertisementID != socks.ID {
		t.Errorf("items = %+v, want only the socks", cart.Items)
	}

	if err := f.r.RemoveCartItem(context.Background(), f.buyer.ID, insoles.ID); !errors.Is(err, app.ErrCartItemNotFound) {
		t.Errorf("second remove: error = %v, want ErrCartItemNotFound", err)
	}
}

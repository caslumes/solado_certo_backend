package postgres

import (
	"context"
	"errors"
	"regexp"
	"sync"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/testutil"
)

func (f fixture) address(t *testing.T, userID string) string {
	t.Helper()
	var id string
	err := f.pool.QueryRow(
		context.Background(),
		"INSERT INTO addresses (user_id, label, receiver, zip_code, street, number, district, city, state, created_at, updated_at) "+
			"VALUES ($1, 'Casa', 'Maria Souza', '13000000', 'Rua A', '10', 'Centro', 'Campinas', 'SP', now(), now()) RETURNING id",
		userID,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert address: %v", err)
	}
	return id
}

func (f fixture) shippingOption(t *testing.T, adID string, name string, price string) string {
	t.Helper()
	var id string
	err := f.pool.QueryRow(
		context.Background(),
		"INSERT INTO shipping_options (advertisement_id, name, carrier, price, delivery_days) VALUES ($1, $2, 'Correios', $3, 5) RETURNING id",
		adID, name, price,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insert shipping option: %v", err)
	}
	return id
}

func (f fixture) count(t *testing.T, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func (f fixture) planFor(t *testing.T, buyerID string, addressID string) *orders.CheckoutPlan {
	t.Helper()
	cart, err := f.r.GetActiveCart(context.Background(), buyerID)
	if err != nil {
		t.Fatalf("GetActiveCart: %v", err)
	}
	shipping := []orders.ShippingSelection{}
	for _, item := range cart.Items {
		shipping = append(shipping, orders.ShippingSelection{AdvertisementID: item.AdvertisementID, ShippingOptionID: item.ShippingOptions[0].ID})
	}
	planned, err := orders.PlanOrders(cart, shipping)
	if err != nil {
		t.Fatalf("PlanOrders: %v", err)
	}
	address, err := f.r.GetAddress(context.Background(), buyerID, addressID)
	if err != nil {
		t.Fatalf("GetAddress: %v", err)
	}
	return &orders.CheckoutPlan{CartID: cart.ID, AddressID: addressID, Address: *address, Orders: planned}
}

type checkoutSetup struct {
	fixture
	addressID   string
	insoles     testutil.Ad
	socks       testutil.Ad
	otherSeller testutil.User
}

func newCheckoutSetup(t *testing.T) checkoutSetup {
	t.Helper()
	f := newFixture(t)
	s := checkoutSetup{fixture: f, addressID: f.address(t, f.buyer.ID), otherSeller: testutil.InsertUser(t, f.pool)}
	s.insoles = f.ad(t, f.seller.ID, "Palmilha", "89.90", 5)
	s.socks = f.ad(t, s.otherSeller.ID, "Meia", "10.00", 10)
	f.shippingOption(t, s.insoles.ID, "PAC", "19.90")
	f.shippingOption(t, s.socks.ID, "Carta", "8.00")
	f.add(t, s.insoles.ID, 2, "89.90")
	f.add(t, s.socks.ID, 3, "10.00")
	return s
}

func TestCreateCheckout(t *testing.T) {
	s := newCheckoutSetup(t)
	if _, err := s.pool.Exec(context.Background(), "UPDATE advertisements SET prescription_required = true WHERE id = $1", s.socks.ID); err != nil {
		t.Fatal(err)
	}
	plan := s.planFor(t, s.buyer.ID, s.addressID)

	id, created, err := s.r.CreateCheckout(context.Background(), s.buyer.ID, "key-1", plan)
	if err != nil {
		t.Fatalf("CreateCheckout: %v", err)
	}
	if !created || id == "" {
		t.Fatalf("id = %q, created = %v", id, created)
	}

	placed, err := s.r.GetOrdersByCheckout(context.Background(), id)
	if err != nil {
		t.Fatalf("GetOrdersByCheckout: %v", err)
	}
	if len(placed) != 2 {
		t.Fatalf("orders = %d, want 2 (one per seller)", len(placed))
	}
	number := regexp.MustCompile(`^SC-\d{8}-[0-9A-Z]{6}$`)
	totals := map[string]string{}
	for _, order := range placed {
		if !number.MatchString(order.Number) || order.Status != orders.OrderStatusAwaitingPayment || order.CheckoutID != id || order.BuyerID != s.buyer.ID {
			t.Errorf("order = %+v", order)
		}
		if order.Address.Receiver != "Maria Souza" || order.Address.City != "Campinas" || order.Address.Label == nil || *order.Address.Label != "Casa" {
			t.Errorf("address = %+v", order.Address)
		}
		if len(order.Items) != 1 {
			t.Fatalf("items = %+v", order.Items)
		}
		needs := order.SellerID == s.otherSeller.ID
		if order.Items[0].PrescriptionRequired != needs || (len(order.MissingPrescriptions()) == 1) != needs {
			t.Errorf("seller %s: prescription required = %v, missing = %v", order.SellerID, order.Items[0].PrescriptionRequired, order.MissingPrescriptions())
		}
		totals[order.SellerID] = order.Subtotal + " + " + order.ShippingTotal + " - " + order.DiscountTotal + " = " + order.Total
	}
	if totals[s.seller.ID] != "179.80 + 19.90 - 0.00 = 199.70" || totals[s.otherSeller.ID] != "30.00 + 8.00 - 0.00 = 38.00" {
		t.Errorf("totals = %v", totals)
	}

	insolesOrder := placed[0]
	if insolesOrder.SellerID != s.seller.ID {
		insolesOrder = placed[1]
	}
	item := insolesOrder.Items[0]
	if item.AdvertisementID != s.insoles.ID || item.Quantity != 2 || item.UnitPrice != "89.90" || item.Subtotal != "179.80" ||
		item.Snapshot.Title != "Palmilha" || item.Snapshot.SellerName != s.seller.Name || item.Snapshot.ShippingOption.Name != "PAC" ||
		item.Snapshot.ShippingOption.Price != "19.90" || insolesOrder.SellerName != s.seller.Name {
		t.Errorf("insoles item = %+v", item)
	}

	if n := s.count(t, "SELECT stock FROM advertisements WHERE id = $1", s.insoles.ID); n != 3 {
		t.Errorf("insoles stock = %d, want 3", n)
	}
	if n := s.count(t, "SELECT stock FROM advertisements WHERE id = $1", s.socks.ID); n != 7 {
		t.Errorf("socks stock = %d, want 7", n)
	}
	if n := s.count(t, "SELECT count(*) FROM carts WHERE user_id = $1 AND status = 'converted'", s.buyer.ID); n != 1 {
		t.Errorf("converted carts = %d, want 1", n)
	}
	if n := s.count(t, "SELECT count(*) FROM order_items WHERE shipping_option_id IS NOT NULL"); n != 2 {
		t.Errorf("items with shipping option = %d, want 2", n)
	}
	if n := s.count(t, "SELECT count(*) FROM order_status_history WHERE previous_status IS NULL AND new_status = 'awaiting_payment' AND changed_by = $1 AND reason = 'checkout'", s.buyer.ID); n != 2 {
		t.Errorf("history rows = %d, want 2", n)
	}
	if n := s.count(t, "SELECT count(*) FROM audit_logs WHERE entity = 'order' AND action = 'order.status_changed' AND user_id = $1 AND context->>'checkout_id' = $2 AND context->>'to' = 'awaiting_payment'", s.buyer.ID, id); n != 2 {
		t.Errorf("audit rows = %d, want 2", n)
	}
	if cart := s.cart(t); cart.ID != "" {
		t.Errorf("active cart = %+v, want none", cart)
	}
}

func TestCreateCheckoutWithAUsedKey(t *testing.T) {
	s := newCheckoutSetup(t)
	plan := s.planFor(t, s.buyer.ID, s.addressID)
	first, _, err := s.r.CreateCheckout(context.Background(), s.buyer.ID, "key-1", plan)
	if err != nil {
		t.Fatalf("CreateCheckout: %v", err)
	}

	second, created, err := s.r.CreateCheckout(context.Background(), s.buyer.ID, "key-1", plan)
	if err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if created || second != first {
		t.Errorf("repeat: id = %q (first %q), created = %v", second, first, created)
	}
	if n := s.count(t, "SELECT count(*) FROM orders"); n != 2 {
		t.Errorf("orders = %d, want 2", n)
	}

	id, found, err := s.r.GetCheckoutID(context.Background(), s.buyer.ID, "key-1")
	if err != nil || !found || id != first {
		t.Errorf("GetCheckoutID = %q, %v, %v", id, found, err)
	}
	if _, found, _ := s.r.GetCheckoutID(context.Background(), s.otherSeller.ID, "key-1"); found {
		t.Error("another buyer's key was found")
	}
}

func TestConcurrentCheckoutsWithTheSameKey(t *testing.T) {
	s := newCheckoutSetup(t)
	plan := s.planFor(t, s.buyer.ID, s.addressID)

	type result struct {
		id      string
		created bool
		err     error
	}
	results := make([]result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() {
			<-start
			id, created, err := s.r.CreateCheckout(context.Background(), s.buyer.ID, "key-1", plan)
			results[i] = result{id, created, err}
		})
	}
	close(start)
	wg.Wait()

	for _, r := range results {
		if r.err != nil {
			t.Fatalf("CreateCheckout: %v", r.err)
		}
	}
	if results[0].id != results[1].id || results[0].created == results[1].created {
		t.Errorf("results = %+v, want the same checkout, created once", results)
	}
	if n := s.count(t, "SELECT count(*) FROM orders"); n != 2 {
		t.Errorf("orders = %d, want 2", n)
	}
	if n := s.count(t, "SELECT stock FROM advertisements WHERE id = $1", s.insoles.ID); n != 3 {
		t.Errorf("stock = %d, want 3 (taken once)", n)
	}
}

func TestConcurrentCheckoutsOfTheLastUnit(t *testing.T) {
	f := newFixture(t)
	ad := f.ad(t, f.seller.ID, "Palmilha", "89.90", 1)
	f.shippingOption(t, ad.ID, "PAC", "19.90")
	buyers := []testutil.User{f.buyer, testutil.InsertUser(t, f.pool)}
	plans := make([]*orders.CheckoutPlan, len(buyers))
	for i, buyer := range buyers {
		if err := f.r.AddCartItem(context.Background(), buyer.ID, ad.ID, 1, "89.90"); err != nil {
			t.Fatalf("AddCartItem: %v", err)
		}
		plans[i] = f.planFor(t, buyer.ID, f.address(t, buyer.ID))
	}

	errs := make([]error, len(buyers))
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, buyer := range buyers {
		wg.Go(func() {
			<-start
			_, _, errs[i] = f.r.CreateCheckout(context.Background(), buyer.ID, "key-"+buyer.ID, plans[i])
		})
	}
	close(start)
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		var outOfStock *app.OutOfStockError
		switch {
		case err == nil:
			succeeded++
		case errors.As(err, &outOfStock):
			if len(outOfStock.Items) != 1 || outOfStock.Items[0] != (app.StockShortage{AdvertisementID: ad.ID, Requested: 1, Available: 0}) {
				t.Errorf("shortage = %+v", outOfStock.Items)
			}
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("succeeded = %d, want exactly 1 (errors %v)", succeeded, errs)
	}
	if n := f.count(t, "SELECT stock FROM advertisements WHERE id = $1", ad.ID); n != 0 {
		t.Errorf("stock = %d, want 0", n)
	}
	if n := f.count(t, "SELECT count(*) FROM checkouts"); n != 1 {
		t.Errorf("checkouts = %d, want 1 (the failed one is rolled back)", n)
	}
	if n := f.count(t, "SELECT count(*) FROM carts WHERE status = 'active'"); n != 1 {
		t.Errorf("active carts = %d, want 1 (the loser keeps their cart)", n)
	}
}

func TestCreateCheckoutRechecksInsideTheTransaction(t *testing.T) {
	tests := []struct {
		name   string
		change func(s checkoutSetup)
		want   error
	}{
		{"price changed", func(s checkoutSetup) {
			s.exec(t, "UPDATE advertisements SET price = 99.90 WHERE id = $1", s.insoles.ID)
		}, app.ErrPricesChanged},
		{"stock dropped", func(s checkoutSetup) {
			s.exec(t, "UPDATE advertisements SET stock = 1 WHERE id = $1", s.insoles.ID)
		}, app.ErrOutOfStock},
		{"ad paused", func(s checkoutSetup) {
			s.exec(t, "UPDATE advertisements SET status = 'paused' WHERE id = $1", s.socks.ID)
		}, app.ErrOutOfStock},
		{"shipping option removed", func(s checkoutSetup) {
			s.exec(t, "UPDATE shipping_options SET active = false WHERE advertisement_id = $1", s.socks.ID)
		}, app.ErrShippingRequired},
		{"shipping price changed", func(s checkoutSetup) {
			s.exec(t, "UPDATE shipping_options SET price = 25.00 WHERE advertisement_id = $1", s.insoles.ID)
		}, app.ErrPricesChanged},
		{"quantity changed", func(s checkoutSetup) {
			s.exec(t, "UPDATE cart_items SET quantity = 1 WHERE advertisement_id = $1", s.insoles.ID)
		}, app.ErrCartChanged},
		{"item removed", func(s checkoutSetup) {
			s.exec(t, "DELETE FROM cart_items WHERE advertisement_id = $1", s.socks.ID)
		}, app.ErrCartChanged},
		{"cart converted", func(s checkoutSetup) {
			s.exec(t, "UPDATE carts SET status = 'converted'")
		}, app.ErrCartChanged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newCheckoutSetup(t)
			plan := s.planFor(t, s.buyer.ID, s.addressID)
			tt.change(s)

			_, _, err := s.r.CreateCheckout(context.Background(), s.buyer.ID, "key-1", plan)

			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
			if n := s.count(t, "SELECT count(*) FROM checkouts") + s.count(t, "SELECT count(*) FROM orders"); n != 0 {
				t.Errorf("rows left behind = %d", n)
			}
			if n := s.count(t, "SELECT stock FROM advertisements WHERE id = $1", s.socks.ID); n != 10 {
				t.Errorf("socks stock = %d, want 10 (untouched)", n)
			}
		})
	}
}

func TestDeletingTheAddressKeepsTheOrder(t *testing.T) {
	s := newCheckoutSetup(t)
	id, _, err := s.r.CreateCheckout(context.Background(), s.buyer.ID, "key-1", s.planFor(t, s.buyer.ID, s.addressID))
	if err != nil {
		t.Fatalf("CreateCheckout: %v", err)
	}

	s.exec(t, "DELETE FROM addresses WHERE id = $1", s.addressID)

	created, err := s.r.GetOrdersByCheckout(context.Background(), id)
	if err != nil {
		t.Fatalf("GetOrdersByCheckout: %v", err)
	}
	if len(created) != 2 || created[0].Address.Street != "Rua A" {
		t.Errorf("orders = %+v", created)
	}
	if n := s.count(t, "SELECT count(*) FROM orders WHERE shipping_address_id IS NULL"); n != 2 {
		t.Errorf("orders without address link = %d, want 2", n)
	}
}

func TestGetAddressIsScopedToTheUser(t *testing.T) {
	s := newCheckoutSetup(t)

	if _, err := s.r.GetAddress(context.Background(), s.seller.ID, s.addressID); !errors.Is(err, app.ErrAddressNotFound) {
		t.Errorf("another user's address: error = %v, want ErrAddressNotFound", err)
	}
	if _, err := s.r.GetAddress(context.Background(), s.buyer.ID, unknownID); !errors.Is(err, app.ErrAddressNotFound) {
		t.Errorf("unknown address: error = %v, want ErrAddressNotFound", err)
	}
}

func TestGetOrdersByUnknownCheckout(t *testing.T) {
	f := newFixture(t)
	found, err := f.r.GetOrdersByCheckout(context.Background(), unknownID)
	if err != nil || found == nil || len(found) != 0 {
		t.Errorf("orders = %v, error = %v, want an empty list", found, err)
	}
}

func TestRefreshCartPrices(t *testing.T) {
	s := newCheckoutSetup(t)
	s.exec(t, "UPDATE advertisements SET price = 79.90 WHERE id = $1", s.insoles.ID)

	if err := s.r.RefreshCartPrices(context.Background(), s.cart(t).ID); err != nil {
		t.Fatalf("RefreshCartPrices: %v", err)
	}

	for _, item := range s.cart(t).Items {
		if item.PriceChanged() {
			t.Errorf("item %s still has the old price: %+v", item.Title, item)
		}
	}
	var subtotal string
	if err := s.pool.QueryRow(context.Background(), "SELECT subtotal::text FROM cart_items WHERE advertisement_id = $1", s.insoles.ID).Scan(&subtotal); err != nil {
		t.Fatalf("read subtotal: %v", err)
	}
	if subtotal != "159.80" {
		t.Errorf("subtotal = %s, want 159.80", subtotal)
	}
}

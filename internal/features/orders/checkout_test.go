package orders

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const otherSellerAdID = "00000000-0000-0000-0000-000000000701"

func withOption(i CartItemEntity, options ...ShippingOptionEntity) CartItemEntity {
	i.ShippingOptions = options
	return i
}

func option(id string, price string) ShippingOptionEntity {
	return ShippingOptionEntity{ID: id, Name: id, Price: price, DeliveryDays: 5}
}

func TestPlanOrdersSplitsBySellerAndChargesTheHighestShipping(t *testing.T) {
	cart := &CartEntity{ID: "cart-1", Items: []CartItemEntity{
		withOption(item("a1", "s1", 2, "10.50", 9), option("a1-pac", "15.00"), option("a1-sedex", "30.00")),
		withOption(item("a2", "s1", 1, "20.00", 9), option("a2-pac", "12.00")),
		withOption(item("a3", "s2", 3, "5.00", 9), option("a3-letter", "8.00")),
	}}

	orders, err := PlanOrders(cart, []ShippingSelection{
		{AdvertisementID: "a1", ShippingOptionID: "a1-pac"},
		{AdvertisementID: "a2", ShippingOptionID: "a2-pac"},
		{AdvertisementID: "a3", ShippingOptionID: "a3-letter"},
	})
	if err != nil {
		t.Fatalf("PlanOrders: %v", err)
	}

	if len(orders) != 2 || orders[0].SellerID != "s1" || orders[1].SellerID != "s2" {
		t.Fatalf("orders = %+v, want s1 then s2", orders)
	}
	first := orders[0]
	if len(first.Items) != 2 || first.Subtotal.String() != "41.00" || first.ShippingTotal.String() != "15.00" ||
		first.DiscountTotal != 0 || first.Total().String() != "56.00" {
		t.Errorf("s1 order: items %d, subtotal %s, shipping %s, total %s; want 2, 41.00, 15.00 (highest option), 56.00",
			len(first.Items), first.Subtotal, first.ShippingTotal, first.Total())
	}
	if first.Items[0].ShippingOption.ID != "a1-pac" {
		t.Errorf("a1 option = %+v", first.Items[0].ShippingOption)
	}
	second := orders[1]
	if second.Subtotal.String() != "15.00" || second.ShippingTotal.String() != "8.00" || second.Total().String() != "23.00" {
		t.Errorf("s2 order: subtotal %s, shipping %s, total %s", second.Subtotal, second.ShippingTotal, second.Total())
	}

	plan := &CheckoutPlan{Orders: orders}
	output := plan.ToOutput()
	if output.Subtotal != "56.00" || output.ShippingTotal != "23.00" || output.DiscountTotal != "0.00" || output.Total != "79.00" {
		t.Errorf("preview totals = %+v", output)
	}
	if output.Orders[0].Items[0].Subtotal != "21.00" || output.Orders[0].Items[0].ShippingOption.Price != "15.00" {
		t.Errorf("preview item = %+v", output.Orders[0].Items[0])
	}
}

func TestPlanOrdersErrors(t *testing.T) {
	ok := withOption(item("a1", "s1", 1, "10.00", 9), option("a1-pac", "15.00"))
	short := withOption(item("a2", "s1", 3, "10.00", 2), option("a2-pac", "15.00"))
	paused := withOption(item("a3", "s1", 1, "10.00", 9), option("a3-pac", "15.00"))
	paused.AdStatus = "paused"
	pac := ShippingSelection{AdvertisementID: "a1", ShippingOptionID: "a1-pac"}

	tests := []struct {
		name     string
		items    []CartItemEntity
		shipping []ShippingSelection
		want     error
	}{
		{"empty cart", nil, nil, app.ErrCartEmpty},
		{"stock problem", []CartItemEntity{ok, short}, []ShippingSelection{pac}, app.ErrOutOfStock},
		{"no shipping", []CartItemEntity{ok}, nil, app.ErrShippingRequired},
		{"option of another ad", []CartItemEntity{ok}, []ShippingSelection{{AdvertisementID: "a1", ShippingOptionID: "a2-pac"}}, app.ErrShippingRequired},
		{"item not in cart", []CartItemEntity{ok}, []ShippingSelection{pac, {AdvertisementID: "zz", ShippingOptionID: "x"}}, app.ErrInvalidInput},
		{"item twice", []CartItemEntity{ok}, []ShippingSelection{pac, pac}, app.ErrInvalidInput},
	}
	for _, tt := range tests {
		_, err := PlanOrders(&CartEntity{ID: "cart-1", Items: tt.items}, tt.shipping)
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: error = %v, want %v", tt.name, err, tt.want)
		}
	}

	_, err := PlanOrders(&CartEntity{ID: "cart-1", Items: []CartItemEntity{ok, short, paused}}, []ShippingSelection{pac})
	var outOfStock *app.OutOfStockError
	if !errors.As(err, &outOfStock) {
		t.Fatalf("error = %v, want OutOfStockError", err)
	}
	want := []app.StockShortage{{AdvertisementID: "a2", Requested: 3, Available: 2}, {AdvertisementID: "a3", Requested: 1, Available: 0}}
	if !slices.Equal(outOfStock.Items, want) {
		t.Errorf("shortages = %+v, want %+v (a paused ad has none available)", outOfStock.Items, want)
	}

	_, err = PlanOrders(&CartEntity{ID: "cart-1", Items: []CartItemEntity{ok, withOption(item("a4", "s2", 1, "10.00", 9), option("a4-pac", "1.00"))}}, nil)
	var shippingRequired *app.ShippingRequiredError
	if !errors.As(err, &shippingRequired) || !slices.Equal(shippingRequired.AdvertisementIDs, []string{"a1", "a4"}) {
		t.Errorf("error = %v, want both items listed", err)
	}
}

func TestNewOrderNumber(t *testing.T) {
	now := time.Date(2026, 10, 19, 15, 0, 0, 0, time.UTC)
	pattern := regexp.MustCompile(`^SC-20261019-[0-9A-HJKMNP-TV-Z]{6}$`)
	seen := map[string]bool{}
	for range 50 {
		number := NewOrderNumber(now)
		if !pattern.MatchString(number) {
			t.Fatalf("number %q does not match %s", number, pattern)
		}
		seen[number] = true
	}
	if len(seen) < 45 {
		t.Errorf("only %d distinct numbers out of 50", len(seen))
	}

	lateEvening := time.Date(2026, 10, 20, 2, 30, 0, 0, time.UTC)
	if number := NewOrderNumber(lateEvening); number[:11] != "SC-20261019" {
		t.Errorf("number at 23:30 in Brasília = %q, want the 19th", number)
	}
}

func checkoutFixture(t *testing.T) (*fakeRepository, *service, CheckoutInput) {
	t.Helper()
	r := newFakeRepository()
	r.ads[otherSellerAdID] = &AdvertisementEntity{ID: otherSellerAdID, SellerID: "seller-2", Price: "15.00", Stock: 3, Status: AdvertisementStatusPublished}
	r.options[otherSellerAdID] = []ShippingOptionEntity{{ID: "motoboy", Name: "Motoboy", Price: "12.00", DeliveryDays: 1}}
	s := NewService(r)
	for _, add := range []struct {
		adID     string
		quantity int
	}{{insolesAdID, 2}, {otherSellerAdID, 1}} {
		if _, err := s.AddCartItem(context.Background(), testBuyerID, add.adID, add.quantity); err != nil {
			t.Fatalf("AddCartItem: %v", err)
		}
	}
	input := CheckoutInput{AddressID: testAddressID, Shipping: []ShippingSelection{
		{AdvertisementID: insolesAdID, ShippingOptionID: "sedex"},
		{AdvertisementID: otherSellerAdID, ShippingOptionID: "motoboy"},
	}}
	return r, s, input
}

func TestPreviewCheckout(t *testing.T) {
	r, s, input := checkoutFixture(t)

	plan, err := s.PreviewCheckout(context.Background(), testBuyerID, input)
	if err != nil {
		t.Fatalf("PreviewCheckout: %v", err)
	}
	if plan.CartID != "cart-"+testBuyerID || plan.AddressID != testAddressID || plan.Address.City != "Campinas" || len(plan.Orders) != 2 {
		t.Errorf("plan = %+v", plan)
	}
	if len(r.plans) != 0 || len(r.carts[testBuyerID]) != 2 {
		t.Error("preview changed something")
	}
}

func TestPreviewCheckoutRejectsAnotherUsersAddress(t *testing.T) {
	_, s, input := checkoutFixture(t)

	if _, err := s.PreviewCheckout(context.Background(), "buyer-2", input); !errors.Is(err, app.ErrAddressNotFound) {
		t.Errorf("error = %v, want ErrAddressNotFound", err)
	}
	input.AddressID = "not-a-uuid"
	if _, err := s.PreviewCheckout(context.Background(), testBuyerID, input); !errors.Is(err, app.ErrAddressNotFound) {
		t.Errorf("bad ID: error = %v, want ErrAddressNotFound", err)
	}
}

func TestCheckoutCreatesOneOrderPerSeller(t *testing.T) {
	_, s, input := checkoutFixture(t)

	checkout, created, err := s.Checkout(context.Background(), testBuyerID, "key-1", input)
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if !created || checkout.ID == "" || len(checkout.Orders) != 2 {
		t.Fatalf("checkout = %+v, created = %v", checkout, created)
	}
	totals := []string{checkout.Orders[0].Total, checkout.Orders[1].Total}
	if !slices.Equal(totals, []string{"214.70", "27.00"}) {
		t.Errorf("totals = %v, want 214.70 (2 × 89.90 + 34.90) and 27.00 (15.00 + 12.00)", totals)
	}
}

func TestCheckoutRepeatsWithTheSameKey(t *testing.T) {
	r, s, input := checkoutFixture(t)
	first, _, err := s.Checkout(context.Background(), testBuyerID, "key-1", input)
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}

	second, created, err := s.Checkout(context.Background(), testBuyerID, "key-1", input)
	if err != nil {
		t.Fatalf("repeat: %v", err)
	}
	if created || second.ID != first.ID || len(second.Orders) != 2 || len(r.plans) != 1 {
		t.Errorf("repeat: checkout %s (first %s), created %v, plans %d", second.ID, first.ID, created, len(r.plans))
	}
}

func TestCheckoutReturnsTheConcurrentRequestsOrders(t *testing.T) {
	r, s, input := checkoutFixture(t)
	r.concurrentCheckoutID = "checkout-other"
	r.orders["checkout-other"] = []*OrderEntity{{ID: "order-1"}}

	checkout, created, err := s.Checkout(context.Background(), testBuyerID, "key-1", input)
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if created || checkout.ID != "checkout-other" || len(checkout.Orders) != 1 {
		t.Errorf("checkout = %+v, created = %v", checkout, created)
	}
}

func TestCheckoutRequiresAKey(t *testing.T) {
	_, s, input := checkoutFixture(t)
	for _, key := range []string{"", string(make([]byte, 121))} {
		if _, _, err := s.Checkout(context.Background(), testBuyerID, key, input); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("key of %d bytes: error = %v, want ErrInvalidInput", len(key), err)
		}
	}
}

func TestCheckoutWithChangedPriceRefreshesTheCart(t *testing.T) {
	r, s, input := checkoutFixture(t)
	r.ads[insolesAdID].Price = "99.90"

	_, _, err := s.Checkout(context.Background(), testBuyerID, "key-1", input)
	if !errors.Is(err, app.ErrPricesChanged) {
		t.Fatalf("error = %v, want ErrPricesChanged", err)
	}
	if len(r.plans) != 0 || !slices.Equal(r.refreshedCarts, []string{"cart-" + testBuyerID}) {
		t.Fatalf("plans = %d, refreshed = %v", len(r.plans), r.refreshedCarts)
	}

	checkout, created, err := s.Checkout(context.Background(), testBuyerID, "key-1", input)
	if err != nil || !created {
		t.Fatalf("confirming again: created = %v, error = %v", created, err)
	}
	if checkout.Orders[0].Total != "234.70" {
		t.Errorf("total = %s, want 234.70 at the new price", checkout.Orders[0].Total)
	}
}

func TestCheckoutWhenTheLockedAdsChanged(t *testing.T) {
	r, s, input := checkoutFixture(t)
	r.createErr = app.ErrPricesChanged

	if _, _, err := s.Checkout(context.Background(), testBuyerID, "key-1", input); !errors.Is(err, app.ErrPricesChanged) {
		t.Fatalf("error = %v, want ErrPricesChanged", err)
	}
	if len(r.refreshedCarts) != 1 {
		t.Errorf("refreshed = %v, want the cart refreshed", r.refreshedCarts)
	}

	for _, err := range []error{&app.OutOfStockError{}, app.ErrCartChanged, &app.ShippingRequiredError{}, errors.New("db down")} {
		r.createErr = err
		if _, _, got := s.Checkout(context.Background(), testBuyerID, "key-1", input); !errors.Is(got, err) {
			t.Errorf("error = %v, want %v", got, err)
		}
	}
}

func TestCheckoutStopsOnPlanErrors(t *testing.T) {
	r, s, input := checkoutFixture(t)
	r.ads[insolesAdID].Stock = 1

	_, _, err := s.Checkout(context.Background(), testBuyerID, "key-1", input)
	if !errors.Is(err, app.ErrOutOfStock) {
		t.Errorf("error = %v, want ErrOutOfStock", err)
	}
	if len(r.plans) != 0 {
		t.Error("checkout created orders")
	}
}

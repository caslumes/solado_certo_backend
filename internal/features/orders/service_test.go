package orders

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const (
	testBuyerID   = "buyer-1"
	testSellerID  = "seller-1"
	insolesAdID   = "00000000-0000-0000-0000-000000000601"
	socksAdID     = "00000000-0000-0000-0000-000000000602"
	pausedAdID    = "00000000-0000-0000-0000-000000000603"
	unknownID     = "00000000-0000-0000-0000-000000000000"
	testAddressID = "00000000-0000-0000-0000-000000000901"
)

type fakeRepository struct {
	ads       map[string]*AdvertisementEntity
	options   map[string][]ShippingOptionEntity
	carts     map[string][]CartItemEntity
	addresses map[string]AddressSnapshot
	checkouts map[string]string
	orders    map[string][]*OrderEntity
	err       error
	// createErr is returned by CreateCheckout, as if the locked ads no longer matched the plan.
	createErr error
	// concurrentCheckoutID simulates another request with the same key committing first.
	concurrentCheckoutID string
	plans                []*CheckoutPlan
	refreshedCarts       []string
	placed               []*OrderEntity
	history              map[string][]StatusChangeEntity
	changes              []StatusChange
	// changeErr is returned by ChangeOrderStatus, as if another request changed the order first.
	changeErr error
	shipments map[string]*ShipmentEntity
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		ads: map[string]*AdvertisementEntity{
			insolesAdID: {ID: insolesAdID, SellerID: testSellerID, Price: "89.90", Stock: 5, Status: AdvertisementStatusPublished},
			socksAdID:   {ID: socksAdID, SellerID: testSellerID, Price: "29.90", Stock: 0, Status: AdvertisementStatusPublished},
			pausedAdID:  {ID: pausedAdID, SellerID: testSellerID, Price: "50.00", Stock: 5, Status: "paused"},
		},
		options: map[string][]ShippingOptionEntity{
			insolesAdID: {{ID: "pac", Name: "PAC", Price: "19.90", DeliveryDays: 7}, {ID: "sedex", Name: "SEDEX", Price: "34.90", DeliveryDays: 2}},
			socksAdID:   {{ID: "letter", Name: "Carta", Price: "9.90", DeliveryDays: 10}},
			pausedAdID:  {{ID: "pac-paused", Name: "PAC", Price: "19.90", DeliveryDays: 7}},
		},
		carts:     map[string][]CartItemEntity{},
		addresses: map[string]AddressSnapshot{testAddressID: {Receiver: "Maria", ZIPCode: "13000000", Street: "Rua A", Number: "10", District: "Centro", City: "Campinas", State: "SP", Country: "BR"}},
		checkouts: map[string]string{},
		orders:    map[string][]*OrderEntity{},
	}
}

func (f *fakeRepository) GetActiveCart(_ context.Context, userID string) (*CartEntity, error) {
	if f.err != nil {
		return nil, f.err
	}
	items, ok := f.carts[userID]
	if !ok {
		return &CartEntity{Items: []CartItemEntity{}}, nil
	}
	cart := &CartEntity{ID: "cart-" + userID, Items: []CartItemEntity{}}
	for _, item := range items {
		ad := f.ads[item.AdvertisementID]
		item.Price = ad.Price
		item.Stock = ad.Stock
		item.AdStatus = ad.Status
		item.SellerName = "Loja " + item.SellerID
		item.ShippingOptions = f.options[item.AdvertisementID]
		cart.Items = append(cart.Items, item)
	}
	return cart, nil
}

func (f *fakeRepository) GetAdvertisement(_ context.Context, id string) (*AdvertisementEntity, error) {
	ad, ok := f.ads[id]
	if !ok {
		return nil, app.ErrAdvertisementNotFound
	}
	copied := *ad
	return &copied, nil
}

func (f *fakeRepository) AddCartItem(_ context.Context, userID string, advertisementID string, quantity int, unitPrice string) error {
	items := f.carts[userID]
	index := slices.IndexFunc(items, func(i CartItemEntity) bool { return i.AdvertisementID == advertisementID })
	if index >= 0 {
		items[index].Quantity += quantity
		items[index].AddedPrice = unitPrice
	} else {
		items = append(items, CartItemEntity{AdvertisementID: advertisementID, SellerID: f.ads[advertisementID].SellerID, Quantity: quantity, AddedPrice: unitPrice})
	}
	f.carts[userID] = items
	return nil
}

func (f *fakeRepository) SetCartItemQuantity(_ context.Context, userID string, advertisementID string, quantity int) error {
	items := f.carts[userID]
	index := slices.IndexFunc(items, func(i CartItemEntity) bool { return i.AdvertisementID == advertisementID })
	if index < 0 {
		return app.ErrCartItemNotFound
	}
	items[index].Quantity = quantity
	return nil
}

func (f *fakeRepository) RemoveCartItem(_ context.Context, userID string, advertisementID string) error {
	items := f.carts[userID]
	index := slices.IndexFunc(items, func(i CartItemEntity) bool { return i.AdvertisementID == advertisementID })
	if index < 0 {
		return app.ErrCartItemNotFound
	}
	f.carts[userID] = slices.Delete(items, index, index+1)
	return nil
}

func (f *fakeRepository) quantity(userID string, advertisementID string) int {
	for _, item := range f.carts[userID] {
		if item.AdvertisementID == advertisementID {
			return item.Quantity
		}
	}
	return 0
}

func TestGetCartWithoutCart(t *testing.T) {
	cart, err := NewService(newFakeRepository()).GetCart(context.Background(), testBuyerID)
	if err != nil {
		t.Fatalf("GetCart: %v", err)
	}
	if cart.ID != "" || len(cart.Items) != 0 {
		t.Errorf("cart = %+v, want empty", cart)
	}
}

func TestGetCartPropagatesErrors(t *testing.T) {
	r := newFakeRepository()
	r.err = errors.New("db down")
	if _, err := NewService(r).GetCart(context.Background(), testBuyerID); err == nil {
		t.Error("want error")
	}
}

func TestAddCartItem(t *testing.T) {
	r := newFakeRepository()
	s := NewService(r)

	cart, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 2)
	if err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 2 || cart.Items[0].AddedPrice != "89.90" {
		t.Fatalf("cart = %+v", cart)
	}

	if _, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 3); err != nil {
		t.Fatalf("second AddCartItem: %v", err)
	}
	if got := r.quantity(testBuyerID, insolesAdID); got != 5 {
		t.Errorf("quantity = %d, want 5 (added to the existing one)", got)
	}
}

func TestAddCartItemUpdatesAddedPrice(t *testing.T) {
	r := newFakeRepository()
	s := NewService(r)
	if _, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 1); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}
	r.ads[insolesAdID].Price = "79.90"

	cart, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 1)
	if err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}
	if cart.Items[0].AddedPrice != "79.90" || cart.Items[0].PriceChanged() {
		t.Errorf("item = %+v, want the new price as added price", cart.Items[0])
	}
}

func TestAddCartItemOverStock(t *testing.T) {
	r := newFakeRepository()
	s := NewService(r)
	if _, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 4); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}

	_, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 2)
	var outOfStock *app.OutOfStockError
	if !errors.As(err, &outOfStock) {
		t.Fatalf("error = %v, want OutOfStockError", err)
	}
	want := app.StockShortage{AdvertisementID: insolesAdID, Requested: 6, Available: 5}
	if len(outOfStock.Items) != 1 || outOfStock.Items[0] != want {
		t.Errorf("items = %+v, want %+v", outOfStock.Items, want)
	}
	if got := r.quantity(testBuyerID, insolesAdID); got != 4 {
		t.Errorf("quantity = %d, want 4 (unchanged)", got)
	}
}

func TestAddCartItemRejects(t *testing.T) {
	tests := []struct {
		name     string
		userID   string
		adID     string
		quantity int
		want     error
	}{
		{"zero quantity", testBuyerID, insolesAdID, 0, app.ErrInvalidInput},
		{"over the limit", testBuyerID, insolesAdID, MaxItemQuantity + 1, app.ErrInvalidInput},
		{"bad ID", testBuyerID, "not-a-uuid", 1, app.ErrAdvertisementNotFound},
		{"unknown ad", testBuyerID, unknownID, 1, app.ErrAdvertisementNotFound},
		{"paused ad", testBuyerID, pausedAdID, 1, app.ErrAdvertisementNotFound},
		{"sold out", testBuyerID, socksAdID, 1, app.ErrOutOfStock},
		{"own ad", testSellerID, insolesAdID, 1, app.ErrOwnAdvertisement},
	}
	for _, tt := range tests {
		r := newFakeRepository()
		_, err := NewService(r).AddCartItem(context.Background(), tt.userID, tt.adID, tt.quantity)
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: error = %v, want %v", tt.name, err, tt.want)
		}
		if len(r.carts) != 0 {
			t.Errorf("%s: cart changed", tt.name)
		}
	}
}

func TestChangeCartItemQuantity(t *testing.T) {
	r := newFakeRepository()
	s := NewService(r)
	if _, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 1); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}

	cart, err := s.ChangeCartItemQuantity(context.Background(), testBuyerID, insolesAdID, 5)
	if err != nil {
		t.Fatalf("ChangeCartItemQuantity: %v", err)
	}
	if cart.Items[0].Quantity != 5 {
		t.Errorf("quantity = %d, want 5", cart.Items[0].Quantity)
	}

	if _, err := s.ChangeCartItemQuantity(context.Background(), testBuyerID, insolesAdID, 6); !errors.Is(err, app.ErrOutOfStock) {
		t.Errorf("over stock: error = %v, want ErrOutOfStock", err)
	}
}

func TestChangeCartItemQuantityCanLowerAnUnavailableItem(t *testing.T) {
	r := newFakeRepository()
	s := NewService(r)
	if _, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 4); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}
	r.ads[insolesAdID].Stock = 2
	r.ads[insolesAdID].Status = "paused"

	if _, err := s.ChangeCartItemQuantity(context.Background(), testBuyerID, insolesAdID, 2); err != nil {
		t.Fatalf("lowering: %v", err)
	}
	if _, err := s.ChangeCartItemQuantity(context.Background(), testBuyerID, insolesAdID, 3); !errors.Is(err, app.ErrAdvertisementNotFound) {
		t.Errorf("raising a paused item: error = %v, want ErrAdvertisementNotFound", err)
	}
}

func TestChangeCartItemQuantityZeroRemoves(t *testing.T) {
	r := newFakeRepository()
	s := NewService(r)
	if _, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 1); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}

	cart, err := s.ChangeCartItemQuantity(context.Background(), testBuyerID, insolesAdID, 0)
	if err != nil {
		t.Fatalf("ChangeCartItemQuantity: %v", err)
	}
	if len(cart.Items) != 0 {
		t.Errorf("items = %+v, want none", cart.Items)
	}
}

func TestChangeCartItemQuantityRejects(t *testing.T) {
	r := newFakeRepository()
	s := NewService(r)
	if _, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 1); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}

	tests := []struct {
		name     string
		adID     string
		quantity int
		want     error
	}{
		{"negative", insolesAdID, -1, app.ErrInvalidInput},
		{"over the limit", insolesAdID, MaxItemQuantity + 1, app.ErrInvalidInput},
		{"bad ID", "not-a-uuid", 1, app.ErrCartItemNotFound},
		{"not in cart", socksAdID, 1, app.ErrCartItemNotFound},
	}
	for _, tt := range tests {
		if _, err := s.ChangeCartItemQuantity(context.Background(), testBuyerID, tt.adID, tt.quantity); !errors.Is(err, tt.want) {
			t.Errorf("%s: error = %v, want %v", tt.name, err, tt.want)
		}
	}
	if got := r.quantity(testBuyerID, insolesAdID); got != 1 {
		t.Errorf("quantity = %d, want 1 (unchanged)", got)
	}
}

func TestRemoveCartItem(t *testing.T) {
	r := newFakeRepository()
	s := NewService(r)
	if _, err := s.AddCartItem(context.Background(), testBuyerID, insolesAdID, 1); err != nil {
		t.Fatalf("AddCartItem: %v", err)
	}

	cart, err := s.RemoveCartItem(context.Background(), testBuyerID, insolesAdID)
	if err != nil {
		t.Fatalf("RemoveCartItem: %v", err)
	}
	if len(cart.Items) != 0 {
		t.Errorf("items = %+v, want none", cart.Items)
	}

	if _, err := s.RemoveCartItem(context.Background(), testBuyerID, insolesAdID); !errors.Is(err, app.ErrCartItemNotFound) {
		t.Errorf("second remove: error = %v, want ErrCartItemNotFound", err)
	}
	if _, err := s.RemoveCartItem(context.Background(), testBuyerID, "not-a-uuid"); !errors.Is(err, app.ErrCartItemNotFound) {
		t.Errorf("bad ID: error = %v, want ErrCartItemNotFound", err)
	}
}

func (f *fakeRepository) RefreshCartPrices(_ context.Context, cartID string) error {
	f.refreshedCarts = append(f.refreshedCarts, cartID)
	for userID, items := range f.carts {
		if "cart-"+userID != cartID {
			continue
		}
		for i := range items {
			items[i].AddedPrice = f.ads[items[i].AdvertisementID].Price
		}
	}
	return nil
}

func (f *fakeRepository) GetAddress(_ context.Context, userID string, addressID string) (*AddressSnapshot, error) {
	address, ok := f.addresses[addressID]
	if !ok || userID != testBuyerID {
		return nil, app.ErrAddressNotFound
	}
	return &address, nil
}

func (f *fakeRepository) GetCheckoutID(_ context.Context, buyerID string, key string) (string, bool, error) {
	id, ok := f.checkouts[buyerID+"/"+key]
	return id, ok, nil
}

func (f *fakeRepository) CreateCheckout(_ context.Context, buyerID string, key string, plan *CheckoutPlan) (string, bool, error) {
	if f.createErr != nil {
		return "", false, f.createErr
	}
	if f.concurrentCheckoutID != "" {
		return f.concurrentCheckoutID, false, nil
	}
	f.plans = append(f.plans, plan)
	id := fmt.Sprintf("checkout-%d", len(f.plans))
	f.checkouts[buyerID+"/"+key] = id
	for _, order := range plan.Orders {
		f.orders[id] = append(f.orders[id], &OrderEntity{CheckoutID: id, SellerID: order.SellerID, Status: OrderStatusAwaitingPayment, Total: order.Total().String()})
	}
	delete(f.carts, buyerID)
	return id, true, nil
}

func (f *fakeRepository) GetOrdersByCheckout(_ context.Context, checkoutID string) ([]*OrderEntity, error) {
	return f.orders[checkoutID], nil
}

// includes tells whether the query keeps an order with the status and the date.
func includes(query OrderQuery, status OrderStatus, at time.Time) bool {
	return (query.Status == "" || status == query.Status) &&
		(query.Since == nil || !at.Before(*query.Since)) &&
		(query.Until == nil || at.Before(*query.Until))
}

func (f *fakeRepository) buyerOrders(buyerID string, query OrderQuery) []*OrderEntity {
	mine := []*OrderEntity{}
	for _, order := range f.placed {
		if order.BuyerID == buyerID && includes(query, order.Status, order.CreatedAt) {
			mine = append(mine, order)
		}
	}
	return mine
}

func (f *fakeRepository) CountOrdersByBuyer(_ context.Context, buyerID string, query OrderQuery) (uint, error) {
	return uint(len(f.buyerOrders(buyerID, query))), f.err
}

func (f *fakeRepository) GetOrdersByBuyer(_ context.Context, buyerID string, query OrderQuery, limit uint, offset uint) ([]*OrderEntity, error) {
	if f.err != nil {
		return nil, f.err
	}
	mine := f.buyerOrders(buyerID, query)
	start := min(int(offset), len(mine))
	end := min(start+int(limit), len(mine))
	return mine[start:end], nil
}

func (f *fakeRepository) GetOrder(_ context.Context, id string) (*OrderEntity, error) {
	if f.err != nil {
		return nil, f.err
	}
	for _, order := range f.placed {
		if order.ID == id {
			copied := *order
			return &copied, nil
		}
	}
	return nil, app.ErrOrderNotFound
}

func (f *fakeRepository) GetStatusHistory(_ context.Context, orderID string) ([]StatusChangeEntity, error) {
	return f.history[orderID], nil
}

func (f *fakeRepository) ChangeOrderStatus(_ context.Context, change StatusChange) error {
	if f.changeErr != nil {
		return f.changeErr
	}
	f.changes = append(f.changes, change)
	for _, order := range f.placed {
		if order.ID == change.OrderID {
			order.Status = change.To
		}
	}
	if change.Shipment != nil {
		if f.shipments == nil {
			f.shipments = map[string]*ShipmentEntity{}
		}
		f.shipments[change.OrderID] = &ShipmentEntity{Carrier: change.Shipment.Carrier, TrackingCode: change.Shipment.TrackingCode, Status: "posted"}
	}
	from := change.From
	reason := change.Reason
	if f.history == nil {
		f.history = map[string][]StatusChangeEntity{}
	}
	f.history[change.OrderID] = append(f.history[change.OrderID], StatusChangeEntity{From: &from, To: change.To, Reason: &reason})
	return nil
}

func (f *fakeRepository) sellerOrders(sellerID string, query OrderQuery) []*OrderEntity {
	mine := []*OrderEntity{}
	for _, order := range f.placed {
		if order.SellerID == sellerID && order.PaidAt != nil && includes(query, order.Status, *order.PaidAt) {
			mine = append(mine, order)
		}
	}
	return mine
}

func (f *fakeRepository) CountOrdersBySeller(_ context.Context, sellerID string, query OrderQuery) (uint, error) {
	return uint(len(f.sellerOrders(sellerID, query))), f.err
}

func (f *fakeRepository) GetOrdersBySeller(_ context.Context, sellerID string, query OrderQuery, limit uint, offset uint) ([]*OrderEntity, error) {
	if f.err != nil {
		return nil, f.err
	}
	mine := f.sellerOrders(sellerID, query)
	start := min(int(offset), len(mine))
	end := min(start+int(limit), len(mine))
	return mine[start:end], nil
}

func (f *fakeRepository) GetShipment(_ context.Context, orderID string) (*ShipmentEntity, error) {
	return f.shipments[orderID], nil
}

package orders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type GetCartUseCase interface {
	GetCart(ctx context.Context, userID string) (*CartEntity, error)
}

type AddCartItemUseCase interface {
	AddCartItem(ctx context.Context, userID string, advertisementID string, quantity int) (*CartEntity, error)
}

type ChangeCartItemQuantityUseCase interface {
	ChangeCartItemQuantity(ctx context.Context, userID string, advertisementID string, quantity int) (*CartEntity, error)
}

type RemoveCartItemUseCase interface {
	RemoveCartItem(ctx context.Context, userID string, advertisementID string) (*CartEntity, error)
}

type PreviewCheckoutUseCase interface {
	PreviewCheckout(ctx context.Context, userID string, input CheckoutInput) (*CheckoutPlan, error)
}

type CheckoutUseCase interface {
	Checkout(ctx context.Context, userID string, idempotencyKey string, input CheckoutInput) (checkout *CheckoutEntity, created bool, err error)
}

type repository interface {
	GetActiveCart(ctx context.Context, userID string) (*CartEntity, error)
	GetAdvertisement(ctx context.Context, id string) (*AdvertisementEntity, error)
	AddCartItem(ctx context.Context, userID string, advertisementID string, quantity int, unitPrice string) error
	SetCartItemQuantity(ctx context.Context, userID string, advertisementID string, quantity int) error
	RemoveCartItem(ctx context.Context, userID string, advertisementID string) error
	RefreshCartPrices(ctx context.Context, cartID string) error
	GetAddress(ctx context.Context, userID string, addressID string) (*AddressSnapshot, error)
	GetCheckoutID(ctx context.Context, buyerID string, idempotencyKey string) (id string, found bool, err error)
	CreateCheckout(ctx context.Context, buyerID string, idempotencyKey string, plan *CheckoutPlan) (id string, created bool, err error)
	GetOrdersByCheckout(ctx context.Context, checkoutID string) ([]*OrderEntity, error)
}

const (
	MaxItemQuantity         = 99
	maxIdempotencyKeyLength = 120
)

type service struct {
	r repository
}

func NewService(r repository) *service {
	return &service{r: r}
}

func isExpected(err error, expected ...error) bool {
	for _, target := range expected {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (s *service) GetCart(ctx context.Context, userID string) (*CartEntity, error) {
	cart, err := s.r.GetActiveCart(ctx, userID)
	if err != nil {
		slog.ErrorContext(ctx, "GetCart failed", "user_id", userID, "error", err)
		return nil, err
	}
	return cart, nil
}

// AddCartItem adds to the quantity already in the cart. Stock is only checked here as a hint;
// checkout is what reserves it (RF-015).
func (s *service) AddCartItem(ctx context.Context, userID string, advertisementID string, quantity int) (*CartEntity, error) {
	if quantity < 1 || quantity > MaxItemQuantity {
		return nil, fmt.Errorf("%w: quantity must be between 1 and %d", app.ErrInvalidInput, MaxItemQuantity)
	}
	ad, err := s.publishedAdvertisement(ctx, advertisementID)
	if err != nil {
		return nil, err
	}
	if ad.SellerID == userID {
		return nil, app.ErrOwnAdvertisement
	}

	cart, err := s.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	total := quantity
	if item := cart.Item(advertisementID); item != nil {
		total += item.Quantity
	}
	if total > MaxItemQuantity {
		return nil, fmt.Errorf("%w: quantity must be between 1 and %d", app.ErrInvalidInput, MaxItemQuantity)
	}
	if total > ad.Stock {
		return nil, &app.OutOfStockError{Items: []app.StockShortage{{AdvertisementID: ad.ID, Requested: total, Available: ad.Stock}}}
	}

	if err := s.r.AddCartItem(ctx, userID, advertisementID, quantity, ad.Price); err != nil {
		slog.ErrorContext(ctx, "AddCartItem failed", "user_id", userID, "advertisement_id", advertisementID, "error", err)
		return nil, err
	}
	return s.GetCart(ctx, userID)
}

// ChangeCartItemQuantity sets the quantity; 0 removes the item. Lowering it is always allowed,
// so the buyer can fix an item whose stock went down.
func (s *service) ChangeCartItemQuantity(ctx context.Context, userID string, advertisementID string, quantity int) (*CartEntity, error) {
	if quantity == 0 {
		return s.RemoveCartItem(ctx, userID, advertisementID)
	}
	if quantity < 0 || quantity > MaxItemQuantity {
		return nil, fmt.Errorf("%w: quantity must be between 0 and %d", app.ErrInvalidInput, MaxItemQuantity)
	}
	if !helpers.IsUUID(advertisementID) {
		return nil, app.ErrCartItemNotFound
	}

	cart, err := s.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	item := cart.Item(advertisementID)
	if item == nil {
		return nil, app.ErrCartItemNotFound
	}
	if quantity > item.Quantity {
		if item.AdStatus != AdvertisementStatusPublished {
			return nil, app.ErrAdvertisementNotFound
		}
		if quantity > item.Stock {
			return nil, &app.OutOfStockError{Items: []app.StockShortage{{AdvertisementID: advertisementID, Requested: quantity, Available: item.Stock}}}
		}
	}

	if err := s.r.SetCartItemQuantity(ctx, userID, advertisementID, quantity); err != nil {
		if !errors.Is(err, app.ErrCartItemNotFound) {
			slog.ErrorContext(ctx, "ChangeCartItemQuantity failed", "user_id", userID, "advertisement_id", advertisementID, "error", err)
		}
		return nil, err
	}
	return s.GetCart(ctx, userID)
}

func (s *service) RemoveCartItem(ctx context.Context, userID string, advertisementID string) (*CartEntity, error) {
	if !helpers.IsUUID(advertisementID) {
		return nil, app.ErrCartItemNotFound
	}
	if err := s.r.RemoveCartItem(ctx, userID, advertisementID); err != nil {
		if !errors.Is(err, app.ErrCartItemNotFound) {
			slog.ErrorContext(ctx, "RemoveCartItem failed", "user_id", userID, "advertisement_id", advertisementID, "error", err)
		}
		return nil, err
	}
	return s.GetCart(ctx, userID)
}

func (s *service) publishedAdvertisement(ctx context.Context, id string) (*AdvertisementEntity, error) {
	if !helpers.IsUUID(id) {
		return nil, app.ErrAdvertisementNotFound
	}
	ad, err := s.r.GetAdvertisement(ctx, id)
	if err != nil {
		if !errors.Is(err, app.ErrAdvertisementNotFound) {
			slog.ErrorContext(ctx, "GetAdvertisement failed", "advertisement_id", id, "error", err)
		}
		return nil, err
	}
	if !ad.Published() {
		return nil, app.ErrAdvertisementNotFound
	}
	return ad, nil
}

// PreviewCheckout shows the orders a checkout would create, without changing anything.
func (s *service) PreviewCheckout(ctx context.Context, userID string, input CheckoutInput) (*CheckoutPlan, error) {
	return s.plan(ctx, userID, input)
}

// Checkout creates one order per seller, awaiting payment, and reserves their stock (RF-014, RF-015, RF-019).
// Repeating a key returns the orders it created (RNF-012). If a price changed since the item was added,
// the cart takes the new prices and the buyer has to confirm again.
func (s *service) Checkout(ctx context.Context, userID string, idempotencyKey string, input CheckoutInput) (*CheckoutEntity, bool, error) {
	if idempotencyKey == "" || len(idempotencyKey) > maxIdempotencyKeyLength {
		return nil, false, fmt.Errorf("%w: the Idempotency-Key header must have 1 to %d characters", app.ErrInvalidInput, maxIdempotencyKeyLength)
	}

	id, found, err := s.r.GetCheckoutID(ctx, userID, idempotencyKey)
	if err != nil {
		slog.ErrorContext(ctx, "GetCheckoutID failed", "user_id", userID, "error", err)
		return nil, false, err
	}
	if found {
		checkout, err := s.checkout(ctx, id)
		return checkout, false, err
	}

	plan, err := s.plan(ctx, userID, input)
	if err != nil {
		return nil, false, err
	}
	for _, order := range plan.Orders {
		for _, item := range order.Items {
			if item.PriceChanged() {
				return nil, false, s.refreshPrices(ctx, plan.CartID)
			}
		}
	}

	id, created, err := s.r.CreateCheckout(ctx, userID, idempotencyKey, plan)
	if err != nil {
		if errors.Is(err, app.ErrPricesChanged) {
			return nil, false, s.refreshPrices(ctx, plan.CartID)
		}
		if !isExpected(err, app.ErrOutOfStock, app.ErrShippingRequired, app.ErrCartChanged) {
			slog.ErrorContext(ctx, "CreateCheckout failed", "user_id", userID, "error", err)
		}
		return nil, false, err
	}
	checkout, err := s.checkout(ctx, id)
	return checkout, created, err
}

func (s *service) plan(ctx context.Context, userID string, input CheckoutInput) (*CheckoutPlan, error) {
	if !helpers.IsUUID(input.AddressID) {
		return nil, app.ErrAddressNotFound
	}
	address, err := s.r.GetAddress(ctx, userID, input.AddressID)
	if err != nil {
		if !errors.Is(err, app.ErrAddressNotFound) {
			slog.ErrorContext(ctx, "GetAddress failed", "user_id", userID, "error", err)
		}
		return nil, err
	}

	cart, err := s.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	orders, err := PlanOrders(cart, input.Shipping)
	if err != nil {
		return nil, err
	}
	return &CheckoutPlan{CartID: cart.ID, AddressID: input.AddressID, Address: *address, Orders: orders}, nil
}

func (s *service) refreshPrices(ctx context.Context, cartID string) error {
	if err := s.r.RefreshCartPrices(ctx, cartID); err != nil {
		slog.ErrorContext(ctx, "RefreshCartPrices failed", "cart_id", cartID, "error", err)
		return err
	}
	return app.ErrPricesChanged
}

func (s *service) checkout(ctx context.Context, id string) (*CheckoutEntity, error) {
	orders, err := s.r.GetOrdersByCheckout(ctx, id)
	if err != nil {
		slog.ErrorContext(ctx, "GetOrdersByCheckout failed", "checkout_id", id, "error", err)
		return nil, err
	}
	return &CheckoutEntity{ID: id, Orders: orders}, nil
}

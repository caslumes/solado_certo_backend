package orders

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type PlanCustomOrderUseCase interface {
	PlanCustomOrder(ctx context.Context, buyerID string, item CustomItem) (*CheckoutPlan, error)
}

type CustomItem struct {
	RequestID        string
	AdvertisementID  string
	AddressID        string
	ShippingOptionID string
	Price            string
}

func (s *service) PlanCustomOrder(ctx context.Context, buyerID string, item CustomItem) (*CheckoutPlan, error) {
	if !helpers.IsUUID(item.AddressID) {
		return nil, app.ErrAddressNotFound
	}
	address, err := s.r.GetAddress(ctx, buyerID, item.AddressID)
	if err != nil {
		if !errors.Is(err, app.ErrAddressNotFound) {
			slog.ErrorContext(ctx, "GetAddress failed", "user_id", buyerID, "error", err)
		}
		return nil, err
	}

	ad, err := s.r.GetAdvertisementItem(ctx, item.AdvertisementID)
	if err != nil {
		if !errors.Is(err, app.ErrAdvertisementNotFound) {
			slog.ErrorContext(ctx, "GetAdvertisementItem failed", "advertisement_id", item.AdvertisementID, "error", err)
		}
		return nil, err
	}
	index := slices.IndexFunc(ad.ShippingOptions, func(o ShippingOptionEntity) bool { return o.ID == item.ShippingOptionID })
	if index < 0 {
		return nil, &app.ShippingRequiredError{AdvertisementIDs: []string{ad.AdvertisementID}}
	}

	ad.Quantity = 1
	ad.Price, ad.AddedPrice = item.Price, item.Price
	planned := PlannedItem{CartItemEntity: *ad, ShippingOption: ad.ShippingOptions[index], CustomOrderRequestID: &item.RequestID}
	order := PlannedOrder{
		SellerID:      ad.SellerID,
		SellerName:    ad.SellerName,
		Items:         []PlannedItem{planned},
		Subtotal:      planned.Subtotal(),
		ShippingTotal: helpers.MustParseCents(planned.ShippingOption.Price),
	}
	return &CheckoutPlan{AddressID: item.AddressID, Address: *address, Orders: []PlannedOrder{order}}, nil
}

func (o *OrderEntity) PendingFittingReports() []string {
	pending := []string{}
	for _, item := range o.Items {
		if item.fittingReportPending() {
			pending = append(pending, item.ID)
		}
	}
	return pending
}

package orders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type ListSellerOrdersUseCase interface {
	ListSellerOrders(ctx context.Context, sellerID string, status OrderStatus, page uint, limit uint) (*OrderPage, error)
}

type GetSellerOrderUseCase interface {
	GetSellerOrder(ctx context.Context, sellerID string, id string) (*OrderDetail, error)
}

type StartProcessingUseCase interface {
	StartProcessing(ctx context.Context, sellerID string, id string) (*OrderDetail, error)
}

type ShipOrderUseCase interface {
	ShipOrder(ctx context.Context, sellerID string, id string, input ShipmentInput) (*OrderDetail, error)
}

type ConfirmDeliveryUseCase interface {
	ConfirmDelivery(ctx context.Context, buyerID string, id string) (*OrderDetail, error)
}

// ListSellerOrders returns the seller's paid orders, most recently paid first, optionally with one
// status (RF-020, RF-025). Orders not paid yet are left out: there is nothing to do with them.
func (s *service) ListSellerOrders(ctx context.Context, sellerID string, status OrderStatus, page uint, limit uint) (*OrderPage, error) {
	if status != "" && !slices.Contains(sellerStatuses, status) {
		return nil, fmt.Errorf("%w: status must be one of paid, processing, shipped, delivered, cancelled, returned", app.ErrInvalidInput)
	}
	if page == 0 {
		page = 1
	}
	if limit == 0 {
		limit = defaultOrdersPerPage
	}
	if limit > maxOrdersPerPage {
		return nil, fmt.Errorf("%w: limit must be at most %d", app.ErrInvalidInput, maxOrdersPerPage)
	}

	total, err := s.r.CountOrdersBySeller(ctx, sellerID, status)
	if err != nil {
		slog.ErrorContext(ctx, "CountOrdersBySeller failed", "user_id", sellerID, "error", err)
		return nil, err
	}
	orders, err := s.r.GetOrdersBySeller(ctx, sellerID, status, limit, (page-1)*limit)
	if err != nil {
		slog.ErrorContext(ctx, "GetOrdersBySeller failed", "user_id", sellerID, "error", err)
		return nil, err
	}
	return &OrderPage{Items: orders, CurrentPage: page, TotalPages: (total + limit - 1) / limit, TotalItems: total}, nil
}

func (s *service) GetSellerOrder(ctx context.Context, sellerID string, id string) (*OrderDetail, error) {
	order, err := s.sellerOrder(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}
	return s.detail(ctx, order)
}

// StartProcessing is the seller acknowledging a paid order; from then on the buyer can no longer
// cancel it.
func (s *service) StartProcessing(ctx context.Context, sellerID string, id string) (*OrderDetail, error) {
	order, err := s.sellerOrder(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}
	notification := notifications.OrderProcessing(order.BuyerID, order.ID, order.Number)
	err = s.changeStatus(ctx, StatusChange{
		OrderID:      order.ID,
		ChangedBy:    sellerID,
		From:         order.Status,
		To:           OrderStatusProcessing,
		Reason:       ReasonSellerProcessing,
		Notification: &notification,
	})
	if err != nil {
		return nil, err
	}
	return s.GetSellerOrder(ctx, sellerID, id)
}

// ShipOrder records the carrier and tracking code of an order being prepared (RF-021, UC-009).
func (s *service) ShipOrder(ctx context.Context, sellerID string, id string, input ShipmentInput) (*OrderDetail, error) {
	shipment, err := ValidateShipment(input)
	if err != nil {
		return nil, err
	}
	order, err := s.sellerOrder(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}
	notification := notifications.OrderShipped(order.BuyerID, order.ID, order.Number, shipment.Carrier, shipment.TrackingCode)
	err = s.changeStatus(ctx, StatusChange{
		OrderID:      order.ID,
		ChangedBy:    sellerID,
		From:         order.Status,
		To:           OrderStatusShipped,
		Reason:       ReasonSellerShipped,
		Shipment:     &shipment,
		Notification: &notification,
	})
	if err != nil {
		return nil, err
	}
	return s.GetSellerOrder(ctx, sellerID, id)
}

// ConfirmDelivery is the buyer saying the order arrived (RF-023); it can then be reviewed.
func (s *service) ConfirmDelivery(ctx context.Context, buyerID string, id string) (*OrderDetail, error) {
	order, err := s.ownedOrder(ctx, buyerID, id)
	if err != nil {
		return nil, err
	}
	notification := notifications.OrderDelivered(order.SellerID, order.ID, order.Number)
	err = s.changeStatus(ctx, StatusChange{
		OrderID:      order.ID,
		ChangedBy:    buyerID,
		From:         order.Status,
		To:           OrderStatusDelivered,
		Reason:       ReasonBuyerConfirmedDelivery,
		Notification: &notification,
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, buyerID, id)
}

// sellerOrder answers 404 for another seller's order, and for one not paid yet.
func (s *service) sellerOrder(ctx context.Context, sellerID string, id string) (*OrderEntity, error) {
	if !helpers.IsUUID(id) {
		return nil, app.ErrOrderNotFound
	}
	order, err := s.r.GetOrder(ctx, id)
	if err != nil {
		if !errors.Is(err, app.ErrOrderNotFound) {
			slog.ErrorContext(ctx, "GetOrder failed", "order_id", id, "error", err)
		}
		return nil, err
	}
	if order.SellerID != sellerID || order.PaidAt == nil {
		return nil, app.ErrOrderNotFound
	}
	return order, nil
}

package orders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type ListSellerOrdersUseCase interface {
	ListSellerOrders(ctx context.Context, sellerID string, filter HistoryFilter, page uint, limit uint) (*OrderPage, error)
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

func (s *service) ListSellerOrders(ctx context.Context, sellerID string, filter HistoryFilter, page uint, limit uint) (*OrderPage, error) {
	query, err := filter.query(sellerStatuses)
	if err != nil {
		return nil, err
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

	total, err := s.r.CountOrdersBySeller(ctx, sellerID, query)
	if err != nil {
		slog.ErrorContext(ctx, "CountOrdersBySeller failed", "user_id", sellerID, "error", err)
		return nil, err
	}
	orders, err := s.r.GetOrdersBySeller(ctx, sellerID, query, limit, (page-1)*limit)
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

func (s *service) StartProcessing(ctx context.Context, sellerID string, id string) (*OrderDetail, error) {
	order, err := s.sellerOrder(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}
	notification := notifications.OrderProcessing(order.BuyerID, order.ID, order.Number)
	err = s.changeStatus(ctx, StatusChange{
		OrderID:       order.ID,
		ChangedBy:     sellerID,
		From:          order.Status,
		To:            OrderStatusProcessing,
		Reason:        ReasonSellerProcessing,
		Notifications: []notifications.NewNotification{notification},
	})
	if err != nil {
		return nil, err
	}
	return s.GetSellerOrder(ctx, sellerID, id)
}

func (s *service) ShipOrder(ctx context.Context, sellerID string, id string, input ShipmentInput) (*OrderDetail, error) {
	shipment, err := ValidateShipment(input)
	if err != nil {
		return nil, err
	}
	order, err := s.sellerOrder(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}
	if missing := order.MissingPrescriptions(); len(missing) > 0 {
		return nil, &app.PrescriptionRequiredError{ItemIDs: missing}
	}
	if pending := order.PendingFittingReports(); len(pending) > 0 {
		return nil, &app.FittingReportRequiredError{ItemIDs: pending}
	}
	notification := notifications.OrderShipped(order.BuyerID, order.ID, order.Number, shipment.Carrier, shipment.TrackingCode)
	err = s.changeStatus(ctx, StatusChange{
		OrderID:       order.ID,
		ChangedBy:     sellerID,
		From:          order.Status,
		To:            OrderStatusShipped,
		Reason:        ReasonSellerShipped,
		Shipment:      &shipment,
		Notifications: []notifications.NewNotification{notification},
	})
	if err != nil {
		return nil, err
	}
	return s.GetSellerOrder(ctx, sellerID, id)
}

func (s *service) ConfirmDelivery(ctx context.Context, buyerID string, id string) (*OrderDetail, error) {
	order, err := s.ownedOrder(ctx, buyerID, id)
	if err != nil {
		return nil, err
	}
	notification := notifications.OrderDelivered(order.SellerID, order.ID, order.Number)
	err = s.changeStatus(ctx, StatusChange{
		OrderID:       order.ID,
		ChangedBy:     buyerID,
		From:          order.Status,
		To:            OrderStatusDelivered,
		Reason:        ReasonBuyerConfirmedDelivery,
		Notifications: []notifications.NewNotification{notification},
	})
	if err != nil {
		return nil, err
	}
	return s.GetOrder(ctx, buyerID, id)
}

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

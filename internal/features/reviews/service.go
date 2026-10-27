package reviews

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type CreateReviewUseCase interface {
	CreateReview(ctx context.Context, buyerID string, orderID string, itemID string, input ReviewInput) (*ReviewEntity, error)
}

type ListReviewsUseCase interface {
	ListReviews(ctx context.Context, advertisementID string, page uint, limit uint) (*ReviewPage, error)
}

type repository interface {
	GetOrderItem(ctx context.Context, orderID string, itemID string) (*ReviewedItem, error)
	CreateReview(ctx context.Context, review NewReview) (*ReviewEntity, error)
	CountReviews(ctx context.Context, advertisementID string) (uint, error)
	GetReviews(ctx context.Context, advertisementID string, limit uint, offset uint) ([]*PublicReviewEntity, error)
}

const (
	orderStatusDelivered  = "delivered"
	defaultReviewsPerPage = 10
	maxReviewsPerPage     = 50
)

type service struct {
	r repository
}

func NewService(r repository) *service {
	return &service{r: r}
}

func (s *service) CreateReview(ctx context.Context, buyerID string, orderID string, itemID string, input ReviewInput) (*ReviewEntity, error) {
	review, err := newReview(itemID, buyerID, input)
	if err != nil {
		return nil, err
	}
	if !helpers.IsUUID(orderID) {
		return nil, app.ErrOrderNotFound
	}
	if !helpers.IsUUID(itemID) {
		return nil, app.ErrOrderItemNotFound
	}

	item, err := s.r.GetOrderItem(ctx, orderID, itemID)
	if err != nil {
		if !errors.Is(err, app.ErrOrderNotFound) {
			slog.ErrorContext(ctx, "GetOrderItem failed", "order_id", orderID, "item_id", itemID, "error", err)
		}
		return nil, err
	}
	if item.BuyerID != buyerID {
		return nil, app.ErrOrderNotFound
	}
	if item.ID == "" {
		return nil, app.ErrOrderItemNotFound
	}
	if item.OrderStatus != orderStatusDelivered {
		return nil, fmt.Errorf("%w: this order is %s", app.ErrReviewNotAllowed, item.OrderStatus)
	}

	created, err := s.r.CreateReview(ctx, review)
	if err != nil {
		if !errors.Is(err, app.ErrAlreadyReviewed) {
			slog.ErrorContext(ctx, "CreateReview failed", "item_id", itemID, "error", err)
		}
		return nil, err
	}
	return created, nil
}

func (s *service) ListReviews(ctx context.Context, advertisementID string, page uint, limit uint) (*ReviewPage, error) {
	if !helpers.IsUUID(advertisementID) {
		return nil, app.ErrAdvertisementNotFound
	}
	if page == 0 {
		page = 1
	}
	if limit == 0 {
		limit = defaultReviewsPerPage
	}
	if limit > maxReviewsPerPage {
		return nil, fmt.Errorf("%w: limit must be at most %d", app.ErrInvalidInput, maxReviewsPerPage)
	}

	total, err := s.r.CountReviews(ctx, advertisementID)
	if err != nil {
		slog.ErrorContext(ctx, "CountReviews failed", "advertisement_id", advertisementID, "error", err)
		return nil, err
	}
	items, err := s.r.GetReviews(ctx, advertisementID, limit, (page-1)*limit)
	if err != nil {
		slog.ErrorContext(ctx, "GetReviews failed", "advertisement_id", advertisementID, "error", err)
		return nil, err
	}
	return &ReviewPage{Items: items, CurrentPage: page, TotalPages: (total + limit - 1) / limit, TotalItems: total}, nil
}

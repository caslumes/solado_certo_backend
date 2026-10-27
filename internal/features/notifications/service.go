package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type ListNotificationsUseCase interface {
	ListNotifications(ctx context.Context, userID string, page uint, limit uint) (*NotificationPage, error)
}

type MarkReadUseCase interface {
	MarkRead(ctx context.Context, userID string, id string) error
}

type MarkAllReadUseCase interface {
	MarkAllRead(ctx context.Context, userID string) error
}

type repository interface {
	CountNotifications(ctx context.Context, userID string) (total uint, unread uint, err error)
	GetNotifications(ctx context.Context, userID string, limit uint, offset uint) ([]*NotificationEntity, error)
	MarkRead(ctx context.Context, userID string, id string) error
	MarkAllRead(ctx context.Context, userID string) error
}

const (
	defaultNotificationsPerPage = 20
	maxNotificationsPerPage     = 100
)

type service struct {
	r repository
}

func NewService(r repository) *service {
	return &service{r: r}
}

func (s *service) ListNotifications(ctx context.Context, userID string, page uint, limit uint) (*NotificationPage, error) {
	if page == 0 {
		page = 1
	}
	if limit == 0 {
		limit = defaultNotificationsPerPage
	}
	if limit > maxNotificationsPerPage {
		return nil, fmt.Errorf("%w: limit must be at most %d", app.ErrInvalidInput, maxNotificationsPerPage)
	}

	total, unread, err := s.r.CountNotifications(ctx, userID)
	if err != nil {
		slog.ErrorContext(ctx, "CountNotifications failed", "user_id", userID, "error", err)
		return nil, err
	}
	items, err := s.r.GetNotifications(ctx, userID, limit, (page-1)*limit)
	if err != nil {
		slog.ErrorContext(ctx, "GetNotifications failed", "user_id", userID, "error", err)
		return nil, err
	}
	return &NotificationPage{
		Items:       items,
		CurrentPage: page,
		TotalPages:  (total + limit - 1) / limit,
		TotalItems:  total,
		UnreadCount: unread,
	}, nil
}

func (s *service) MarkRead(ctx context.Context, userID string, id string) error {
	if !helpers.IsUUID(id) {
		return app.ErrNotificationNotFound
	}
	if err := s.r.MarkRead(ctx, userID, id); err != nil {
		if !errors.Is(err, app.ErrNotificationNotFound) {
			slog.ErrorContext(ctx, "MarkRead failed", "notification_id", id, "error", err)
		}
		return err
	}
	return nil
}

func (s *service) MarkAllRead(ctx context.Context, userID string) error {
	if err := s.r.MarkAllRead(ctx, userID); err != nil {
		slog.ErrorContext(ctx, "MarkAllRead failed", "user_id", userID, "error", err)
		return err
	}
	return nil
}

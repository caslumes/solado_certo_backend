package admin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/app/enum"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type ListUsersUseCase interface {
	ListUsers(ctx context.Context, query UserQuery) (*Page[*UserEntity], error)
}

type ModerateUserUseCase interface {
	BlockUser(ctx context.Context, adminID string, userID string, input ModerationInput) (*UserEntity, error)
	SuspendUser(ctx context.Context, adminID string, userID string, input ModerationInput) (*UserEntity, error)
	ReactivateUser(ctx context.Context, adminID string, userID string, input ModerationInput) (*UserEntity, error)
}

type ListAdsUseCase interface {
	ListAds(ctx context.Context, query AdQuery) (*Page[*AdEntity], error)
}

type RemoveAdUseCase interface {
	RemoveAd(ctx context.Context, adminID string, adID string, reason string) (*AdEntity, error)
}

type ListOrdersUseCase interface {
	ListOrders(ctx context.Context, query OrderQuery) (*Page[*OrderEntity], error)
}

type ListPaymentsUseCase interface {
	ListPayments(ctx context.Context, query PaymentQuery) (*Page[*PaymentEntity], error)
}

type ListAuditLogsUseCase interface {
	ListAuditLogs(ctx context.Context, query AuditQuery) (*Page[*AuditEntity], error)
}

type GetIndicatorsUseCase interface {
	GetIndicators(ctx context.Context, from string, to string) (*Indicators, error)
}

type repository interface {
	ListUsers(ctx context.Context, filter UserFilter, limit uint, offset uint) ([]*UserEntity, uint, error)
	GetUser(ctx context.Context, id string) (*UserEntity, error)
	StatusBeforeBlock(ctx context.Context, userID string) (enum.ProfileStatus, error)
	ModerateUser(ctx context.Context, moderation UserModeration) error
	ListAds(ctx context.Context, filter AdFilter, limit uint, offset uint) ([]*AdEntity, uint, error)
	GetAd(ctx context.Context, id string) (*AdEntity, error)
	RemoveAd(ctx context.Context, removal AdRemoval) error
	ListOrders(ctx context.Context, filter OrderFilter, limit uint, offset uint) ([]*OrderEntity, uint, error)
	ListPayments(ctx context.Context, filter PaymentFilter, limit uint, offset uint) ([]*PaymentEntity, uint, error)
	ListAuditLogs(ctx context.Context, filter AuditFilter, limit uint, offset uint) ([]*AuditEntity, uint, error)
	GetIndicators(ctx context.Context, period Period, now time.Time) (*Indicators, error)
}

type service struct {
	r   repository
	now func() time.Time
}

func NewService(r repository) *service {
	return &service{r: r, now: time.Now}
}

func (s *service) ListUsers(ctx context.Context, query UserQuery) (*Page[*UserEntity], error) {
	paging, err := query.Paging.check()
	if err != nil {
		return nil, err
	}
	search, err := checkSearch(query.Search)
	if err != nil {
		return nil, err
	}
	if err := checkOneOf("status", query.Status, userStatuses); err != nil {
		return nil, err
	}
	if err := checkOneOf("role", query.Role, []string{"buyer", "seller", "admin"}); err != nil {
		return nil, err
	}

	users, total, err := s.r.ListUsers(ctx, UserFilter{Search: search, Status: query.Status, Role: query.Role, Now: s.now()}, paging.Limit, paging.offset())
	if err != nil {
		slog.ErrorContext(ctx, "ListUsers failed", "error", err)
		return nil, err
	}
	return newPage(users, paging, total), nil
}

func (s *service) BlockUser(ctx context.Context, adminID string, userID string, input ModerationInput) (*UserEntity, error) {
	reason, err := checkReason(input.Reason)
	if err != nil {
		return nil, err
	}
	user, err := s.target(ctx, adminID, userID)
	if err != nil {
		return nil, err
	}
	if user.Status == enum.ProfileStatusBlocked {
		return nil, fmt.Errorf("%w: the account is already blocked", app.ErrInvalidStatusTransition)
	}

	details := map[string]any{"reason": reason, "previous_status": user.Status}
	if user.StatusAt(s.now()) == enum.ProfileStatusSuspended {
		details["suspended_until"] = user.SuspendedUntil
	}
	return s.moderate(ctx, UserModeration{
		UserID: user.ID, AdminID: adminID, FromStatus: user.Status, ToStatus: enum.ProfileStatusBlocked,
		RevokeSessions: true, Action: ActionBlockUser, Details: details,
	})
}

func (s *service) SuspendUser(ctx context.Context, adminID string, userID string, input ModerationInput) (*UserEntity, error) {
	reason, err := checkReason(input.Reason)
	if err != nil {
		return nil, err
	}
	if input.Until == nil {
		return nil, &app.MissingFieldsError{Fields: []string{"until"}}
	}
	now := s.now()
	until := input.Until.UTC()
	if !until.After(now) || until.Sub(now) > maxSuspension {
		return nil, fmt.Errorf("%w: until must be in the future and at most %d days away", app.ErrInvalidInput, int(maxSuspension.Hours()/24))
	}
	user, err := s.target(ctx, adminID, userID)
	if err != nil {
		return nil, err
	}
	if user.Status == enum.ProfileStatusBlocked {
		return nil, fmt.Errorf("%w: a blocked account must be reactivated before a suspension", app.ErrInvalidStatusTransition)
	}

	details := map[string]any{"reason": reason, "until": until}
	if user.StatusAt(now) == enum.ProfileStatusSuspended {
		details["previous_until"] = user.SuspendedUntil
	}
	return s.moderate(ctx, UserModeration{
		UserID: user.ID, AdminID: adminID, FromStatus: user.Status, ToStatus: user.Status, SuspendedUntil: &until,
		RevokeSessions: true, Action: ActionSuspendUser, Details: details,
	})
}

func (s *service) ReactivateUser(ctx context.Context, adminID string, userID string, input ModerationInput) (*UserEntity, error) {
	reason, err := checkReason(input.Reason)
	if err != nil {
		return nil, err
	}
	user, err := s.target(ctx, adminID, userID)
	if err != nil {
		return nil, err
	}

	status := user.StatusAt(s.now())
	to := user.Status
	switch status {
	case enum.ProfileStatusSuspended:
	case enum.ProfileStatusBlocked:
		previous, err := s.r.StatusBeforeBlock(ctx, user.ID)
		if err != nil {
			slog.ErrorContext(ctx, "StatusBeforeBlock failed", "user_id", user.ID, "error", err)
			return nil, err
		}
		to = enum.ProfileStatusActive
		if slices.Contains([]enum.ProfileStatus{enum.ProfileStatusPending, enum.ProfileStatusInactive}, previous) {
			to = previous
		}
	default:
		return nil, fmt.Errorf("%w: the account is not blocked or suspended", app.ErrInvalidStatusTransition)
	}

	return s.moderate(ctx, UserModeration{
		UserID: user.ID, AdminID: adminID, FromStatus: user.Status, ToStatus: to,
		Action: ActionReactivateUser, Details: map[string]any{"reason": reason, "previous_status": status, "status": to},
	})
}

func (s *service) target(ctx context.Context, adminID string, userID string) (*UserEntity, error) {
	if !helpers.IsUUID(userID) {
		return nil, app.ErrUserNotFound
	}
	if userID == adminID {
		return nil, fmt.Errorf("%w: you cannot moderate your own account", app.ErrForbidden)
	}
	user, err := s.r.GetUser(ctx, userID)
	if err != nil {
		if !errors.Is(err, app.ErrUserNotFound) {
			slog.ErrorContext(ctx, "GetUser failed", "user_id", userID, "error", err)
		}
		return nil, err
	}
	if isAdminRole(user.Roles) {
		return nil, fmt.Errorf("%w: administrators cannot be moderated", app.ErrForbidden)
	}
	return user, nil
}

func (s *service) moderate(ctx context.Context, moderation UserModeration) (*UserEntity, error) {
	if err := s.r.ModerateUser(ctx, moderation); err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) && !errors.Is(err, app.ErrUserNotFound) {
			slog.ErrorContext(ctx, "ModerateUser failed", "user_id", moderation.UserID, "action", moderation.Action, "error", err)
		}
		return nil, err
	}
	user, err := s.r.GetUser(ctx, moderation.UserID)
	if err != nil {
		slog.ErrorContext(ctx, "GetUser failed", "user_id", moderation.UserID, "error", err)
		return nil, err
	}
	return user, nil
}

func (s *service) ListAds(ctx context.Context, query AdQuery) (*Page[*AdEntity], error) {
	paging, err := query.Paging.check()
	if err != nil {
		return nil, err
	}
	search, err := checkSearch(query.Search)
	if err != nil {
		return nil, err
	}
	if err := checkOneOf("status", query.Status, adStatuses); err != nil {
		return nil, err
	}
	if query.SellerID != "" && !helpers.IsUUID(query.SellerID) {
		return nil, fmt.Errorf("%w: seller_id must be a UUID", app.ErrInvalidInput)
	}

	ads, total, err := s.r.ListAds(ctx, AdFilter{Search: search, Status: query.Status, SellerID: query.SellerID}, paging.Limit, paging.offset())
	if err != nil {
		slog.ErrorContext(ctx, "ListAds failed", "error", err)
		return nil, err
	}
	return newPage(ads, paging, total), nil
}

func (s *service) RemoveAd(ctx context.Context, adminID string, adID string, reason string) (*AdEntity, error) {
	text, err := checkReason(reason)
	if err != nil {
		return nil, err
	}
	if !helpers.IsUUID(adID) {
		return nil, app.ErrAdvertisementNotFound
	}
	ad, err := s.r.GetAd(ctx, adID)
	if err != nil {
		if !errors.Is(err, app.ErrAdvertisementNotFound) {
			slog.ErrorContext(ctx, "GetAd failed", "advertisement_id", adID, "error", err)
		}
		return nil, err
	}
	if ad.RemovedAt != nil {
		return nil, fmt.Errorf("%w: the advertisement is already removed", app.ErrInvalidStatusTransition)
	}

	err = s.r.RemoveAd(ctx, AdRemoval{
		AdID: ad.ID, AdminID: adminID, Reason: text, FromStatus: ad.Status,
		Notification: notifications.AdvertisementRemoved(ad.SellerID, ad.ID, ad.Title, text),
	})
	if err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) {
			slog.ErrorContext(ctx, "RemoveAd failed", "advertisement_id", ad.ID, "error", err)
		}
		return nil, err
	}

	removed, err := s.r.GetAd(ctx, ad.ID)
	if err != nil {
		slog.ErrorContext(ctx, "GetAd failed", "advertisement_id", ad.ID, "error", err)
		return nil, err
	}
	return removed, nil
}

func (s *service) ListOrders(ctx context.Context, query OrderQuery) (*Page[*OrderEntity], error) {
	paging, err := query.Paging.check()
	if err != nil {
		return nil, err
	}
	search, err := checkSearch(query.Search)
	if err != nil {
		return nil, err
	}
	if err := checkOneOf("status", query.Status, orderStatuses); err != nil {
		return nil, err
	}
	since, until, err := parseRange(query.From, query.To)
	if err != nil {
		return nil, err
	}

	orders, total, err := s.r.ListOrders(ctx, OrderFilter{Search: search, Status: query.Status, Since: since, Until: until}, paging.Limit, paging.offset())
	if err != nil {
		slog.ErrorContext(ctx, "ListOrders failed", "error", err)
		return nil, err
	}
	return newPage(orders, paging, total), nil
}

func (s *service) ListPayments(ctx context.Context, query PaymentQuery) (*Page[*PaymentEntity], error) {
	paging, err := query.Paging.check()
	if err != nil {
		return nil, err
	}
	if err := checkOneOf("status", query.Status, paymentStatuses); err != nil {
		return nil, err
	}
	if err := checkOneOf("method", query.Method, paymentMethods); err != nil {
		return nil, err
	}
	since, until, err := parseRange(query.From, query.To)
	if err != nil {
		return nil, err
	}

	filter := PaymentFilter{Status: query.Status, Method: query.Method, RefundGivenUp: query.RefundGivenUp, Since: since, Until: until}
	list, total, err := s.r.ListPayments(ctx, filter, paging.Limit, paging.offset())
	if err != nil {
		slog.ErrorContext(ctx, "ListPayments failed", "error", err)
		return nil, err
	}
	return newPage(list, paging, total), nil
}

func (s *service) ListAuditLogs(ctx context.Context, query AuditQuery) (*Page[*AuditEntity], error) {
	paging, err := query.Paging.check()
	if err != nil {
		return nil, err
	}
	for name, id := range map[string]string{"entity_id": query.EntityID, "user_id": query.UserID} {
		if id != "" && !helpers.IsUUID(id) {
			return nil, fmt.Errorf("%w: %s must be a UUID", app.ErrInvalidInput, name)
		}
	}
	since, until, err := parseRange(query.From, query.To)
	if err != nil {
		return nil, err
	}

	filter := AuditFilter{Entity: query.Entity, EntityID: query.EntityID, UserID: query.UserID, Action: query.Action, Since: since, Until: until}
	logs, total, err := s.r.ListAuditLogs(ctx, filter, paging.Limit, paging.offset())
	if err != nil {
		slog.ErrorContext(ctx, "ListAuditLogs failed", "error", err)
		return nil, err
	}
	return newPage(logs, paging, total), nil
}

func (s *service) GetIndicators(ctx context.Context, from string, to string) (*Indicators, error) {
	now := s.now()
	period, err := indicatorPeriod(from, to, now)
	if err != nil {
		return nil, err
	}

	indicators, err := s.r.GetIndicators(ctx, period, now)
	if err != nil {
		slog.ErrorContext(ctx, "GetIndicators failed", "error", err)
		return nil, err
	}
	indicators.Period = period
	return indicators, nil
}

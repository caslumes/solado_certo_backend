package customorders

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type CreateRequestUseCase interface {
	CreateRequest(ctx context.Context, buyerID string, input RequestInput) (*RequestDetailEntity, error)
}

type ListRequestsUseCase interface {
	ListRequests(ctx context.Context, buyerID string, status Status, page uint, limit uint) (*RequestPage, error)
	ListSellerRequests(ctx context.Context, sellerID string, status Status, page uint, limit uint) (*RequestPage, error)
}

type GetRequestUseCase interface {
	GetRequest(ctx context.Context, buyerID string, id string) (*RequestDetailEntity, error)
	GetSellerRequest(ctx context.Context, sellerID string, id string) (*RequestDetailEntity, error)
}

type BuyerActionsUseCase interface {
	ComplementRequest(ctx context.Context, buyerID string, id string, input ComplementInput) (*RequestDetailEntity, error)
	CancelRequest(ctx context.Context, buyerID string, id string, input CancelInput) (*RequestDetailEntity, error)
}

type SellerActionsUseCase interface {
	ReviewRequest(ctx context.Context, sellerID string, id string) (*RequestDetailEntity, error)
	AcceptRequest(ctx context.Context, sellerID string, id string, input AcceptInput) (*RequestDetailEntity, error)
	RejectRequest(ctx context.Context, sellerID string, id string, input ReasonInput) (*RequestDetailEntity, error)
	RequestInfo(ctx context.Context, sellerID string, id string, input ReasonInput) (*RequestDetailEntity, error)
}

// ConsentChecker is the me service, which knows the current terms of each consent.
type ConsentChecker interface {
	RequireConsent(ctx context.Context, userID string, purpose me.ConsentPurpose) error
}

type repository interface {
	GetAdvertisement(ctx context.Context, id string) (*CustomizableAd, error)
	CreateRequest(ctx context.Context, request NewRequest) (string, error)
	GetRequest(ctx context.Context, id string) (*RequestDetailEntity, error)
	CountRequests(ctx context.Context, filter Filter) (uint, error)
	GetRequests(ctx context.Context, filter Filter, limit uint, offset uint) ([]*RequestEntity, error)
	ChangeStatus(ctx context.Context, change StatusChange) error
}

const (
	defaultRequestsPerPage = 10
	maxRequestsPerPage     = 100
)

type service struct {
	r        repository
	consents ConsentChecker
}

func NewService(r repository, consents ConsentChecker) *service {
	return &service{r: r, consents: consents}
}

// CreateRequest asks the seller of an ad that takes custom orders to make the item from the buyer's
// measurements (RF-037, UC-008). The measurements are shared with the seller, so they need the buyer's
// consent (LGPD art. 11, I).
func (s *service) CreateRequest(ctx context.Context, buyerID string, input RequestInput) (*RequestDetailEntity, error) {
	if input.AdvertisementID == "" || input.Measurements == nil {
		var missing []string
		if input.AdvertisementID == "" {
			missing = append(missing, "advertisement_id")
		}
		if input.Measurements == nil {
			missing = append(missing, "measurements")
		}
		return nil, &app.MissingFieldsError{Fields: missing}
	}

	measurements, err := input.Measurements.check()
	if err != nil {
		return nil, err
	}

	notes, err := text("technical_notes", input.TechnicalNotes, maxNotesLength)
	if err != nil {
		return nil, err
	}
	if !helpers.IsUUID(input.AdvertisementID) {
		return nil, app.ErrAdvertisementNotFound
	}

	ad, err := s.r.GetAdvertisement(ctx, input.AdvertisementID)
	if err != nil {
		if !errors.Is(err, app.ErrAdvertisementNotFound) {
			slog.ErrorContext(ctx, "GetAdvertisement failed", "advertisement_id", input.AdvertisementID, "error", err)
		}
		return nil, err
	}
	if !ad.Published {
		return nil, app.ErrAdvertisementNotFound
	}
	if !ad.CustomMadeAllowed {
		return nil, app.ErrCustomMadeNotAllowed
	}
	if ad.SellerID == buyerID {
		return nil, app.ErrOwnAdvertisement
	}
	if err := s.consents.RequireConsent(ctx, buyerID, me.ConsentPurposeCustomOrder); err != nil {
		return nil, err
	}

	id, err := s.r.CreateRequest(ctx, NewRequest{
		BuyerID:         buyerID,
		AdvertisementID: ad.ID,
		Measurements:    measurements,
		TechnicalNotes:  notes,
		Notification: func(requestID string) notifications.NewNotification {
			return notifications.CustomOrderRequested(ad.SellerID, requestID, ad.Title)
		},
	})
	if err != nil {
		slog.ErrorContext(ctx, "CreateRequest failed", "advertisement_id", ad.ID, "error", err)
		return nil, err
	}

	return s.load(ctx, id)
}

func (s *service) ListRequests(ctx context.Context, buyerID string, status Status, page uint, limit uint) (*RequestPage, error) {
	return s.list(ctx, Filter{BuyerID: buyerID, Status: status}, page, limit)
}

func (s *service) ListSellerRequests(ctx context.Context, sellerID string, status Status, page uint, limit uint) (*RequestPage, error) {
	return s.list(ctx, Filter{SellerID: sellerID, Status: status}, page, limit)
}

// list returns a party's requests, the most recently changed first.
func (s *service) list(ctx context.Context, filter Filter, page uint, limit uint) (*RequestPage, error) {
	if filter.Status != "" && !slices.Contains(statuses, filter.Status) {
		names := make([]string, len(statuses))
		for i, status := range statuses {
			names[i] = string(status)
		}
		return nil, fmt.Errorf("%w: status must be one of %s", app.ErrInvalidInput, strings.Join(names, ", "))
	}
	if page == 0 {
		page = 1
	}
	if limit == 0 {
		limit = defaultRequestsPerPage
	}
	if limit > maxRequestsPerPage {
		return nil, fmt.Errorf("%w: limit must be at most %d", app.ErrInvalidInput, maxRequestsPerPage)
	}

	total, err := s.r.CountRequests(ctx, filter)
	if err != nil {
		slog.ErrorContext(ctx, "CountRequests failed", "error", err)
		return nil, err
	}

	items, err := s.r.GetRequests(ctx, filter, limit, (page-1)*limit)
	if err != nil {
		slog.ErrorContext(ctx, "GetRequests failed", "error", err)
		return nil, err
	}

	return &RequestPage{Items: items, CurrentPage: page, TotalPages: (total + limit - 1) / limit, TotalItems: total}, nil
}

func (s *service) GetRequest(ctx context.Context, buyerID string, id string) (*RequestDetailEntity, error) {
	request, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if request.BuyerID != buyerID {
		return nil, app.ErrCustomOrderNotFound
	}

	return request, nil
}

func (s *service) GetSellerRequest(ctx context.Context, sellerID string, id string) (*RequestDetailEntity, error) {
	request, err := s.load(ctx, id)
	if err != nil {
		return nil, err
	}
	if request.SellerID != sellerID {
		return nil, app.ErrCustomOrderNotFound
	}

	return request, nil
}

// ComplementRequest answers what the seller asked (UC-008 alt. flow); the request goes back to review.
func (s *service) ComplementRequest(ctx context.Context, buyerID string, id string, input ComplementInput) (*RequestDetailEntity, error) {
	message, err := requiredText("message", input.Message, maxMessageLength)
	if err != nil {
		return nil, err
	}

	var measurements *Measurements
	if input.Measurements != nil {
		checked, err := input.Measurements.check()
		if err != nil {
			return nil, err
		}
		measurements = &checked
	}

	notes, err := text("technical_notes", input.TechnicalNotes, maxNotesLength)
	if err != nil {
		return nil, err
	}

	request, err := s.GetRequest(ctx, buyerID, id)
	if err != nil {
		return nil, err
	}
	if err := s.consents.RequireConsent(ctx, buyerID, me.ConsentPurposeCustomOrder); err != nil {
		return nil, err
	}

	return s.change(ctx, request, ActionComplement, buyerID, StatusChange{
		Reason:         &message,
		Measurements:   measurements,
		TechnicalNotes: notes,
		Notification:   notifications.CustomOrderComplemented(request.SellerID, request.ID, request.AdvertisementTitle),
	})
}

// CancelRequest is the buyer giving up a request before paying for it.
func (s *service) CancelRequest(ctx context.Context, buyerID string, id string, input CancelInput) (*RequestDetailEntity, error) {
	reason, err := text("reason", input.Reason, maxMessageLength)
	if err != nil {
		return nil, err
	}

	request, err := s.GetRequest(ctx, buyerID, id)
	if err != nil {
		return nil, err
	}
	if request.OrderID != nil {
		return nil, fmt.Errorf("%w: the request already has an order; cancel the order instead", app.ErrInvalidStatusTransition)
	}

	return s.change(ctx, request, ActionCancel, buyerID, StatusChange{
		Reason:       reason,
		Notification: notifications.CustomOrderCancelled(request.SellerID, request.ID, request.AdvertisementTitle),
	})
}

// ReviewRequest tells the buyer that the seller is looking at the request.
func (s *service) ReviewRequest(ctx context.Context, sellerID string, id string) (*RequestDetailEntity, error) {
	request, err := s.GetSellerRequest(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}

	return s.change(ctx, request, ActionReview, sellerID, StatusChange{
		Notification: notifications.CustomOrderUnderReview(request.BuyerID, request.ID, request.AdvertisementTitle),
	})
}

// AcceptRequest is the seller agreeing to make the item for the quoted price (RF-038), which the buyer
// confirms by paying.
func (s *service) AcceptRequest(ctx context.Context, sellerID string, id string, input AcceptInput) (*RequestDetailEntity, error) {
	if strings.TrimSpace(input.QuotedPrice) == "" {
		return nil, &app.MissingFieldsError{Fields: []string{"quoted_price"}}
	}

	price, err := helpers.ParseCents(strings.TrimSpace(input.QuotedPrice))
	if err != nil || price <= 0 || price > maxQuotedPrice {
		return nil, fmt.Errorf("%w: quoted_price must be a positive decimal like \"189.90\"", app.ErrInvalidInput)
	}
	quoted := price.String()

	request, err := s.GetSellerRequest(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}

	return s.change(ctx, request, ActionAccept, sellerID, StatusChange{
		QuotedPrice:  &quoted,
		Notification: notifications.CustomOrderAccepted(request.BuyerID, request.ID, request.AdvertisementTitle, quoted),
	})
}

// RejectRequest closes the request with the reason, which the buyer sees (RF-038).
func (s *service) RejectRequest(ctx context.Context, sellerID string, id string, input ReasonInput) (*RequestDetailEntity, error) {
	reason, err := requiredText("reason", input.Reason, maxMessageLength)
	if err != nil {
		return nil, err
	}

	request, err := s.GetSellerRequest(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}

	return s.change(ctx, request, ActionReject, sellerID, StatusChange{
		Reason:          &reason,
		RejectionReason: &reason,
		Notification:    notifications.CustomOrderRejected(request.BuyerID, request.ID, request.AdvertisementTitle, reason),
	})
}

// RequestInfo asks the buyer for what is missing to decide (RF-038); the question is the reason.
func (s *service) RequestInfo(ctx context.Context, sellerID string, id string, input ReasonInput) (*RequestDetailEntity, error) {
	question, err := requiredText("reason", input.Reason, maxMessageLength)
	if err != nil {
		return nil, err
	}

	request, err := s.GetSellerRequest(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}

	return s.change(ctx, request, ActionRequestInfo, sellerID, StatusChange{
		Reason:       &question,
		Notification: notifications.CustomOrderNeedsInfo(request.BuyerID, request.ID, request.AdvertisementTitle, question),
	})
}

// change applies an action to the request, if its status allows it, and returns the request as it is now.
func (s *service) change(ctx context.Context, request *RequestDetailEntity, action Action, userID string, change StatusChange) (*RequestDetailEntity, error) {
	if !request.Status.Allows(action) {
		return nil, fmt.Errorf("%w: a request that is %s can't be changed by %s", app.ErrInvalidStatusTransition, request.Status, action)
	}
	change.RequestID = request.ID
	change.From = request.Status
	change.To = transitions[action].to
	change.Action = action
	change.ChangedBy = userID

	if err := s.r.ChangeStatus(ctx, change); err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) {
			slog.ErrorContext(ctx, "ChangeStatus failed", "request_id", request.ID, "action", action, "error", err)
		}
		return nil, err
	}

	return s.load(ctx, request.ID)
}

func (s *service) load(ctx context.Context, id string) (*RequestDetailEntity, error) {
	if !helpers.IsUUID(id) {
		return nil, app.ErrCustomOrderNotFound
	}

	request, err := s.r.GetRequest(ctx, id)
	if err != nil {
		if !errors.Is(err, app.ErrCustomOrderNotFound) {
			slog.ErrorContext(ctx, "GetRequest failed", "request_id", id, "error", err)
		}
		return nil, err
	}

	return request, nil
}

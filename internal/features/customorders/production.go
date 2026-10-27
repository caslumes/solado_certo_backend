package customorders

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/documents"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

func (s *service) Checkout(ctx context.Context, buyerID string, id string, idempotencyKey string, input CheckoutInput) (*RequestDetailEntity, bool, error) {
	if idempotencyKey == "" || len(idempotencyKey) > orders.MaxIdempotencyKeyLength {
		return nil, false, fmt.Errorf("%w: the Idempotency-Key header must have 1 to %d characters", app.ErrInvalidInput, orders.MaxIdempotencyKeyLength)
	}
	var missing []string
	if strings.TrimSpace(input.AddressID) == "" {
		missing = append(missing, "address_id")
	}
	if strings.TrimSpace(input.ShippingOptionID) == "" {
		missing = append(missing, "shipping_option_id")
	}
	if len(missing) > 0 {
		return nil, false, &app.MissingFieldsError{Fields: missing}
	}

	request, err := s.GetRequest(ctx, buyerID, id)
	if err != nil {
		return nil, false, err
	}
	found, err := s.r.CheckoutExists(ctx, buyerID, idempotencyKey)
	if err != nil {
		slog.ErrorContext(ctx, "CheckoutExists failed", "request_id", id, "error", err)
		return nil, false, err
	}
	if found {
		return request, false, nil
	}
	if !request.can(ActionCheckout) {
		return nil, false, fmt.Errorf("%w: only an approved request without an order can be checked out, this one is %s", app.ErrInvalidStatusTransition, request.Status)
	}

	plan, err := s.orders.PlanCustomOrder(ctx, buyerID, orders.CustomItem{
		RequestID:        request.ID,
		AdvertisementID:  request.AdvertisementID,
		AddressID:        input.AddressID,
		ShippingOptionID: input.ShippingOptionID,
		Price:            *request.QuotedPrice,
	})
	if err != nil {
		return nil, false, err
	}

	created, err := s.r.Checkout(ctx, Checkout{RequestID: request.ID, BuyerID: buyerID, IdempotencyKey: idempotencyKey, Plan: plan})
	if err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) && !errors.Is(err, app.ErrShippingRequired) && !errors.Is(err, app.ErrPricesChanged) {
			slog.ErrorContext(ctx, "Checkout failed", "request_id", request.ID, "error", err)
		}
		return nil, false, err
	}
	request, err = s.load(ctx, request.ID)
	return request, created, err
}

func (s *service) ReportRequest(ctx context.Context, sellerID string, id string, instructions string, content io.Reader) (*RequestDetailEntity, error) {
	text, err := requiredText("adaptation_instructions", instructions, maxInstructionsLength)
	if err != nil {
		return nil, err
	}

	request, err := s.GetSellerRequest(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}
	if !request.can(ActionReport) {
		return nil, fmt.Errorf("%w: the report is sent once the order is paid, this request is %s", app.ErrInvalidStatusTransition, request.Status)
	}

	upload, err := storage.ReadUpload(content, app.MaxUploadSize, documents.FileTypes)
	if err != nil {
		return nil, err
	}
	if err := documents.CheckReadable(upload); err != nil {
		return nil, err
	}
	key, err := s.files.Save(ctx, storage.AreaPrivate, reportsFolder, upload.Extension, upload.Reader())
	if err != nil {
		slog.ErrorContext(ctx, "ReportRequest could not save file", "request_id", id, "error", err)
		return nil, err
	}

	completed, err := s.change(ctx, request, ActionReport, sellerID, StatusChange{
		AdaptationInstructions: &text,
		ReportKey:              &key,
		Notification:           notifications.CustomOrderCompleted(request.BuyerID, request.ID, request.AdvertisementTitle),
	})
	if err != nil {
		s.deleteFile(ctx, key)
		return nil, err
	}
	return completed, nil
}

func (s *service) SendPhoto(ctx context.Context, buyerID string, id string, content io.Reader) (*RequestDetailEntity, error) {
	request, err := s.GetRequest(ctx, buyerID, id)
	if err != nil {
		return nil, err
	}
	if !request.can(ActionSendPhoto) {
		return nil, fmt.Errorf("%w: a photo can only be sent before the seller decides, this request is %s", app.ErrInvalidStatusTransition, request.Status)
	}
	if err := s.consents.RequireConsent(ctx, buyerID, me.ConsentPurposeCustomOrder); err != nil {
		return nil, err
	}

	upload, err := storage.ReadUpload(content, app.MaxUploadSize, photoTypes)
	if err != nil {
		return nil, err
	}
	if err := documents.CheckReadable(upload); err != nil {
		return nil, err
	}
	key, err := s.files.Save(ctx, storage.AreaPrivate, photosFolder, upload.Extension, upload.Reader())
	if err != nil {
		slog.ErrorContext(ctx, "SendPhoto could not save file", "request_id", id, "error", err)
		return nil, err
	}

	previous, err := s.r.SetFootPhoto(ctx, PhotoChange{RequestID: request.ID, BuyerID: buyerID, Key: key})
	if err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) {
			slog.ErrorContext(ctx, "SetFootPhoto failed", "request_id", id, "error", err)
		}
		s.deleteFile(ctx, key)
		return nil, err
	}
	if previous != nil {
		s.deleteFile(ctx, *previous)
	}
	return s.load(ctx, request.ID)
}

func (s *service) OpenPhoto(ctx context.Context, buyerID string, id string) (*PhotoFile, error) {
	request, err := s.GetRequest(ctx, buyerID, id)
	if err != nil {
		return nil, err
	}
	return s.openPhoto(ctx, request, buyerID)
}

func (s *service) OpenSellerPhoto(ctx context.Context, sellerID string, id string) (*PhotoFile, error) {
	request, err := s.GetSellerRequest(ctx, sellerID, id)
	if err != nil {
		return nil, err
	}
	return s.openPhoto(ctx, request, sellerID)
}

func (s *service) openPhoto(ctx context.Context, request *RequestDetailEntity, userID string) (*PhotoFile, error) {
	if request.FootPhotoKey == nil {
		return nil, app.ErrFootPhotoNotFound
	}
	if userID != request.BuyerID {
		if err := s.r.RecordPhotoView(ctx, request.ID, userID); err != nil {
			slog.ErrorContext(ctx, "RecordPhotoView failed", "request_id", request.ID, "error", err)
			return nil, err
		}
	}
	file, err := s.files.Open(ctx, storage.AreaPrivate, *request.FootPhotoKey)
	if err != nil {
		slog.ErrorContext(ctx, "OpenPhoto could not open file", "request_id", request.ID, "error", err)
		if errors.Is(err, storage.ErrNotFound) {
			return nil, app.ErrFootPhotoNotFound
		}
		return nil, err
	}
	return &PhotoFile{File: file, ContentType: storage.ContentType(*request.FootPhotoKey), Name: "foot-photo" + path.Ext(*request.FootPhotoKey)}, nil
}

func (s *service) deleteFile(ctx context.Context, key string) {
	if err := s.files.Delete(ctx, storage.AreaPrivate, key); err != nil {
		slog.ErrorContext(ctx, "could not delete file", "key", key, "error", err)
	}
}

package documents

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/me"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

type UploadPrescriptionUseCase interface {
	UploadPrescription(ctx context.Context, buyerID string, orderID string, itemID string, content io.Reader) (*DocumentEntity, error)
}

type OpenDocumentUseCase interface {
	OpenDocument(ctx context.Context, userID string, id string) (*DocumentFile, error)
}

type RejectPrescriptionUseCase interface {
	RejectPrescription(ctx context.Context, sellerID string, orderID string, id string, input RejectionInput) (*DocumentEntity, error)
}

type ConsentChecker interface {
	RequireConsent(ctx context.Context, userID string, purpose me.ConsentPurpose) error
}

type repository interface {
	GetPrescriptionItem(ctx context.Context, orderID string, itemID string) (*PrescriptionItem, error)
	CreateDocument(ctx context.Context, document NewDocument) (*DocumentEntity, error)
	GetDocument(ctx context.Context, id string) (*StoredDocument, error)
	IsAdmin(ctx context.Context, userID string) (bool, error)
	RecordView(ctx context.Context, documentID string, userID string) error
	RejectPrescription(ctx context.Context, rejection Rejection) (*DocumentEntity, error)
}

type service struct {
	r        repository
	files    storage.Storage
	consents ConsentChecker
}

func NewService(r repository, files storage.Storage, consents ConsentChecker) *service {
	return &service{r: r, files: files, consents: consents}
}

func (s *service) UploadPrescription(ctx context.Context, buyerID string, orderID string, itemID string, content io.Reader) (*DocumentEntity, error) {
	if !helpers.IsUUID(orderID) {
		return nil, app.ErrOrderNotFound
	}
	if !helpers.IsUUID(itemID) {
		return nil, app.ErrOrderItemNotFound
	}

	item, err := s.r.GetPrescriptionItem(ctx, orderID, itemID)
	if err != nil {
		if !errors.Is(err, app.ErrOrderNotFound) {
			slog.ErrorContext(ctx, "GetPrescriptionItem failed", "order_id", orderID, "item_id", itemID, "error", err)
		}
		return nil, err
	}
	if item.BuyerID != buyerID {
		return nil, app.ErrOrderNotFound
	}
	if item.ID == "" {
		return nil, app.ErrOrderItemNotFound
	}
	if !item.PrescriptionRequired {
		return nil, app.ErrPrescriptionNotNeeded
	}
	if !orders.CanUploadPrescription(item.OrderStatus, item.Latest) {
		return nil, fmt.Errorf("%w: a prescription can't be sent for an order that is %s and already has an approved one", app.ErrInvalidStatusTransition, item.OrderStatus)
	}
	if err := s.consents.RequireConsent(ctx, buyerID, me.ConsentPurposePrescription); err != nil {
		return nil, err
	}

	upload, err := storage.ReadUpload(content, app.MaxUploadSize, FileTypes)
	if err != nil {
		return nil, err
	}
	if err := CheckReadable(upload); err != nil {
		return nil, err
	}

	key, err := s.files.Save(ctx, storage.AreaPrivate, prescriptionsFolder, upload.Extension, upload.Reader())
	if err != nil {
		slog.ErrorContext(ctx, "UploadPrescription could not save file", "item_id", itemID, "error", err)
		return nil, err
	}
	document := NewDocument{
		OrderID:     orderID,
		OrderItemID: itemID,
		UploadedBy:  buyerID,
		Type:        DocumentTypePrescription,
		Key:         key,
		Status:      orders.DocumentStatusApproved,
	}
	if item.OrderStatus != orders.OrderStatusAwaitingPayment {
		notification := notifications.PrescriptionSent(item.SellerID, orderID, item.OrderNumber, item.Title)
		document.Notification = &notification
	}

	created, err := s.r.CreateDocument(ctx, document)
	if err != nil {
		slog.ErrorContext(ctx, "CreateDocument failed", "item_id", itemID, "error", err)
		if err := s.files.Delete(ctx, storage.AreaPrivate, key); err != nil {
			slog.ErrorContext(ctx, "could not delete file", "key", key, "error", err)
		}
		return nil, err
	}
	return created, nil
}

func (s *service) OpenDocument(ctx context.Context, userID string, id string) (*DocumentFile, error) {
	document, err := s.document(ctx, id)
	if err != nil {
		return nil, err
	}
	allowed := userID == document.BuyerID || userID == document.SellerID && document.OrderPaid
	if !allowed {
		admin, err := s.r.IsAdmin(ctx, userID)
		if err != nil {
			slog.ErrorContext(ctx, "IsAdmin failed", "user_id", userID, "error", err)
			return nil, err
		}
		if !admin {
			return nil, app.ErrDocumentNotFound
		}
	}

	if userID != document.UploadedBy {
		if err := s.r.RecordView(ctx, document.ID, userID); err != nil {
			slog.ErrorContext(ctx, "RecordView failed", "document_id", document.ID, "error", err)
			return nil, err
		}
	}
	file, err := s.files.Open(ctx, storage.AreaPrivate, document.Key)
	if err != nil {
		slog.ErrorContext(ctx, "OpenDocument could not open file", "document_id", document.ID, "error", err)
		if errors.Is(err, storage.ErrNotFound) {
			return nil, app.ErrDocumentNotFound
		}
		return nil, err
	}
	return &DocumentFile{
		File:        file,
		ContentType: storage.ContentType(document.Key),
		Name:        string(document.Type) + "-" + document.OrderNumber + path.Ext(document.Key),
	}, nil
}

func (s *service) RejectPrescription(ctx context.Context, sellerID string, orderID string, id string, input RejectionInput) (*DocumentEntity, error) {
	reason := strings.TrimSpace(input.Reason)
	if reason == "" {
		return nil, &app.MissingFieldsError{Fields: []string{"reason"}}
	}
	if len([]rune(reason)) > maxReasonLength {
		return nil, fmt.Errorf("%w: reason must have at most %d characters", app.ErrInvalidInput, maxReasonLength)
	}

	document, err := s.document(ctx, id)
	if err != nil {
		return nil, err
	}
	if document.OrderID != orderID || document.SellerID != sellerID || !document.OrderPaid ||
		document.Type != DocumentTypePrescription || document.OrderItemID == nil {
		return nil, app.ErrDocumentNotFound
	}

	rejected, err := s.r.RejectPrescription(ctx, Rejection{
		DocumentID:   document.ID,
		OrderID:      document.OrderID,
		OrderItemID:  *document.OrderItemID,
		SellerID:     sellerID,
		Reason:       reason,
		Notification: notifications.PrescriptionRejected(document.BuyerID, document.OrderID, document.OrderNumber, document.ItemTitle, reason),
	})
	if err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) {
			slog.ErrorContext(ctx, "RejectPrescription failed", "document_id", document.ID, "error", err)
		}
		return nil, err
	}
	return rejected, nil
}

func (s *service) document(ctx context.Context, id string) (*StoredDocument, error) {
	if !helpers.IsUUID(id) {
		return nil, app.ErrDocumentNotFound
	}
	document, err := s.r.GetDocument(ctx, id)
	if err != nil {
		if !errors.Is(err, app.ErrDocumentNotFound) {
			slog.ErrorContext(ctx, "GetDocument failed", "document_id", id, "error", err)
		}
		return nil, err
	}
	return document, nil
}

package documents

import (
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/pkg/storage"
)

type DocumentType string

const (
	DocumentTypePrescription    DocumentType = "prescription"
	DocumentTypeTechnicalReport DocumentType = "technical_report"
)

var FileTypes = storage.AllowedTypes{"image/jpeg": ".jpg", "image/png": ".png", "application/pdf": ".pdf"}

const (
	prescriptionsFolder = "prescriptions"
	minImageSide        = 300
	maxImagePixels      = 40_000_000
	maxReasonLength     = 500
	pdfTrailerWindow    = 1024
)

func CheckReadable(upload *storage.Upload) error {
	if upload.ContentType == "application/pdf" {
		tail := upload.Content[max(0, len(upload.Content)-pdfTrailerWindow):]
		if !bytes.Contains(tail, []byte("%%EOF")) {
			return fmt.Errorf("%w: the PDF is incomplete", app.ErrUnreadableDocument)
		}
		return nil
	}

	config, _, err := image.DecodeConfig(upload.Reader())
	if err != nil {
		return fmt.Errorf("%w: the image could not be opened", app.ErrUnreadableDocument)
	}
	if config.Width < minImageSide || config.Height < minImageSide {
		return fmt.Errorf("%w: the image must be at least %dx%d pixels", app.ErrUnreadableDocument, minImageSide, minImageSide)
	}
	if config.Width*config.Height > maxImagePixels {
		return fmt.Errorf("%w: the image must have at most %d megapixels", app.ErrUnreadableDocument, maxImagePixels/1_000_000)
	}
	if _, _, err := image.Decode(upload.Reader()); err != nil {
		return fmt.Errorf("%w: the image is damaged", app.ErrUnreadableDocument)
	}
	return nil
}

type PrescriptionItem struct {
	ID                   string
	Title                string
	OrderID              string
	OrderNumber          string
	OrderStatus          orders.OrderStatus
	BuyerID              string
	SellerID             string
	PrescriptionRequired bool
	Latest               *orders.PrescriptionEntity
}

type NewDocument struct {
	OrderID      string
	OrderItemID  string
	UploadedBy   string
	Type         DocumentType
	Key          string
	Status       orders.DocumentStatus
	Notification *notifications.NewNotification
}

type DocumentEntity struct {
	ID              string
	OrderID         string
	OrderItemID     *string
	Type            DocumentType
	Status          orders.DocumentStatus
	RejectionReason *string
	CreatedAt       time.Time
}

func (d *DocumentEntity) ToOutput() DocumentOutput {
	return DocumentOutput{
		ID:              d.ID,
		OrderID:         d.OrderID,
		OrderItemID:     d.OrderItemID,
		Type:            string(d.Type),
		Status:          string(d.Status),
		RejectionReason: d.RejectionReason,
		URL:             orders.DocumentPath(d.ID),
		CreatedAt:       d.CreatedAt,
	}
}

type StoredDocument struct {
	DocumentEntity
	Key         string
	UploadedBy  string
	OrderNumber string
	ItemTitle   string
	BuyerID     string
	SellerID    string
	OrderPaid   bool
}

type RejectionInput struct {
	Reason string `json:"reason"`
}

type Rejection struct {
	DocumentID   string
	OrderID      string
	OrderItemID  string
	SellerID     string
	Reason       string
	Notification notifications.NewNotification
}

type DocumentFile struct {
	*storage.File
	ContentType string
	Name        string
}

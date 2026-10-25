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

// prescriptionTypes are the formats a prescription may be sent in (UC-007 step 4).
var prescriptionTypes = storage.AllowedTypes{"image/jpeg": ".jpg", "image/png": ".png", "application/pdf": ".pdf"}

const (
	prescriptionsFolder = "prescriptions"
	// minImageSide is the smallest width and height of a photo that can still be read.
	minImageSide = 300
	// maxImagePixels keeps a small file that decodes to a huge image from using up the memory.
	maxImagePixels  = 40_000_000
	maxReasonLength = 500
	// pdfTrailerWindow is how far from the end of a PDF its %%EOF marker is looked for.
	pdfTrailerWindow = 1024
)

// checkReadable is the automatic check of a prescription: the platform checks that a document is there
// and can be opened, not what it says (T08). A photo must decode in full and be large enough to read;
// a PDF must be complete.
func checkReadable(upload *storage.Upload) error {
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

// PrescriptionItem is the order item a prescription is sent for. ID is empty when the order has no such item.
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
	OrderID     string
	OrderItemID string
	UploadedBy  string
	Type        DocumentType
	Key         string
	Status      orders.DocumentStatus
	// Notification, when set, is written in the same transaction as the document.
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

// StoredDocument is a document with its file and the people of its order, who may read it.
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

// Rejection is the seller refusing a prescription. It only applies while the document is the item's
// latest, approved one and the order hasn't shipped (orders.CanRejectPrescription).
type Rejection struct {
	DocumentID   string
	OrderID      string
	OrderItemID  string
	SellerID     string
	Reason       string
	Notification notifications.NewNotification
}

// DocumentFile is a document's content, to be closed after reading.
type DocumentFile struct {
	*storage.File
	ContentType string
	Name        string
}

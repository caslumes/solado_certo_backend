package orders

import "time"

type DocumentStatus string

const (
	DocumentStatusPending  DocumentStatus = "pending"
	DocumentStatusApproved DocumentStatus = "approved"
	DocumentStatusRejected DocumentStatus = "rejected"
	DocumentStatusExpired  DocumentStatus = "expired"
)

// DocumentPath is where a document is downloaded from; only the buyer, the order's seller and
// admins may read it.
func DocumentPath(id string) string {
	return "/documents/" + id
}

// PrescriptionEntity is the latest prescription sent for an order item (RF-035). Older ones are kept
// for the record but no longer count.
type PrescriptionEntity struct {
	ID              string
	Status          DocumentStatus
	RejectionReason *string
	CreatedAt       time.Time
}

func (p *PrescriptionEntity) approved() bool {
	return p != nil && p.Status == DocumentStatusApproved
}

func (p *PrescriptionEntity) ToOutput() *PrescriptionOutput {
	if p == nil {
		return nil
	}
	return &PrescriptionOutput{
		ID:              p.ID,
		Status:          string(p.Status),
		RejectionReason: p.RejectionReason,
		URL:             DocumentPath(p.ID),
		CreatedAt:       p.CreatedAt,
	}
}

// CanUploadPrescription tells whether the buyer may send a prescription for an item: before paying,
// to add or replace it; after paying and until shipping, only when there is no approved one (the
// seller rejected it).
func CanUploadPrescription(status OrderStatus, latest *PrescriptionEntity) bool {
	switch status {
	case OrderStatusAwaitingPayment:
		return true
	case OrderStatusPaid, OrderStatusProcessing:
		return !latest.approved()
	default:
		return false
	}
}

// CanRejectPrescription tells whether the seller may still reject an approved prescription: only
// before shipping, since the item can't be held back afterwards.
func CanRejectPrescription(status OrderStatus, latest *PrescriptionEntity) bool {
	return (status == OrderStatusPaid || status == OrderStatusProcessing) && latest.approved()
}

// MissingPrescriptions returns the items that need a prescription and have no approved one. The
// order can't be paid (RF-036) or shipped while there are any.
func (o *OrderEntity) MissingPrescriptions() []string {
	missing := []string{}
	for _, item := range o.Items {
		if item.PrescriptionRequired && !item.Prescription.approved() {
			missing = append(missing, item.ID)
		}
	}
	return missing
}

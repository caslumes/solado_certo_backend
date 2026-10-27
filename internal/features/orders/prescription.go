package orders

import "time"

type DocumentStatus string

const (
	DocumentStatusPending  DocumentStatus = "pending"
	DocumentStatusApproved DocumentStatus = "approved"
	DocumentStatusRejected DocumentStatus = "rejected"
	DocumentStatusExpired  DocumentStatus = "expired"
)

func DocumentPath(id string) string {
	return "/documents/" + id
}

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

func CanRejectPrescription(status OrderStatus, latest *PrescriptionEntity) bool {
	return (status == OrderStatusPaid || status == OrderStatusProcessing) && latest.approved()
}

func (o *OrderEntity) MissingPrescriptions() []string {
	missing := []string{}
	for _, item := range o.Items {
		if item.PrescriptionRequired && !item.Prescription.approved() {
			missing = append(missing, item.ID)
		}
	}
	return missing
}

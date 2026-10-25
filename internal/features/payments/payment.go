package payments

import (
	"time"

	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

// OrderEntity is what a payment needs to know about its order.
type OrderEntity struct {
	ID            string
	BuyerID       string
	Number        string
	Status        orders.OrderStatus
	Subtotal      string
	ShippingTotal string
	DiscountTotal string
	Total         string
	PostalCode    string
	AddressNumber string
	CreatedAt     time.Time
	// MissingPrescriptions are the items that need a prescription and have no approved one (RF-036).
	MissingPrescriptions []string
}

// Expired tells whether the order's due date is over.
func (o *OrderEntity) Expired(now time.Time) bool {
	return !now.Before(EndOfDay(o.DueDate()))
}

// DueDate is the last day to pay, PaymentDays after checkout in Brazil's calendar.
func (o *OrderEntity) DueDate() time.Time {
	created := o.CreatedAt.In(brazilTime)
	return time.Date(created.Year(), created.Month(), created.Day()+PaymentDays, 0, 0, 0, 0, time.UTC)
}

// EndOfDay is the moment a due date ends in Brazil.
func EndOfDay(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), date.Day()+1, 0, 0, 0, 0, brazilTime)
}

// expiredBefore returns the checkout time before which an order is expired at now.
func expiredBefore(now time.Time) time.Time {
	today := now.In(brazilTime)
	return time.Date(today.Year(), today.Month(), today.Day()-PaymentDays, 0, 0, 0, 0, brazilTime)
}

// Totals gives the discount and total the order has when paid with method: Pix takes
// PixDiscountPercent off the subtotal, rounded to the nearest cent (RF-013).
func (o *OrderEntity) Totals(method Method) (discount string, total string) {
	subtotal := helpers.MustParseCents(o.Subtotal)
	var off helpers.Cents
	if method == MethodPix {
		off = (subtotal*PixDiscountPercent + 50) / 100
	}
	return off.String(), (subtotal + helpers.MustParseCents(o.ShippingTotal) - off).String()
}

type BuyerEntity struct {
	ID    string
	Name  string
	Email string
	Phone string
	TaxID *string
}

// PaymentDetails is stored in payments.gateway_payload: what the app shows for each method. Card
// numbers and CVVs are never part of it.
type PaymentDetails struct {
	GatewayStatus string     `json:"gateway_status,omitempty"`
	GatewayError  string     `json:"gateway_error,omitempty"`
	InvoiceURL    string     `json:"invoice_url,omitempty"`
	BoletoURL     string     `json:"boleto_url,omitempty"`
	PixPayload    string     `json:"pix_payload,omitempty"`
	PixExpiresAt  *time.Time `json:"pix_expires_at,omitempty"`
	CardLast4     string     `json:"card_last4,omitempty"`
	CardBrand     string     `json:"card_brand,omitempty"`
	// RefundRequestedAt is set once the money is asked back from the gateway; the payment stays captured
	// until the gateway reports it refunded.
	RefundRequestedAt *time.Time `json:"refund_requested_at,omitempty"`
	// RefundURL is where the payer of a boleto gives the bank account to refund.
	RefundURL string `json:"refund_url,omitempty"`
	// RefundError is the gateway's last refusal of the refund, and RefundAttempts how many it refused.
	// The poller asks again until MaxRefundAttempts; then support takes over.
	RefundError    string `json:"refund_error,omitempty"`
	RefundAttempts int    `json:"refund_attempts,omitempty"`
}

type PaymentEntity struct {
	ID            string
	OrderID       string
	Gateway       string
	TransactionID *string
	Method        Method
	Status        Status
	Amount        string
	DueDate       *time.Time
	FailureReason *string
	Details       PaymentDetails
	CapturedAt    *time.Time
	RefundedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewPayment starts a payment attempt; the order's discount and total change with it.
type NewPayment struct {
	OrderID        string
	CreatedBy      string
	IdempotencyKey string
	Gateway        string
	Method         Method
	Amount         string
	DiscountTotal  string
	DueDate        time.Time
}

// StatusChange moves a payment to a new status. A capture also marks its order as paid.
type StatusChange struct {
	PaymentID     string
	To            Status
	FailureReason string
	Source        string
	Details       PaymentDetails
}

type CardInput struct {
	HolderName  string `json:"holder_name"`
	Number      string `json:"number"`
	ExpiryMonth string `json:"expiry_month"`
	ExpiryYear  string `json:"expiry_year"`
	CVV         string `json:"cvv"`
}

type CreatePaymentInput struct {
	Method   Method     `json:"method"`
	TaxID    string     `json:"tax_id"`
	Card     *CardInput `json:"card"`
	RemoteIP string     `json:"-"`
}

func (p *PaymentEntity) ToOutput() PaymentOutput {
	output := PaymentOutput{
		ID:                p.ID,
		OrderID:           p.OrderID,
		Method:            string(p.Method),
		Status:            string(p.Status),
		Amount:            p.Amount,
		FailureReason:     p.FailureReason,
		FailureMessage:    optional(p.Details.GatewayError),
		BoletoURL:         optional(p.Details.BoletoURL),
		InvoiceURL:        optional(p.Details.InvoiceURL),
		CapturedAt:        p.CapturedAt,
		RefundedAt:        p.RefundedAt,
		RefundRequestedAt: p.Details.RefundRequestedAt,
		RefundURL:         optional(p.Details.RefundURL),
		CreatedAt:         p.CreatedAt,
		UpdatedAt:         p.UpdatedAt,
	}
	if p.DueDate != nil {
		due := p.DueDate.Format(time.DateOnly)
		output.DueDate = &due
	}
	if p.Details.PixPayload != "" {
		output.Pix = &PixOutput{Payload: p.Details.PixPayload, ExpiresAt: p.Details.PixExpiresAt}
	}
	if p.Details.CardLast4 != "" {
		output.Card = &CardOutput{Last4: p.Details.CardLast4, Brand: p.Details.CardBrand}
	}
	return output
}

func PaymentsToOutput(payments []*PaymentEntity) []PaymentOutput {
	outputs := make([]PaymentOutput, len(payments))
	for i, p := range payments {
		outputs[i] = p.ToOutput()
	}
	return outputs
}

func optional(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

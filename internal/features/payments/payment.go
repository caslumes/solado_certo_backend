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
}

// Expired tells whether the order's payment window (PaymentWindow) is over.
func (o *OrderEntity) Expired(now time.Time) bool {
	return !now.Before(o.CreatedAt.Add(PaymentWindow))
}

// DueDate is the last day to pay a Pix or a boleto, in Brazil's calendar.
func (o *OrderEntity) DueDate() time.Time {
	due := o.CreatedAt.Add(PaymentWindow).In(brazilTime)
	return time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, time.UTC)
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

package payments

import "time"

type PixOutput struct {
	Payload   string     `json:"payload"`
	ExpiresAt *time.Time `json:"expires_at"`
}

type CardOutput struct {
	Last4 string `json:"last4"`
	Brand string `json:"brand"`
}

type PaymentOutput struct {
	ID                string      `json:"id"`
	OrderID           string      `json:"order_id"`
	Method            string      `json:"method"`
	Status            string      `json:"status"`
	Amount            string      `json:"amount"`
	DueDate           *string     `json:"due_date"`
	FailureReason     *string     `json:"failure_reason"`
	FailureMessage    *string     `json:"failure_message"`
	Pix               *PixOutput  `json:"pix"`
	BoletoURL         *string     `json:"boleto_url"`
	InvoiceURL        *string     `json:"invoice_url"`
	Card              *CardOutput `json:"card"`
	CapturedAt        *time.Time  `json:"captured_at"`
	RefundedAt        *time.Time  `json:"refunded_at"`
	RefundRequestedAt *time.Time  `json:"refund_requested_at"`
	RefundURL         *string     `json:"refund_url"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

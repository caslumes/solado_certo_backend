package gateway

import (
	"context"
	"errors"
	"time"
)

type PaymentGateway interface {
	Name() string
	CreateCustomer(ctx context.Context, customer Customer) (id string, err error)
	CreateCharge(ctx context.Context, request ChargeRequest) (*Charge, error)
	GetCharge(ctx context.Context, id string) (*Charge, error)
	GetPixQRCode(ctx context.Context, chargeID string) (*PixQRCode, error)
	CancelCharge(ctx context.Context, id string) error
	Refund(ctx context.Context, id string, method Method) (*Charge, error)
}

type Method string

const (
	MethodPix        Method = "pix"
	MethodBoleto     Method = "boleto"
	MethodCreditCard Method = "credit_card"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusCaptured  Status = "captured"
	StatusFailed    Status = "failed"
	StatusRefunded  Status = "refunded"
	StatusCancelled Status = "cancelled"
)

var ErrUnavailable = errors.New("payment gateway unavailable")

var ErrCardRefused = errors.New("card refused")

type CardRefusedError struct {
	Message string
}

func (e *CardRefusedError) Error() string {
	return ErrCardRefused.Error() + ": " + e.Message
}

func (e *CardRefusedError) Unwrap() error {
	return ErrCardRefused
}

type Customer struct {
	Name  string
	Email string
	Phone string
	TaxID string
}

type Card struct {
	HolderName  string
	Number      string
	ExpiryMonth string
	ExpiryYear  string
	CVV         string
}

type CardHolder struct {
	Name          string
	Email         string
	Phone         string
	TaxID         string
	PostalCode    string
	AddressNumber string
}

type ChargeRequest struct {
	CustomerID  string
	Method      Method
	Amount      string
	DueDate     time.Time
	Description string
	Reference   string
	Card        *Card
	Holder      *CardHolder
	RemoteIP    string
}

type Charge struct {
	ID              string
	Status          Status
	GatewayStatus   string
	InvoiceURL      string
	BoletoURL       string
	CardLast4       string
	CardBrand       string
	RefundRequested bool
	RefundURL       string
}

type PixQRCode struct {
	Payload   string
	ExpiresAt *time.Time
}

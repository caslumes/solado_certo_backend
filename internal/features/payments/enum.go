package payments

import (
	"slices"
	"time"
)

type Method string

const (
	MethodPix        Method = "pix"
	MethodBoleto     Method = "boleto"
	MethodCreditCard Method = "credit_card"
)

func (m Method) Valid() bool {
	return m == MethodPix || m == MethodBoleto || m == MethodCreditCard
}

type Status string

const (
	StatusPending   Status = "pending"
	StatusCaptured  Status = "captured"
	StatusFailed    Status = "failed"
	StatusRefunded  Status = "refunded"
	StatusCancelled Status = "cancelled"
)

var statusTransitions = map[Status][]Status{
	StatusPending:  {StatusCaptured, StatusFailed, StatusCancelled},
	StatusFailed:   {StatusCaptured},
	StatusCaptured: {StatusRefunded},
}

func (s Status) CanBecome(next Status) bool {
	return slices.Contains(statusTransitions[s], next)
}

const (
	FailureCardRefused        = "card_refused"
	FailureGatewayUnavailable = "gateway_unavailable"
	FailureGatewayRejected    = "gateway_rejected"
	FailureOverdue            = "overdue"
)

const (
	SourceCharge      = "charge"
	SourceWebhook     = "webhook"
	SourcePoll        = "poll"
	SourceReplaced    = "replaced"
	SourceOrderClosed = "order_closed"
	SourceRefund      = "refund"
)

const (
	ReasonPaymentCaptured = "payment_captured"
	ReasonPaymentExpired  = "payment_expired"

	PaymentDays = 3

	PixDiscountPercent = 5

	MaxRefundAttempts = 10
)

var brazilTime = time.FixedZone("BRT", -3*60*60)

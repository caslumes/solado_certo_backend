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

// statusTransitions is the payment status machine (RF-017). A failed boleto can still be paid late,
// so failed → captured is allowed; anything else arriving out of order (a PENDING webhook after
// the capture, for example) is ignored.
var statusTransitions = map[Status][]Status{
	StatusPending:  {StatusCaptured, StatusFailed, StatusCancelled},
	StatusFailed:   {StatusCaptured},
	StatusCaptured: {StatusRefunded},
}

func (s Status) CanBecome(next Status) bool {
	return slices.Contains(statusTransitions[s], next)
}

// Failure reasons stored in payments.failure_reason.
const (
	FailureCardRefused        = "card_refused"
	FailureGatewayUnavailable = "gateway_unavailable"
	FailureGatewayRejected    = "gateway_rejected"
	FailureOverdue            = "overdue"
)

// Where a status change came from, recorded in the audit log.
const (
	SourceCharge   = "charge"
	SourceWebhook  = "webhook"
	SourcePoll     = "poll"
	SourceReplaced = "replaced"
	// SourceOrderClosed cancels the charge of an order that expired or was cancelled by the buyer.
	SourceOrderClosed = "order_closed"
	// SourceRefund is a refund asked by the system: the order was cancelled, or another attempt paid it.
	SourceRefund = "refund"
)

const (
	ReasonPaymentCaptured = "payment_captured"
	ReasonPaymentExpired  = "payment_expired"

	// PaymentDays is how many days after checkout an order waits for payment. Pix and boleto are
	// due on the last one, and the order expires when that day ends, in Brazil's calendar.
	PaymentDays = 3

	// PixDiscountPercent applies to the subtotal; shipping is not discounted.
	PixDiscountPercent = 5

	// MaxRefundAttempts is how many refusals of a refund the poller takes before leaving it to support.
	// Asaas refuses to refund a card right after its capture ("try again in a few moments"), and a Pix
	// while the account has no balance, so a refusal is asked again on the next rounds (RNF-013).
	MaxRefundAttempts = 10
)

// brazilTime gives due dates in the buyer's calendar (Brazil has no daylight saving time since 2019).
var brazilTime = time.FixedZone("BRT", -3*60*60)

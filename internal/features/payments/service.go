package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

type CreatePaymentUseCase interface {
	CreatePayment(ctx context.Context, userID string, orderID string, idempotencyKey string, input CreatePaymentInput) (payment *PaymentEntity, created bool, err error)
}

type ListPaymentsUseCase interface {
	ListPayments(ctx context.Context, userID string, orderID string) ([]*PaymentEntity, error)
}

type ApplyPaymentStatusUseCase interface {
	ApplyPaymentStatus(ctx context.Context, transactionID string, source string) error
}

type RefundOrderUseCase interface {
	RefundOrder(ctx context.Context, orderID string) error
}

type repository interface {
	GetOrder(ctx context.Context, id string) (*OrderEntity, error)
	GetBuyer(ctx context.Context, id string) (*BuyerEntity, error)
	SetTaxID(ctx context.Context, userID string, taxID string) error
	GetCustomerID(ctx context.Context, userID string, gateway string) (id string, found bool, err error)
	SaveCustomerID(ctx context.Context, userID string, gateway string, customerID string) error
	GetPaymentByKey(ctx context.Context, idempotencyKey string) (*PaymentEntity, error)
	GetPaymentByTransaction(ctx context.Context, gateway string, transactionID string) (*PaymentEntity, error)
	GetPayments(ctx context.Context, orderID string) ([]*PaymentEntity, error)
	GetPayment(ctx context.Context, id string) (*PaymentEntity, error)
	StartPayment(ctx context.Context, payment NewPayment) (p *PaymentEntity, created bool, err error)
	AttachCharge(ctx context.Context, paymentID string, transactionID string, details PaymentDetails) error
	ChangePaymentStatus(ctx context.Context, change StatusChange) (changed bool, err error)
	GetPendingPayments(ctx context.Context) ([]*PaymentEntity, error)
	GetExpiredOrderIDs(ctx context.Context, createdBefore time.Time) ([]string, error)
	ExpireOrder(ctx context.Context, orderID string, createdBefore time.Time) (expired bool, err error)
	GetClosedOrderPayments(ctx context.Context, startedBefore time.Time) ([]*PaymentEntity, error)
	GetPaymentsToRefund(ctx context.Context) ([]*PaymentEntity, error)
	RecordRefund(ctx context.Context, record RefundRecord) error
}

const maxIdempotencyKeyLength = 120

var (
	cardNumberPattern = regexp.MustCompile(`^\d{13,19}$`)
	cvvPattern        = regexp.MustCompile(`^\d{3,4}$`)
	monthPattern      = regexp.MustCompile(`^(0?[1-9]|1[0-2])$`)
	yearPattern       = regexp.MustCompile(`^\d{4}$`)
)

type service struct {
	r        repository
	provider gateway.PaymentGateway
	now      func() time.Time
}

func NewService(r repository, provider gateway.PaymentGateway) *service {
	return &service{r: r, provider: provider, now: time.Now}
}

// CreatePayment charges an order awaiting payment (RF-016). Repeating a key returns the payment it
// created (RNF-012). A pending charge of another attempt is cancelled first, so only one can be paid.
// A refused card is not an error: the payment comes back failed and the buyer can try again
// (UC-006 alternative flow).
func (s *service) CreatePayment(ctx context.Context, userID string, orderID string, idempotencyKey string, input CreatePaymentInput) (*PaymentEntity, bool, error) {
	if idempotencyKey == "" || len(idempotencyKey) > maxIdempotencyKeyLength {
		return nil, false, fmt.Errorf("%w: the Idempotency-Key header must have 1 to %d characters", app.ErrInvalidInput, maxIdempotencyKeyLength)
	}
	card, err := validateMethod(input)
	if err != nil {
		return nil, false, err
	}

	order, err := s.ownedOrder(ctx, userID, orderID)
	if err != nil {
		return nil, false, err
	}
	existing, err := s.r.GetPaymentByKey(ctx, idempotencyKey)
	if err == nil {
		if existing.OrderID != order.ID {
			return nil, false, app.ErrIdempotencyKeyReused
		}
		return existing, false, nil
	}
	if !errors.Is(err, app.ErrPaymentNotFound) {
		slog.ErrorContext(ctx, "GetPaymentByKey failed", "order_id", order.ID, "error", err)
		return nil, false, err
	}
	if err := s.payable(order); err != nil {
		return nil, false, err
	}

	buyer, err := s.buyerWithTaxID(ctx, userID, input.TaxID)
	if err != nil {
		return nil, false, err
	}
	if err := s.replacePending(ctx, order.ID); err != nil {
		return nil, false, err
	}
	customerID, err := s.customerID(ctx, buyer)
	if err != nil {
		return nil, false, err
	}

	discount, total := order.Totals(input.Method)
	payment, created, err := s.r.StartPayment(ctx, NewPayment{
		OrderID:        order.ID,
		CreatedBy:      userID,
		IdempotencyKey: idempotencyKey,
		Gateway:        s.provider.Name(),
		Method:         input.Method,
		Amount:         total,
		DiscountTotal:  discount,
		DueDate:        order.DueDate(),
	})
	if err != nil {
		if !errors.Is(err, app.ErrInvalidStatusTransition) && !errors.Is(err, app.ErrPaymentInProgress) {
			slog.ErrorContext(ctx, "StartPayment failed", "order_id", order.ID, "error", err)
		}
		return nil, false, err
	}
	if !created {
		return payment, false, nil
	}

	request := gateway.ChargeRequest{
		CustomerID:  customerID,
		Method:      gateway.Method(input.Method),
		Amount:      total,
		DueDate:     order.DueDate(),
		Description: "Pedido " + order.Number,
		Reference:   payment.ID,
		RemoteIP:    input.RemoteIP,
	}
	if card != nil {
		request.Card = card
		request.Holder = &gateway.CardHolder{
			Name:          buyer.Name,
			Email:         buyer.Email,
			Phone:         buyer.Phone,
			TaxID:         *buyer.TaxID,
			PostalCode:    order.PostalCode,
			AddressNumber: order.AddressNumber,
		}
	}
	if err := s.charge(ctx, payment, request); err != nil {
		return nil, false, err
	}

	charged, err := s.r.GetPayment(ctx, payment.ID)
	if err != nil {
		slog.ErrorContext(ctx, "GetPayment failed", "payment_id", payment.ID, "error", err)
		return nil, false, err
	}
	return charged, true, nil
}

// validateMethod checks the method and returns the card to charge, if any.
func validateMethod(input CreatePaymentInput) (*gateway.Card, error) {
	if !input.Method.Valid() {
		return nil, fmt.Errorf("%w: method must be pix, boleto or credit_card", app.ErrInvalidInput)
	}
	if input.Method != MethodCreditCard {
		if input.Card != nil {
			return nil, fmt.Errorf("%w: card is only accepted with credit_card", app.ErrInvalidInput)
		}
		return nil, nil
	}
	if input.Card == nil {
		return nil, fmt.Errorf("%w: card is required with credit_card", app.ErrInvalidInput)
	}

	card := gateway.Card{
		HolderName:  strings.TrimSpace(input.Card.HolderName),
		Number:      strings.ReplaceAll(input.Card.Number, " ", ""),
		ExpiryMonth: input.Card.ExpiryMonth,
		ExpiryYear:  input.Card.ExpiryYear,
		CVV:         input.Card.CVV,
	}
	var invalid []string
	if card.HolderName == "" {
		invalid = append(invalid, "holder_name")
	}
	if !cardNumberPattern.MatchString(card.Number) {
		invalid = append(invalid, "number")
	}
	if !monthPattern.MatchString(card.ExpiryMonth) {
		invalid = append(invalid, "expiry_month")
	}
	if !yearPattern.MatchString(card.ExpiryYear) {
		invalid = append(invalid, "expiry_year")
	}
	if !cvvPattern.MatchString(card.CVV) {
		invalid = append(invalid, "cvv")
	}
	if len(invalid) > 0 {
		return nil, fmt.Errorf("%w: invalid card %s", app.ErrInvalidInput, strings.Join(invalid, ", "))
	}
	return &card, nil
}

func (s *service) payable(order *OrderEntity) error {
	if order.Status != orders.OrderStatusAwaitingPayment {
		return fmt.Errorf("%w: only an order awaiting payment can be paid, this one is %s", app.ErrInvalidStatusTransition, order.Status)
	}
	if order.Expired(s.now()) {
		return app.ErrOrderExpired
	}
	if len(order.MissingPrescriptions) > 0 {
		return &app.PrescriptionRequiredError{ItemIDs: order.MissingPrescriptions}
	}
	return nil
}

// buyerWithTaxID returns the buyer with a CPF: the one saved, or the one sent, which is then saved.
func (s *service) buyerWithTaxID(ctx context.Context, userID string, taxID string) (*BuyerEntity, error) {
	buyer, err := s.r.GetBuyer(ctx, userID)
	if err != nil {
		slog.ErrorContext(ctx, "GetBuyer failed", "user_id", userID, "error", err)
		return nil, err
	}
	if buyer.TaxID != nil {
		return buyer, nil
	}
	if taxID == "" {
		return nil, app.ErrTaxIDRequired
	}
	normalized, err := helpers.NormalizeCPF(taxID)
	if err != nil {
		return nil, fmt.Errorf("%w: tax_id must be a valid CPF", app.ErrInvalidInput)
	}
	if err := s.r.SetTaxID(ctx, userID, normalized); err != nil {
		if !errors.Is(err, app.ErrTaxIDInUse) {
			slog.ErrorContext(ctx, "SetTaxID failed", "user_id", userID, "error", err)
		}
		return nil, err
	}
	buyer.TaxID = &normalized
	return buyer, nil
}

// replacePending cancels the order's pending charges before a new one is made. If one of them was
// paid in the meantime, the order is now paid and no new charge is made.
func (s *service) replacePending(ctx context.Context, orderID string) error {
	payments, err := s.r.GetPayments(ctx, orderID)
	if err != nil {
		slog.ErrorContext(ctx, "GetPayments failed", "order_id", orderID, "error", err)
		return err
	}
	for _, payment := range payments {
		if payment.Status != StatusPending {
			continue
		}
		if payment.TransactionID == nil {
			// The gateway call of this attempt never returned; it has no charge to cancel.
			if err := s.changeStatus(ctx, StatusChange{PaymentID: payment.ID, To: StatusFailed, FailureReason: FailureGatewayUnavailable, Source: SourceReplaced}); err != nil {
				return err
			}
			continue
		}

		status, err := s.cancelCharge(ctx, payment, SourceReplaced)
		if err != nil {
			return err
		}
		if status == StatusCaptured {
			return fmt.Errorf("%w: the order was already paid", app.ErrInvalidStatusTransition)
		}
	}
	return nil
}

// cancelCharge cancels a payment's pending charge, unless the gateway says it was paid or closed
// meanwhile, and returns the payment's status afterwards.
func (s *service) cancelCharge(ctx context.Context, payment *PaymentEntity, source string) (Status, error) {
	status, err := s.sync(ctx, payment, source)
	if err != nil || status != StatusPending {
		return status, err
	}
	if err := s.provider.CancelCharge(ctx, *payment.TransactionID); err != nil {
		slog.ErrorContext(ctx, "CancelCharge failed", "payment_id", payment.ID, "error", err)
		return status, app.ErrGatewayUnavailable
	}
	if err := s.changeStatus(ctx, StatusChange{PaymentID: payment.ID, To: StatusCancelled, Source: source}); err != nil {
		return status, err
	}
	return StatusCancelled, nil
}

func (s *service) customerID(ctx context.Context, buyer *BuyerEntity) (string, error) {
	name := s.provider.Name()
	id, found, err := s.r.GetCustomerID(ctx, buyer.ID, name)
	if err != nil {
		slog.ErrorContext(ctx, "GetCustomerID failed", "user_id", buyer.ID, "error", err)
		return "", err
	}
	if found {
		return id, nil
	}

	id, err = s.provider.CreateCustomer(ctx, gateway.Customer{Name: buyer.Name, Email: buyer.Email, Phone: buyer.Phone, TaxID: *buyer.TaxID})
	if err != nil {
		slog.ErrorContext(ctx, "CreateCustomer failed", "user_id", buyer.ID, "gateway", name, "error", err)
		return "", app.ErrGatewayUnavailable
	}
	if err := s.r.SaveCustomerID(ctx, buyer.ID, name, id); err != nil {
		slog.ErrorContext(ctx, "SaveCustomerID failed", "user_id", buyer.ID, "error", err)
		return "", err
	}
	return id, nil
}

// charge creates the gateway charge of a started payment and records the outcome.
func (s *service) charge(ctx context.Context, payment *PaymentEntity, request gateway.ChargeRequest) error {
	charge, err := s.provider.CreateCharge(ctx, request)
	if err != nil {
		if refused, ok := errors.AsType[*gateway.CardRefusedError](err); ok {
			return s.changeStatus(ctx, StatusChange{
				PaymentID:     payment.ID,
				To:            StatusFailed,
				FailureReason: FailureCardRefused,
				Source:        SourceCharge,
				Details:       PaymentDetails{GatewayError: refused.Message, CardLast4: last4(request.Card)},
			})
		}

		slog.ErrorContext(ctx, "CreateCharge failed", "payment_id", payment.ID, "order_id", payment.OrderID, "error", err)
		failure, result := FailureGatewayRejected, err
		if errors.Is(err, gateway.ErrUnavailable) {
			failure, result = FailureGatewayUnavailable, app.ErrGatewayUnavailable
		}
		if changeErr := s.changeStatus(ctx, StatusChange{PaymentID: payment.ID, To: StatusFailed, FailureReason: failure, Source: SourceCharge}); changeErr != nil {
			return changeErr
		}
		return result
	}

	details := chargeDetails(charge)
	if request.Method == gateway.MethodPix {
		qrCode, err := s.provider.GetPixQRCode(ctx, charge.ID)
		if err != nil {
			// The charge exists; the QR code is fetched again when the payment is listed.
			slog.WarnContext(ctx, "GetPixQRCode failed", "payment_id", payment.ID, "error", err)
		} else {
			details.PixPayload = qrCode.Payload
			details.PixExpiresAt = pixExpiry(qrCode, request.DueDate)
		}
	}
	if err := s.r.AttachCharge(ctx, payment.ID, charge.ID, details); err != nil {
		slog.ErrorContext(ctx, "AttachCharge failed", "payment_id", payment.ID, "transaction_id", charge.ID, "error", err)
		return err
	}
	if status := Status(charge.Status); status != StatusPending {
		return s.changeStatus(ctx, StatusChange{PaymentID: payment.ID, To: status, Source: SourceCharge, Details: details})
	}
	return nil
}

// pixExpiry is when the Pix code stops being payable: the gateway's expiry, but no later than the
// end of the due date, when the order expires (Asaas keeps its codes valid for a year).
func pixExpiry(qrCode *gateway.PixQRCode, dueDate time.Time) *time.Time {
	end := EndOfDay(dueDate)
	if qrCode.ExpiresAt != nil && qrCode.ExpiresAt.Before(end) {
		return qrCode.ExpiresAt
	}
	return &end
}

func chargeDetails(charge *gateway.Charge) PaymentDetails {
	return PaymentDetails{
		GatewayStatus: charge.GatewayStatus,
		InvoiceURL:    charge.InvoiceURL,
		BoletoURL:     charge.BoletoURL,
		CardLast4:     charge.CardLast4,
		CardBrand:     charge.CardBrand,
	}
}

func last4(card *gateway.Card) string {
	if card == nil || len(card.Number) < 4 {
		return ""
	}
	return card.Number[len(card.Number)-4:]
}

// ListPayments returns the order's payment attempts, newest first. Pending ones are checked with
// the gateway first, so the app sees a payment as soon as it is confirmed, even without a webhook.
func (s *service) ListPayments(ctx context.Context, userID string, orderID string) ([]*PaymentEntity, error) {
	order, err := s.ownedOrder(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	payments, err := s.r.GetPayments(ctx, order.ID)
	if err != nil {
		slog.ErrorContext(ctx, "GetPayments failed", "order_id", order.ID, "error", err)
		return nil, err
	}

	refreshed := false
	for _, payment := range payments {
		if payment.Status != StatusPending || payment.TransactionID == nil {
			continue
		}
		status, err := s.sync(ctx, payment, SourcePoll)
		if err != nil {
			// The stored status is still right, only not the newest; the poller tries again.
			slog.WarnContext(ctx, "payment status check failed", "payment_id", payment.ID, "error", err)
			continue
		}
		if status != payment.Status || (payment.Method == MethodPix && payment.Details.PixPayload == "") {
			refreshed = true
		}
	}
	if !refreshed {
		return payments, nil
	}
	payments, err = s.r.GetPayments(ctx, order.ID)
	if err != nil {
		slog.ErrorContext(ctx, "GetPayments failed", "order_id", order.ID, "error", err)
		return nil, err
	}
	return payments, nil
}

// ApplyPaymentStatus reads a charge from the gateway and applies its status. Webhooks and the poller
// call it; the payload of a webhook is not trusted, only the gateway's answer. Applying the same
// status twice changes nothing (RNF-012).
func (s *service) ApplyPaymentStatus(ctx context.Context, transactionID string, source string) error {
	payment, err := s.r.GetPaymentByTransaction(ctx, s.provider.Name(), transactionID)
	if err != nil {
		if !errors.Is(err, app.ErrPaymentNotFound) {
			slog.ErrorContext(ctx, "GetPaymentByTransaction failed", "transaction_id", transactionID, "error", err)
		}
		return err
	}
	_, err = s.sync(ctx, payment, source)
	return err
}

// sync asks the gateway for the payment's charge and applies its status, returning the status the
// payment has afterwards.
func (s *service) sync(ctx context.Context, payment *PaymentEntity, source string) (Status, error) {
	charge, err := s.provider.GetCharge(ctx, *payment.TransactionID)
	if err != nil {
		slog.ErrorContext(ctx, "GetCharge failed", "payment_id", payment.ID, "transaction_id", *payment.TransactionID, "error", err)
		return payment.Status, app.ErrGatewayUnavailable
	}

	status := Status(charge.Status)
	details := chargeDetails(charge)
	if payment.Method == MethodPix && payment.Details.PixPayload == "" && status == StatusPending {
		if qrCode, err := s.provider.GetPixQRCode(ctx, charge.ID); err == nil {
			details.PixPayload = qrCode.Payload
			details.PixExpiresAt = pixExpiry(qrCode, *payment.DueDate)
			if err := s.r.AttachCharge(ctx, payment.ID, charge.ID, details); err != nil {
				slog.ErrorContext(ctx, "AttachCharge failed", "payment_id", payment.ID, "error", err)
				return payment.Status, err
			}
		}
	}
	if status == payment.Status || !payment.Status.CanBecome(status) {
		return payment.Status, nil
	}

	change := StatusChange{PaymentID: payment.ID, To: status, Source: source, Details: details}
	if status == StatusFailed {
		change.FailureReason = FailureOverdue
	}
	if err := s.changeStatus(ctx, change); err != nil {
		return payment.Status, err
	}
	return status, nil
}

// changeStatus is the single way a payment changes status, so each change is audited (RNF-010) and
// a capture marks the order as paid in the same transaction (RNF-011).
func (s *service) changeStatus(ctx context.Context, change StatusChange) error {
	if _, err := s.r.ChangePaymentStatus(ctx, change); err != nil {
		slog.ErrorContext(ctx, "ChangePaymentStatus failed", "payment_id", change.PaymentID, "to", change.To, "error", err)
		return err
	}
	return nil
}

// ownedOrder answers 404 for another buyer's order, so order IDs can't be probed.
func (s *service) ownedOrder(ctx context.Context, userID string, id string) (*OrderEntity, error) {
	if !helpers.IsUUID(id) {
		return nil, app.ErrOrderNotFound
	}
	order, err := s.r.GetOrder(ctx, id)
	if err != nil {
		if !errors.Is(err, app.ErrOrderNotFound) {
			slog.ErrorContext(ctx, "GetOrder failed", "order_id", id, "error", err)
		}
		return nil, err
	}
	if order.BuyerID != userID {
		return nil, app.ErrOrderNotFound
	}
	return order, nil
}

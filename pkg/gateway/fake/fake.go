// Package fake is a payment gateway that runs in memory, for tests and offline development
// (PAYMENT_GATEWAY=fake). Cards are approved, except RefusedCardNumber; Pix and boleto stay pending
// until Confirm is called or, when ConfirmAfter is set, until that much time has passed. Boleto
// refunds wait for CompleteRefund, or ConfirmAfter, in the same way.
//
// Its customers and charges are lost when the server restarts, while the database keeps their ids:
// any customer id is accepted, and a charge it doesn't know is reported deleted, so the payment is
// closed and the buyer can pay again.
package fake

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

const RefusedCardNumber = "4000000000000002"

type charge struct {
	gateway.Charge
	method    gateway.Method
	dueDate   time.Time
	createdAt time.Time
	// refundRequestedAt is when a boleto's refund was asked for.
	refundRequestedAt time.Time
}

func (c *charge) refund() {
	c.Status = gateway.StatusRefunded
	c.GatewayStatus = "REFUNDED"
	c.RefundRequested = false
}

type Gateway struct {
	mu        sync.Mutex
	charges   map[string]*charge
	customers map[string]gateway.Customer
	sequence  int
	// run makes ids unique across restarts, as they are stored in the database.
	run string
	// ConfirmAfter confirms Pix and boleto charges this long after they are created (0 never does).
	ConfirmAfter time.Duration
	// Unavailable makes every call fail as if the gateway were down.
	Unavailable bool
	// RefuseRefunds makes Refund answer as a gateway that won't refund (no balance, for example).
	RefuseRefunds bool
	Now           func() time.Time
	Requests      []gateway.ChargeRequest
}

func New(confirmAfter time.Duration) *Gateway {
	return &Gateway{
		charges:      map[string]*charge{},
		customers:    map[string]gateway.Customer{},
		ConfirmAfter: confirmAfter,
		Now:          time.Now,
		run:          strconv.FormatUint(uint64(rand.Uint32()), 36),
	}
}

func (g *Gateway) Name() string {
	return "fake"
}

func (g *Gateway) nextID(prefix string) string {
	g.sequence++
	return fmt.Sprintf("%s_%s_%06d", prefix, g.run, g.sequence)
}

func (g *Gateway) CreateCustomer(_ context.Context, customer gateway.Customer) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Unavailable {
		return "", gateway.ErrUnavailable
	}
	id := g.nextID("cus")
	g.customers[id] = customer
	return id, nil
}

func (g *Gateway) CreateCharge(_ context.Context, request gateway.ChargeRequest) (*gateway.Charge, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Unavailable {
		return nil, gateway.ErrUnavailable
	}
	if request.CustomerID == "" {
		return nil, fmt.Errorf("fake gateway: missing customer")
	}
	// The card is not kept, as a real gateway client wouldn't.
	recorded := request
	recorded.Card = nil
	g.Requests = append(g.Requests, recorded)

	c := &charge{method: request.Method, dueDate: request.DueDate, createdAt: g.Now()}
	c.ID = g.nextID("pay")
	c.Status = gateway.StatusPending
	c.GatewayStatus = "PENDING"
	c.InvoiceURL = "https://fake.gateway/i/" + c.ID
	switch request.Method {
	case gateway.MethodBoleto:
		c.BoletoURL = "https://fake.gateway/b/" + c.ID
	case gateway.MethodCreditCard:
		if request.Card.Number == RefusedCardNumber {
			return nil, &gateway.CardRefusedError{Message: "Transação não autorizada"}
		}
		c.Status = gateway.StatusCaptured
		c.GatewayStatus = "CONFIRMED"
		c.CardLast4 = request.Card.Number[len(request.Card.Number)-4:]
		c.CardBrand = "VISA"
	}
	g.charges[c.ID] = c
	copied := c.Charge
	return &copied, nil
}

func (g *Gateway) GetCharge(_ context.Context, id string) (*gateway.Charge, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Unavailable {
		return nil, gateway.ErrUnavailable
	}
	c, ok := g.charges[id]
	if !ok {
		return &gateway.Charge{ID: id, Status: gateway.StatusCancelled, GatewayStatus: "DELETED"}, nil
	}
	if c.Status == gateway.StatusPending && g.ConfirmAfter > 0 && !g.Now().Before(c.createdAt.Add(g.ConfirmAfter)) {
		c.Status = gateway.StatusCaptured
		c.GatewayStatus = "RECEIVED"
	}
	if c.RefundRequested && g.ConfirmAfter > 0 && !g.Now().Before(c.refundRequestedAt.Add(g.ConfirmAfter)) {
		c.refund()
	}
	copied := c.Charge
	return &copied, nil
}

func (g *Gateway) GetPixQRCode(_ context.Context, chargeID string) (*gateway.PixQRCode, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c, err := g.find(chargeID)
	if err != nil {
		return nil, err
	}
	if c.method != gateway.MethodPix {
		return nil, fmt.Errorf("fake gateway: charge %q is not a Pix charge", chargeID)
	}
	// The code is valid until the end of the due date.
	expires := c.dueDate.Add(24 * time.Hour)
	return &gateway.PixQRCode{Payload: "00020101021226800014br.gov.bcb.pix" + c.ID + "5204000053039865802BR6304FAKE", ExpiresAt: &expires}, nil
}

func (g *Gateway) CancelCharge(_ context.Context, id string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Unavailable {
		return gateway.ErrUnavailable
	}
	c, ok := g.charges[id]
	if !ok {
		return nil
	}
	if c.Status != gateway.StatusPending {
		return fmt.Errorf("fake gateway: charge %q is %s and can't be cancelled", id, c.Status)
	}
	c.Status = gateway.StatusCancelled
	c.GatewayStatus = "DELETED"
	return nil
}

// Refund returns a Pix or card charge at once. A boleto's refund waits for the payer's bank account,
// given at its RefundURL: CompleteRefund stands for that, or ConfirmAfter when set. A charge lost in
// a restart is reported refunded, so offline development can go on.
func (g *Gateway) Refund(_ context.Context, id string, method gateway.Method) (*gateway.Charge, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.Unavailable {
		return nil, gateway.ErrUnavailable
	}
	c, ok := g.charges[id]
	if !ok {
		return &gateway.Charge{ID: id, Status: gateway.StatusRefunded, GatewayStatus: "REFUNDED"}, nil
	}
	if g.RefuseRefunds {
		return nil, fmt.Errorf("fake gateway: refund of %q refused", id)
	}
	if c.Status != gateway.StatusCaptured || c.RefundRequested {
		return nil, fmt.Errorf("fake gateway: charge %q is %s and can't be refunded", id, c.GatewayStatus)
	}
	if method == gateway.MethodBoleto {
		c.RefundRequested = true
		c.GatewayStatus = "REFUND_REQUESTED"
		c.RefundURL = "https://fake.gateway/r/" + c.ID
		c.refundRequestedAt = g.Now()
	} else {
		c.refund()
	}
	copied := c.Charge
	return &copied, nil
}

// CompleteRefund refunds a boleto whose refund was requested, as the payer giving a bank account.
func (g *Gateway) CompleteRefund(id string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.charges[id]; ok && c.RefundRequested {
		c.refund()
	}
}

// Confirm pays a pending charge, as the buyer paying the Pix or the boleto.
func (g *Gateway) Confirm(id string) {
	g.set(id, gateway.StatusCaptured, "RECEIVED")
}

// Expire makes a pending charge overdue.
func (g *Gateway) Expire(id string) {
	g.set(id, gateway.StatusFailed, "OVERDUE")
}

func (g *Gateway) set(id string, status gateway.Status, gatewayStatus string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if c, ok := g.charges[id]; ok {
		c.Status = status
		c.GatewayStatus = gatewayStatus
	}
}

func (g *Gateway) find(id string) (*charge, error) {
	if g.Unavailable {
		return nil, gateway.ErrUnavailable
	}
	c, ok := g.charges[id]
	if !ok {
		return nil, fmt.Errorf("fake gateway: unknown charge %q", id)
	}
	return c, nil
}

package fake

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/pkg/gateway"
)

func newCharge(t *testing.T, g *Gateway, method gateway.Method) *gateway.Charge {
	t.Helper()
	ctx := context.Background()
	customer, err := g.CreateCustomer(ctx, gateway.Customer{Name: "Maria", TaxID: "52998224725"})
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	charge, err := g.CreateCharge(ctx, gateway.ChargeRequest{CustomerID: customer, Method: method, Amount: "10.00"})
	if err != nil {
		t.Fatalf("CreateCharge: %v", err)
	}
	return charge
}

func TestConfirmAfter(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	g := New(10 * time.Second)
	g.Now = func() time.Time { return now }
	charge := newCharge(t, g, gateway.MethodPix)

	if got, _ := g.GetCharge(context.Background(), charge.ID); got.Status != gateway.StatusPending {
		t.Errorf("right away = %s, want pending", got.Status)
	}
	now = now.Add(10 * time.Second)
	if got, _ := g.GetCharge(context.Background(), charge.ID); got.Status != gateway.StatusCaptured || got.GatewayStatus != "RECEIVED" {
		t.Errorf("after 10 s = %s, want captured", got.Status)
	}
}

func TestPixQRCode(t *testing.T) {
	g := New(0)
	pix := newCharge(t, g, gateway.MethodPix)
	boleto := newCharge(t, g, gateway.MethodBoleto)

	if qr, err := g.GetPixQRCode(context.Background(), pix.ID); err != nil || qr.Payload == "" || qr.ExpiresAt == nil {
		t.Errorf("Pix QR code = %+v, %v", qr, err)
	}
	if _, err := g.GetPixQRCode(context.Background(), boleto.ID); err == nil {
		t.Error("a boleto has no Pix code")
	}
	if boleto.BoletoURL == "" {
		t.Error("boleto without a link")
	}
}

func TestCancelAndRefund(t *testing.T) {
	ctx := context.Background()
	g := New(0)
	pending := newCharge(t, g, gateway.MethodBoleto)

	if _, err := g.Refund(ctx, pending.ID, gateway.MethodBoleto); err == nil {
		t.Error("a pending charge was refunded")
	}
	if err := g.CancelCharge(ctx, pending.ID); err != nil {
		t.Fatalf("CancelCharge: %v", err)
	}
	if err := g.CancelCharge(ctx, pending.ID); err == nil {
		t.Error("a cancelled charge was cancelled again")
	}

	paid := newCharge(t, g, gateway.MethodPix)
	g.Confirm(paid.ID)
	refunded, err := g.Refund(ctx, paid.ID, gateway.MethodPix)
	if err != nil || refunded.Status != gateway.StatusRefunded {
		t.Errorf("Refund = %+v, %v", refunded, err)
	}
}

func TestRefundBoleto(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	g := New(0)
	g.Now = func() time.Time { return now }
	boleto := newCharge(t, g, gateway.MethodBoleto)
	g.Confirm(boleto.ID)

	requested, err := g.Refund(ctx, boleto.ID, gateway.MethodBoleto)
	if err != nil || requested.Status != gateway.StatusCaptured || !requested.RefundRequested || requested.RefundURL == "" {
		t.Fatalf("Refund = %+v, %v, want captured with a refund URL", requested, err)
	}
	if _, err := g.Refund(ctx, boleto.ID, gateway.MethodBoleto); err == nil {
		t.Error("a refund was requested twice")
	}
	g.CompleteRefund(boleto.ID)
	if got, _ := g.GetCharge(ctx, boleto.ID); got.Status != gateway.StatusRefunded || got.RefundRequested {
		t.Errorf("after CompleteRefund = %+v, want refunded", got)
	}

	g.ConfirmAfter = time.Minute
	other := newCharge(t, g, gateway.MethodBoleto)
	g.Confirm(other.ID)
	if _, err := g.Refund(ctx, other.ID, gateway.MethodBoleto); err != nil {
		t.Fatalf("Refund: %v", err)
	}
	now = now.Add(time.Minute)
	if got, _ := g.GetCharge(ctx, other.ID); got.Status != gateway.StatusRefunded {
		t.Errorf("after ConfirmAfter = %s, want refunded", got.Status)
	}
}

// A charge lost in a restart is reported refunded, so the order's refund doesn't stay stuck.
func TestRefundUnknownCharge(t *testing.T) {
	g := New(0)
	if got, err := g.Refund(context.Background(), "pay_lost", gateway.MethodPix); err != nil || got.Status != gateway.StatusRefunded {
		t.Errorf("Refund = %+v, %v", got, err)
	}
	g.Unavailable = true
	if _, err := g.Refund(context.Background(), "pay_lost", gateway.MethodPix); !errors.Is(err, gateway.ErrUnavailable) {
		t.Errorf("err = %v, want unavailable", err)
	}
}

// A charge made before the server restarted is gone from memory; it can no longer be paid.
func TestUnknownChargesAreDeleted(t *testing.T) {
	ctx := context.Background()
	g := New(0)

	lost, err := g.GetCharge(ctx, "pay_lost")
	if err != nil || lost.ID != "pay_lost" || lost.Status != gateway.StatusCancelled {
		t.Errorf("GetCharge = %+v, %v, want cancelled", lost, err)
	}
	if err := g.CancelCharge(ctx, "pay_lost"); err != nil {
		t.Errorf("CancelCharge: %v", err)
	}
	if _, err := g.GetPixQRCode(ctx, "pay_lost"); err == nil {
		t.Error("a QR code was made for an unknown charge")
	}

	// Customers too: one created before the restart still works, and ids aren't reused.
	charge, err := g.CreateCharge(ctx, gateway.ChargeRequest{CustomerID: "cus_lost", Method: gateway.MethodBoleto, Amount: "10.00"})
	if err != nil || charge.Status != gateway.StatusPending {
		t.Errorf("CreateCharge = %+v, %v", charge, err)
	}
	if other := New(0); other.nextID("pay") == g.nextID("pay") {
		t.Error("two runs made the same id")
	}
}

func TestCardCharges(t *testing.T) {
	ctx := context.Background()
	g := New(0)
	customer, _ := g.CreateCustomer(ctx, gateway.Customer{Name: "Maria"})
	request := gateway.ChargeRequest{CustomerID: customer, Method: gateway.MethodCreditCard, Amount: "10.00", Card: &gateway.Card{Number: "4111111111111111"}}

	approved, err := g.CreateCharge(ctx, request)
	if err != nil || approved.Status != gateway.StatusCaptured || approved.CardLast4 != "1111" {
		t.Errorf("approved card = %+v, %v", approved, err)
	}
	if g.Requests[0].Card != nil {
		t.Error("the card was kept")
	}

	request.Card = &gateway.Card{Number: RefusedCardNumber}
	if _, err := g.CreateCharge(ctx, request); !errors.Is(err, gateway.ErrCardRefused) {
		t.Errorf("refused card: error = %v, want ErrCardRefused", err)
	}

	request.CustomerID = ""
	if _, err := g.CreateCharge(ctx, request); err == nil {
		t.Error("a charge was made without a customer")
	}
}

func TestUnavailable(t *testing.T) {
	ctx := context.Background()
	g := New(0)
	charge := newCharge(t, g, gateway.MethodPix)
	g.Unavailable = true

	if _, err := g.CreateCustomer(ctx, gateway.Customer{}); !errors.Is(err, gateway.ErrUnavailable) {
		t.Errorf("CreateCustomer: error = %v", err)
	}
	if _, err := g.CreateCharge(ctx, gateway.ChargeRequest{}); !errors.Is(err, gateway.ErrUnavailable) {
		t.Errorf("CreateCharge: error = %v", err)
	}
	if _, err := g.GetCharge(ctx, charge.ID); !errors.Is(err, gateway.ErrUnavailable) {
		t.Errorf("GetCharge: error = %v", err)
	}
}

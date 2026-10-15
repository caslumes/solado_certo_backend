package payments

import (
	"testing"
	"time"
)

func TestStatusCanBecome(t *testing.T) {
	all := []Status{StatusPending, StatusCaptured, StatusFailed, StatusRefunded, StatusCancelled}
	allowed := map[[2]Status]bool{
		{StatusPending, StatusCaptured}:  true,
		{StatusPending, StatusFailed}:    true,
		{StatusPending, StatusCancelled}: true,
		{StatusFailed, StatusCaptured}:   true,
		{StatusCaptured, StatusRefunded}: true,
	}
	for _, from := range all {
		for _, to := range all {
			if got := from.CanBecome(to); got != allowed[[2]Status{from, to}] {
				t.Errorf("%s.CanBecome(%s) = %v", from, to, got)
			}
		}
	}
}

func TestMethodValid(t *testing.T) {
	for _, m := range []Method{MethodPix, MethodBoleto, MethodCreditCard} {
		if !m.Valid() {
			t.Errorf("%s should be valid", m)
		}
	}
	for _, m := range []Method{"", "debit_card", "PIX"} {
		if m.Valid() {
			t.Errorf("%q should be invalid", m)
		}
	}
}

func TestOrderTotals(t *testing.T) {
	cases := []struct {
		subtotal, shipping string
		method             Method
		discount, total    string
	}{
		{"100.00", "20.00", MethodPix, "5.00", "115.00"},
		{"89.90", "19.90", MethodPix, "4.50", "105.30"},
		{"0.10", "0.00", MethodPix, "0.01", "0.09"},
		{"100.00", "20.00", MethodBoleto, "0.00", "120.00"},
		{"100.00", "20.00", MethodCreditCard, "0.00", "120.00"},
	}
	for _, c := range cases {
		order := OrderEntity{Subtotal: c.subtotal, ShippingTotal: c.shipping}
		discount, total := order.Totals(c.method)
		if discount != c.discount || total != c.total {
			t.Errorf("Totals(%s) of %s + %s = %s, %s, want %s, %s", c.method, c.subtotal, c.shipping, discount, total, c.discount, c.total)
		}
	}
}

func TestOrderPaymentWindow(t *testing.T) {
	// 22:30 in Brazil on Oct 14 is already Oct 15 in UTC; the due date follows Brazil's calendar.
	created := time.Date(2026, 10, 15, 1, 30, 0, 0, time.UTC)
	order := OrderEntity{CreatedAt: created}

	if due := order.DueDate(); !due.Equal(time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("DueDate = %v, want 2026-10-17", due)
	}
	if order.Expired(created.Add(PaymentWindow - time.Second)) {
		t.Error("expired before the window ended")
	}
	if !order.Expired(created.Add(PaymentWindow)) {
		t.Error("not expired at the end of the window")
	}
}

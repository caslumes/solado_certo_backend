package payments

import (
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/pkg/gateway"
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
	// The order expires when Oct 17 ends in Brazil, 03:00 UTC on Oct 18.
	end := time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)
	if order.Expired(end.Add(-time.Second)) {
		t.Error("expired before the due date ended")
	}
	if !order.Expired(end) {
		t.Error("not expired when the due date ended")
	}
	// The poller's cutoff agrees with Expired at both moments.
	if !created.Before(expiredBefore(end)) || created.Before(expiredBefore(end.Add(-time.Second))) {
		t.Errorf("expiredBefore = %v, %v", expiredBefore(end), expiredBefore(end.Add(-time.Second)))
	}
}

func TestPixExpiry(t *testing.T) {
	due := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 10, 19, 3, 0, 0, 0, time.UTC)
	nextYear := time.Date(2027, 10, 19, 2, 59, 59, 0, time.UTC)
	sooner := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	tests := map[string]struct {
		expires *time.Time
		want    time.Time
	}{
		"a year later is capped": {&nextYear, end},
		"sooner is kept":         {&sooner, sooner},
		"missing is the end":     {nil, end},
	}
	for name, tt := range tests {
		if got := pixExpiry(&gateway.PixQRCode{ExpiresAt: tt.expires}, due); !got.Equal(tt.want) {
			t.Errorf("%s: pixExpiry = %v, want %v", name, got, tt.want)
		}
	}
}

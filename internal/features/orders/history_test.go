package orders

import (
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

func TestHistoryFilterCoversWholeDaysInBrasilia(t *testing.T) {
	query, err := HistoryFilter{Status: OrderStatusPaid, From: "2026-10-01", To: "2026-10-31"}.query(sellerStatuses)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	since := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	until := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	if query.Status != OrderStatusPaid || !query.Since.Equal(since) || !query.Until.Equal(until) {
		t.Errorf("query = %v, %v, %v; want paid from %v until %v", query.Status, query.Since, query.Until, since, until)
	}

	query, err = HistoryFilter{From: "2026-10-21", To: "2026-10-21"}.query(buyerStatuses)
	if err != nil || query.Until.Sub(*query.Since) != 24*time.Hour {
		t.Errorf("one day: %v to %v, %v", query.Since, query.Until, err)
	}

	query, err = HistoryFilter{}.query(buyerStatuses)
	if err != nil || query.Status != "" || query.Since != nil || query.Until != nil {
		t.Errorf("no filter = %+v, %v", query, err)
	}
}

func TestHistoryFilterRejects(t *testing.T) {
	tests := []struct {
		filter   HistoryFilter
		statuses []OrderStatus
	}{
		{HistoryFilter{Status: OrderStatusAwaitingPayment}, sellerStatuses},
		{HistoryFilter{Status: "lost"}, buyerStatuses},
		{HistoryFilter{From: "2026-10-32"}, buyerStatuses},
		{HistoryFilter{To: "2026-10-21T10:00:00Z"}, buyerStatuses},
		{HistoryFilter{From: "2026-10-22", To: "2026-10-21"}, buyerStatuses},
	}
	for _, tt := range tests {
		if _, err := tt.filter.query(tt.statuses); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("%+v: error = %v, want ErrInvalidInput", tt.filter, err)
		}
	}
}

package orders

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const (
	testOrderID  = "00000000-0000-0000-0000-000000000801"
	otherOrderID = "00000000-0000-0000-0000-000000000802"
)

func TestOrderStatusMachine(t *testing.T) {
	allowed := map[OrderStatus][]OrderStatus{
		OrderStatusAwaitingPayment: {OrderStatusPaid, OrderStatusCancelled},
		OrderStatusPaid:            {OrderStatusProcessing, OrderStatusCancelled},
		OrderStatusProcessing:      {OrderStatusShipped},
		OrderStatusShipped:         {OrderStatusDelivered},
		OrderStatusDelivered:       {OrderStatusReturned},
	}
	all := []OrderStatus{OrderStatusAwaitingPayment, OrderStatusPaid, OrderStatusProcessing, OrderStatusShipped,
		OrderStatusDelivered, OrderStatusCancelled, OrderStatusReturned}
	for _, from := range all {
		for _, to := range all {
			want := false
			for _, next := range allowed[from] {
				want = want || next == to
			}
			if got := from.CanBecome(to); got != want {
				t.Errorf("%s → %s = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestOrderDetailOutput(t *testing.T) {
	reason := "checkout"
	from := OrderStatusAwaitingPayment
	detail := &OrderDetail{
		Order: &OrderEntity{ID: testOrderID, Status: OrderStatusCancelled, Items: []OrderItemEntity{}},
		History: []StatusChangeEntity{
			{To: OrderStatusAwaitingPayment, Reason: &reason},
			{From: &from, To: OrderStatusCancelled},
		},
	}

	output := detail.ToOutput()

	if output.ID != testOrderID || output.CanCancel || len(output.History) != 2 {
		t.Fatalf("output = %+v", output)
	}
	if output.History[0].From != nil || output.History[0].To != "awaiting_payment" || *output.History[0].Reason != "checkout" {
		t.Errorf("first entry = %+v", output.History[0])
	}
	if output.History[1].From == nil || *output.History[1].From != "awaiting_payment" || output.History[1].Reason != nil {
		t.Errorf("second entry = %+v", output.History[1])
	}

	detail.Order.Status = OrderStatusAwaitingPayment
	if !detail.ToOutput().CanCancel {
		t.Error("an order awaiting payment can be cancelled")
	}
	detail.Order.Status = OrderStatusPaid
	if !detail.ToOutput().CanCancel {
		t.Error("a paid order can be cancelled")
	}
	detail.Order.Status = OrderStatusShipped
	if detail.ToOutput().CanCancel {
		t.Error("a shipped order can't be cancelled")
	}
}

func ordersFixture(count int) (*fakeRepository, *service) {
	r := newFakeRepository()
	for i := range count {
		r.placed = append(r.placed, &OrderEntity{ID: fmt.Sprintf("00000000-0000-0000-0000-%012d", 801+i), BuyerID: testBuyerID, Status: OrderStatusAwaitingPayment})
	}
	r.placed = append(r.placed, &OrderEntity{ID: "00000000-0000-0000-0000-000000000999", BuyerID: "buyer-2", Status: OrderStatusAwaitingPayment})
	return r, NewService(r)
}

func TestListOrdersPaginates(t *testing.T) {
	_, s := ordersFixture(25)

	tests := []struct {
		page, limit         uint
		wantItems, wantPage uint
		wantPages           uint
	}{
		{0, 0, 10, 1, 3},
		{3, 10, 5, 3, 3},
		{2, 20, 5, 2, 2},
		{4, 10, 0, 4, 3},
	}
	for _, tt := range tests {
		page, err := s.ListOrders(context.Background(), testBuyerID, HistoryFilter{}, tt.page, tt.limit)
		if err != nil {
			t.Fatalf("ListOrders: %v", err)
		}
		if uint(len(page.Items)) != tt.wantItems || page.CurrentPage != tt.wantPage || page.TotalPages != tt.wantPages || page.TotalItems != 25 {
			t.Errorf("page %d limit %d: items %d, page %d of %d (%d total)", tt.page, tt.limit, len(page.Items), page.CurrentPage, page.TotalPages, page.TotalItems)
		}
	}

	output := (&OrderPage{Items: []*OrderEntity{}, CurrentPage: 2, TotalPages: 3}).ToOutput()
	if !output.HasNext || !output.HasPrevious || output.Items == nil {
		t.Errorf("output = %+v", output)
	}
}

func TestListOrdersWithoutOrders(t *testing.T) {
	_, s := ordersFixture(0)
	page, err := s.ListOrders(context.Background(), testBuyerID, HistoryFilter{}, 1, 10)
	if err != nil || len(page.Items) != 0 || page.TotalPages != 0 {
		t.Errorf("page = %+v, error = %v", page, err)
	}
}

func TestListOrdersRejectsAndPropagates(t *testing.T) {
	r, s := ordersFixture(1)
	if _, err := s.ListOrders(context.Background(), testBuyerID, HistoryFilter{}, 1, maxOrdersPerPage+1); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("limit too big: error = %v, want ErrInvalidInput", err)
	}
	for _, filter := range []HistoryFilter{
		{Status: "lost"},
		{From: "2026-13-01"},
		{To: "21/10/2026"},
		{From: "2026-10-21", To: "2026-10-20"},
	} {
		if _, err := s.ListOrders(context.Background(), testBuyerID, filter, 1, 10); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("filter %+v: error = %v, want ErrInvalidInput", filter, err)
		}
	}
	r.err = errors.New("db down")
	if _, err := s.ListOrders(context.Background(), testBuyerID, HistoryFilter{}, 1, 10); err == nil {
		t.Error("want error")
	}
}

func TestGetOrder(t *testing.T) {
	r, s := ordersFixture(1)
	reason := "checkout"
	r.history = map[string][]StatusChangeEntity{testOrderID: {{To: OrderStatusAwaitingPayment, Reason: &reason}}}

	detail, err := s.GetOrder(context.Background(), testBuyerID, testOrderID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if detail.Order.ID != testOrderID || len(detail.History) != 1 {
		t.Errorf("detail = %+v", detail)
	}
}

func TestGetOrderOnlyForItsBuyer(t *testing.T) {
	r, s := ordersFixture(1)
	tests := []struct {
		name   string
		userID string
		id     string
	}{
		{"another buyer", "buyer-2", testOrderID},
		{"unknown order", testBuyerID, otherOrderID},
		{"bad ID", testBuyerID, "not-a-uuid"},
	}
	for _, tt := range tests {
		if _, err := s.GetOrder(context.Background(), tt.userID, tt.id); !errors.Is(err, app.ErrOrderNotFound) {
			t.Errorf("%s: error = %v, want ErrOrderNotFound", tt.name, err)
		}
	}

	r.err = errors.New("db down")
	if _, err := s.GetOrder(context.Background(), testBuyerID, testOrderID); err == nil || errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("db error = %v", err)
	}
}

func TestCancelOrder(t *testing.T) {
	r, s := ordersFixture(1)

	detail, err := s.CancelOrder(context.Background(), testBuyerID, testOrderID)
	if err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if detail.Order.Status != OrderStatusCancelled || len(detail.History) != 1 {
		t.Errorf("detail = %+v", detail)
	}
	want := StatusChange{OrderID: testOrderID, ChangedBy: testBuyerID, From: OrderStatusAwaitingPayment, To: OrderStatusCancelled, Reason: ReasonBuyerCancelled, ReleaseStock: true}
	if len(r.changes) != 1 || r.changes[0] != want {
		t.Errorf("changes = %+v, want %+v", r.changes, want)
	}
}

func TestCancelOrderRejects(t *testing.T) {
	r, s := ordersFixture(1)
	r.placed[0].Status = OrderStatusShipped

	if _, err := s.CancelOrder(context.Background(), testBuyerID, testOrderID); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("shipped order: error = %v, want ErrInvalidStatusTransition", err)
	}
	if _, err := s.CancelOrder(context.Background(), "buyer-2", testOrderID); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("another buyer: error = %v, want ErrOrderNotFound", err)
	}
	if len(r.changes) != 0 {
		t.Errorf("changes = %+v, want none", r.changes)
	}
}

type fakeRefunder struct {
	orderIDs []string
	err      error
}

func (f *fakeRefunder) RefundOrder(_ context.Context, orderID string) error {
	f.orderIDs = append(f.orderIDs, orderID)
	return f.err
}

// A paid order can be cancelled; it is refunded after the cancellation is saved, and a refund that
// fails doesn't undo it (the payments poller tries again).
func TestCancelPaidOrderRefundsIt(t *testing.T) {
	for _, refundErr := range []error{nil, app.ErrGatewayUnavailable} {
		r, s := ordersFixture(1)
		r.placed[0].Status = OrderStatusPaid
		refunder := &fakeRefunder{err: refundErr}
		s.SetRefunder(refunder)

		detail, err := s.CancelOrder(context.Background(), testBuyerID, testOrderID)
		if err != nil {
			t.Fatalf("CancelOrder (refund error %v): %v", refundErr, err)
		}
		if detail.Order.Status != OrderStatusCancelled || len(r.changes) != 1 || r.changes[0].From != OrderStatusPaid || !r.changes[0].ReleaseStock {
			t.Errorf("detail = %+v, changes = %+v", detail.Order, r.changes)
		}
		if len(refunder.orderIDs) != 1 || refunder.orderIDs[0] != testOrderID {
			t.Errorf("refunded = %v, want the order", refunder.orderIDs)
		}
	}
}

func TestCancelUnpaidOrderDoesNotRefund(t *testing.T) {
	_, s := ordersFixture(1)
	refunder := &fakeRefunder{}
	s.SetRefunder(refunder)

	if _, err := s.CancelOrder(context.Background(), testBuyerID, testOrderID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if len(refunder.orderIDs) != 0 {
		t.Errorf("refunded = %v, want none", refunder.orderIDs)
	}
}

func TestCancelOrderWhenTheOrderChangedMeanwhile(t *testing.T) {
	for _, changeErr := range []error{app.ErrInvalidStatusTransition, errors.New("db down")} {
		r, s := ordersFixture(1)
		r.changeErr = changeErr
		if _, err := s.CancelOrder(context.Background(), testBuyerID, testOrderID); !errors.Is(err, changeErr) {
			t.Errorf("error = %v, want %v", err, changeErr)
		}
	}
}

func TestChangeStatusFollowsTheMachine(t *testing.T) {
	r, s := ordersFixture(1)
	err := s.changeStatus(context.Background(), StatusChange{OrderID: testOrderID, From: OrderStatusAwaitingPayment, To: OrderStatusShipped})
	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("error = %v, want ErrInvalidStatusTransition", err)
	}
	if len(r.changes) != 0 {
		t.Error("the repository was called")
	}
}

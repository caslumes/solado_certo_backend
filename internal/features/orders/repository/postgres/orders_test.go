package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
)

// placeOrders checks out the setup's cart and returns the insoles order (2 units) and the socks order (3 units).
func (s checkoutSetup) placeOrders(t *testing.T) (*orders.OrderEntity, *orders.OrderEntity) {
	t.Helper()
	id, _, err := s.r.CreateCheckout(context.Background(), s.buyer.ID, "key-"+s.buyer.ID, s.planFor(t, s.buyer.ID, s.addressID))
	if err != nil {
		t.Fatalf("CreateCheckout: %v", err)
	}
	placed, err := s.r.GetOrdersByCheckout(context.Background(), id)
	if err != nil {
		t.Fatalf("GetOrdersByCheckout: %v", err)
	}
	if placed[0].SellerID == s.seller.ID {
		return placed[0], placed[1]
	}
	return placed[1], placed[0]
}

func cancellation(orderID string, userID string) orders.StatusChange {
	return orders.StatusChange{
		OrderID:      orderID,
		ChangedBy:    userID,
		From:         orders.OrderStatusAwaitingPayment,
		To:           orders.OrderStatusCancelled,
		Reason:       orders.ReasonBuyerCancelled,
		ReleaseStock: true,
	}
}

func TestOrdersByBuyer(t *testing.T) {
	s := newCheckoutSetup(t)
	insolesOrder, socksOrder := s.placeOrders(t)
	s.exec(t, "UPDATE orders SET created_at = created_at - interval '1 day' WHERE id = $1", socksOrder.ID)

	total, err := s.r.CountOrdersByBuyer(context.Background(), s.buyer.ID)
	if err != nil || total != 2 {
		t.Fatalf("count = %d, error = %v", total, err)
	}
	first, err := s.r.GetOrdersByBuyer(context.Background(), s.buyer.ID, 1, 0)
	if err != nil {
		t.Fatalf("GetOrdersByBuyer: %v", err)
	}
	second, err := s.r.GetOrdersByBuyer(context.Background(), s.buyer.ID, 1, 1)
	if err != nil {
		t.Fatalf("GetOrdersByBuyer: %v", err)
	}
	if len(first) != 1 || first[0].ID != insolesOrder.ID || len(second) != 1 || second[0].ID != socksOrder.ID {
		t.Errorf("pages = %v, %v, want the newest first", first, second)
	}
	if len(first[0].Items) != 1 || first[0].Items[0].Quantity != 2 {
		t.Errorf("items = %+v", first[0].Items)
	}

	total, err = s.r.CountOrdersByBuyer(context.Background(), s.seller.ID)
	none, _ := s.r.GetOrdersByBuyer(context.Background(), s.seller.ID, 10, 0)
	if err != nil || total != 0 || none == nil || len(none) != 0 {
		t.Errorf("another user: count = %d, orders = %v, error = %v", total, none, err)
	}
}

func TestGetOrder(t *testing.T) {
	s := newCheckoutSetup(t)
	insolesOrder, _ := s.placeOrders(t)

	got, err := s.r.GetOrder(context.Background(), insolesOrder.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if got.ID != insolesOrder.ID || got.BuyerID != s.buyer.ID || got.Total != "199.70" || len(got.Items) != 1 {
		t.Errorf("order = %+v", got)
	}
	if _, err := s.r.GetOrder(context.Background(), unknownID); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("unknown: error = %v, want ErrOrderNotFound", err)
	}
}

func TestGetOrderItemsCarryTheirReview(t *testing.T) {
	s := newCheckoutSetup(t)
	insolesOrder, _ := s.placeOrders(t)
	item := insolesOrder.Items[0]
	if item.ID == "" || item.Review != nil {
		t.Fatalf("item = %+v, want an id and no review", item)
	}

	s.exec(t, "INSERT INTO reviews (order_item_id, reviewer_id, product_rating, seller_rating, comfort_rating, adaptation_feedback, created_at, updated_at) "+
		"VALUES ($1, $2, 4, 5, 3, 'Adaptei bem', now(), now())", item.ID, s.buyer.ID)

	got, err := s.r.GetOrder(context.Background(), insolesOrder.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	review := got.Items[0].Review
	if review == nil || review.ProductRating != 4 || review.SellerRating != 5 || *review.ComfortRating != 3 ||
		*review.AdaptationFeedback != "Adaptei bem" || review.Comment != nil || review.CreatedAt.IsZero() {
		t.Errorf("review = %+v", review)
	}
}

func TestCancelReleasesStockAndRecordsTheChange(t *testing.T) {
	s := newCheckoutSetup(t)
	insolesOrder, socksOrder := s.placeOrders(t)

	if err := s.r.ChangeOrderStatus(context.Background(), cancellation(insolesOrder.ID, s.buyer.ID)); err != nil {
		t.Fatalf("ChangeOrderStatus: %v", err)
	}

	got, err := s.r.GetOrder(context.Background(), insolesOrder.ID)
	if err != nil || got.Status != orders.OrderStatusCancelled || !got.UpdatedAt.After(got.CreatedAt) {
		t.Errorf("order = %+v, error = %v", got, err)
	}
	if n := s.count(t, "SELECT stock FROM advertisements WHERE id = $1", s.insoles.ID); n != 5 {
		t.Errorf("insoles stock = %d, want 5 (2 back)", n)
	}
	if n := s.count(t, "SELECT stock FROM advertisements WHERE id = $1", s.socks.ID); n != 7 {
		t.Errorf("socks stock = %d, want 7 (other order untouched)", n)
	}
	if other, _ := s.r.GetOrder(context.Background(), socksOrder.ID); other.Status != orders.OrderStatusAwaitingPayment {
		t.Errorf("socks order = %s", other.Status)
	}

	history, err := s.r.GetStatusHistory(context.Background(), insolesOrder.ID)
	if err != nil {
		t.Fatalf("GetStatusHistory: %v", err)
	}
	if len(history) != 2 || history[0].From != nil || history[0].To != orders.OrderStatusAwaitingPayment ||
		history[1].From == nil || *history[1].From != orders.OrderStatusAwaitingPayment || history[1].To != orders.OrderStatusCancelled ||
		history[1].Reason == nil || *history[1].Reason != orders.ReasonBuyerCancelled {
		t.Errorf("history = %+v", history)
	}
	if n := s.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND user_id = $2 AND context->>'to' = 'cancelled' "+
		"AND context->>'from' = 'awaiting_payment' AND (context->>'stock_released')::boolean", insolesOrder.ID, s.buyer.ID); n != 1 {
		t.Errorf("audit rows = %d, want 1", n)
	}
}

func TestChangeOrderStatusChecksTheCurrentStatus(t *testing.T) {
	s := newCheckoutSetup(t)
	insolesOrder, _ := s.placeOrders(t)
	if err := s.r.ChangeOrderStatus(context.Background(), cancellation(insolesOrder.ID, s.buyer.ID)); err != nil {
		t.Fatalf("first cancel: %v", err)
	}

	if err := s.r.ChangeOrderStatus(context.Background(), cancellation(insolesOrder.ID, s.buyer.ID)); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("second cancel: error = %v, want ErrInvalidStatusTransition", err)
	}
	if n := s.count(t, "SELECT stock FROM advertisements WHERE id = $1", s.insoles.ID); n != 5 {
		t.Errorf("stock = %d, want 5 (released once)", n)
	}
	if err := s.r.ChangeOrderStatus(context.Background(), cancellation(unknownID, s.buyer.ID)); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("unknown order: error = %v, want ErrOrderNotFound", err)
	}
}

func TestConcurrentCancelsReleaseStockOnce(t *testing.T) {
	s := newCheckoutSetup(t)
	insolesOrder, _ := s.placeOrders(t)

	errs := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			<-start
			errs[i] = s.r.ChangeOrderStatus(context.Background(), cancellation(insolesOrder.ID, s.buyer.ID))
		})
	}
	close(start)
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case !errors.Is(err, app.ErrInvalidStatusTransition):
			t.Errorf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Errorf("succeeded = %d, want 1", succeeded)
	}
	if n := s.count(t, "SELECT stock FROM advertisements WHERE id = $1", s.insoles.ID); n != 5 {
		t.Errorf("stock = %d, want 5", n)
	}
}

func TestSystemStatusChangeHasNoUser(t *testing.T) {
	s := newCheckoutSetup(t)
	insolesOrder, _ := s.placeOrders(t)
	change := orders.StatusChange{OrderID: insolesOrder.ID, From: orders.OrderStatusAwaitingPayment, To: orders.OrderStatusPaid, Reason: "payment_confirmed"}

	if err := s.r.ChangeOrderStatus(context.Background(), change); err != nil {
		t.Fatalf("ChangeOrderStatus: %v", err)
	}

	if n := s.count(t, "SELECT count(*) FROM order_status_history WHERE order_id = $1 AND new_status = 'paid' AND changed_by IS NULL", insolesOrder.ID); n != 1 {
		t.Errorf("history rows without user = %d, want 1", n)
	}
	if n := s.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND user_id IS NULL", insolesOrder.ID); n != 1 {
		t.Errorf("audit rows without user = %d, want 1", n)
	}
	if n := s.count(t, "SELECT stock FROM advertisements WHERE id = $1", s.insoles.ID); n != 3 {
		t.Errorf("stock = %d, want 3 (not released)", n)
	}
}

package orders

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
)

// fulfillmentFixture has a paid order of testSellerID (testOrderID), an unpaid one of the same seller
// (otherOrderID) and a paid one of another seller.
func fulfillmentFixture() (*fakeRepository, *service) {
	paidAt := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	r := newFakeRepository()
	r.placed = []*OrderEntity{
		{ID: testOrderID, Number: "SC-1", BuyerID: testBuyerID, SellerID: testSellerID, Status: OrderStatusPaid, PaidAt: &paidAt},
		{ID: otherOrderID, Number: "SC-2", BuyerID: testBuyerID, SellerID: testSellerID, Status: OrderStatusAwaitingPayment},
		{ID: "00000000-0000-0000-0000-000000000803", Number: "SC-3", BuyerID: testBuyerID, SellerID: "seller-2", Status: OrderStatusPaid, PaidAt: &paidAt},
	}
	return r, NewService(r)
}

// notified returns the change's single notification, or nil when it has none or several.
func notified(change StatusChange) *notifications.NewNotification {
	if len(change.Notifications) != 1 {
		return nil
	}
	return &change.Notifications[0]
}

func TestValidateShipment(t *testing.T) {
	valid := []struct {
		input ShipmentInput
		want  NewShipment
	}{
		{ShipmentInput{Carrier: " Correios ", TrackingCode: "aa123456789br"}, NewShipment{Carrier: "Correios", TrackingCode: "AA123456789BR"}},
		{ShipmentInput{Carrier: "Correios (SEDEX)", TrackingCode: "QB 123 456 789 BR"}, NewShipment{Carrier: "Correios (SEDEX)", TrackingCode: "QB123456789BR"}},
		{ShipmentInput{Carrier: "Jadlog", TrackingCode: "10084512345678"}, NewShipment{Carrier: "Jadlog", TrackingCode: "10084512345678"}},
		{ShipmentInput{Carrier: "Loggi", TrackingCode: "lg-2026-abc"}, NewShipment{Carrier: "Loggi", TrackingCode: "LG-2026-ABC"}},
	}
	for _, test := range valid {
		got, err := ValidateShipment(test.input)
		if err != nil || got != test.want {
			t.Errorf("ValidateShipment(%+v) = %+v, %v, want %+v", test.input, got, err, test.want)
		}
	}

	invalid := []struct {
		input ShipmentInput
		want  error
	}{
		{ShipmentInput{Carrier: "", TrackingCode: "AA123456789BR"}, app.ErrInvalidInput},
		{ShipmentInput{Carrier: fmt.Sprintf("%081d", 0), TrackingCode: "AA123456789BR"}, app.ErrInvalidInput},
		{ShipmentInput{Carrier: "Correios", TrackingCode: "AA12345678BR"}, app.ErrInvalidTrackingCode},
		{ShipmentInput{Carrier: "Correios", TrackingCode: "AA123456789US"}, app.ErrInvalidTrackingCode},
		{ShipmentInput{Carrier: "correios", TrackingCode: "123456789012"}, app.ErrInvalidTrackingCode},
		{ShipmentInput{Carrier: "Jadlog", TrackingCode: "1234567"}, app.ErrInvalidTrackingCode},
		{ShipmentInput{Carrier: "Jadlog", TrackingCode: fmt.Sprintf("%041d", 0)}, app.ErrInvalidTrackingCode},
		{ShipmentInput{Carrier: "Jadlog", TrackingCode: "ABC/12345"}, app.ErrInvalidTrackingCode},
	}
	for _, test := range invalid {
		if _, err := ValidateShipment(test.input); !errors.Is(err, test.want) {
			t.Errorf("ValidateShipment(%+v): error = %v, want %v", test.input, err, test.want)
		}
	}
}

func TestListSellerOrders(t *testing.T) {
	_, s := fulfillmentFixture()

	page, err := s.ListSellerOrders(context.Background(), testSellerID, HistoryFilter{}, 0, 0)
	if err != nil {
		t.Fatalf("ListSellerOrders: %v", err)
	}
	if page.TotalItems != 1 || len(page.Items) != 1 || page.Items[0].ID != testOrderID || page.CurrentPage != 1 {
		t.Errorf("page = %+v, want only the seller's paid order", page)
	}

	page, err = s.ListSellerOrders(context.Background(), testSellerID, HistoryFilter{Status: OrderStatusShipped}, 1, 10)
	if err != nil || page.TotalItems != 0 {
		t.Errorf("shipped = %+v, %v, want none", page, err)
	}
}

func TestListSellerOrdersRejects(t *testing.T) {
	r, s := fulfillmentFixture()
	for _, status := range []OrderStatus{OrderStatusAwaitingPayment, "lost"} {
		if _, err := s.ListSellerOrders(context.Background(), testSellerID, HistoryFilter{Status: status}, 1, 10); !errors.Is(err, app.ErrInvalidInput) {
			t.Errorf("status %q: error = %v", status, err)
		}
	}
	if _, err := s.ListSellerOrders(context.Background(), testSellerID, HistoryFilter{}, 1, maxOrdersPerPage+1); !errors.Is(err, app.ErrInvalidInput) {
		t.Errorf("limit too big: error = %v", err)
	}
	r.err = errors.New("db down")
	if _, err := s.ListSellerOrders(context.Background(), testSellerID, HistoryFilter{}, 1, 10); !errors.Is(err, r.err) {
		t.Errorf("repository failure: error = %v", err)
	}
}

func TestGetSellerOrderOnlyForItsSellerOncePaid(t *testing.T) {
	_, s := fulfillmentFixture()

	detail, err := s.GetSellerOrder(context.Background(), testSellerID, testOrderID)
	if err != nil || detail.Order.ID != testOrderID || detail.Shipment != nil {
		t.Errorf("GetSellerOrder = %+v, %v", detail, err)
	}
	for _, test := range []struct{ seller, order string }{
		{"seller-2", testOrderID},
		{testSellerID, otherOrderID},
		{testSellerID, "not-a-uuid"},
		{testSellerID, unknownID},
	} {
		if _, err := s.GetSellerOrder(context.Background(), test.seller, test.order); !errors.Is(err, app.ErrOrderNotFound) {
			t.Errorf("seller %s, order %s: error = %v, want ErrOrderNotFound", test.seller, test.order, err)
		}
	}
}

func TestStartProcessing(t *testing.T) {
	r, s := fulfillmentFixture()

	detail, err := s.StartProcessing(context.Background(), testSellerID, testOrderID)

	if err != nil {
		t.Fatalf("StartProcessing: %v", err)
	}
	if detail.Order.Status != OrderStatusProcessing || !detail.ToSellerOutput().CanShip || detail.ToSellerOutput().CanStartProcessing {
		t.Errorf("detail = %+v", detail.ToSellerOutput())
	}
	change := r.changes[0]
	if change.ChangedBy != testSellerID || change.From != OrderStatusPaid || change.To != OrderStatusProcessing || change.Reason != ReasonSellerProcessing || change.ReleaseStock {
		t.Errorf("change = %+v", change)
	}
	if n := notified(change); n == nil || n.UserID != testBuyerID || n.Type != notifications.TypeOrderProcessing || n.Data["order_id"] != testOrderID {
		t.Errorf("notification = %+v, want the buyer's", n)
	}

	// Again: the order is no longer paid.
	if _, err := s.StartProcessing(context.Background(), testSellerID, testOrderID); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("again: error = %v, want ErrInvalidStatusTransition", err)
	}
	if _, err := s.StartProcessing(context.Background(), "seller-2", testOrderID); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("another seller: error = %v, want ErrOrderNotFound", err)
	}
}

func TestShipOrder(t *testing.T) {
	r, s := fulfillmentFixture()
	input := ShipmentInput{Carrier: "Correios", TrackingCode: "aa123456789br"}

	// It must be in preparation first.
	if _, err := s.ShipOrder(context.Background(), testSellerID, testOrderID, input); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Fatalf("paid order: error = %v, want ErrInvalidStatusTransition", err)
	}
	r.placed[0].Status = OrderStatusProcessing

	detail, err := s.ShipOrder(context.Background(), testSellerID, testOrderID, input)

	if err != nil {
		t.Fatalf("ShipOrder: %v", err)
	}
	if detail.Order.Status != OrderStatusShipped || detail.Shipment == nil || detail.Shipment.TrackingCode != "AA123456789BR" {
		t.Errorf("detail = %+v, shipment = %+v", detail.Order, detail.Shipment)
	}
	change := r.changes[len(r.changes)-1]
	if change.Shipment == nil || *change.Shipment != (NewShipment{Carrier: "Correios", TrackingCode: "AA123456789BR"}) || change.Reason != ReasonSellerShipped {
		t.Errorf("change = %+v", change)
	}
	if n := notified(change); n == nil || n.UserID != testBuyerID || n.Type != notifications.TypeOrderShipped {
		t.Errorf("notification = %+v, want the buyer's", n)
	}
}

func TestShipOrderRejectsAnInvalidCode(t *testing.T) {
	r, s := fulfillmentFixture()
	r.placed[0].Status = OrderStatusProcessing

	_, err := s.ShipOrder(context.Background(), testSellerID, testOrderID, ShipmentInput{Carrier: "Correios", TrackingCode: "123"})

	if !errors.Is(err, app.ErrInvalidTrackingCode) {
		t.Errorf("error = %v, want ErrInvalidTrackingCode", err)
	}
	if len(r.changes) != 0 || r.placed[0].Status != OrderStatusProcessing {
		t.Errorf("changes = %+v, want none (UC-009 alternative flow)", r.changes)
	}
}

func TestConfirmDelivery(t *testing.T) {
	r, s := fulfillmentFixture()

	if _, err := s.ConfirmDelivery(context.Background(), testBuyerID, testOrderID); !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("paid order: error = %v, want ErrInvalidStatusTransition", err)
	}
	r.placed[0].Status = OrderStatusShipped
	if _, err := s.ConfirmDelivery(context.Background(), "buyer-2", testOrderID); !errors.Is(err, app.ErrOrderNotFound) {
		t.Errorf("another buyer: error = %v, want ErrOrderNotFound", err)
	}

	detail, err := s.ConfirmDelivery(context.Background(), testBuyerID, testOrderID)

	if err != nil {
		t.Fatalf("ConfirmDelivery: %v", err)
	}
	if detail.Order.Status != OrderStatusDelivered || detail.ToOutput().CanConfirmDelivery {
		t.Errorf("detail = %+v", detail.ToOutput())
	}
	change := r.changes[len(r.changes)-1]
	if change.ChangedBy != testBuyerID || change.Reason != ReasonBuyerConfirmedDelivery {
		t.Errorf("change = %+v", change)
	}
	if n := notified(change); n == nil || n.UserID != testSellerID || n.Type != notifications.TypeOrderDelivered {
		t.Errorf("notification = %+v, want the seller's", n)
	}
}

func TestCancelPaidOrderNotifiesTheSeller(t *testing.T) {
	r, s := fulfillmentFixture()

	if _, err := s.CancelOrder(context.Background(), testBuyerID, testOrderID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if n := notified(r.changes[0]); n == nil || n.UserID != testSellerID || n.Type != notifications.TypeOrderCancelled {
		t.Errorf("notification = %+v, want the seller's", n)
	}

	// An unpaid order was never shown to the seller.
	if _, err := s.CancelOrder(context.Background(), testBuyerID, otherOrderID); err != nil {
		t.Fatalf("CancelOrder: %v", err)
	}
	if n := r.changes[1].Notifications; n != nil {
		t.Errorf("notification = %+v, want none", n)
	}
}

func TestSellerOrderOutput(t *testing.T) {
	posted := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	detail := &OrderDetail{
		Order:    &OrderEntity{ID: testOrderID, BuyerID: testBuyerID, BuyerName: "Maria", Status: OrderStatusShipped, Items: []OrderItemEntity{}},
		Shipment: &ShipmentEntity{Carrier: "Correios", TrackingCode: "AA123456789BR", Status: "posted", PostedAt: &posted},
	}

	output := detail.ToSellerOutput()

	if output.Buyer.Name != "Maria" || output.Shipment == nil || output.Shipment.TrackingCode != "AA123456789BR" || output.CanShip || output.CanStartProcessing {
		t.Errorf("seller output = %+v", output)
	}
	if buyer := detail.ToOutput(); !buyer.CanConfirmDelivery || buyer.Shipment == nil || buyer.CanCancel {
		t.Errorf("buyer output = %+v", buyer)
	}
	page := (&OrderPage{Items: []*OrderEntity{detail.Order}, CurrentPage: 1, TotalPages: 2, TotalItems: 11}).ToSellerOutput()
	if len(page.Items) != 1 || page.Items[0].Buyer.ID != testBuyerID || !page.HasNext || page.HasPrevious {
		t.Errorf("page = %+v", page)
	}
}

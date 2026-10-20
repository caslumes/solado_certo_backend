package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
)

// advance moves the order one step through the fulfillment flow.
func (s checkoutSetup) advance(t *testing.T, orderID string, from orders.OrderStatus, to orders.OrderStatus, shipment *orders.NewShipment) {
	t.Helper()
	err := s.r.ChangeOrderStatus(context.Background(), orders.StatusChange{OrderID: orderID, From: from, To: to, Reason: "test", Shipment: shipment})
	if err != nil {
		t.Fatalf("%s → %s: %v", from, to, err)
	}
}

func TestOrdersBySeller(t *testing.T) {
	s := newCheckoutSetup(t)
	ctx := context.Background()
	insolesOrder, socksOrder := s.placeOrders(t)

	// Not paid yet: the seller doesn't see it.
	if total, err := s.r.CountOrdersBySeller(ctx, s.seller.ID, ""); err != nil || total != 0 {
		t.Errorf("before payment: total = %d, %v", total, err)
	}

	s.exec(t, "UPDATE orders SET status = 'paid', paid_at = now() WHERE id = ANY($1::uuid[])", []string{insolesOrder.ID, socksOrder.ID})

	total, err := s.r.CountOrdersBySeller(ctx, s.seller.ID, "")
	list, listErr := s.r.GetOrdersBySeller(ctx, s.seller.ID, "", 10, 0)
	if err != nil || listErr != nil || total != 1 || len(list) != 1 || list[0].ID != insolesOrder.ID {
		t.Fatalf("seller orders = %d, %+v, %v, %v", total, list, err, listErr)
	}
	if list[0].BuyerName != s.buyer.Name || list[0].PaidAt == nil || list[0].DeliveredAt != nil {
		t.Errorf("order = %+v", list[0])
	}

	if total, _ := s.r.CountOrdersBySeller(ctx, s.seller.ID, orders.OrderStatusPaid); total != 1 {
		t.Errorf("paid = %d, want 1", total)
	}
	shipped, _ := s.r.GetOrdersBySeller(ctx, s.seller.ID, orders.OrderStatusShipped, 10, 0)
	if total, _ := s.r.CountOrdersBySeller(ctx, s.seller.ID, orders.OrderStatusShipped); total != 0 || len(shipped) != 0 {
		t.Errorf("shipped = %d, %v, want none", total, shipped)
	}
	if other, _ := s.r.GetOrdersBySeller(ctx, s.otherSeller.ID, "", 10, 0); len(other) != 1 || other[0].ID != socksOrder.ID {
		t.Errorf("other seller = %+v", other)
	}
}

func TestShipmentAndDelivery(t *testing.T) {
	s := newCheckoutSetup(t)
	ctx := context.Background()
	insolesOrder, _ := s.placeOrders(t)
	s.advance(t, insolesOrder.ID, orders.OrderStatusAwaitingPayment, orders.OrderStatusPaid, nil)
	s.advance(t, insolesOrder.ID, orders.OrderStatusPaid, orders.OrderStatusProcessing, nil)

	if shipment, err := s.r.GetShipment(ctx, insolesOrder.ID); err != nil || shipment != nil {
		t.Errorf("before shipping: shipment = %+v, %v", shipment, err)
	}

	s.advance(t, insolesOrder.ID, orders.OrderStatusProcessing, orders.OrderStatusShipped, &orders.NewShipment{Carrier: "Correios", TrackingCode: "AA123456789BR"})

	shipment, err := s.r.GetShipment(ctx, insolesOrder.ID)
	if err != nil || shipment == nil || shipment.Carrier != "Correios" || shipment.TrackingCode != "AA123456789BR" || shipment.Status != "posted" || shipment.PostedAt == nil {
		t.Fatalf("shipment = %+v, %v", shipment, err)
	}
	if n := s.count(t, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND context->>'to' = 'shipped' AND context->>'tracking_code' = 'AA123456789BR'", insolesOrder.ID); n != 1 {
		t.Errorf("shipping audit rows = %d, want 1", n)
	}

	s.advance(t, insolesOrder.ID, orders.OrderStatusShipped, orders.OrderStatusDelivered, nil)

	shipment, _ = s.r.GetShipment(ctx, insolesOrder.ID)
	order, _ := s.r.GetOrder(ctx, insolesOrder.ID)
	if shipment.Status != "delivered" || shipment.DeliveredAt == nil || order.DeliveredAt == nil || order.Status != orders.OrderStatusDelivered {
		t.Errorf("after delivery: shipment = %+v, order = %+v", shipment, order)
	}
}

func TestShipmentWithATrackingCodeInUse(t *testing.T) {
	s := newCheckoutSetup(t)
	insolesOrder, socksOrder := s.placeOrders(t)
	s.exec(t, "UPDATE orders SET status = 'processing', paid_at = now() WHERE id = ANY($1::uuid[])", []string{insolesOrder.ID, socksOrder.ID})
	shipment := &orders.NewShipment{Carrier: "Correios", TrackingCode: "AA123456789BR"}
	s.advance(t, insolesOrder.ID, orders.OrderStatusProcessing, orders.OrderStatusShipped, shipment)

	err := s.r.ChangeOrderStatus(context.Background(), orders.StatusChange{
		OrderID: socksOrder.ID, From: orders.OrderStatusProcessing, To: orders.OrderStatusShipped, Shipment: shipment,
	})

	if !errors.Is(err, app.ErrTrackingCodeInUse) {
		t.Fatalf("error = %v, want ErrTrackingCodeInUse", err)
	}
	if order, _ := s.r.GetOrder(context.Background(), socksOrder.ID); order.Status != orders.OrderStatusProcessing {
		t.Errorf("status = %s, want still processing", order.Status)
	}
}

func TestStatusChangeWritesItsNotification(t *testing.T) {
	s := newCheckoutSetup(t)
	insolesOrder, socksOrder := s.placeOrders(t)
	s.exec(t, "UPDATE orders SET status = 'paid', paid_at = now() WHERE id = $1", insolesOrder.ID)
	notification := notifications.OrderProcessing(s.buyer.ID, insolesOrder.ID, insolesOrder.Number)

	err := s.r.ChangeOrderStatus(context.Background(), orders.StatusChange{
		OrderID: insolesOrder.ID, From: orders.OrderStatusPaid, To: orders.OrderStatusProcessing, Notification: &notification,
	})

	if err != nil {
		t.Fatalf("ChangeOrderStatus: %v", err)
	}
	if n := s.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'order_processing' AND data->>'order_id' = $2", s.buyer.ID, insolesOrder.ID); n != 1 {
		t.Errorf("notifications = %d, want 1", n)
	}

	// A change that fails writes no notification.
	err = s.r.ChangeOrderStatus(context.Background(), orders.StatusChange{
		OrderID: socksOrder.ID, From: orders.OrderStatusPaid, To: orders.OrderStatusProcessing, Notification: &notification,
	})
	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("error = %v, want ErrInvalidStatusTransition", err)
	}
	if n := s.count(t, "SELECT count(*) FROM notifications"); n != 1 {
		t.Errorf("notifications = %d, want still 1", n)
	}
}

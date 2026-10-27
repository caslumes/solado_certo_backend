package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app"
	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/features/orders"
)

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

	if total, err := s.r.CountOrdersBySeller(ctx, s.seller.ID, orders.OrderQuery{}); err != nil || total != 0 {
		t.Errorf("before payment: total = %d, %v", total, err)
	}

	s.exec(t, "UPDATE orders SET status = 'paid', paid_at = now() WHERE id = ANY($1::uuid[])", []string{insolesOrder.ID, socksOrder.ID})

	total, err := s.r.CountOrdersBySeller(ctx, s.seller.ID, orders.OrderQuery{})
	list, listErr := s.r.GetOrdersBySeller(ctx, s.seller.ID, orders.OrderQuery{}, 10, 0)
	if err != nil || listErr != nil || total != 1 || len(list) != 1 || list[0].ID != insolesOrder.ID {
		t.Fatalf("seller orders = %d, %+v, %v, %v", total, list, err, listErr)
	}
	if list[0].BuyerName != s.buyer.Name || list[0].PaidAt == nil || list[0].DeliveredAt != nil {
		t.Errorf("order = %+v", list[0])
	}

	paid := orders.OrderQuery{Status: orders.OrderStatusPaid}
	if total, _ := s.r.CountOrdersBySeller(ctx, s.seller.ID, paid); total != 1 {
		t.Errorf("paid = %d, want 1", total)
	}
	shippedQuery := orders.OrderQuery{Status: orders.OrderStatusShipped}
	shipped, _ := s.r.GetOrdersBySeller(ctx, s.seller.ID, shippedQuery, 10, 0)
	if total, _ := s.r.CountOrdersBySeller(ctx, s.seller.ID, shippedQuery); total != 0 || len(shipped) != 0 {
		t.Errorf("shipped = %d, %v, want none", total, shipped)
	}
	if other, _ := s.r.GetOrdersBySeller(ctx, s.otherSeller.ID, orders.OrderQuery{}, 10, 0); len(other) != 1 || other[0].ID != socksOrder.ID {
		t.Errorf("other seller = %+v", other)
	}

	tomorrow := time.Now().Add(24 * time.Hour)
	later := orders.OrderQuery{Since: &tomorrow}
	if total, _ := s.r.CountOrdersBySeller(ctx, s.seller.ID, later); total != 0 {
		t.Errorf("paid after tomorrow = %d, want 0", total)
	}
	if list, _ := s.r.GetOrdersBySeller(ctx, s.seller.ID, orders.OrderQuery{Until: &tomorrow}, 10, 0); len(list) != 1 {
		t.Errorf("paid before tomorrow = %d, want 1", len(list))
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
		OrderID: insolesOrder.ID, From: orders.OrderStatusPaid, To: orders.OrderStatusProcessing, Notifications: []notifications.NewNotification{notification},
	})

	if err != nil {
		t.Fatalf("ChangeOrderStatus: %v", err)
	}
	if n := s.count(t, "SELECT count(*) FROM notifications WHERE user_id = $1 AND type = 'order_processing' AND data->>'order_id' = $2", s.buyer.ID, insolesOrder.ID); n != 1 {
		t.Errorf("notifications = %d, want 1", n)
	}

	err = s.r.ChangeOrderStatus(context.Background(), orders.StatusChange{
		OrderID: socksOrder.ID, From: orders.OrderStatusPaid, To: orders.OrderStatusProcessing, Notifications: []notifications.NewNotification{notification},
	})
	if !errors.Is(err, app.ErrInvalidStatusTransition) {
		t.Errorf("error = %v, want ErrInvalidStatusTransition", err)
	}
	if n := s.count(t, "SELECT count(*) FROM notifications"); n != 1 {
		t.Errorf("notifications = %d, want still 1", n)
	}
}

func TestGetOverdueShippedOrders(t *testing.T) {
	s := newCheckoutSetup(t)
	ctx := context.Background()
	insolesOrder, socksOrder := s.placeOrders(t)
	s.exec(t, "UPDATE orders SET status = 'processing', paid_at = now() WHERE id = ANY($1::uuid[])", []string{insolesOrder.ID, socksOrder.ID})
	s.advance(t, insolesOrder.ID, orders.OrderStatusProcessing, orders.OrderStatusShipped, &orders.NewShipment{Carrier: "Correios", TrackingCode: "AA123456789BR"})
	s.advance(t, socksOrder.ID, orders.OrderStatusProcessing, orders.OrderStatusShipped, &orders.NewShipment{Carrier: "Correios", TrackingCode: "AA987654321BR"})
	s.exec(t, "UPDATE shipments SET posted_at = now() - interval '31 days' WHERE order_id = $1", insolesOrder.ID)
	cutoff := time.Now().Add(-orders.DeliveryDeadline)

	overdue, err := s.r.GetOverdueShippedOrders(ctx, cutoff)

	if err != nil || len(overdue) != 1 || overdue[0].ID != insolesOrder.ID || overdue[0].SellerID != s.seller.ID {
		t.Fatalf("overdue = %+v, %v, want the insoles order", overdue, err)
	}

	err = s.r.ChangeOrderStatus(ctx, orders.StatusChange{
		OrderID: insolesOrder.ID, From: orders.OrderStatusShipped, To: orders.OrderStatusDelivered, Reason: orders.ReasonDeliveryDeadline,
		Notifications: []notifications.NewNotification{
			notifications.BuyerDeliveryDeadline(s.buyer.ID, insolesOrder.ID, insolesOrder.Number),
			notifications.SellerDeliveryDeadline(s.seller.ID, insolesOrder.ID, insolesOrder.Number),
		},
	})
	if err != nil {
		t.Fatalf("ChangeOrderStatus: %v", err)
	}
	if n := s.count(t, "SELECT count(*) FROM notifications WHERE data->>'order_id' = $1 AND user_id = ANY($2::uuid[])", insolesOrder.ID, []string{s.buyer.ID, s.seller.ID}); n != 2 {
		t.Errorf("notifications = %d, want 2", n)
	}
	if overdue, _ := s.r.GetOverdueShippedOrders(ctx, cutoff); len(overdue) != 0 {
		t.Errorf("overdue after delivery = %+v, want none", overdue)
	}
}

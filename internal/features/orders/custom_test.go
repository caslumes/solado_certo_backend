package orders

import (
	"context"
	"errors"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

const testCustomRequestID = "00000000-0000-0000-0000-000000000901"

func customItem() CustomItem {
	return CustomItem{RequestID: testCustomRequestID, AdvertisementID: insolesAdID, AddressID: testAddressID, ShippingOptionID: "sedex", Price: "189.90"}
}

func TestPlanCustomOrder(t *testing.T) {
	r := newFakeRepository()
	r.ads[insolesAdID].Stock = 0
	s := NewService(r)

	plan, err := s.PlanCustomOrder(context.Background(), testBuyerID, customItem())

	if err != nil {
		t.Fatalf("PlanCustomOrder: %v", err)
	}
	if plan.AddressID != testAddressID || plan.Address.City != "Campinas" || plan.CartID != "" || len(plan.Orders) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	order := plan.Orders[0]
	if order.SellerID != testSellerID || len(order.Items) != 1 || order.Subtotal.String() != "189.90" ||
		order.ShippingTotal.String() != "34.90" || order.Total().String() != "224.80" {
		t.Errorf("order = %+v, want one unit at the quoted price, made to order even without stock", order)
	}
	item := order.Items[0]
	if item.Quantity != 1 || item.Price != "189.90" || item.PriceChanged() || item.ShippingOption.ID != "sedex" ||
		item.CustomOrderRequestID == nil || *item.CustomOrderRequestID != testCustomRequestID {
		t.Errorf("item = %+v", item)
	}
}

func TestPlanCustomOrderErrors(t *testing.T) {
	tests := []struct {
		name   string
		change func(*CustomItem)
		want   error
	}{
		{"address not a UUID", func(i *CustomItem) { i.AddressID = "x" }, app.ErrAddressNotFound},
		{"unknown address", func(i *CustomItem) { i.AddressID = otherOrderID }, app.ErrAddressNotFound},
		{"unknown ad", func(i *CustomItem) { i.AdvertisementID = otherSellerAdID }, app.ErrAdvertisementNotFound},
		{"option of another ad", func(i *CustomItem) { i.ShippingOptionID = "letter" }, app.ErrShippingRequired},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := customItem()
			tt.change(&item)

			_, err := NewService(newFakeRepository()).PlanCustomOrder(context.Background(), testBuyerID, item)

			if !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestShipOrderWaitsForTheFittingReport(t *testing.T) {
	r, s := fulfillmentFixture()
	requestID := testCustomRequestID
	r.placed[0].Status = OrderStatusProcessing
	r.placed[0].Items = []OrderItemEntity{{ID: "item-1", CustomOrderRequestID: &requestID}}

	_, err := s.ShipOrder(context.Background(), testSellerID, testOrderID, ShipmentInput{Carrier: "Correios", TrackingCode: "AA123456789BR"})

	if reports, ok := errors.AsType[*app.FittingReportRequiredError](err); !ok || len(reports.ItemIDs) != 1 || reports.ItemIDs[0] != "item-1" {
		t.Fatalf("error = %v, want FittingReportRequiredError for item-1", err)
	}
	if len(r.changes) != 0 {
		t.Errorf("changes = %+v, want none", r.changes)
	}
	detail, err := s.GetSellerOrder(context.Background(), testSellerID, testOrderID)
	if err != nil {
		t.Fatalf("GetSellerOrder: %v", err)
	}
	output := detail.ToSellerOutput()
	if output.CanShip || !output.Items[0].FittingReportPending || output.Items[0].CustomOrderID == nil {
		t.Errorf("output = %+v, want can_ship false and the report pending", output)
	}

	r.placed[0].Items[0].FittingReportSent = true
	if _, err := s.ShipOrder(context.Background(), testSellerID, testOrderID, ShipmentInput{Carrier: "Correios", TrackingCode: "AA123456789BR"}); err != nil {
		t.Errorf("ShipOrder after the report: %v", err)
	}
}

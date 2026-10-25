package orders

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/caslumes/solado_certo_backend/internal/app"
)

var (
	approvedPrescription = &PrescriptionEntity{ID: "doc-1", Status: DocumentStatusApproved}
	rejectedPrescription = &PrescriptionEntity{ID: "doc-2", Status: DocumentStatusRejected}
)

func TestCanUploadPrescription(t *testing.T) {
	tests := []struct {
		status OrderStatus
		latest *PrescriptionEntity
		want   bool
	}{
		{OrderStatusAwaitingPayment, nil, true},
		{OrderStatusAwaitingPayment, approvedPrescription, true},
		{OrderStatusPaid, nil, true},
		{OrderStatusPaid, rejectedPrescription, true},
		{OrderStatusPaid, approvedPrescription, false},
		{OrderStatusProcessing, rejectedPrescription, true},
		{OrderStatusProcessing, approvedPrescription, false},
		{OrderStatusShipped, rejectedPrescription, false},
		{OrderStatusCancelled, nil, false},
	}
	for _, test := range tests {
		if got := CanUploadPrescription(test.status, test.latest); got != test.want {
			t.Errorf("CanUploadPrescription(%s, %+v) = %v, want %v", test.status, test.latest, got, test.want)
		}
	}
}

func TestCanRejectPrescription(t *testing.T) {
	tests := []struct {
		status OrderStatus
		latest *PrescriptionEntity
		want   bool
	}{
		{OrderStatusPaid, approvedPrescription, true},
		{OrderStatusProcessing, approvedPrescription, true},
		{OrderStatusProcessing, rejectedPrescription, false},
		{OrderStatusPaid, nil, false},
		{OrderStatusAwaitingPayment, approvedPrescription, false},
		{OrderStatusShipped, approvedPrescription, false},
	}
	for _, test := range tests {
		if got := CanRejectPrescription(test.status, test.latest); got != test.want {
			t.Errorf("CanRejectPrescription(%s, %+v) = %v, want %v", test.status, test.latest, got, test.want)
		}
	}
}

func prescriptionOrder(status OrderStatus) *OrderEntity {
	return &OrderEntity{
		ID:     testOrderID,
		Status: status,
		Items: []OrderItemEntity{
			{ID: "item-1"},
			{ID: "item-2", PrescriptionRequired: true},
			{ID: "item-3", PrescriptionRequired: true, Prescription: rejectedPrescription},
			{ID: "item-4", PrescriptionRequired: true, Prescription: approvedPrescription},
		},
	}
}

func TestMissingPrescriptions(t *testing.T) {
	if got := prescriptionOrder(OrderStatusAwaitingPayment).MissingPrescriptions(); !slices.Equal(got, []string{"item-2", "item-3"}) {
		t.Errorf("MissingPrescriptions = %v, want [item-2 item-3]", got)
	}
	if got := (&OrderEntity{Items: []OrderItemEntity{{ID: "item-1"}}}).MissingPrescriptions(); len(got) != 0 {
		t.Errorf("no prescription items: MissingPrescriptions = %v", got)
	}
}

func TestPrescriptionOutput(t *testing.T) {
	buyer := prescriptionOrder(OrderStatusProcessing).ToOutput().Items
	upload := []bool{buyer[0].CanUploadPrescription, buyer[1].CanUploadPrescription, buyer[2].CanUploadPrescription, buyer[3].CanUploadPrescription}
	if !slices.Equal(upload, []bool{false, true, true, false}) {
		t.Errorf("buyer can upload = %v", upload)
	}
	for _, item := range buyer {
		if item.CanRejectPrescription {
			t.Errorf("buyer item %s: can reject = true", item.ID)
		}
	}
	if buyer[3].Prescription == nil || buyer[3].Prescription.URL != "/documents/doc-1" || buyer[3].Prescription.Status != "approved" {
		t.Errorf("prescription = %+v", buyer[3].Prescription)
	}
	if buyer[0].PrescriptionRequired || buyer[0].Prescription != nil {
		t.Errorf("item without prescription = %+v", buyer[0])
	}

	seller := prescriptionOrder(OrderStatusProcessing).ToSellerOutput().Items
	for i, item := range seller {
		if item.CanUploadPrescription || item.CanRejectPrescription != (i == 3) {
			t.Errorf("seller item %s: can upload = %v, can reject = %v", item.ID, item.CanUploadPrescription, item.CanRejectPrescription)
		}
	}
}

func TestCannotShipWithoutPrescriptions(t *testing.T) {
	r, s := fulfillmentFixture()
	r.placed[0].Status = OrderStatusProcessing
	r.placed[0].Items = []OrderItemEntity{{ID: "item-1", PrescriptionRequired: true, Prescription: rejectedPrescription}}

	detail, err := s.GetSellerOrder(context.Background(), testSellerID, testOrderID)
	if err != nil || detail.ToSellerOutput().CanShip {
		t.Fatalf("GetSellerOrder: can ship = %v, error = %v", detail != nil && detail.ToSellerOutput().CanShip, err)
	}

	_, err = s.ShipOrder(context.Background(), testSellerID, testOrderID, ShipmentInput{Carrier: "Correios", TrackingCode: "AA123456789BR"})

	var missing *app.PrescriptionRequiredError
	if !errors.As(err, &missing) || !slices.Equal(missing.ItemIDs, []string{"item-1"}) {
		t.Fatalf("ShipOrder: error = %v, want PrescriptionRequiredError for item-1", err)
	}
	if len(r.changes) != 0 {
		t.Errorf("changes = %+v, want none", r.changes)
	}
}

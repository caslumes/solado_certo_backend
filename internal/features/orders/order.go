package orders

import (
	"time"

	"github.com/caslumes/solado_certo_backend/internal/features/notifications"
	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

type ProductSnapshot struct {
	Title          string               `json:"title"`
	CoverImageURL  *string              `json:"cover_image_url"`
	SellerName     string               `json:"seller_name"`
	ShippingOption ShippingOptionOutput `json:"shipping_option"`
}

type OrderItemEntity struct {
	ID                   string
	AdvertisementID      string
	Quantity             int
	UnitPrice            string
	Subtotal             string
	Snapshot             ProductSnapshot
	Review               *ItemReviewEntity
	PrescriptionRequired bool
	Prescription         *PrescriptionEntity
	CustomOrderRequestID *string
	FittingReportSent    bool
}

func (i *OrderItemEntity) fittingReportPending() bool {
	return i.CustomOrderRequestID != nil && !i.FittingReportSent
}

type ItemReviewEntity struct {
	ProductRating      int
	SellerRating       int
	ComfortRating      *int
	Comment            *string
	AdaptationFeedback *string
	CreatedAt          time.Time
}

func (r *ItemReviewEntity) ToOutput() *ItemReviewOutput {
	if r == nil {
		return nil
	}
	return &ItemReviewOutput{
		ProductRating:      r.ProductRating,
		SellerRating:       r.SellerRating,
		ComfortRating:      r.ComfortRating,
		Comment:            r.Comment,
		AdaptationFeedback: r.AdaptationFeedback,
		CreatedAt:          r.CreatedAt,
	}
}

type OrderEntity struct {
	ID            string
	CheckoutID    string
	BuyerID       string
	BuyerName     string
	SellerID      string
	SellerName    string
	Number        string
	Status        OrderStatus
	Address       AddressSnapshot
	Subtotal      string
	ShippingTotal string
	DiscountTotal string
	Total         string
	Items         []OrderItemEntity
	PaidAt        *time.Time
	DeliveredAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (o *OrderEntity) ToOutput() OrderOutput {
	return o.output(false)
}

func (o *OrderEntity) output(seller bool) OrderOutput {
	items := make([]OrderItemOutput, len(o.Items))
	for i, item := range o.Items {
		items[i] = OrderItemOutput{
			ID:                   item.ID,
			AdvertisementID:      item.AdvertisementID,
			Title:                item.Snapshot.Title,
			CoverImageURL:        item.Snapshot.CoverImageURL,
			Quantity:             item.Quantity,
			UnitPrice:            item.UnitPrice,
			Subtotal:             item.Subtotal,
			ShippingOption:       item.Snapshot.ShippingOption,
			Review:               item.Review.ToOutput(),
			PrescriptionRequired: item.PrescriptionRequired,
			Prescription:         item.Prescription.ToOutput(),
			CustomOrderID:        item.CustomOrderRequestID,
			FittingReportPending: item.fittingReportPending(),
		}
		if item.PrescriptionRequired {
			items[i].CanUploadPrescription = !seller && CanUploadPrescription(o.Status, item.Prescription)
			items[i].CanRejectPrescription = seller && CanRejectPrescription(o.Status, item.Prescription)
		}
	}
	return OrderOutput{
		ID:            o.ID,
		Number:        o.Number,
		Status:        string(o.Status),
		Seller:        SellerOutput{ID: o.SellerID, Name: o.SellerName},
		Address:       o.Address,
		Items:         items,
		Subtotal:      o.Subtotal,
		ShippingTotal: o.ShippingTotal,
		DiscountTotal: o.DiscountTotal,
		Total:         o.Total,
		PaidAt:        o.PaidAt,
		DeliveredAt:   o.DeliveredAt,
		CreatedAt:     o.CreatedAt,
		UpdatedAt:     o.UpdatedAt,
	}
}

func (o *OrderEntity) ToSellerOutput() SellerOrderOutput {
	return SellerOrderOutput{OrderOutput: o.output(true), Buyer: BuyerOutput{ID: o.BuyerID, Name: o.BuyerName}}
}

type CheckoutEntity struct {
	ID     string
	Orders []*OrderEntity
}

func (c *CheckoutEntity) ToOutput() *CheckoutOutput {
	output := &CheckoutOutput{ID: c.ID, Orders: make([]OrderOutput, len(c.Orders))}
	var total helpers.Cents
	for i, order := range c.Orders {
		output.Orders[i] = order.ToOutput()
		total += helpers.MustParseCents(order.Total)
	}
	output.Total = total.String()
	return output
}

type StatusChangeEntity struct {
	From      *OrderStatus
	To        OrderStatus
	Reason    *string
	CreatedAt time.Time
}

type StatusChange struct {
	OrderID       string
	ChangedBy     string
	From          OrderStatus
	To            OrderStatus
	Reason        string
	ReleaseStock  bool
	Shipment      *NewShipment
	Notifications []notifications.NewNotification
}

type OrderDetail struct {
	Order    *OrderEntity
	History  []StatusChangeEntity
	Shipment *ShipmentEntity
}

func (d *OrderDetail) ToOutput() *OrderDetailOutput {
	return &OrderDetailOutput{
		OrderOutput:        d.Order.ToOutput(),
		History:            d.historyOutput(),
		Shipment:           d.Shipment.ToOutput(),
		CanCancel:          d.Order.Status.BuyerCanCancel(),
		CanConfirmDelivery: d.Order.Status == OrderStatusShipped,
	}
}

func (d *OrderDetail) ToSellerOutput() *SellerOrderDetailOutput {
	return &SellerOrderDetailOutput{
		SellerOrderOutput:  d.Order.ToSellerOutput(),
		History:            d.historyOutput(),
		Shipment:           d.Shipment.ToOutput(),
		CanStartProcessing: d.Order.Status == OrderStatusPaid,
		CanShip: d.Order.Status == OrderStatusProcessing && len(d.Order.MissingPrescriptions()) == 0 &&
			len(d.Order.PendingFittingReports()) == 0,
	}
}

func (d *OrderDetail) historyOutput() []StatusChangeOutput {
	history := make([]StatusChangeOutput, len(d.History))
	for i, change := range d.History {
		history[i] = StatusChangeOutput{To: string(change.To), Reason: change.Reason, CreatedAt: change.CreatedAt}
		if change.From != nil {
			from := string(*change.From)
			history[i].From = &from
		}
	}
	return history
}

type OrderPage struct {
	Items       []*OrderEntity
	CurrentPage uint
	TotalPages  uint
	TotalItems  uint
}

func (p *OrderPage) ToOutput() *OrderPageOutput {
	items := make([]OrderOutput, len(p.Items))
	for i, order := range p.Items {
		items[i] = order.ToOutput()
	}
	return &OrderPageOutput{
		Items:       items,
		CurrentPage: p.CurrentPage,
		TotalPages:  p.TotalPages,
		TotalItems:  p.TotalItems,
		HasNext:     p.CurrentPage < p.TotalPages,
		HasPrevious: p.CurrentPage > 1,
	}
}

func (p *OrderPage) ToSellerOutput() *SellerOrderPageOutput {
	items := make([]SellerOrderOutput, len(p.Items))
	for i, order := range p.Items {
		items[i] = order.ToSellerOutput()
	}
	return &SellerOrderPageOutput{
		Items:       items,
		CurrentPage: p.CurrentPage,
		TotalPages:  p.TotalPages,
		TotalItems:  p.TotalItems,
		HasNext:     p.CurrentPage < p.TotalPages,
		HasPrevious: p.CurrentPage > 1,
	}
}

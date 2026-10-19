package orders

import (
	"time"

	"github.com/caslumes/solado_certo_backend/internal/helpers"
)

// ProductSnapshot is stored in order_items.product_snapshot: what the buyer saw when the order was placed.
type ProductSnapshot struct {
	Title          string               `json:"title"`
	CoverImageURL  *string              `json:"cover_image_url"`
	SellerName     string               `json:"seller_name"`
	ShippingOption ShippingOptionOutput `json:"shipping_option"`
}

type OrderItemEntity struct {
	ID              string
	AdvertisementID string
	Quantity        int
	UnitPrice       string
	Subtotal        string
	Snapshot        ProductSnapshot
}

type OrderEntity struct {
	ID            string
	CheckoutID    string
	BuyerID       string
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
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (o *OrderEntity) ToOutput() OrderOutput {
	items := make([]OrderItemOutput, len(o.Items))
	for i, item := range o.Items {
		items[i] = OrderItemOutput{
			AdvertisementID: item.AdvertisementID,
			Title:           item.Snapshot.Title,
			CoverImageURL:   item.Snapshot.CoverImageURL,
			Quantity:        item.Quantity,
			UnitPrice:       item.UnitPrice,
			Subtotal:        item.Subtotal,
			ShippingOption:  item.Snapshot.ShippingOption,
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
		CreatedAt:     o.CreatedAt,
		UpdatedAt:     o.UpdatedAt,
	}
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
	OrderID      string
	ChangedBy    string
	From         OrderStatus
	To           OrderStatus
	Reason       string
	ReleaseStock bool
}

type OrderDetail struct {
	Order   *OrderEntity
	History []StatusChangeEntity
}

func (d *OrderDetail) ToOutput() *OrderDetailOutput {
	history := make([]StatusChangeOutput, len(d.History))
	for i, change := range d.History {
		history[i] = StatusChangeOutput{To: string(change.To), Reason: change.Reason, CreatedAt: change.CreatedAt}
		if change.From != nil {
			from := string(*change.From)
			history[i].From = &from
		}
	}
	return &OrderDetailOutput{
		OrderOutput: d.Order.ToOutput(),
		History:     history,
		CanCancel:   d.Order.Status.BuyerCanCancel(),
	}
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

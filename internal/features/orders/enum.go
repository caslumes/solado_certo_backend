package orders

import "slices"

const AdvertisementStatusPublished = "published"

type ItemProblem string

const (
	ItemProblemUnavailable       ItemProblem = "unavailable"
	ItemProblemOutOfStock        ItemProblem = "out_of_stock"
	ItemProblemInsufficientStock ItemProblem = "insufficient_stock"
)

type OrderStatus string

const (
	OrderStatusAwaitingPayment OrderStatus = "awaiting_payment"
	OrderStatusPaid            OrderStatus = "paid"
	OrderStatusProcessing      OrderStatus = "processing"
	OrderStatusShipped         OrderStatus = "shipped"
	OrderStatusDelivered       OrderStatus = "delivered"
	OrderStatusCancelled       OrderStatus = "cancelled"
	OrderStatusReturned        OrderStatus = "returned"
)

// orderTransitions is the order status machine. Payment (Slice 6) and shipping (Slice 7) use the
// other transitions; BuyerCanCancel says which of them the buyer may cancel.
var orderTransitions = map[OrderStatus][]OrderStatus{
	OrderStatusAwaitingPayment: {OrderStatusPaid, OrderStatusCancelled},
	OrderStatusPaid:            {OrderStatusProcessing, OrderStatusCancelled},
	OrderStatusProcessing:      {OrderStatusShipped},
	OrderStatusShipped:         {OrderStatusDelivered},
	OrderStatusDelivered:       {OrderStatusReturned},
}

func (s OrderStatus) CanBecome(next OrderStatus) bool {
	return slices.Contains(orderTransitions[s], next)
}

// BuyerCanCancel tells whether the buyer may still cancel: before paying, or paid while the seller
// hasn't started preparing the order (a paid order is refunded).
func (s OrderStatus) BuyerCanCancel() bool {
	return s == OrderStatusAwaitingPayment || s == OrderStatusPaid
}

var buyerStatuses = []OrderStatus{
	OrderStatusAwaitingPayment, OrderStatusPaid, OrderStatusProcessing, OrderStatusShipped, OrderStatusDelivered, OrderStatusCancelled, OrderStatusReturned,
}

// sellerStatuses are the statuses a seller can filter their orders by: those after payment.
var sellerStatuses = []OrderStatus{
	OrderStatusPaid, OrderStatusProcessing, OrderStatusShipped, OrderStatusDelivered, OrderStatusCancelled, OrderStatusReturned,
}

const (
	ReasonBuyerCancelled         = "buyer_cancelled"
	ReasonSellerProcessing       = "seller_started_processing"
	ReasonSellerShipped          = "seller_shipped"
	ReasonBuyerConfirmedDelivery = "buyer_confirmed_delivery"
)

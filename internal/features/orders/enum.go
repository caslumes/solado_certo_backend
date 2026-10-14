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
// other transitions; a buyer can only cancel an order that is still awaiting payment.
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

const ReasonBuyerCancelled = "buyer_cancelled"

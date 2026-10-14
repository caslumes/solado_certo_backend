package orders

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

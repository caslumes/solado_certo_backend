package notifications

import "time"

type Type string

const (
	TypeOrderPaid       Type = "order_paid"
	TypeOrderRefunded   Type = "order_refunded"
	TypeOrderCancelled  Type = "order_cancelled"
	TypeOrderProcessing Type = "order_processing"
	TypeOrderShipped    Type = "order_shipped"
	TypeOrderDelivered  Type = "order_delivered"
)

type NotificationEntity struct {
	ID        string
	UserID    string
	Type      Type
	Title     string
	Body      string
	Data      map[string]string
	ReadAt    *time.Time
	CreatedAt time.Time
}

func (n *NotificationEntity) ToOutput() NotificationOutput {
	return NotificationOutput{
		ID:        n.ID,
		Type:      string(n.Type),
		Title:     n.Title,
		Body:      n.Body,
		Data:      n.Data,
		Read:      n.ReadAt != nil,
		ReadAt:    n.ReadAt,
		CreatedAt: n.CreatedAt,
	}
}

// NewNotification is a notification to write together with the change it tells about.
type NewNotification struct {
	UserID string
	Type   Type
	Title  string
	Body   string
	// Data tells the app where to go when the notification is tapped, e.g. {"order_id": "..."}.
	Data map[string]string
}

// OrderPaid tells a seller that an order with their items was paid and can be prepared (RF-020).
func OrderPaid(sellerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: sellerID,
		Type:   TypeOrderPaid,
		Title:  "Novo pedido pago",
		Body:   "O pedido " + number + " foi pago. Inicie a preparação e informe o envio.",
		Data:   map[string]string{"order_id": orderID},
	}
}

// OrderRefunded tells the buyer that the money of an order came back.
func OrderRefunded(buyerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: buyerID,
		Type:   TypeOrderRefunded,
		Title:  "Reembolso concluído",
		Body:   "O pagamento do pedido " + number + " foi estornado.",
		Data:   map[string]string{"order_id": orderID},
	}
}

// OrderCancelled tells the seller that the buyer cancelled a paid order, which won't be shipped.
func OrderCancelled(sellerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: sellerID,
		Type:   TypeOrderCancelled,
		Title:  "Pedido cancelado",
		Body:   "O comprador cancelou o pedido " + number + ". Não é preciso enviá-lo; o pagamento será estornado.",
		Data:   map[string]string{"order_id": orderID},
	}
}

// OrderProcessing tells the buyer that the seller started preparing the order.
func OrderProcessing(buyerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: buyerID,
		Type:   TypeOrderProcessing,
		Title:  "Pedido em preparação",
		Body:   "O vendedor começou a preparar o pedido " + number + ".",
		Data:   map[string]string{"order_id": orderID},
	}
}

// OrderShipped tells the buyer how to track the order (RF-022).
func OrderShipped(buyerID string, orderID string, number string, carrier string, trackingCode string) NewNotification {
	return NewNotification{
		UserID: buyerID,
		Type:   TypeOrderShipped,
		Title:  "Pedido enviado",
		Body:   "O pedido " + number + " foi enviado por " + carrier + ". Código de rastreio: " + trackingCode + ".",
		Data:   map[string]string{"order_id": orderID},
	}
}

// OrderDelivered tells the seller that the buyer received the order.
func OrderDelivered(sellerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: sellerID,
		Type:   TypeOrderDelivered,
		Title:  "Pedido entregue",
		Body:   "O comprador confirmou o recebimento do pedido " + number + ".",
		Data:   map[string]string{"order_id": orderID},
	}
}

type NotificationPage struct {
	Items       []*NotificationEntity
	CurrentPage uint
	TotalPages  uint
	TotalItems  uint
	UnreadCount uint
}

func (p *NotificationPage) ToOutput() *NotificationPageOutput {
	items := make([]NotificationOutput, len(p.Items))
	for i, notification := range p.Items {
		items[i] = notification.ToOutput()
	}
	return &NotificationPageOutput{
		Items:       items,
		CurrentPage: p.CurrentPage,
		TotalPages:  p.TotalPages,
		TotalItems:  p.TotalItems,
		HasNext:     p.CurrentPage < p.TotalPages,
		HasPrevious: p.CurrentPage > 1,
		UnreadCount: p.UnreadCount,
	}
}

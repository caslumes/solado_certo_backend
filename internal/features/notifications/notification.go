package notifications

import (
	"strings"
	"time"
)

type Type string

const (
	TypeOrderPaid            Type = "order_paid"
	TypeOrderRefunded        Type = "order_refunded"
	TypeOrderCancelled       Type = "order_cancelled"
	TypeOrderProcessing      Type = "order_processing"
	TypeOrderShipped         Type = "order_shipped"
	TypeOrderDelivered       Type = "order_delivered"
	TypeDeliveryDeadline     Type = "delivery_deadline"
	TypePrescriptionRejected Type = "prescription_rejected"
	TypePrescriptionSent     Type = "prescription_sent"
	TypeCustomOrderRequested Type = "custom_order_requested"
	TypeCustomOrderUpdated   Type = "custom_order_updated"
)

const (
	RecipientBuyer  = "buyer"
	RecipientSeller = "seller"
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

type NewNotification struct {
	UserID string
	Type   Type
	Title  string
	Body   string
	Data   map[string]string
}

func OrderPaid(sellerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: sellerID,
		Type:   TypeOrderPaid,
		Title:  "Novo pedido pago",
		Body:   "O pedido " + number + " foi pago. Inicie a preparação e informe o envio.",
		Data:   map[string]string{"order_id": orderID},
	}
}

func OrderRefunded(buyerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: buyerID,
		Type:   TypeOrderRefunded,
		Title:  "Reembolso concluído",
		Body:   "O pagamento do pedido " + number + " foi estornado.",
		Data:   map[string]string{"order_id": orderID},
	}
}

func OrderCancelled(sellerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: sellerID,
		Type:   TypeOrderCancelled,
		Title:  "Pedido cancelado",
		Body:   "O comprador cancelou o pedido " + number + ". Não é preciso enviá-lo; o pagamento será estornado.",
		Data:   map[string]string{"order_id": orderID},
	}
}

func OrderProcessing(buyerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: buyerID,
		Type:   TypeOrderProcessing,
		Title:  "Pedido em preparação",
		Body:   "O vendedor começou a preparar o pedido " + number + ".",
		Data:   map[string]string{"order_id": orderID},
	}
}

func OrderShipped(buyerID string, orderID string, number string, carrier string, trackingCode string) NewNotification {
	return NewNotification{
		UserID: buyerID,
		Type:   TypeOrderShipped,
		Title:  "Pedido enviado",
		Body:   "O pedido " + number + " foi enviado por " + carrier + ". Código de rastreio: " + trackingCode + ".",
		Data:   map[string]string{"order_id": orderID},
	}
}

func OrderDelivered(sellerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: sellerID,
		Type:   TypeOrderDelivered,
		Title:  "Pedido entregue",
		Body:   "O comprador confirmou o recebimento do pedido " + number + ".",
		Data:   map[string]string{"order_id": orderID},
	}
}

func BuyerDeliveryDeadline(buyerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: buyerID,
		Type:   TypeDeliveryDeadline,
		Title:  "Pedido dado como entregue",
		Body:   "O pedido " + number + " foi enviado há 30 dias e foi dado como entregue. Avalie os produtos; se ele não chegou, procure o suporte.",
		Data:   map[string]string{"order_id": orderID},
	}
}

func SellerDeliveryDeadline(sellerID string, orderID string, number string) NewNotification {
	return NewNotification{
		UserID: sellerID,
		Type:   TypeOrderDelivered,
		Title:  "Pedido entregue",
		Body:   "O pedido " + number + " foi enviado há 30 dias e foi dado como entregue.",
		Data:   map[string]string{"order_id": orderID},
	}
}

func PrescriptionRejected(buyerID string, orderID string, number string, title string, reason string) NewNotification {
	return NewNotification{
		UserID: buyerID,
		Type:   TypePrescriptionRejected,
		Title:  "Receita recusada",
		Body:   "O vendedor recusou a receita de " + title + " no pedido " + number + ": " + strings.TrimRight(reason, ".") + ". Envie uma nova receita para o pedido seguir.",
		Data:   map[string]string{"order_id": orderID},
	}
}

func PrescriptionSent(sellerID string, orderID string, number string, title string) NewNotification {
	return NewNotification{
		UserID: sellerID,
		Type:   TypePrescriptionSent,
		Title:  "Nova receita enviada",
		Body:   "O comprador enviou uma nova receita de " + title + " no pedido " + number + ".",
		Data:   map[string]string{"order_id": orderID},
	}
}

func CustomOrderRequested(sellerID string, requestID string, title string) NewNotification {
	return NewNotification{
		UserID: sellerID,
		Type:   TypeCustomOrderRequested,
		Title:  "Nova solicitação sob medida",
		Body:   "Um comprador pediu " + title + " sob medida. Analise as medidas e responda.",
		Data:   map[string]string{"custom_order_id": requestID, "recipient": RecipientSeller},
	}
}

func CustomOrderUnderReview(buyerID string, requestID string, title string) NewNotification {
	return customOrderUpdated(buyerID, requestID, RecipientBuyer, "Solicitação em análise", "O vendedor está analisando sua solicitação de "+title+" sob medida.")
}

func CustomOrderNeedsInfo(buyerID string, requestID string, title string, question string) NewNotification {
	return customOrderUpdated(buyerID, requestID, RecipientBuyer, "O vendedor precisa de mais informações",
		"Sobre "+title+" sob medida: "+strings.TrimRight(question, ".")+". Responda para a análise continuar.")
}

func CustomOrderComplemented(sellerID string, requestID string, title string) NewNotification {
	return customOrderUpdated(sellerID, requestID, RecipientSeller, "Solicitação complementada", "O comprador respondeu sobre "+title+" sob medida.")
}

func CustomOrderAccepted(buyerID string, requestID string, title string, price string) NewNotification {
	return customOrderUpdated(buyerID, requestID, RecipientBuyer, "Solicitação aceita",
		"O vendedor aceitou fazer "+title+" sob medida por R$ "+strings.Replace(price, ".", ",", 1)+". Confirme e pague para a produção começar.")
}

func CustomOrderRejected(buyerID string, requestID string, title string, reason string) NewNotification {
	return customOrderUpdated(buyerID, requestID, RecipientBuyer, "Solicitação recusada", "O vendedor recusou fazer "+title+" sob medida: "+strings.TrimRight(reason, ".")+".")
}

func CustomOrderCancelled(sellerID string, requestID string, title string) NewNotification {
	return customOrderUpdated(sellerID, requestID, RecipientSeller, "Solicitação cancelada", "O comprador cancelou a solicitação de "+title+" sob medida.")
}

func CustomOrderCompleted(buyerID string, requestID string, title string) NewNotification {
	return customOrderUpdated(buyerID, requestID, RecipientBuyer, "Laudo e instruções de adaptação",
		"O vendedor enviou o laudo de "+title+" sob medida e as instruções de adaptação gradual. Leia antes de usar.")
}

func customOrderUpdated(userID string, requestID string, recipient string, title string, body string) NewNotification {
	return NewNotification{
		UserID: userID,
		Type:   TypeCustomOrderUpdated,
		Title:  title,
		Body:   body,
		Data:   map[string]string{"custom_order_id": requestID, "recipient": recipient},
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

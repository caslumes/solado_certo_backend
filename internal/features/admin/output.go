package admin

import (
	"encoding/json"
	"time"

	"github.com/caslumes/solado_certo_backend/internal/app/enum"
)

type PageOutput[T any] struct {
	Items       []T  `json:"items"`
	CurrentPage uint `json:"current_page"`
	TotalPages  uint `json:"total_pages"`
	TotalItems  uint `json:"total_items"`
	HasNext     bool `json:"has_next"`
	HasPrevious bool `json:"has_previous"`
}

func PageToOutput[T any, O any](p *Page[T], convert func(T) O) *PageOutput[O] {
	items := make([]O, len(p.Items))
	for i, item := range p.Items {
		items[i] = convert(item)
	}
	return &PageOutput[O]{
		Items:       items,
		CurrentPage: p.CurrentPage,
		TotalPages:  p.TotalPages,
		TotalItems:  p.TotalItems,
		HasNext:     p.CurrentPage < p.TotalPages,
		HasPrevious: p.CurrentPage > 1,
	}
}

type PersonOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type UserOutput struct {
	ID             string             `json:"id"`
	Name           string             `json:"name"`
	Email          string             `json:"email"`
	Phone          string             `json:"phone"`
	Status         enum.ProfileStatus `json:"status"`
	SuspendedUntil *time.Time         `json:"suspended_until"`
	Roles          []string           `json:"roles"`
	CreatedAt      time.Time          `json:"created_at"`
}

func (u *UserEntity) ToOutput(now time.Time) UserOutput {
	status := u.StatusAt(now)
	var until *time.Time
	if status == enum.ProfileStatusSuspended {
		until = u.SuspendedUntil
	}
	roles := u.Roles
	if roles == nil {
		roles = []string{}
	}
	return UserOutput{
		ID:             u.ID,
		Name:           u.Name,
		Email:          u.Email,
		Phone:          u.Phone,
		Status:         status,
		SuspendedUntil: until,
		Roles:          roles,
		CreatedAt:      u.CreatedAt,
	}
}

type AdOutput struct {
	ID        string       `json:"id"`
	Title     string       `json:"title"`
	Status    string       `json:"status"`
	Price     string       `json:"price"`
	Stock     int          `json:"stock"`
	Seller    PersonOutput `json:"seller"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
	RemovedAt *time.Time   `json:"removed_at"`
}

func (a *AdEntity) ToOutput() AdOutput {
	return AdOutput{
		ID:        a.ID,
		Title:     a.Title,
		Status:    a.Status,
		Price:     a.Price,
		Stock:     a.Stock,
		Seller:    PersonOutput{ID: a.SellerID, Name: a.SellerName},
		CreatedAt: a.CreatedAt,
		UpdatedAt: a.UpdatedAt,
		RemovedAt: a.RemovedAt,
	}
}

type BuyerOutput struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type OrderOutput struct {
	ID          string        `json:"id"`
	Number      string        `json:"number"`
	Status      string        `json:"status"`
	Total       string        `json:"total"`
	Buyer       BuyerOutput   `json:"buyer"`
	Seller      *PersonOutput `json:"seller"`
	CreatedAt   time.Time     `json:"created_at"`
	PaidAt      *time.Time    `json:"paid_at"`
	DeliveredAt *time.Time    `json:"delivered_at"`
}

func (o *OrderEntity) ToOutput() OrderOutput {
	var seller *PersonOutput
	if o.SellerID != nil {
		seller = &PersonOutput{ID: *o.SellerID}
		if o.SellerName != nil {
			seller.Name = *o.SellerName
		}
	}
	return OrderOutput{
		ID:          o.ID,
		Number:      o.Number,
		Status:      o.Status,
		Total:       o.Total,
		Buyer:       BuyerOutput{ID: o.BuyerID, Name: o.BuyerName, Email: o.BuyerEmail},
		Seller:      seller,
		CreatedAt:   o.CreatedAt,
		PaidAt:      o.PaidAt,
		DeliveredAt: o.DeliveredAt,
	}
}

type PaymentOutput struct {
	ID            string     `json:"id"`
	OrderID       string     `json:"order_id"`
	OrderNumber   string     `json:"order_number"`
	Gateway       string     `json:"gateway"`
	Method        string     `json:"method"`
	Status        string     `json:"status"`
	Amount        string     `json:"amount"`
	FailureReason *string    `json:"failure_reason"`
	RefundError   *string    `json:"refund_error"`
	RefundGivenUp bool       `json:"refund_given_up"`
	CreatedAt     time.Time  `json:"created_at"`
	CapturedAt    *time.Time `json:"captured_at"`
	RefundedAt    *time.Time `json:"refunded_at"`
}

func (p *PaymentEntity) ToOutput() PaymentOutput {
	return PaymentOutput{
		ID:            p.ID,
		OrderID:       p.OrderID,
		OrderNumber:   p.OrderNumber,
		Gateway:       p.Gateway,
		Method:        p.Method,
		Status:        p.Status,
		Amount:        p.Amount,
		FailureReason: p.FailureReason,
		RefundError:   p.RefundError,
		RefundGivenUp: p.RefundGivenUp,
		CreatedAt:     p.CreatedAt,
		CapturedAt:    p.CapturedAt,
		RefundedAt:    p.RefundedAt,
	}
}

type AuditOutput struct {
	ID        string          `json:"id"`
	User      *PersonOutput   `json:"user"`
	Entity    string          `json:"entity"`
	EntityID  string          `json:"entity_id"`
	Action    string          `json:"action"`
	Context   json.RawMessage `json:"context"`
	CreatedAt time.Time       `json:"created_at"`
}

func (a *AuditEntity) ToOutput() AuditOutput {
	var user *PersonOutput
	if a.UserID != nil {
		user = &PersonOutput{ID: *a.UserID}
		if a.UserName != nil {
			user.Name = *a.UserName
		}
	}
	context := a.Context
	if len(context) == 0 {
		context = json.RawMessage("null")
	}
	return AuditOutput{
		ID:        a.ID,
		User:      user,
		Entity:    a.Entity,
		EntityID:  a.EntityID,
		Action:    a.Action,
		Context:   context,
		CreatedAt: a.CreatedAt,
	}
}

type AmountOutput struct {
	Count  uint   `json:"count"`
	Amount string `json:"amount"`
}

type IndicatorsOutput struct {
	From             string                  `json:"from"`
	To               string                  `json:"to"`
	NewUsers         uint                    `json:"new_users"`
	BlockedUsers     uint                    `json:"blocked_users"`
	SuspendedUsers   uint                    `json:"suspended_users"`
	NewAds           uint                    `json:"new_ads"`
	PublishedAds     uint                    `json:"published_ads"`
	Orders           uint                    `json:"orders"`
	OrdersByStatus   map[string]uint         `json:"orders_by_status"`
	PaidOrders       uint                    `json:"paid_orders"`
	PaidAmount       string                  `json:"paid_amount"`
	PaymentsByStatus map[string]AmountOutput `json:"payments_by_status"`
	RefundsGivenUp   uint                    `json:"refunds_given_up"`
	TicketsOpened    uint                    `json:"tickets_opened"`
	TicketsResolved  uint                    `json:"tickets_resolved"`
	OpenTickets      uint                    `json:"open_tickets"`
}

func (i *Indicators) ToOutput() *IndicatorsOutput {
	orders := map[string]uint{}
	for _, status := range orderStatuses {
		orders[status] = i.OrdersByStatus[status]
	}
	payments := map[string]AmountOutput{}
	for _, status := range paymentStatuses {
		amount, ok := i.PaymentsByStatus[status]
		if !ok {
			amount.Amount = "0.00"
		}
		payments[status] = AmountOutput{Count: amount.Count, Amount: amount.Amount}
	}
	return &IndicatorsOutput{
		From:             i.Period.From.In(brasilia).Format(time.DateOnly),
		To:               i.Period.Until.In(brasilia).AddDate(0, 0, -1).Format(time.DateOnly),
		NewUsers:         i.NewUsers,
		BlockedUsers:     i.BlockedUsers,
		SuspendedUsers:   i.SuspendedUsers,
		NewAds:           i.NewAds,
		PublishedAds:     i.PublishedAds,
		Orders:           i.Orders,
		OrdersByStatus:   orders,
		PaidOrders:       i.PaidOrders,
		PaidAmount:       i.PaidAmount,
		PaymentsByStatus: payments,
		RefundsGivenUp:   i.RefundsGivenUp,
		TicketsOpened:    i.TicketsOpened,
		TicketsResolved:  i.TicketsResolved,
		OpenTickets:      i.OpenTickets,
	}
}

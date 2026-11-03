package support

import "time"

type TicketSummaryOutput struct {
	ID           string     `json:"id"`
	Category     string     `json:"category"`
	Status       string     `json:"status"`
	Subject      string     `json:"subject"`
	OrderID      *string    `json:"order_id"`
	OrderNumber  *string    `json:"order_number"`
	OpenedByMe   bool       `json:"opened_by_me"`
	AssignedToMe bool       `json:"assigned_to_me"`
	OpenedAt     time.Time  `json:"opened_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at"`
}

type MessageOutput struct {
	ID            string    `json:"id"`
	SenderRole    string    `json:"sender_role"`
	SenderName    string    `json:"sender_name"`
	Mine          bool      `json:"mine"`
	Message       string    `json:"message"`
	AttachmentURL *string   `json:"attachment_url"`
	CreatedAt     time.Time `json:"created_at"`
}

type TicketDetailOutput struct {
	TicketSummaryOutput
	Description string          `json:"description"`
	Resolution  *string         `json:"resolution"`
	CanReply    bool            `json:"can_reply"`
	Messages    []MessageOutput `json:"messages"`
}

type TicketPageOutput struct {
	Items       []TicketSummaryOutput `json:"items"`
	CurrentPage uint                  `json:"current_page"`
	TotalPages  uint                  `json:"total_pages"`
	TotalItems  uint                  `json:"total_items"`
	HasNext     bool                  `json:"has_next"`
	HasPrevious bool                  `json:"has_previous"`
}

type PersonOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type QueueItemOutput struct {
	TicketSummaryOutput
	OpenedBy   PersonOutput  `json:"opened_by"`
	AssignedTo *PersonOutput `json:"assigned_to"`
}

type QueuePageOutput struct {
	Items       []QueueItemOutput `json:"items"`
	CurrentPage uint              `json:"current_page"`
	TotalPages  uint              `json:"total_pages"`
	TotalItems  uint              `json:"total_items"`
	HasNext     bool              `json:"has_next"`
	HasPrevious bool              `json:"has_previous"`
}
